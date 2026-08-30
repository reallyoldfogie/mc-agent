package actions

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
)

const helpText = "Commands: help, pos, say <text>, testMove, moveTo <x> <y> <z> (pathfinding), lineTo <x> <y> <z> (straight-line), moveForward <distance>, moveUp <distance>, moveUpAndSneak <distance>, moveToAndSneak <x> <y> <z>, lineToAndSneak <x> <y> <z>, stopSneak, findPath <x> <y> <z>, testPath, follow [<player>], stopFollow, followStatus, startTracking, stopTracking, fireBow, mount <entityID | entityType>, dismount, vehiclejump [power], equip <item>, useItem [offhand], flyTo <x> <y> <z>, planStatus, planStop"

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

type Help struct{}

func (Help) Name() string  { return "help" }
func (Help) Usage() string { return "help" }
func (Help) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	_ = agent.SendChat(helpText)
	return nil
}

type Pos struct{}

func (Pos) Name() string  { return "pos" }
func (Pos) Usage() string { return "pos" }
func (Pos) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	pos, _, _, ok := agent.GetPosition()
	if !ok {
		_ = agent.SendChat("Bot position not initialized")
		return nil
	}
	_ = agent.SendChat(fmt.Sprintf("Current position: %.2f, %.2f, %.2f", pos.X, pos.Y, pos.Z))
	return nil
}

type Say struct{}

func (Say) Name() string  { return "say" }
func (Say) Usage() string { return "say <text>" }
func (Say) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	_ = agent.SendChat(strings.Join(args, " "))
	return nil
}

type TestMove struct{}

func (TestMove) Name() string  { return "testmove" }
func (TestMove) Usage() string { return "testMove" }
func (TestMove) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	go agent.TestMove()
	return nil
}

type MoveTo struct{}

func (MoveTo) Name() string  { return "moveto" }
func (MoveTo) Usage() string { return "moveTo <x> <y> <z>" }
func (MoveTo) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: moveTo <x> <y> <z>")
		return nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}
	go func() {
		if err := agent.MoveToWithChat(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat(fmt.Sprintf("MoveToWithChat - Pathfinding failed: %v", err))
		}
	}()
	return nil
}

type LineTo struct{}

func (LineTo) Name() string  { return "lineto" }
func (LineTo) Usage() string { return "lineTo <x> <y> <z> (straight-line, flat world only)" }
func (LineTo) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: lineTo <x> <y> <z> (straight-line, flat world only)")
		return nil
	}

	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}

	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}

	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}

	pos, _, _, ok := agent.GetPosition()
	if !ok {
		_ = agent.SendChat("Bot position not initialized")
		return nil
	}

	total := pos.DistanceTo(models.V3{X: tx, Y: ty, Z: tz})
	_ = agent.SendChat(fmt.Sprintf("Moving direct from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [%.2f blocks]", pos.X, pos.Y, pos.Z, tx, ty, tz, total))
	go func() {
		if err := agent.LineTo(ctx, tx, ty, tz, true); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
		}
	}()
	return nil
}

type MoveForward struct{}

func (MoveForward) Name() string  { return "moveforward" }
func (MoveForward) Usage() string { return "moveForward <distance>" }
func (MoveForward) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: moveForward <distance>")
		return nil
	}
	dist, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid distance")
		return nil
	}
	go func() {
		_ = agent.SendChat(fmt.Sprintf("Moving forward %.2f blocks", dist))
		if err := agent.MoveForward(ctx, dist); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			return
		}
		_ = agent.SendChat("Move forward complete")
	}()
	return nil
}

type MoveUp struct{}

func (MoveUp) Name() string  { return "moveup" }
func (MoveUp) Usage() string { return "moveUp <distance>" }
func (MoveUp) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: moveUp <distance>")
		return nil
	}
	dist, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid distance")
		return nil
	}
	go func() {
		if err := agent.MoveUp(ctx, dist); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			return
		}
		_ = agent.SendChat("Move up complete")
	}()
	return nil
}

type MoveUpAndSneak struct{}

func (MoveUpAndSneak) Name() string  { return "moveupandsneak" }
func (MoveUpAndSneak) Usage() string { return "moveUpAndSneak <distance>" }
func (MoveUpAndSneak) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: moveUpAndSneak <distance>")
		return nil
	}
	dist, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid distance")
		return nil
	}
	go func() {
		if err := agent.MoveUpAndSneak(ctx, dist); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			return
		}
		_ = agent.SendChat("Move up complete, now sneaking")
	}()
	return nil
}

type MoveToAndSneak struct{}

func (MoveToAndSneak) Name() string  { return "movetoandsneak" }
func (MoveToAndSneak) Usage() string { return "moveToAndSneak <x> <y> <z>" }
func (MoveToAndSneak) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: moveToAndSneak <x> <y> <z>")
		return nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}
	go func() {
		if err := agent.MoveToAndSneak(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			return
		}
		_ = agent.SendChat("Movement complete, now sneaking")
	}()
	return nil
}

type LineToAndSneak struct{}

func (LineToAndSneak) Name() string  { return "linetoandsneak" }
func (LineToAndSneak) Usage() string { return "lineToAndSneak <x> <y> <z>" }
func (LineToAndSneak) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: lineToAndSneak <x> <y> <z>")
		return nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}
	go func() {
		if err := agent.LineToAndSneak(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
			return
		}
		_ = agent.SendChat("Movement complete, now sneaking")
	}()
	return nil
}

type StopSneak struct{}

func (StopSneak) Name() string  { return "stopsneak" }
func (StopSneak) Usage() string { return "stopSneak" }
func (StopSneak) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	if err := agent.StopSneaking(); err != nil {
		_ = agent.SendChat("Failed to stop sneaking: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Stopped sneaking")
	return nil
}

type FindPath struct{}

func (FindPath) Name() string  { return "findpath" }
func (FindPath) Usage() string { return "findPath <x> <y> <z>" }
func (FindPath) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: findPath <x> <y> <z>")
		return nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}
	go func() {
		if _, err := agent.FindPath(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Path find failed: " + err.Error())
			return
		}
		_ = agent.SendChat("Path computed")
	}()
	return nil
}

type TestPath struct{}

func (TestPath) Name() string  { return "testpath" }
func (TestPath) Usage() string { return "testPath" }
func (TestPath) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	go agent.TestPath()
	return nil
}

type Follow struct{}

func (Follow) Name() string  { return "follow" }
func (Follow) Usage() string { return "follow [<player>]" }
func (Follow) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 1 {
		nearest, ok := agent.NearestPlayerInfo(ctx, true)
		if !ok {
			_ = agent.SendChat("Failed to find nearest player")
			return nil
		}
		_ = agent.SendChat(fmt.Sprintf("Found nearest player at %.1f blocks, starting to follow...", nearest.Distance))
		_ = agent.SendChat("Note: Following nearest player requires name. Use 'follow <playername>' instead.")
		return nil
	}
	if !agent.HasFollowManager() {
		_ = agent.SendChat("Follow system not available")
		return nil
	}
	if err := agent.Follow(ctx, args[0]); err != nil {
		_ = agent.SendChat("Follow error: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Following " + args[0])
	return nil
}

type StopFollow struct{}

func (StopFollow) Name() string  { return "stopfollow" }
func (StopFollow) Usage() string { return "stopFollow" }
func (StopFollow) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	if !agent.HasFollowManager() {
		_ = agent.SendChat("Follow system not available")
		return nil
	}
	if !agent.IsFollowing() {
		_ = agent.SendChat("Not currently following anyone")
		return nil
	}
	if err := agent.StopFollow(ctx); err != nil {
		_ = agent.SendChat("Stop error: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Stopped following")
	return nil
}

type FollowStatus struct{}

func (FollowStatus) Name() string  { return "followstatus" }
func (FollowStatus) Usage() string { return "followStatus" }
func (FollowStatus) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	_ = agent.SendChat(agent.FollowStatus(ctx))
	return nil
}

type StartTracking struct{}

func (StartTracking) Name() string  { return "starttracking" }
func (StartTracking) Usage() string { return "startTracking" }
func (StartTracking) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	agent.StartTracking()
	return nil
}

type StopTracking struct{}

func (StopTracking) Name() string  { return "stoptracking" }
func (StopTracking) Usage() string { return "stopTracking" }
func (StopTracking) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	agent.StopTracking()
	return nil
}

type PlanStatus struct{}

func (PlanStatus) Name() string  { return "planstatus" }
func (PlanStatus) Usage() string { return "planStatus" }
func (PlanStatus) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	status := agent.PlanStatus(ctx)
	_ = agent.SendChat(status.String())
	return nil
}

type PlanStop struct{}

func (PlanStop) Name() string  { return "planstop" }
func (PlanStop) Usage() string { return "planStop" }
func (PlanStop) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	if err := agent.StopPlan(ctx); err != nil {
		_ = agent.SendChat("Plan stop error: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Plan stopped")
	return nil
}

type FireBow struct{}

func (FireBow) Name() string  { return "firebow" }
func (FireBow) Usage() string { return "fireBow" }
func (FireBow) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	go func() {
		if err := agent.FireBow(ctx); err != nil {
			_ = agent.SendChat("Fire bow error: " + err.Error())
		}
	}()
	return nil
}

type FireBowAt struct{}

func (FireBowAt) Name() string { return "firebowat" }
func (FireBowAt) Usage() string {
	return "fireBowAt <x> <y> <z> | fireBowAt nearest | fireBowAt <player>"
}
func (FireBowAt) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) == 0 {
		go func() {
			if err := agent.FireBow(ctx); err != nil {
				_ = agent.SendChat("Fire bow error: " + err.Error())
			}
		}()
		return nil
	}
	if len(args) == 1 {
		if args[0] == "nearest" {
			playerInfo, found := agent.NearestPlayerInfo(ctx, true)
			if found {
				go func() {
					if _, err := agent.FireBowAt(ctx, playerInfo.X, playerInfo.Y, playerInfo.Z); err != nil {
						_ = agent.SendChat("Fire bow at error: " + err.Error())
					}
				}()
				return nil
			}
		} else {
			x, y, z, found, err := agent.FindPlayerByName(ctx, args[0])
			if err == nil && found {
				go func() {
					if _, err := agent.FireBowAt(ctx, x, y, z); err != nil {
						_ = agent.SendChat("Fire bow at error: " + err.Error())
					}
				}()
				return nil
			}
			_ = agent.SendChat("Player not found: " + args[0])
			return nil
		}
	}
	if len(args) < 3 {
		_ = agent.SendChat("Usage: fireBowAt <x> <y> <z> | fireBowAt nearest | fireBowAt <player>")
		return nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}
	go func() {
		if _, err := agent.FireBowAt(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat("Fire bow at error: " + err.Error())
		}
	}()
	return nil
}

type Mount struct{}

func (Mount) Name() string  { return "mount" }
func (Mount) Usage() string { return "mount <entityID | entityType>" }
func (Mount) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: mount <entityID | entityType>")
		return nil
	}
	if entityID, err := strconv.ParseInt(args[0], 10, 32); err == nil {
		if err := agent.MountEntity(ctx, int32(entityID)); err != nil {
			_ = agent.SendChat(fmt.Sprintf("Mount failed: %v", err))
			return nil
		}
		_ = agent.SendChat(fmt.Sprintf("Attempting to mount entity %d", entityID))
		return nil
	}

	entityType := args[0]
	if err := agent.MountNearest(ctx, entityType); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Mount failed: %v", err))
		return nil
	}
	_ = agent.SendChat("Attempting to mount nearest " + entityType)
	return nil
}

type Dismount struct{}

func (Dismount) Name() string  { return "dismount" }
func (Dismount) Usage() string { return "dismount" }
func (Dismount) Execute(ctx context.Context, agent models.CommandAgent, _ []string) error {
	if err := agent.DismountEntity(); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Dismount failed: %v", err))
		return nil
	}
	_ = agent.SendChat("Dismounting...")
	return nil
}

type VehicleJump struct{}

func (VehicleJump) Name() string  { return "vehiclejump" }
func (VehicleJump) Usage() string { return "vehiclejump [power]" }
func (VehicleJump) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	power := int32(100) // Default to maximum power
	if len(args) > 0 {
		p, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil {
			_ = agent.SendChat("Usage: vehiclejump [power] - power must be 0-100")
			return nil
		}
		power = int32(p)
	}

	if err := agent.JumpVehicle(ctx, power); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Jump failed: %v", err))
		return nil
	}
	_ = agent.SendChat(fmt.Sprintf("Jumping with power %d", power))
	return nil
}

type Equip struct{}

func (Equip) Name() string  { return "equip" }
func (Equip) Usage() string { return "equip <item>" }
func (Equip) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 1 {
		_ = agent.SendChat("Usage: equip <item>")
		return nil
	}
	itemName := strings.Join(args, " ")
	if err := agent.Equip(ctx, itemName); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Equip failed: %v", err))
		return nil
	}
	_ = agent.SendChat("Equipped " + itemName)
	return nil
}

type UseItem struct{}

func (UseItem) Name() string  { return "useitem" }
func (UseItem) Usage() string { return "useItem [offhand]" }
func (UseItem) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	hand := models.MainHand
	if len(args) > 0 && strings.EqualFold(args[0], "offhand") {
		hand = models.OffHand
	}
	if err := agent.UseItem(ctx, hand); err != nil {
		_ = agent.SendChat(fmt.Sprintf("Use item failed: %v", err))
		return nil
	}
	return nil
}

type FlyTo struct{}

func (FlyTo) Name() string  { return "flyto" }
func (FlyTo) Usage() string { return "flyTo <x> <y> <z>" }
func (FlyTo) Execute(ctx context.Context, agent models.CommandAgent, args []string) error {
	if len(args) < 3 {
		_ = agent.SendChat("Usage: flyTo <x> <y> <z>")
		return nil
	}
	tx, err := parseFloat(args[0])
	if err != nil {
		_ = agent.SendChat("Invalid X coordinate")
		return nil
	}
	ty, err := parseFloat(args[1])
	if err != nil {
		_ = agent.SendChat("Invalid Y coordinate")
		return nil
	}
	tz, err := parseFloat(args[2])
	if err != nil {
		_ = agent.SendChat("Invalid Z coordinate")
		return nil
	}
	go func() {
		if err := agent.FlyTo(ctx, tx, ty, tz); err != nil {
			_ = agent.SendChat(fmt.Sprintf("FlyTo failed: %v", err))
		}
	}()
	return nil
}
