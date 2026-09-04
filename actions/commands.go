package actions

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
)

const helpText = "Commands: help, pos, say <text>, testMove, moveTo <x> <y> <z> (pathfinding), lineTo <x> <y> <z> (straight-line), moveForward <distance>, moveUp <distance>, moveUpAndSneak <distance>, moveToAndSneak <x> <y> <z>, lineToAndSneak <x> <y> <z>, stopSneak, findPath <x> <y> <z>, testPath, follow [<player>], stopFollow, followStatus, startTracking, stopTracking, fireBow, mount <entityID | entityType>, dismount, vehiclejump [power], mine <x> <y> <z> | <blockName>, lookAround [radius], pickUpNearbyItem [maxDistance], craft <itemName>, equip <item>, useItem [offhand], flyTo <x> <y> <z>, fly, land, followCam <playerName> [maxDistance], stopFollowCam, planStatus, planStop"

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
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
// "mine <blockName>"'s FindVisibleBlock lookup.
const mineSearchRadius = 32

type Mine struct{}

func (Mine) Name() string  { return "mine" }
func (Mine) Usage() string { return "mine <x> <y> <z> | mine <blockName>" }
func (Mine) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) == 1 {
		blockName := args[0]
		x, y, z, found, err := agent.FindVisibleBlock(ctx, blockName, mineSearchRadius)
		if err != nil {
			_ = agent.SendChat("Find block error: " + err.Error())
			return models.Done(nil), nil
		}
		if !found {
			_ = agent.SendChat(fmt.Sprintf("No visible %s found within %d blocks", blockName, mineSearchRadius))
			return models.Done(nil), nil
		}
		return mineAt(ctx, agent, x, y, z), nil
	}

	if len(args) != 3 {
		_ = agent.SendChat("Usage: mine <x> <y> <z> | mine <blockName>")
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
