package actions

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/structure"
)

const helpText = "Commands: help, pos, say <text>, testMove, moveTo <x> <y> <z> (pathfinding), lineTo <x> <y> <z> (straight-line), moveForward <distance>, moveUp <distance>, moveUpAndSneak <distance>, moveToAndSneak <x> <y> <z>, lineToAndSneak <x> <y> <z>, stopSneak, findPath <x> <y> <z>, testPath, follow [<player>], stopFollow, followStatus, startTracking, stopTracking, fireBow, fireBowAt <x> <y> <z> | nearest | <player>, attackEntity <entityID> [sneaking], shield <raise|lower>, fireCrossbowAt <entityID>, throwTridentAt <entityID>, spearJab <entityID> <itemName>, spearCharge <entityID> <itemName> <holdMs> <engagedMs> <tiredMs> [minSpeed] [minAlignment], maceAttack <entityID> <itemName>, maceSmash <entityID> <itemName>, runCombat [radius] [includeNeutral], runCombatWithPolicy <radius> [includePlayers] [includeNeutral], mount <entityID | entityType>, dismount, vehiclejump [power], mine <x> <y> <z> | <blockName> [radius], lookAround [radius], pickUpNearbyItem [maxDistance], killCreeperForGunpowder, craft <itemName>, place <itemName>, buildStructure <path> <x> <y> <z>, materialList <structurePath> [exportPath], equip <item>, useItem [offhand], flyTo <x> <y> <z>, fly, land, followCam <playerName> [maxDistance], stopFollowCam, planStatus, planStop"

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// parsePositiveIntArg parses s as a positive integer, for chat commands
// that take an optional trailing count/radius argument (e.g. Mine's
// "[radius]"). ok=false means s wasn't a positive integer.
func parsePositiveIntArg(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

type Help struct{}

func (Help) Name() string  { return "help" }
func (Help) Usage() string { return "help" }
func (Help) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	_ = agent.SendChat(helpText)
	return models.Done(nil), nil
}

type Pos struct{}

func (Pos) Name() string  { return "pos" }
func (Pos) Usage() string { return "pos" }
func (Pos) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	pos, _, _, ok := agent.GetPosition()
	if !ok {
		_ = agent.SendChat("Bot position not initialized")
		return models.Done(nil), nil
	}
	_ = agent.SendChat(fmt.Sprintf("Current position: %.2f, %.2f, %.2f", pos.X, pos.Y, pos.Z))
	return models.Done(nil), nil
}

type Say struct{}

func (Say) Name() string  { return "say" }
func (Say) Usage() string { return "say <text>" }
func (Say) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	_ = agent.SendChat(strings.Join(args, " "))
	return models.Done(nil), nil
}

type TestMove struct{}

func (TestMove) Name() string  { return "testmove" }
func (TestMove) Usage() string { return "testMove" }
func (TestMove) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	completion, resolve := models.NewCompletion()
	go func() {
		agent.TestMove()
		resolve(nil)
	}()
	return completion, nil
}

type MoveTo struct{}

func (MoveTo) Name() string  { return "moveto" }
func (MoveTo) Usage() string { return "moveTo <x> <y> <z>" }
func (MoveTo) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: moveTo <x> <y> <z>")
		return models.Done(nil), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		err := agent.MoveToWithChat(ctx, tx, ty, tz)
		if err != nil {
			_ = agent.SendChat(fmt.Sprintf("MoveToWithChat - Pathfinding failed: %v", err))
		}
		resolve(err)
	}()
	return completion, nil
}

// MoveToQuiet is MoveTo without the chat narration — for callers that
// dispatch movement far more often than a human types a chat command (in
// particular, rlenv's RL training loop; see rlenv/action.go's
// ActionGoToTarget). Found live, not anticipated: an
// RL-driven session that dispatched "moveto" every Step got kicked from a
// real server for spamming once rollout collection reached the same
// "Already at target position"/"Navigating..." message fast enough for
// vanilla's anti-spam to flag it — see docs/plans/RL_TRAINING_LOOP_PLAN.md.
// Registered like any other action (reachable via chat as
// "movetoquiet <x> <y> <z>" too, same as "moveto"), but exists
// specifically so rlenv can dispatch to it by name; not intended as a
// documented player-facing command.
type MoveToQuiet struct{}

func (MoveToQuiet) Name() string  { return "movetoquiet" }
func (MoveToQuiet) Usage() string { return "movetoquiet <x> <y> <z>" }
func (MoveToQuiet) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		return models.Done(fmt.Errorf("movetoquiet: usage: movetoquiet <x> <y> <z>")), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		return models.Done(fmt.Errorf("movetoquiet: invalid X coordinate: %w", err)), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		return models.Done(fmt.Errorf("movetoquiet: invalid Y coordinate: %w", err)), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		return models.Done(fmt.Errorf("movetoquiet: invalid Z coordinate: %w", err)), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		resolve(agent.MoveTo(ctx, tx, ty, tz, false))
	}()
	return completion, nil
}

type LineTo struct{}

func (LineTo) Name() string  { return "lineto" }
func (LineTo) Usage() string { return "lineTo <x> <y> <z> (straight-line, flat world only)" }
func (LineTo) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: lineTo <x> <y> <z> (straight-line, flat world only)")
		return models.Done(nil), nil
	}

	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}

	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}

	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}

	pos, _, _, ok := agent.GetPosition()
	if !ok {
		_ = agent.SendChat("Bot position not initialized")
		return models.Done(nil), nil
	}

	total := pos.DistanceTo(models.V3{X: tx, Y: ty, Z: tz})
	_ = agent.SendChat(fmt.Sprintf("Moving direct from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [%.2f blocks]", pos.X, pos.Y, pos.Z, tx, ty, tz, total))
	completion, resolve := models.NewCompletion()
	go func() {
		err := agent.LineTo(ctx, tx, ty, tz, true)
		if err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
		}
		resolve(err)
	}()
	return completion, nil
}

type MoveForward struct{}

func (MoveForward) Name() string  { return "moveforward" }
func (MoveForward) Usage() string { return "moveForward <distance>" }
func (MoveForward) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: moveForward <distance>")
		return models.Done(nil), nil
	}
	dist, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid distance")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		_ = agent.SendChat(fmt.Sprintf("Moving forward %.2f blocks", dist))
		if err := agent.MoveForward(ctx, dist); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat("Move forward complete")
		resolve(nil)
	}()
	return completion, nil
}

type MoveUp struct{}

func (MoveUp) Name() string  { return "moveup" }
func (MoveUp) Usage() string { return "moveUp <distance>" }
func (MoveUp) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: moveUp <distance>")
		return models.Done(nil), nil
	}
	dist, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid distance")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		if err := agent.MoveUp(ctx, dist); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat("Move up complete")
		resolve(nil)
	}()
	return completion, nil
}

type MoveUpAndSneak struct{}

func (MoveUpAndSneak) Name() string  { return "moveupandsneak" }
func (MoveUpAndSneak) Usage() string { return "moveUpAndSneak <distance>" }
func (MoveUpAndSneak) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: moveUpAndSneak <distance>")
		return models.Done(nil), nil
	}
	dist, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid distance")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		if err := agent.MoveUpAndSneak(ctx, dist); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat("Move up complete, now sneaking")
		resolve(nil)
	}()
	return completion, nil
}

type MoveToAndSneak struct{}

func (MoveToAndSneak) Name() string  { return "movetoandsneak" }
func (MoveToAndSneak) Usage() string { return "moveToAndSneak <x> <y> <z>" }
func (MoveToAndSneak) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: moveToAndSneak <x> <y> <z>")
		return models.Done(nil), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		if err := agent.MoveToAndSneak(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat("Movement complete, now sneaking")
		resolve(nil)
	}()
	return completion, nil
}

type LineToAndSneak struct{}

func (LineToAndSneak) Name() string  { return "linetoandsneak" }
func (LineToAndSneak) Usage() string { return "lineToAndSneak <x> <y> <z>" }
func (LineToAndSneak) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: lineToAndSneak <x> <y> <z>")
		return models.Done(nil), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		if err := agent.LineToAndSneak(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat("Movement complete, now sneaking")
		resolve(nil)
	}()
	return completion, nil
}

type StopSneak struct{}

func (StopSneak) Name() string  { return "stopsneak" }
func (StopSneak) Usage() string { return "stopSneak" }
func (StopSneak) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if err := agent.StopSneaking(); err != nil {
		_ = agent.SendChat("Failed to stop sneaking: " + err.Error())
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Stopped sneaking")
	return models.Done(nil), nil
}

type FindPath struct{}

func (FindPath) Name() string  { return "findpath" }
func (FindPath) Usage() string { return "findPath <x> <y> <z>" }
func (FindPath) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: findPath <x> <y> <z>")
		return models.Done(nil), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		if _, err := agent.FindPath(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Path find failed: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat("Path computed")
		resolve(nil)
	}()
	return completion, nil
}

type TestPath struct{}

func (TestPath) Name() string  { return "testpath" }
func (TestPath) Usage() string { return "testPath" }
func (TestPath) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	completion, resolve := models.NewCompletion()
	go func() {
		agent.TestPath()
		resolve(nil)
	}()
	return completion, nil
}

type Follow struct{}

func (Follow) Name() string  { return "follow" }
func (Follow) Usage() string { return "follow [<player>]" }
func (Follow) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		nearest, ok := agent.NearestPlayerInfo(ctx, true)
		if !ok {
			_ = agent.SendChat("Failed to find nearest player")
			return models.Done(nil), nil
		}
		_ = agent.SendChat(fmt.Sprintf("Found nearest player at %.1f blocks, starting to follow...", nearest.Distance))
		_ = agent.SendChat("Note: Following nearest player requires name. Use 'follow <playername>' instead.")
		return models.Done(nil), nil
	}
	if !agent.HasFollowManager() {
		_ = agent.SendChat("Follow system not available")
		return models.Done(nil), nil
	}
	if err := agent.Follow(ctx, args[0]); err != nil {
		_ = agent.SendChat("Follow error: " + err.Error())
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Following " + args[0])
	return models.Done(nil), nil
}

type StopFollow struct{}

func (StopFollow) Name() string  { return "stopfollow" }
func (StopFollow) Usage() string { return "stopFollow" }
func (StopFollow) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if !agent.HasFollowManager() {
		_ = agent.SendChat("Follow system not available")
		return models.Done(nil), nil
	}
	if !agent.IsFollowing() {
		_ = agent.SendChat("Not currently following anyone")
		return models.Done(nil), nil
	}
	if err := agent.StopFollow(ctx); err != nil {
		_ = agent.SendChat("Stop error: " + err.Error())
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Stopped following")
	return models.Done(nil), nil
}

type FollowStatus struct{}

func (FollowStatus) Name() string  { return "followstatus" }
func (FollowStatus) Usage() string { return "followStatus" }
func (FollowStatus) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	_ = agent.SendChat(agent.FollowStatus(ctx))
	return models.Done(nil), nil
}

type StartTracking struct{}

func (StartTracking) Name() string  { return "starttracking" }
func (StartTracking) Usage() string { return "startTracking" }
func (StartTracking) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	agent.StartTracking()
	return models.Done(nil), nil
}

type StopTracking struct{}

func (StopTracking) Name() string  { return "stoptracking" }
func (StopTracking) Usage() string { return "stopTracking" }
func (StopTracking) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	agent.StopTracking()
	return models.Done(nil), nil
}

type PlanStatus struct{}

func (PlanStatus) Name() string  { return "planstatus" }
func (PlanStatus) Usage() string { return "planStatus" }
func (PlanStatus) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	status := agent.PlanStatus(ctx)
	_ = agent.SendChat(status.String())
	return models.Done(nil), nil
}

type PlanStop struct{}

func (PlanStop) Name() string  { return "planstop" }
func (PlanStop) Usage() string { return "planStop" }
func (PlanStop) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if err := agent.StopPlan(ctx); err != nil {
		_ = agent.SendChat("Plan stop error: " + err.Error())
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Plan stopped")
	return models.Done(nil), nil
}

type FireBow struct{}

func (FireBow) Name() string  { return "firebow" }
func (FireBow) Usage() string { return "fireBow" }
func (FireBow) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	completion, resolve := models.NewCompletion()
	go func() {
		err := agent.FireBow(ctx)
		if err != nil {
			_ = agent.SendChat("Fire bow error: " + err.Error())
		}
		resolve(err)
	}()
	return completion, nil
}

type FireBowAt struct{}

func (FireBowAt) Name() string { return "firebowat" }
func (FireBowAt) Usage() string {
	return "fireBowAt <x> <y> <z> | fireBowAt nearest | fireBowAt <player>"
}
func (FireBowAt) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) == 0 {
		completion, resolve := models.NewCompletion()
		go func() {
			err := agent.FireBow(ctx)
			if err != nil {
				_ = agent.SendChat("Fire bow error: " + err.Error())
			}
			resolve(err)
		}()
		return completion, nil
	}
	if len(args) == 1 {
		if args[0] == "nearest" {
			playerInfo, found := agent.NearestPlayerInfo(ctx, true)
			if found {
				completion, resolve := models.NewCompletion()
				go func() {
					_, err := agent.FireBowAt(ctx, playerInfo.X, playerInfo.Y, playerInfo.Z)
					if err != nil {
						_ = agent.SendChat("Fire bow at error: " + err.Error())
					}
					resolve(err)
				}()
				return completion, nil
			}
		} else {
			x, y, z, found, err := agent.FindPlayerByName(ctx, args[0])
			if err == nil && found {
				completion, resolve := models.NewCompletion()
				go func() {
					_, err := agent.FireBowAt(ctx, x, y, z)
					if err != nil {
						_ = agent.SendChat("Fire bow at error: " + err.Error())
					}
					resolve(err)
				}()
				return completion, nil
			}
			_ = agent.SendChat("Player not found: " + args[0])
			return models.Done(nil), nil
		}
	}
	if len(args) < 3 {
		_ = agent.SendChat("Usage: fireBowAt <x> <y> <z> | fireBowAt nearest | fireBowAt <player>")
		return models.Done(nil), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		_, err := agent.FireBowAt(ctx, tx, ty, tz)
		if err != nil {
			_ = agent.SendChat("Fire bow at error: " + err.Error())
		}
		resolve(err)
	}()
	return completion, nil
}

type AttackEntity struct{}

func (AttackEntity) Name() string  { return "attackentity" }
func (AttackEntity) Usage() string { return "attackEntity <entityID> [sneaking]" }
func (AttackEntity) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 || len(args) > 2 {
		_ = agent.SendChat("Usage: attackEntity <entityID> [sneaking]")
		return models.Done(nil), nil
	}
	entityID, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		_ = agent.SendChat("Invalid entity ID")
		return models.Done(nil), nil
	}
	sneaking := false
	if len(args) == 2 {
		sneaking, err = strconv.ParseBool(args[1])
		if err != nil {
			_ = agent.SendChat("Usage: attackEntity <entityID> [sneaking] - sneaking must be true or false")
			return models.Done(nil), nil
		}
	}
	attacker, ok := agent.(interface {
		AttackEntity(context.Context, int32, bool) error
	})
	if !ok {
		return models.Done(fmt.Errorf("attack entity action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(attacker.AttackEntity(ctx, int32(entityID), sneaking)) }()
	return completion, nil
}

type RunCombat struct{}

type Shield struct{}

func (Shield) Name() string  { return "shield" }
func (Shield) Usage() string { return "shield <raise|lower>" }
func (Shield) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 1 || (args[0] != "raise" && args[0] != "lower") {
		_ = agent.SendChat("Usage: shield <raise|lower>")
		return models.Done(nil), nil
	}
	shielder, ok := agent.(interface {
		SetCombatShield(context.Context, bool) error
	})
	if !ok {
		return models.Done(fmt.Errorf("shield action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(shielder.SetCombatShield(ctx, args[0] == "raise")) }()
	return completion, nil
}

type FireCrossbowAt struct{}

func (FireCrossbowAt) Name() string  { return "firecrossbowat" }
func (FireCrossbowAt) Usage() string { return "fireCrossbowAt <entityID>" }
func (FireCrossbowAt) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	attacker, ok := agent.(interface {
		FireCrossbowAt(context.Context, int32) error
	})
	if !ok {
		return models.Done(fmt.Errorf("crossbow action is not supported by this agent")), nil
	}
	return executeCombatEntityAction(ctx, agent, args, "fireCrossbowAt <entityID>", func(targetID int32) error {
		return attacker.FireCrossbowAt(ctx, targetID)
	})
}

type ThrowTridentAt struct{}

func (ThrowTridentAt) Name() string  { return "throwtridentat" }
func (ThrowTridentAt) Usage() string { return "throwTridentAt <entityID>" }
func (ThrowTridentAt) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	attacker, ok := agent.(interface {
		ThrowTridentAt(context.Context, int32) error
	})
	if !ok {
		return models.Done(fmt.Errorf("trident action is not supported by this agent")), nil
	}
	return executeCombatEntityAction(ctx, agent, args, "throwTridentAt <entityID>", func(targetID int32) error {
		return attacker.ThrowTridentAt(ctx, targetID)
	})
}

type SpearJab struct{}

func (SpearJab) Name() string  { return "spearjab" }
func (SpearJab) Usage() string { return "spearJab <entityID> <itemName>" }
func (SpearJab) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 2 {
		_ = agent.SendChat("Usage: spearJab <entityID> <itemName>")
		return models.Done(nil), nil
	}
	targetID, err := parseCombatEntityID(args[0], agent)
	if err != nil {
		return models.Done(nil), nil
	}
	spear, ok := agent.(interface {
		SpearJabAt(context.Context, int32, string) error
	})
	if !ok {
		return models.Done(fmt.Errorf("spear Jab action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(spear.SpearJabAt(ctx, targetID, args[1])) }()
	return completion, nil
}

type SpearCharge struct{}

func (SpearCharge) Name() string { return "spearcharge" }
func (SpearCharge) Usage() string {
	return "spearCharge <entityID> <itemName> <holdMs> <engagedMs> <tiredMs> [minSpeed] [minAlignment]"
}
func (SpearCharge) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 5 || len(args) > 7 {
		_ = agent.SendChat("Usage: spearCharge <entityID> <itemName> <holdMs> <engagedMs> <tiredMs> [minSpeed] [minAlignment]")
		return models.Done(nil), nil
	}
	targetID, err := parseCombatEntityID(args[0], agent)
	if err != nil {
		return models.Done(nil), nil
	}
	ms := make([]float64, 5)
	for i := 0; i < 3; i++ {
		n, parseErr := strconv.Atoi(args[i+2])
		if parseErr != nil || n <= 0 {
			_ = agent.SendChat("Spear charge durations must be positive milliseconds")
			return models.Done(nil), nil
		}
		ms[i] = float64(n)
	}
	if len(args) >= 6 {
		ms[3], err = strconv.ParseFloat(args[5], 64)
		if err != nil {
			_ = agent.SendChat("Invalid spear minimum speed")
			return models.Done(nil), nil
		}
	}
	if len(args) == 7 {
		ms[4], err = strconv.ParseFloat(args[6], 64)
		if err != nil || ms[4] < -1 || ms[4] > 1 {
			_ = agent.SendChat("Invalid spear minimum alignment")
			return models.Done(nil), nil
		}
	}
	spear, ok := agent.(interface {
		SpearChargeAt(context.Context, int32, string, time.Duration, time.Duration, time.Duration, float64, float64) error
	})
	if !ok {
		return models.Done(fmt.Errorf("spear Charge action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		resolve(spear.SpearChargeAt(ctx, targetID, args[1], time.Duration(ms[0])*time.Millisecond,
			time.Duration(ms[1])*time.Millisecond, time.Duration(ms[2])*time.Millisecond, ms[3], ms[4]))
	}()
	return completion, nil
}

func parseCombatEntityID(s string, agent models.CommandAgent) (int32, error) {
	value, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		_ = agent.SendChat("Invalid entity ID")
	}
	return int32(value), err
}

func executeCombatEntityAction(ctx context.Context, agent models.CommandAgent, args []string, usage string, execute func(int32) error) (models.Completion, error) {
	if len(args) != 1 {
		_ = agent.SendChat("Usage: " + usage)
		return models.Done(nil), nil
	}
	targetID, err := parseCombatEntityID(args[0], agent)
	if err != nil {
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(execute(targetID)) }()
	return completion, nil
}

func (RunCombat) Name() string  { return "runcombat" }
func (RunCombat) Usage() string { return "runCombat [radius] [includeNeutral]" }
func (RunCombat) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) > 2 {
		_ = agent.SendChat("Usage: runCombat [radius] [includeNeutral]")
		return models.Done(nil), nil
	}
	radius := 16.0
	includeNeutral := false
	var err error
	if len(args) >= 1 {
		radius, err = parseFloat(args[0])
		if err != nil || radius <= 0 {
			_ = agent.SendChat("Usage: runCombat [radius] [includeNeutral] - radius must be positive")
			return models.Done(nil), nil
		}
	}
	if len(args) == 2 {
		includeNeutral, err = strconv.ParseBool(args[1])
		if err != nil {
			_ = agent.SendChat("Usage: runCombat [radius] [includeNeutral] - includeNeutral must be true or false")
			return models.Done(nil), nil
		}
	}
	runner, ok := agent.(interface {
		RunCombat(context.Context, float64, bool) error
	})
	if !ok {
		return models.Done(fmt.Errorf("combat action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(runner.RunCombat(ctx, radius, includeNeutral)) }()
	return completion, nil
}

type RunCombatWithPolicy struct{}

func (RunCombatWithPolicy) Name() string { return "runcombatwithpolicy" }
func (RunCombatWithPolicy) Usage() string {
	return "runCombatWithPolicy <radius> [includePlayers] [includeNeutral]"
}
func (RunCombatWithPolicy) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 || len(args) > 3 {
		_ = agent.SendChat("Usage: runCombatWithPolicy <radius> [includePlayers] [includeNeutral]")
		return models.Done(nil), nil
	}
	radius, err := parseFloat(args[0])
	if err != nil || radius <= 0 {
		_ = agent.SendChat("Usage: runCombatWithPolicy <radius> [includePlayers] [includeNeutral] - radius must be positive")
		return models.Done(nil), nil
	}
	includePlayers, includeNeutral := false, false
	if len(args) >= 2 {
		includePlayers, err = strconv.ParseBool(args[1])
		if err != nil {
			_ = agent.SendChat("Usage: runCombatWithPolicy <radius> [includePlayers] [includeNeutral] - flags must be true or false")
			return models.Done(nil), nil
		}
	}
	if len(args) == 3 {
		includeNeutral, err = strconv.ParseBool(args[2])
		if err != nil {
			_ = agent.SendChat("Usage: runCombatWithPolicy <radius> [includePlayers] [includeNeutral] - flags must be true or false")
			return models.Done(nil), nil
		}
	}
	runner, ok := agent.(interface {
		RunCombatWithPolicy(context.Context, float64, combat.TargetPolicy) error
	})
	if !ok {
		return models.Done(fmt.Errorf("combat policy action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		resolve(runner.RunCombatWithPolicy(ctx, radius, combat.TargetPolicy{
			IncludePlayers: includePlayers,
			IncludeNeutral: includeNeutral,
		}))
	}()
	return completion, nil
}

type Mount struct{}

func (Mount) Name() string  { return "mount" }
func (Mount) Usage() string { return "mount <entityID | entityType>" }
func (Mount) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: mount <entityID | entityType>")
		return models.Done(nil), nil
	}
	if entityID, err := strconv.ParseInt(args[0], 10, 32); err == nil {
		if err := agent.MountEntity(ctx, int32(entityID)); err != nil {
			_ = agent.SendChat(fmt.Sprintf("Mount failed: %v", err))
			return models.Done(nil), nil
		}
		_ = agent.SendChat(fmt.Sprintf("Attempting to mount entity %d", entityID))
		return models.Done(nil), nil
	}

	entityType := args[0]
	if err := agent.MountNearest(ctx, entityType); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Mount failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Attempting to mount nearest " + entityType)
	return models.Done(nil), nil
}

type Dismount struct{}

func (Dismount) Name() string  { return "dismount" }
func (Dismount) Usage() string { return "dismount" }
func (Dismount) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if err := agent.DismountEntity(); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Dismount failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Dismounting...")
	return models.Done(nil), nil
}

type VehicleJump struct{}

func (VehicleJump) Name() string  { return "vehiclejump" }
func (VehicleJump) Usage() string { return "vehiclejump [power]" }
func (VehicleJump) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	power := int32(100) // Default to maximum power
	if len(args) > 0 {
		p, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil {
			_ = agent.SendChat("Usage: vehiclejump [power] - power must be 0-100")
			return models.Done(nil), nil
		}
		power = int32(p)
	}

	if err := agent.JumpVehicle(ctx, power); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Jump failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat(fmt.Sprintf("Jumping with power %d", power))
	return models.Done(nil), nil
}

// mineSearchRadius is the default search radius (in blocks) for
// "mine <blockName>"'s FindVisibleBlock lookup, when no explicit radius
// argument is given — see Mine.Usage. Not the only radius available: a
// hardcoded-with-no-override radius here previously let this dispatch a
// FindVisibleBlock search entirely disconnected from a caller's own idea
// of a reasonable search volume (rlenv's Config.MineSearchRadius, in
// particular — see rlenv/action.go's ActionMine, which no longer even
// dispatches through this by-name form for exactly that reason), so an
// explicit override is worth having even though this default itself
// rarely needs to change.
const mineSearchRadius = 32

type Mine struct{}

func (Mine) Name() string  { return "mine" }
func (Mine) Usage() string { return "mine <x> <y> <z> | mine <blockName> [radius]" }
func (Mine) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) == 1 || len(args) == 2 {
		blockName := args[0]
		radius := mineSearchRadius
		if len(args) == 2 {
			r, ok := parsePositiveIntArg(args[1])
			if !ok {
				_ = agent.SendChat("Usage: mine <blockName> [radius] - radius must be a positive integer")
				return models.Done(nil), nil
			}
			radius = r
		}
		x, y, z, found, err := agent.FindVisibleBlock(ctx, blockName, radius)
		if err != nil {
			_ = agent.SendChat("Find block error: " + err.Error())
			return models.Done(nil), nil
		}
		if !found {
			_ = agent.SendChat(fmt.Sprintf("No visible %s found within %d blocks", blockName, radius))
			return models.Done(nil), nil
		}
		return mineAt(ctx, agent, x, y, z), nil
	}

	if len(args) != 3 {
		_ = agent.SendChat("Usage: mine <x> <y> <z> | mine <blockName> [radius]")
		return models.Done(nil), nil
	}
	x, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	y, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	z, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	return mineAt(ctx, agent, x, y, z), nil
}

// mineAt launches MineBlockAt in a goroutine (it blocks for the block's real
// break time) and returns a Completion that resolves with the outcome —
// mirrors FireBowAt's pattern for actions with real wait time. The face
// argument to MineBlockAt is a placeholder: the real implementation
// computes the best face itself from the agent's position (see
// agent/actions.go's MineBlockAt doc comment).
func mineAt(ctx context.Context, agent models.CommandAgent, x, y, z float64) models.Completion {
	completion, resolve := models.NewCompletion()
	go func() {
		err := agent.MineBlockAt(ctx, models.V3{X: x, Y: y, Z: z}, models.FaceDown)
		if err != nil {
			_ = agent.SendChat("Mine error: " + err.Error())
		}
		resolve(err)
	}()
	return completion
}

// lookAroundDefaultRadius is the search radius (in blocks) "lookAround"
// uses when no explicit radius argument is given.
const lookAroundDefaultRadius = 16.0

// lookAroundChatLimit caps how many results "lookAround" lists in chat —
// FindAllVisibleEntitiesInSphere itself returns the full, untruncated list
// for programmatic callers (e.g. a future rlenv observation).
const lookAroundChatLimit = 8

type LookAround struct{}

func (LookAround) Name() string  { return "lookaround" }
func (LookAround) Usage() string { return "lookAround [radius]" }
func (LookAround) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	radius := lookAroundDefaultRadius
	if len(args) > 0 {
		r, err := parseFloat(args[0])
		if err != nil || r <= 0 {
			_ = agent.SendChat("Usage: lookAround [radius] - radius must be a positive number")
			return models.Done(nil), nil
		}
		radius = r
	}

	entities, err := agent.FindAllVisibleEntitiesInSphere(ctx, radius)
	if err != nil {
		_ = agent.SendChat("Look around error: " + err.Error())
		return models.Done(nil), nil
	}
	if len(entities) == 0 {
		_ = agent.SendChat(fmt.Sprintf("Nothing visible within %.0f blocks", radius))
		return models.Done(nil), nil
	}

	shown := entities
	if len(shown) > lookAroundChatLimit {
		shown = shown[:lookAroundChatLimit]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Visible within %.0f blocks (%d total):", radius, len(entities))
	for _, e := range shown {
		fmt.Fprintf(&b, " %s@(%.0f,%.0f,%.0f,%.1fm)", e.TypeName, e.X, e.Y, e.Z, e.Distance)
	}
	if len(entities) > len(shown) {
		fmt.Fprintf(&b, " ...+%d more", len(entities)-len(shown))
	}
	_ = agent.SendChat(b.String())
	return models.Done(nil), nil
}

// pickUpNearbyItemDefaultMaxDistance is the search radius (in blocks)
// "pickUpNearbyItem" uses when no explicit maxDistance argument is given —
// see the item auto-pickup discussion in testing/mine_test.go's doc
// comments for why this is deliberately larger than vanilla's much shorter
// (~1.5 block) auto-pickup radius: this command finds a *visible* item and
// walks to it, it doesn't require starting already in pickup range.
const pickUpNearbyItemDefaultMaxDistance = 8.0

type PickUpNearbyItem struct{}

func (PickUpNearbyItem) Name() string  { return "pickupnearbyitem" }
func (PickUpNearbyItem) Usage() string { return "pickUpNearbyItem [maxDistance]" }
func (PickUpNearbyItem) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	maxDistance := pickUpNearbyItemDefaultMaxDistance
	if len(args) > 0 {
		d, err := parseFloat(args[0])
		if err != nil || d <= 0 {
			_ = agent.SendChat("Usage: pickUpNearbyItem [maxDistance] - maxDistance must be a positive number")
			return models.Done(nil), nil
		}
		maxDistance = d
	}

	// An agent that can collect by getting within pickup range does so:
	// walking to the item's own cell fails whenever that cell is not
	// walkable (a drop under a still-standing log, in a hole), where a cell
	// beside it works just as well.
	if collector, ok := agent.(models.ItemCollector); ok {
		completion, resolve := models.NewCompletion()
		go func() {
			n, err := collector.CollectNearbyItems(ctx, maxDistance)
			if err != nil {
				_ = agent.SendChat("Pick up item error: " + err.Error())
			} else if n == 0 {
				_ = agent.SendChat(fmt.Sprintf("No visible item found within %.0f blocks", maxDistance))
			}
			resolve(err)
		}()
		return completion, nil
	}

	_, x, y, z, found, err := agent.FindNearestVisibleItem(ctx, maxDistance)
	if err != nil {
		_ = agent.SendChat("Find item error: " + err.Error())
		return models.Done(nil), nil
	}
	if !found {
		_ = agent.SendChat(fmt.Sprintf("No visible item found within %.0f blocks", maxDistance))
		return models.Done(nil), nil
	}

	completion, resolve := models.NewCompletion()
	go func() {
		// Vanilla auto-collects any item the player walks within ~1 block
		// of, so walking to the item's own position is sufficient — no
		// separate "pick up" packet/interaction is needed once there.
		err := agent.MoveToWithChat(ctx, x, y, z)
		if err != nil {
			_ = agent.SendChat("Pick up item error: " + err.Error())
		}
		resolve(err)
	}()
	return completion, nil
}

type KillCreeperForGunpowder struct{}

func (KillCreeperForGunpowder) Name() string  { return "killcreeperforgunpowder" }
func (KillCreeperForGunpowder) Usage() string { return "killCreeperForGunpowder" }
func (KillCreeperForGunpowder) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	killer, ok := agent.(interface {
		KillCreeperForGunpowder(context.Context) (bool, error)
	})
	if !ok {
		return models.Done(fmt.Errorf("kill creeper action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		_, err := killer.KillCreeperForGunpowder(ctx)
		resolve(err)
	}()
	return completion, nil
}

type Craft struct{}

func (Craft) Name() string  { return "craft" }
func (Craft) Usage() string { return "craft <itemName>" }
func (Craft) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 1 {
		_ = agent.SendChat("Usage: craft <itemName>")
		return models.Done(nil), nil
	}
	itemName := args[0]

	// CraftItem's placement/collection sequence involves several inventory
	// clicks, each with its own wait-for-confirmation timeout, so it can
	// take real time — same goroutine+Completion pattern as Mine's mineAt.
	completion, resolve := models.NewCompletion()
	go func() {
		err := agent.CraftItem(ctx, itemName)
		if err != nil {
			_ = agent.SendChat("Craft error: " + err.Error())
		}
		resolve(err)
	}()
	return completion, nil
}

var (
	placeErrorChatMu   sync.Mutex
	placeErrorChatLast time.Time
)

// placeErrorChatEvery is the least gap between two "Place error" chat lines.
const placeErrorChatEvery = 5 * time.Second

// placeErrorChatAllowed reports whether a Place error may be chatted at now,
// and if so records it.
func placeErrorChatAllowed(now time.Time) bool {
	placeErrorChatMu.Lock()
	defer placeErrorChatMu.Unlock()
	if now.Sub(placeErrorChatLast) < placeErrorChatEvery {
		return false
	}
	placeErrorChatLast = now
	return true
}

// Place places a block from the inventory on the ground next to the bot and
// confirms it appeared (models.BlockPlacer).
type Place struct{}

func (Place) Name() string  { return "place" }
func (Place) Usage() string { return "place <itemName>" }
func (Place) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 1 {
		_ = agent.SendChat("Usage: place <itemName>")
		return models.Done(nil), nil
	}
	placer, ok := agent.(models.BlockPlacer)
	if !ok {
		_ = agent.SendChat("Place error: this agent cannot place blocks")
		return models.Done(nil), nil
	}
	itemName := args[0]
	completion, resolve := models.NewCompletion()
	go func() {
		cell, err := placer.PlaceHeldBlock(ctx, itemName)
		if err != nil {
			// A placement that fails instantly can be retried a step later,
			// again and again; a chat line each time gets the bot kicked for
			// spamming (found live: 55 in a few milliseconds). Say so at most
			// every few seconds.
			if placeErrorChatAllowed(time.Now()) {
				_ = agent.SendChat("Place error: " + err.Error())
			}
		} else {
			_ = agent.SendChat(fmt.Sprintf("Placed %s at (%.0f, %.0f, %.0f)", itemName, cell.X, cell.Y, cell.Z))
		}
		resolve(err)
	}()
	return completion, nil
}

// buildStructureSummaryMaxFailures bounds how many individual failures
// BuildStructure lists in its chat summary - full detail for every failure
// in a large build would risk the same chat-spam problem
// placeErrorChatAllowed guards against elsewhere.
const buildStructureSummaryMaxFailures = 5

// BuildStructure loads a structure/template file and builds it in-world via
// real block placement (models.CommandAgent.BuildStructure) - see
// docs/STRUCTURE_LOADER.md.
type BuildStructure struct{}

func (BuildStructure) Name() string  { return "buildstructure" }
func (BuildStructure) Usage() string { return "buildStructure <path> <x> <y> <z>" }
func (BuildStructure) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 4 {
		_ = agent.SendChat("Usage: buildStructure <path> <x> <y> <z>")
		return models.Done(nil), nil
	}
	path := args[0]
	x, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	y, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	z, err := parseFloat(args[3])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}

	// A real structure can take minutes to place block-by-block - same
	// goroutine+Completion pattern as Craft/Mine's longer-running actions.
	completion, resolve := models.NewCompletion()
	go func() {
		result, err := agent.BuildStructure(ctx, path, models.V3{X: x, Y: y, Z: z})
		if err != nil {
			_ = agent.SendChat("Build structure error: " + err.Error())
			resolve(err)
			return
		}
		_ = agent.SendChat(buildStructureSummary(result))
		resolve(nil)
	}()
	return completion, nil
}

// buildStructureSummary formats BuildStructure's result as a single chat
// line: a placed/failed count, plus up to buildStructureSummaryMaxFailures
// failure positions so a caller knows what to look at without needing logs
// for a typical small number of failures.
func buildStructureSummary(result models.BuildStructureResult) string {
	if len(result.Failed) == 0 {
		return fmt.Sprintf("Build complete: placed %d blocks", result.Placed)
	}
	shown := result.Failed
	more := 0
	if len(shown) > buildStructureSummaryMaxFailures {
		more = len(shown) - buildStructureSummaryMaxFailures
		shown = shown[:buildStructureSummaryMaxFailures]
	}
	parts := make([]string, 0, len(shown))
	for _, f := range shown {
		parts = append(parts, fmt.Sprintf("%s at (%.0f, %.0f, %.0f)", f.Item, f.Pos.X, f.Pos.Y, f.Pos.Z))
	}
	msg := fmt.Sprintf("Build complete: placed %d blocks, %d failed: %s", result.Placed, len(result.Failed), strings.Join(parts, "; "))
	if more > 0 {
		msg += fmt.Sprintf(" (+%d more)", more)
	}
	return msg
}

// materialListSummaryMaxItems bounds how many distinct item types
// MaterialList's chat line lists directly - same spam-avoidance reasoning
// as buildStructureSummaryMaxFailures. The exported file (when a second
// argument is given) always has the complete list regardless.
const materialListSummaryMaxItems = 10

// MaterialList loads a structure file and reports (and, optionally,
// persists to a JSON file) everything it needs to build - the "what do I
// need to gather" list a caller can check against the bot's current
// inventory today, or - once a long-term-memory system exists - against
// whatever's stored across chests/shulker boxes/barrels/etc., without this
// command or the underlying structure.MaterialList needing to change; see
// docs/STRUCTURE_LOADER.md. Doesn't need anything from the agent beyond
// SendChat - the computation itself is pure file-and-math,
// independent of world/inventory state.
type MaterialList struct{}

func (MaterialList) Name() string  { return "materiallist" }
func (MaterialList) Usage() string { return "materialList <structurePath> [exportPath]" }
func (MaterialList) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 1 && len(args) != 2 {
		_ = agent.SendChat("Usage: materialList <structurePath> [exportPath]")
		return models.Done(nil), nil
	}

	s, err := structure.LoadFile(args[0])
	if err != nil {
		_ = agent.SendChat("materialList error: " + err.Error())
		return models.Done(nil), nil
	}
	ml := structure.ComputeMaterialList(s)
	ml.Source = args[0]

	exported := len(args) == 2
	if exported {
		if err := ml.WriteJSON(args[1]); err != nil {
			_ = agent.SendChat("materialList error: write " + args[1] + ": " + err.Error())
			return models.Done(nil), nil
		}
	}

	_ = agent.SendChat(materialListSummary(ml, exported))
	return models.Done(nil), nil
}

// materialListSummary formats ml as a single chat line: total block count,
// distinct item-type count, up to materialListSummaryMaxItems "<count>x
// <item>" entries, and - if exported - a note that the complete list was
// written to a file (the chat line itself may be truncated; the file never
// is).
func materialListSummary(ml structure.MaterialList, exported bool) string {
	if len(ml.Items) == 0 {
		return "Material list: structure is empty (no non-air blocks)"
	}
	shown := ml.Items
	more := 0
	if len(shown) > materialListSummaryMaxItems {
		more = len(shown) - materialListSummaryMaxItems
		shown = shown[:materialListSummaryMaxItems]
	}
	parts := make([]string, 0, len(shown))
	for _, e := range shown {
		parts = append(parts, fmt.Sprintf("%dx %s", e.Count, e.Item))
	}
	msg := fmt.Sprintf("Materials needed (%d blocks, %d item types): %s", ml.Total(), len(ml.Items), strings.Join(parts, ", "))
	if more > 0 {
		msg += fmt.Sprintf(" (+%d more types)", more)
	}
	if exported {
		msg += " - full list written to file"
	}
	return msg
}

type Equip struct{}

func (Equip) Name() string  { return "equip" }
func (Equip) Usage() string { return "equip <item>" }
func (Equip) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: equip <item>")
		return models.Done(nil), nil
	}
	itemName := strings.Join(args, " ")
	if err := agent.Equip(ctx, itemName); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Equip failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Equipped " + itemName)
	return models.Done(nil), nil
}

type UseItem struct{}

func (UseItem) Name() string  { return "useitem" }
func (UseItem) Usage() string { return "useItem [offhand]" }
func (UseItem) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	hand := models.MainHand
	if len(args) > 0 && strings.EqualFold(args[0], "offhand") {
		hand = models.OffHand
	}
	if err := agent.UseItem(ctx, hand); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Use item failed: %v", err))
		return models.Done(nil), nil
	}
	return models.Done(nil), nil
}

type FlyTo struct{}

func (FlyTo) Name() string  { return "flyto" }
func (FlyTo) Usage() string { return "flyTo <x> <y> <z>" }
func (FlyTo) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: flyTo <x> <y> <z>")
		return models.Done(nil), nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return models.Done(nil), nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return models.Done(nil), nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return models.Done(nil), nil
	}
	completion, resolve := models.NewCompletion()
	go func() {
		err := agent.FlyTo(ctx, tx, ty, tz)
		if err != nil {
			_ = agent.SendChat(fmt.Sprintf("FlyTo failed: %v", err))
		}
		resolve(err)
	}()
	return completion, nil
}

// Fly and Land toggle creative/spectator-style flying (see
// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4) - distinct from FlyTo,
// which pilots an elytra glide and requires one equipped. SetFlying itself
// refuses to enable flying unless the server has granted AllowFlying
// (creative/spectator, or a survival player an op granted it to), so a
// misuse in survival reports a clear chat error rather than silently
// no-oping.
type Fly struct{}

func (Fly) Name() string  { return "fly" }
func (Fly) Usage() string { return "fly" }
func (Fly) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if err := agent.SetFlying(ctx, true); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Fly failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Flying enabled")
	return models.Done(nil), nil
}

type Land struct{}

func (Land) Name() string  { return "land" }
func (Land) Usage() string { return "land" }
func (Land) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if err := agent.SetFlying(ctx, false); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Land failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Flying disabled")
	return models.Done(nil), nil
}

// defaultCamFollowDistance is used when followCam's optional distance
// argument is omitted.
const defaultCamFollowDistance = 8.0

type FollowCam struct{}

func (FollowCam) Name() string  { return "followcam" }
func (FollowCam) Usage() string { return "followCam <playerName> [maxDistance]" }
func (FollowCam) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: followCam <playerName> [maxDistance]")
		return models.Done(nil), nil
	}
	targetName := args[0]
	maxDistance := defaultCamFollowDistance
	if len(args) > 1 {
		d, err := parseFloat(args[1])
		if err != nil {
			_ = agent.SendChat("Invalid maxDistance")
			return models.Done(nil), nil
		}
		maxDistance = d
	}
	if err := agent.StartCamFollow(ctx, targetName, maxDistance); err != nil {
		_ = agent.SendChat(fmt.Sprintf("FollowCam failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat(fmt.Sprintf("Following %s (spectator, max %.1f blocks)", targetName, maxDistance))
	return models.Done(nil), nil
}

type StopFollowCam struct{}

func (StopFollowCam) Name() string  { return "stopfollowcam" }
func (StopFollowCam) Usage() string { return "stopFollowCam" }
func (StopFollowCam) Execute(ctx context.Context, agent models.CommandAgent, _ []string) (models.Completion, error) {
	if err := agent.StopCamFollow(); err != nil {
		_ = agent.SendChat(fmt.Sprintf("StopFollowCam failed: %v", err))
		return models.Done(nil), nil
	}
	_ = agent.SendChat("Stopped following")
	return models.Done(nil), nil
}
