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
	// server to actually move the bot into the bed after the
	// UseItemOnBlock click - mirrors every other client-sync wait in
	// this codebase (see place_block.go's placeVerifyWait doc comment
	// for the convention this follows).
	sleepConfirmTimeout = 10 * time.Second
	sleepConfirmPoll    = 100 * time.Millisecond

	// sleepSnapHorizontalDistance bounds how far the agent's tracked X/Z
	// may sit from bedPos's own column to count as "the server actually
	// put me in the bed." EntityPose metadata (which would otherwise
	// confirm a sleep attempt directly) is never echoed back to the
	// sleeping entity's own connection - standard Minecraft behavior, not
	// something this codebase can control - so a position snap is the
	// most direct signal available (see docs/plans/SLEEP_ACTION_PLAN.md's
	// own "Open question" section).
	//
	// Horizontally this is a weak discriminator on its own: a live run
	// (testing/sleep_action_test.go, 2026-10-27) showed
	// TryInteractPositions' own approach cell landing only ~0.68 blocks
	// (XZ) from the bed's column even while merely standing beside it,
	// comfortably inside any reasonable horizontal snap radius - keeping
	// this check mainly rules out confirming against the wrong bed
	// entirely, not standing-vs-sleeping.
	sleepSnapHorizontalDistance = 1.0

	// sleepSnapMaxYAboveBed is the real discriminator, found from that
	// same live run: standing on the floor beside a bed sitting at
	// bedPos.Y consistently reads pos.Y == bedPos.Y+1.0 (the floor cell
	// one block above the bed), while vanilla's own lying-down position
	// sets Y close to the bed block's own level (roughly bedPos.Y+0.5-0.7,
	// per public client-side sleep-pose documentation) - a clear gap
	// below the floor-standing value. 0.85 sits strictly between the two,
	// closer to the standing value so it still fails fast instead of
	// risking a confirm on partial/glitchy Y data.
	sleepSnapMaxYAboveBed = 0.85
)

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
	err := models.TryInteractPositions(ctx, a, bedPos, func(standAt models.V3) error {
		if err := a.MoveTo(ctx, standAt.X, standAt.Y, standAt.Z, false); err != nil {
			return err
		}
		return a.UseItemOnBlock(ctx, bedPos.X, bedPos.Y, bedPos.Z, models.FaceUp, models.MainHand)
	})
	if err != nil {
		return fmt.Errorf("sleep at (%v): %w", bedPos, err)
	}
	if err := a.awaitSleepConfirmed(ctx, bedPos); err != nil {
		return fmt.Errorf("sleep at (%v): %w", bedPos, err)
	}
	return nil
}

// awaitSleepConfirmed polls the agent's own tracked position until it
// snaps within sleepSnapHorizontalDistance (XZ) and sleepSnapMaxYAboveBed
// (Y) of bedPos, or sleepConfirmTimeout passes. The Y gate is what
// actually discriminates "lying in the bed" from "merely standing beside
// it" - see sleepSnapMaxYAboveBed's own doc comment.
func (a *agent) awaitSleepConfirmed(ctx context.Context, bedPos models.V3) error {
	deadline := time.Now().Add(sleepConfirmTimeout)
	bedCenterX, bedCenterZ := bedPos.X+0.5, bedPos.Z+0.5
	for {
		pos, _, _, ok := a.GetPosition()
		if ok {
			dx, dy, dz := pos.X-bedCenterX, pos.Y-bedPos.Y, pos.Z-bedCenterZ
			if dx*dx+dz*dz <= sleepSnapHorizontalDistance*sleepSnapHorizontalDistance && dy <= sleepSnapMaxYAboveBed {
				return nil
			}
		}
		if !time.Now().Before(deadline) {
			return errors.New("position never snapped into the bed - likely rejected (not night, or bed occupied/obstructed)")
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
