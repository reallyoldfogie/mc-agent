package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	versions_common "github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
)

// seedBedColor is the fixed bed color SeedBed places - FindNearestBed is
// color-agnostic (models.BlockShapeManager.IsBed matches any of the 16),
// so which one is used here is arbitrary.
const seedBedColor = "minecraft:red_bed"

// SeedBed implements rlenv.SeedBedAgent: ensures a complete, sleepable bed
// exists within radius blocks of the bot's current position, placing one
// beside it (same "beside the bot" coordinate convention as
// SeedNearbyBlock - agent/rl_seed.go's seedBlockCoords) if none is already
// visible. Unlike SeedNearbyBlock, a bed can't be placed as a single
// /setblock: it's two blocks (foot + head, each with its own "part"
// blockstate, sharing a "facing" that points from the foot toward the
// head) - placing only one half leaves it incomplete and unsleepable, the
// same way a real player's own bed does. facing=north (head placed one
// block north - -Z, this codebase's existing convention, see
// place_block.go's supportFaceOrder) is an arbitrary fixed choice;
// nothing about a sleep episode cares which way the bed faces.
func (a *agent) SeedBed(ctx context.Context, radius int) error {
	if a.cfg.RCON == nil {
		return errors.New("seed bed: RCON not configured for this agent")
	}
	if _, found, err := a.FindNearestBed(ctx, radius); err == nil && found {
		return nil
	}
	pos, ok := a.GetPositionSimple()
	if !ok {
		return errors.New("seed bed: position not yet known")
	}
	x, y, z := seedBlockCoords(pos.X, pos.Y, pos.Z)
	footState := seedBedColor + "[facing=north,part=foot]"
	headState := seedBedColor + "[facing=north,part=head]"
	if _, err := a.cfg.RCON.SetBlock(ctx, x, y, z, footState, "replace").Exec(ctx); err != nil {
		return fmt.Errorf("setblock bed foot via RCON: %w", err)
	}
	if _, err := a.cfg.RCON.SetBlock(ctx, x, y, z-1, headState, "replace").Exec(ctx); err != nil {
		return fmt.Errorf("setblock bed head via RCON: %w", err)
	}

	deadline := time.Now().Add(seedSyncTimeout)
	for {
		if _, found, err := a.FindNearestBed(ctx, radius); err == nil && found {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("seed bed: placed it but it never became visible to this bot within %s", seedSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(seedSyncPollInterval):
		}
	}
}

// FindNearestBed searches for the nearest bed block (any of the 16 colors)
// within radius blocks that the agent has a clear line of sight to - the
// color-agnostic sibling of FindVisibleBlock (agent/actions.go), keyed on
// models.BlockShapeManager.IsBed instead of an exact block name, since no
// single block name covers every bed color. Mirrors FindVisibleBlock's own
// shell-expansion/line-of-sight search exactly (shellOffsets,
// hasLineOfSightForAccess, the same findVisibleBlockTimeout bound), just
// swapped for a predicate match instead of a name match - see that
// function's own doc comment for why the search is structured this way.
func (a *agent) FindNearestBed(ctx context.Context, radius int) (models.V3, bool, error) {
	callerCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, findVisibleBlockTimeout)
	defer cancel()
	world := a.GetWorld()
	if world == nil {
		return models.V3{}, false, errors.New("find nearest bed: world not available")
	}
	shapeMgr := a.BlockShapeManager()
	if shapeMgr == nil {
		return models.V3{}, false, errors.New("find nearest bed: block shape manager not available")
	}
	pos, _, _, ok := a.GetPosition()
	if !ok {
		return models.V3{}, false, errors.New("find nearest bed: position not initialized")
	}
	if radius <= 0 {
		radius = 8
	}
	baseX, baseY, baseZ := math.Floor(pos.X), math.Floor(pos.Y), math.Floor(pos.Z)

	bestDistSq := math.MaxFloat64
	var bestX, bestY, bestZ float64
	// Memoizes IsBed per state ID for this call only, same reasoning as
	// FindVisibleBlock's own nameMatchCache.
	isBedCache := make(map[uint32]bool)

	for r := 0; r <= radius; r++ {
		rr := float64(r) * float64(r)
		if bestDistSq != math.MaxFloat64 && rr > bestDistSq {
			break
		}
		for _, off := range shellOffsets(r) {
			if ctx.Err() != nil {
				if findVisibleBlockTimedOut(ctx, callerCtx) {
					return models.V3{}, false, nil
				}
				return models.V3{}, false, ctx.Err()
			}
			cx := baseX + float64(off[0])
			cy := baseY + float64(off[1])
			cz := baseZ + float64(off[2])
			stateID, loaded := world.GetBlockAt(cx, cy, cz)
			if !loaded || stateID == 0 {
				continue
			}
			isBed, cached := isBedCache[stateID]
			if !cached {
				isBed = shapeMgr.IsBed(stateID)
				isBedCache[stateID] = isBed
			}
			if !isBed {
				continue
			}

			ddx := (cx + 0.5) - pos.X
			ddy := (cy + 0.5) - pos.Y
			ddz := (cz + 0.5) - pos.Z
			distSq := ddx*ddx + ddy*ddy + ddz*ddz
			if distSq >= bestDistSq {
				continue
			}

			visible, _, _, _, err := a.hasLineOfSightForAccess(ctx, cx, cy, cz)
			if err != nil {
				if findVisibleBlockTimedOut(ctx, callerCtx) {
					return models.V3{}, false, nil
				}
				return models.V3{}, false, err
			}
			if !visible {
				continue
			}

			bestDistSq = distSq
			bestX, bestY, bestZ = cx, cy, cz
		}
	}
	if bestDistSq == math.MaxFloat64 {
		return models.V3{}, false, nil
	}
	return models.V3{X: bestX, Y: bestY, Z: bestZ}, true, nil
}

const (
	// sleepConfirmTimeout bounds how long SleepInBed waits for the
	// server's own sleep-status broadcast (sleepSuccessMessageKeys/
	// sleepRejectionMessageKeys) after the UseItemOnBlock click - mirrors
	// every other client-sync wait in this codebase (see place_block.go's
	// placeVerifyWait doc comment for the convention this follows).
	sleepConfirmTimeout = 10 * time.Second
	sleepConfirmPoll    = 100 * time.Millisecond

	// sleepStatusEventsCap bounds agent.sleepStatusEvents - a handful of
	// entries is enough; awaitSleepConfirmed only ever looks for the first
	// one after its own interact packet, so nothing needs a deep history.
	sleepStatusEventsCap = 8
)

// sleepStatusEvent is one recognized vanilla sleep-status or
// bed-rejection system/action-bar message, as recorded by
// recordSleepStatusMessage.
type sleepStatusEvent struct {
	key     string
	success bool
	at      time.Time
}

// sleepSuccessMessageKeys are the exact vanilla translate keys
// ServerWorld.sendSleepingStatus (net/minecraft/server/world/ServerWorld.java)
// broadcasts to every connected player the moment ANY player's own
// trySleep succeeds - confirmed from decompiled 1.21.5 source
// (mc-data-gen/extractedSrc/1.21.5), not a guess: entering a bed never
// moves the sleeping entity's own tracked position (no teleport/position
// packet is sent on entry - only PlayerEntity.wakeUp() sends one), so a
// position-based confirmation (this package's own earlier, now-removed
// attempt) can never work here. "sleep.players_sleeping" fires when not
// enough players are in bed yet to skip the night; "sleep.skipping_night"
// fires once enough are. Either one means THIS bot's own attempt was
// accepted - vanilla resets its time_since_rest stat (what prevents
// phantom spawns, PlayerEntity.sleep) as part of that same success,
// independent of whether the aggregate night-skip itself ever happens
// (see docs/plans/SLEEP_ACTION_PLAN.md).
var sleepSuccessMessageKeys = map[string]bool{
	"sleep.players_sleeping": true,
	"sleep.skipping_night":   true,
}

// sleepRejectionMessageKeys are PlayerEntity.SleepFailureReason's own
// translate keys (net/minecraft/entity/player/PlayerEntity.java,
// BedBlock.onUse), sent only to the player whose own attempt failed.
// NOT_POSSIBLE_HERE (wrong dimension) and OTHER_PROBLEM (already
// sleeping, or dead) have no message in vanilla at all - an attempt
// failing for one of those two reasons looks identical to
// awaitSleepConfirmed's own timeout case.
var sleepRejectionMessageKeys = map[string]string{
	"block.minecraft.bed.too_far_away": "too far away from the bed",
	"block.minecraft.bed.obstructed":   "bed is obstructed (something above it)",
	"block.minecraft.bed.no_sleep":     "not the right time to sleep",
	"block.minecraft.bed.not_safe":     "hostile mobs are too close",
	"block.minecraft.bed.occupied":     "bed is already occupied",
}

// recordSleepStatusMessage checks whether text - OnSystemChat's already-
// resolved message, see handler_versions/*/chat.go's extractNBTString,
// which falls back to the raw "translate" key for components like these
// that carry no literal "text" field - is one of sleepSuccessMessageKeys/
// sleepRejectionMessageKeys, and if so appends it to sleepStatusEvents for
// awaitSleepConfirmed to find. A no-op for every other message (ordinary
// chat, unrelated action-bar text, etc).
func (a *agent) recordSleepStatusMessage(text string) {
	isSuccess := sleepSuccessMessageKeys[text]
	_, isRejection := sleepRejectionMessageKeys[text]
	if !isSuccess && !isRejection {
		return
	}
	a.sleepStatusMu.Lock()
	defer a.sleepStatusMu.Unlock()
	a.sleepStatusEvents = append(a.sleepStatusEvents, sleepStatusEvent{key: text, success: isSuccess, at: time.Now()})
	if len(a.sleepStatusEvents) > sleepStatusEventsCap {
		a.sleepStatusEvents = a.sleepStatusEvents[len(a.sleepStatusEvents)-sleepStatusEventsCap:]
	}
}

// sleepStatusSince returns the first recorded sleepStatusEvent at or after
// since, if any - the one awaitSleepConfirmed is waiting for.
func (a *agent) sleepStatusSince(since time.Time) (sleepStatusEvent, bool) {
	a.sleepStatusMu.Lock()
	defer a.sleepStatusMu.Unlock()
	for _, e := range a.sleepStatusEvents {
		if !e.at.Before(since) {
			return e, true
		}
	}
	return sleepStatusEvent{}, false
}

// SleepInBed walks to a reachable position beside bedPos (the same
// ApproachBlock/TryInteractPositions pattern PlaceBlockAt/MineBlockAt use)
// and right-clicks it, then confirms the attempt actually took effect via
// awaitSleepConfirmed. Returns an error if no reachable standing position
// exists, or if the confirmation deadline passes - most likely because
// it isn't night, or the bed is already occupied/obstructed; vanilla
// rejects the attempt outright rather than queuing it, so there is no
// "waiting to fall asleep" state to distinguish from a real failure here.
func (a *agent) SleepInBed(ctx context.Context, bedPos models.V3) error {
	world, shapeMgr := a.GetWorld(), a.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return fmt.Errorf("sleep at (%v): world not available", bedPos)
	}
	var interactSentAt time.Time
	err := models.TryInteractPositions(ctx, a, bedPos, func(standAt models.V3) error {
		if err := a.MoveTo(ctx, standAt.X, standAt.Y, standAt.Z, false); err != nil {
			return err
		}
		interactSentAt = time.Now()
		return a.UseItemOnBlock(ctx, bedPos.X, bedPos.Y, bedPos.Z, models.FaceUp, models.MainHand)
	})
	if err != nil {
		return fmt.Errorf("sleep at (%v): %w", bedPos, err)
	}
	if err := a.awaitSleepConfirmed(ctx, interactSentAt); err != nil {
		return fmt.Errorf("sleep at (%v): %w", bedPos, err)
	}
	return nil
}

// awaitSleepConfirmed polls for the server's own sleep-status broadcast
// (recordSleepStatusMessage) arriving at or after interactSentAt, within
// sleepConfirmTimeout. A success key (sleepSuccessMessageKeys) returns
// nil; a rejection key (sleepRejectionMessageKeys) returns its reason as
// an error immediately, without waiting out the rest of the timeout -
// vanilla rejects a bad attempt outright, so there's nothing further to
// wait for. No matching message within the timeout is itself ambiguous
// (NOT_POSSIBLE_HERE/OTHER_PROBLEM have no message at all - see
// sleepRejectionMessageKeys' own doc comment) but is still reported as an
// error, the same conservative default every other confirmation in this
// package uses.
func (a *agent) awaitSleepConfirmed(ctx context.Context, interactSentAt time.Time) error {
	deadline := time.Now().Add(sleepConfirmTimeout)
	for {
		if event, found := a.sleepStatusSince(interactSentAt); found {
			if event.success {
				return nil
			}
			if reason, ok := sleepRejectionMessageKeys[event.key]; ok {
				return fmt.Errorf("rejected: %s", reason)
			}
			return fmt.Errorf("rejected: %s", event.key)
		}
		if !time.Now().Before(deadline) {
			return errors.New("no sleep-status message received within the timeout - likely rejected for a reason vanilla sends no message for (wrong dimension, already sleeping, or dead), or the interaction never reached the server")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepConfirmPoll):
		}
	}
}

// GetTimeOfDay implements rlenv.TimeOfDayAgent: forwards directly to the
// tracked world's own GetTimeOfDay, returning (0, false) if the world
// isn't available yet (mirrors every other "not initialized" accessor in
// this package rather than treating it as an error).
func (a *agent) GetTimeOfDay() (int64, bool) {
	world := a.GetWorld()
	if world == nil {
		return 0, false
	}
	return world.GetTimeOfDay()
}

// WakeUp leaves the bed the agent is currently sleeping in: a single
// fire-and-forget EntityAction dispatch, mirroring StartSneaking/
// StopSneaking's own one-shot packet send (movement/movement_packet_sender.go)
// but via versions_common.ActionLeaveBed instead of the sneak action IDs,
// and sent directly through the version handler rather than through
// models.MovementExecutor - unlike sneak, nothing needs to track "is this
// agent currently in a bed" state across calls, so there's no executor-level
// state to add this to. Vanilla itself ignores a leave-bed action sent while
// not sleeping, so this is a no-op rather than an error in that case -
// nothing here can distinguish the two outcomes without a sleeping-state
// signal, and both end with the agent out of bed either way.
func (a *agent) WakeUp(ctx context.Context) error {
	if a.client == nil {
		return errors.New("wake up: client not available")
	}
	if a.versionHandler == nil {
		return errors.New("wake up: version handler not available")
	}
	if err := a.versionHandler.Play().Movement().SendPlayerCommand(a.client.Conn(), a.GetEntityID(), versions_common.ActionLeaveBed); err != nil {
		return fmt.Errorf("wake up: %w", err)
	}
	return nil
}
