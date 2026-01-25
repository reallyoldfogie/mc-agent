package actions

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const helpText = "Commands: help, pos, say <text>, testMove, moveTo <x> <y> <z> (pathfinding), lineTo <x> <y> <z> (straight-line), moveForward <distance>, moveUp <distance>, moveUpAndSneak <distance>, moveToAndSneak <x> <y> <z>, lineToAndSneak <x> <y> <z>, stopSneak, findPath <x> <y> <z>, testPath, follow [<player>], stopFollow, followStatus, startTracking, stopTracking, fireBow, planStatus, planStop"

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

type Help struct{}

func (Help) Name() string  { return "help" }
func (Help) Usage() string { return "help" }
func (Help) Execute(agent CommandAgent, _ []string) error {
	_ = agent.SendChat(helpText)
	return nil
}

type Pos struct{}

func (Pos) Name() string  { return "pos" }
func (Pos) Usage() string { return "pos" }
func (Pos) Execute(agent CommandAgent, _ []string) error {
	x, y, z, _, _, ok := agent.GetPosition()
	if !ok {
		_ = agent.SendChat("Bot position not initialized")
		return nil
	}
	_ = agent.SendChat(fmt.Sprintf("Current position: %.2f, %.2f, %.2f", x, y, z))
	return nil
}

type Say struct{}

func (Say) Name() string  { return "say" }
func (Say) Usage() string { return "say <text>" }
func (Say) Execute(agent CommandAgent, args []string) error {
	_ = agent.SendChat(strings.Join(args, " "))
	return nil
}

type TestMove struct{}

func (TestMove) Name() string  { return "testmove" }
func (TestMove) Usage() string { return "testMove" }
func (TestMove) Execute(agent CommandAgent, _ []string) error {
	go agent.TestMove()
	return nil
}

type MoveTo struct{}

func (MoveTo) Name() string  { return "moveto" }
func (MoveTo) Usage() string { return "moveTo <x> <y> <z>" }
func (MoveTo) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.MoveToWithChat(context.Background(), tx, ty, tz); err != nil {
			_ = agent.SendChat(fmt.Sprintf("Pathfinding failed: %v", err))
		}
	}()
	return nil
}

type LineTo struct{}

func (LineTo) Name() string  { return "lineto" }
func (LineTo) Usage() string { return "lineTo <x> <y> <z> (straight-line, flat world only)" }
func (LineTo) Execute(agent CommandAgent, args []string) error {
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
	x, y, z, _, _, ok := agent.GetPosition()
	if !ok {
		_ = agent.SendChat("Bot position not initialized")
		return nil
	}
	dx, dy, dz := tx-x, ty-y, tz-z
	total := math.Sqrt(dx*dx + dy*dy + dz*dz)
	_ = agent.SendChat(fmt.Sprintf("Moving direct from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [%.2f blocks]", x, y, z, tx, ty, tz, total))
	go func() {
		if err := agent.LineTo(context.Background(), tx, ty, tz, true); err != nil {
			_ = agent.SendChat("Movement failed: " + err.Error())
		}
	}()
	return nil
}

type MoveForward struct{}

func (MoveForward) Name() string  { return "moveforward" }
func (MoveForward) Usage() string { return "moveForward <distance>" }
func (MoveForward) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.MoveForward(context.Background(), dist); err != nil {
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
func (MoveUp) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.MoveUp(context.Background(), dist); err != nil {
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
func (MoveUpAndSneak) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.MoveUpAndSneak(context.Background(), dist); err != nil {
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
func (MoveToAndSneak) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.MoveToAndSneak(context.Background(), tx, ty, tz); err != nil {
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
func (LineToAndSneak) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.LineToAndSneak(context.Background(), tx, ty, tz); err != nil {
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
func (StopSneak) Execute(agent CommandAgent, _ []string) error {
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
func (FindPath) Execute(agent CommandAgent, args []string) error {
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
		if err := agent.FindPath(context.Background(), tx, ty, tz); err != nil {
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
func (TestPath) Execute(agent CommandAgent, _ []string) error {
	go agent.TestPath()
	return nil
}

type Follow struct{}

func (Follow) Name() string  { return "follow" }
func (Follow) Usage() string { return "follow [<player>]" }
func (Follow) Execute(agent CommandAgent, args []string) error {
	if len(args) < 1 {
		nearest, ok := agent.NearestPlayerInfo()
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
	if err := agent.Follow(context.Background(), args[0]); err != nil {
		_ = agent.SendChat("Follow error: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Following " + args[0])
	return nil
}

type StopFollow struct{}

func (StopFollow) Name() string  { return "stopfollow" }
func (StopFollow) Usage() string { return "stopFollow" }
func (StopFollow) Execute(agent CommandAgent, _ []string) error {
	if !agent.HasFollowManager() {
		_ = agent.SendChat("Follow system not available")
		return nil
	}
	if !agent.IsFollowing() {
		_ = agent.SendChat("Not currently following anyone")
		return nil
	}
	if err := agent.StopFollow(context.Background()); err != nil {
		_ = agent.SendChat("Stop error: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Stopped following")
	return nil
}

type FollowStatus struct{}

func (FollowStatus) Name() string  { return "followstatus" }
func (FollowStatus) Usage() string { return "followStatus" }
func (FollowStatus) Execute(agent CommandAgent, _ []string) error {
	_ = agent.SendChat(agent.FollowStatus())
	return nil
}

type StartTracking struct{}

func (StartTracking) Name() string  { return "starttracking" }
func (StartTracking) Usage() string { return "startTracking" }
func (StartTracking) Execute(agent CommandAgent, _ []string) error {
	agent.StartTracking()
	return nil
}

type StopTracking struct{}

func (StopTracking) Name() string  { return "stoptracking" }
func (StopTracking) Usage() string { return "stopTracking" }
func (StopTracking) Execute(agent CommandAgent, _ []string) error {
	agent.StopTracking()
	return nil
}

type PlanStatus struct{}

func (PlanStatus) Name() string  { return "planstatus" }
func (PlanStatus) Usage() string { return "planStatus" }
func (PlanStatus) Execute(agent CommandAgent, _ []string) error {
	status := agent.PlanStatus()
	_ = agent.SendChat(status.String())
	return nil
}

type PlanStop struct{}

func (PlanStop) Name() string  { return "planstop" }
func (PlanStop) Usage() string { return "planStop" }
func (PlanStop) Execute(agent CommandAgent, _ []string) error {
	if err := agent.StopPlan(); err != nil {
		_ = agent.SendChat("Plan stop error: " + err.Error())
		return nil
	}
	_ = agent.SendChat("Plan stopped")
	return nil
}

type FireBow struct{}

func (FireBow) Name() string  { return "firebow" }
func (FireBow) Usage() string { return "fireBow" }
func (FireBow) Execute(agent CommandAgent, _ []string) error {
	go agent.FireBow()
	return nil
}

type FireBowAt struct{}

func (FireBowAt) Name() string { return "firebowat" }
func (FireBowAt) Usage() string {
	return "fireBowAt <x> <y> <z> | fireBowAt nearest | fireBowAt <player>"
}
func (FireBowAt) Execute(agent CommandAgent, args []string) error {
	if len(args) == 0 {
		agent.FireBow()
		return nil
	}
	if len(args) == 1 {
		if args[0] == "nearest" {
			playerInfo, found := agent.NearestPlayerInfo()
			if found {
				go agent.FireBowAt(playerInfo.X, playerInfo.Y, playerInfo.Z)
				return nil
			}
		} else {
			x, y, z, found, err := agent.FindPlayerByName(args[0])
			if err == nil && found {
				go agent.FireBowAt(x, y, z)
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
	go agent.FireBowAt(tx, ty, tz)
	return nil
}
