package agent

import (
	"fmt"
	"math"
	"strings"
	"time"

	"log"
	"strconv"

	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// handleChatCommand parses and executes simple chat commands.
// This initial set is minimal and safe; expand as more subsystems migrate.
func (a *agent) handleChatCommand(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	parts := strings.Fields(cmd)
	name := strings.ToLower(parts[0])
	args := parts[1:]

	switch name {
	case "help":
		_ = a.SendChat("Commands: help, pos, say <text>, testMove, moveTo <x> <y> <z> (pathfinding), lineTo <x> <y> <z> (straight-line), moveForward <distance>, moveUp <distance>, findPath <x> <y> <z>, testPath, follow [<player>], stopFollow, followStatus, startTracking, stopTracking, fireBow")
	case "pos":
		x, y, z, _, _, ok := a.GetPosition()
		if !ok {
			_ = a.SendChat("Bot position not initialized")
			return
		}
		_ = a.SendChat(fmt.Sprintf("Current position: %.2f, %.2f, %.2f", x, y, z))
	case "say":
		_ = a.SendChat(strings.Join(args, " "))
	case "testmove":
		go a.cmdTestMove()
	case "moveto":
		if len(args) < 3 {
			_ = a.SendChat("Usage: moveTo <x> <y> <z>")
			return
		}
		go a.cmdMoveTo(args[0], args[1], args[2])
	case "lineto":
		if len(args) < 3 {
			_ = a.SendChat("Usage: lineTo <x> <y> <z> (straight-line, flat world only)")
			return
		}
		go a.cmdLineTo(args[0], args[1], args[2])
	case "moveforward":
		if len(args) < 1 {
			_ = a.SendChat("Usage: moveForward <distance>")
			return
		}
		go a.cmdMoveForward(args[0])
	case "moveup":
		if len(args) < 1 {
			_ = a.SendChat("Usage: moveUp <distance>")
			return
		}
		go a.cmdMoveUp(args[0])
	case "findpath":
		if len(args) < 3 {
			_ = a.SendChat("Usage: findPath <x> <y> <z>")
			return
		}
		go a.cmdFindPath(args[0], args[1], args[2])
	case "testpath":
		go a.cmdTestPath()
	case "follow":
		if len(args) < 1 {
			go a.cmdStartFollowingNearest()
			return
		}
		a.mu.Lock()
		fm := a.followMgr
		a.mu.Unlock()
		if fm == nil {
			_ = a.SendChat("Follow system not available")
			return
		}
		if err := fm.Start(args[0]); err != nil {
			_ = a.SendChat("Follow error: " + err.Error())
			return
		}
		_ = a.SendChat("Following " + args[0])
	case "stopfollow":
		a.mu.Lock()
		fm := a.followMgr
		a.mu.Unlock()
		if fm == nil {
			_ = a.SendChat("Follow system not available")
			return
		}
		if !fm.IsActive() {
			_ = a.SendChat("Not currently following anyone")
			return
		}
		if err := fm.Stop(); err != nil {
			_ = a.SendChat("Stop error: " + err.Error())
			return
		}
		_ = a.SendChat("Stopped following")
	case "followstatus":
		a.mu.Lock()
		fm := a.followMgr
		a.mu.Unlock()
		if fm == nil {
			_ = a.SendChat("Follow system not available")
			return
		}
		_ = a.SendChat(fm.GetStatus())
	case "starttracking":
		a.cmdStartTracking()
	case "stoptracking":
		a.cmdStopTracking()
	case "firebow":
		go a.cmdFireBow()
	case "firebowat":
		if len(args) == 0 {
			a.cmdFireBow()
		} else if len(args) == 1 {
			if args[0] == "nearest" {
				playerInfo, found := a.findNearestPlayer()
				if found {
					go a.cmdFireBowAt(playerInfo.X, playerInfo.Y, playerInfo.Z)
					return
				}
			} else {
				if playerInfo, err := a.targetSelector.FindPlayerByName(args[0]); err == nil {
					go a.cmdFireBowAt(playerInfo.X, playerInfo.Y, playerInfo.Z)
					return
				} else {
					_ = a.SendChat("Player not found: " + args[0])
					return
				}
			}
		}
		if len(args) < 3 {
			_ = a.SendChat("Usage: fireBowAt <x> <y> <z> | fireBowAt nearest | fireBowAt <player>")
			return
		}
	default:
		_ = a.SendChat("Unknown command. Try: help, pos, say")
	}
}

// parse helpers
func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// testMove: move +1 on X in small steps
func (a *agent) cmdTestMove() {
	if a.moveExec == nil {
		_ = a.SendChat("Movement not available")
		return
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	_ = a.SendChat(fmt.Sprintf("Current position: %.2f, %.2f, %.2f", x, y, z))
	targetX := x + 1.0
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(1.0 / stepSize))
	for i := 0; i < steps; i++ {
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		nextX := x + 1.0*prog
		if err := a.moveExec.SendPosition(nextX, y, z, true); err != nil {
			_ = a.SendChat("Movement failed: " + err.Error())
			return
		}
		time.Sleep(stepDelay)
	}
	_ = a.SendChat(fmt.Sprintf("Moved to: %.2f, %.2f, %.2f", targetX, y, z))
}

// cmdLineTo performs straight-line movement (for flat worlds only)
func (a *agent) cmdLineTo(xs, ys, zs string) {
	if a.moveExec == nil {
		_ = a.SendChat("Movement executor not available")
		return
	}
	tx, err := parseFloat(xs)
	if err != nil {
		_ = a.SendChat("Invalid X coordinate")
		return
	}
	ty, err := parseFloat(ys)
	if err != nil {
		_ = a.SendChat("Invalid Y coordinate")
		return
	}
	tz, err := parseFloat(zs)
	if err != nil {
		_ = a.SendChat("Invalid Z coordinate")
		return
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	dx, dy, dz := tx-x, ty-y, tz-z
	total := math.Sqrt(dx*dx + dy*dy + dz*dz)
	_ = a.SendChat(fmt.Sprintf("Moving direct from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [%.2f blocks]", x, y, z, tx, ty, tz, total))
	_ = a.moveExec.LookAt(tx, ty, tz, true)
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(total / stepSize))
	if steps == 0 {
		_ = a.SendChat("Already at target position")
		return
	}
	for i := range steps {
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		nx, ny, nz := x+dx*prog, y+dy*prog, z+dz*prog
		if err := a.moveExec.SendPosition(nx, ny, nz, true); err != nil {
			_ = a.SendChat(fmt.Sprintf("Movement failed at step %d: %v", i+1, err))
			return
		}
		time.Sleep(stepDelay)
	}
	_ = a.SendChat(fmt.Sprintf("Arrived at (%.2f, %.2f, %.2f)", tx, ty, tz))
}

// cmdMoveTo performs pathfinding-based movement with segmentation for long distances
func (a *agent) cmdMoveTo(xs, ys, zs string) {
	// Validate coordinates first before checking for subsystems
	tx, err := parseFloat(xs)
	if err != nil {
		_ = a.SendChat("Invalid X coordinate")
		return
	}
	ty, err := parseFloat(ys)
	if err != nil {
		_ = a.SendChat("Invalid Y coordinate")
		return
	}
	tz, err := parseFloat(zs)
	if err != nil {
		_ = a.SendChat("Invalid Z coordinate")
		return
	}

	if a.pathfind == nil {
		_ = a.SendChat("Pathfinding not available")
		return
	}
	if a.moveExec == nil {
		_ = a.SendChat("Movement executor not available")
		return
	}

	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}

	// Convert to block coordinates for pathfinding
	finalGoal := pathfinding.V3{
		X: math.Floor(tx),
		Y: math.Floor(ty),
		Z: math.Floor(tz),
	}

	currentPos := pathfinding.V3{
		X: math.Floor(x),
		Y: math.Floor(y),
		Z: math.Floor(z),
	}

	// Check if already at target block
	if currentPos == finalGoal {
		_ = a.SendChat("Already at target position")
		return
	}

	log.Printf("[moveTo] Moving from (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)",
		currentPos.X, currentPos.Y, currentPos.Z, finalGoal.X, finalGoal.Y, finalGoal.Z)

	// Use segmented pathfinding for long distances
	const segmentDistance = 20.0  // Pathfind in 20-block segments
	const waypointDistance = 10.0 // Waypoints every 10 blocks

	for {
		distToGoal := currentPos.DistanceTo(finalGoal)
		log.Printf("[moveTo] Distance to goal: %.1f blocks", distToGoal)

		// If close enough, pathfind directly to goal
		if distToGoal <= segmentDistance {
			log.Printf("[moveTo] Close to goal, pathfinding directly")
			if err := a.pathfindAndFollow(currentPos, finalGoal, distToGoal); err != nil {
				_ = a.SendChat(fmt.Sprintf("Pathfinding failed: %v", err))
				return
			}
			break
		}

		// For long distances, create intermediate waypoint
		dx := finalGoal.X - currentPos.X
		dy := finalGoal.Y - currentPos.Y
		dz := finalGoal.Z - currentPos.Z

		// Normalize direction and scale to waypoint distance
		factor := waypointDistance / distToGoal
		waypoint := pathfinding.V3{
			X: math.Floor(currentPos.X + dx*factor),
			Y: math.Floor(currentPos.Y + dy*factor),
			Z: math.Floor(currentPos.Z + dz*factor),
		}

		log.Printf("[moveTo] Segmented pathfinding to waypoint (%.0f, %.0f, %.0f)",
			waypoint.X, waypoint.Y, waypoint.Z)
		_ = a.SendChat(fmt.Sprintf("Waypoint: (%.0f, %.0f, %.0f)", waypoint.X, waypoint.Y, waypoint.Z))

		// Pathfind to waypoint
		if err := a.pathfindAndFollow(currentPos, waypoint, waypointDistance); err != nil {
			_ = a.SendChat(fmt.Sprintf("Waypoint pathfinding failed: %v", err))
			return
		}

		// Update current position for next segment
		x, y, z, _, _, ok = a.GetPosition()
		if !ok {
			_ = a.SendChat("Lost position")
			return
		}
		currentPos = pathfinding.V3{
			X: math.Floor(x),
			Y: math.Floor(y),
			Z: math.Floor(z),
		}
	}

	_ = a.SendChat(fmt.Sprintf("Arrived at (%.0f, %.0f, %.0f)", finalGoal.X, finalGoal.Y, finalGoal.Z))
	log.Printf("[moveTo] Successfully reached final goal")
}

// pathfindAndFollow computes and follows a path from start to goal
func (a *agent) pathfindAndFollow(start, goal pathfinding.V3, distance float64) error {
	log.Printf("[pathfindAndFollow] From (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z)

	// Calculate step limit based on distance
	maxSteps := int(distance * 100) // Allow 100x distance for complex terrain
	if maxSteps < 2000 {
		maxSteps = 2000 // Minimum 2000 steps for short segments
	}

	path, err := a.pathfind.FindPath(start, goal, maxSteps)
	if err != nil {
		log.Printf("[pathfindAndFollow] FindPath error: %v", err)
		return err
	}

	if !path.Found {
		log.Printf("[pathfindAndFollow] No path found")
		return fmt.Errorf("no path found")
	}

	log.Printf("[pathfindAndFollow] %s", path.LogSummary())
	log.Printf("[pathfindAndFollow] Path details:\n%s", path.LogDetails())

	// Follow the path
	const stepDelay = 100 * time.Millisecond
	for i, step := range path.Steps {
		stepX := float64(step.Position.X) + 0.5
		stepY := float64(step.Position.Y)
		stepZ := float64(step.Position.Z) + 0.5

		log.Printf("[pathfindAndFollow] Step %d/%d: %s to (%.1f, %.1f, %.1f)",
			i+1, len(path.Steps), step.Movement, stepX, stepY, stepZ)

		if err := a.moveExec.LookAt(stepX, stepY, stepZ, true); err != nil {
			log.Printf("[pathfindAndFollow] LookAt failed: %v", err)
		}

		if err := a.moveExec.SendPosition(stepX, stepY, stepZ, true); err != nil {
			log.Printf("[pathfindAndFollow] Movement failed: %v", err)
			return fmt.Errorf("movement failed at step %d: %w", i+1, err)
		}

		time.Sleep(stepDelay)
	}

	return nil
}

func (a *agent) cmdMoveForward(ds string) {
	if a.moveExec == nil {
		_ = a.SendChat("Movement not available")
		return
	}
	dist, err := parseFloat(ds)
	if err != nil {
		_ = a.SendChat("Invalid distance")
		return
	}
	x, y, z, yaw, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	yawRad := float64(yaw) * math.Pi / 180
	dx := -math.Sin(yawRad) * dist
	dz := math.Cos(yawRad) * dist
	tx, tz := x+dx, z+dz
	_ = a.SendChat(fmt.Sprintf("Moving forward %.2f blocks", dist))
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	total := math.Abs(dist)
	steps := int(math.Ceil(total / stepSize))
	for i := 0; i < steps; i++ {
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		nx := x + dx*prog
		nz := z + dz*prog
		if err := a.moveExec.SendPosition(nx, y, nz, true); err != nil {
			_ = a.SendChat("Movement failed: " + err.Error())
			return
		}
		time.Sleep(stepDelay)
	}
	_ = a.SendChat(fmt.Sprintf("Moved to (%.2f, %.2f, %.2f)", tx, y, tz))
}

func (a *agent) cmdMoveUp(ds string) {
	if a.moveExec == nil {
		_ = a.SendChat("Movement not available")
		return
	}
	dist, err := parseFloat(ds)
	if err != nil {
		_ = a.SendChat("Invalid distance")
		return
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(math.Abs(dist) / stepSize))
	for i := 0; i < steps; i++ {
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		ny := y + dist*prog
		if err := a.moveExec.SendPosition(x, ny, z, true); err != nil {
			_ = a.SendChat("Movement failed: " + err.Error())
			return
		}
		time.Sleep(stepDelay)
	}
	_ = a.SendChat(fmt.Sprintf("Moved to (%.2f, %.2f, %.2f)", x, y+dist, z))
}

func (a *agent) cmdTestPath() {
	if a.pathfind == nil {
		_ = a.SendChat("Pathfinder not available")
		return
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	start := pathfinding.V3{X: x, Y: y, Z: z}
	goal := pathfinding.V3{X: x, Y: y, Z: z + 5}
	if _, err := a.pathfind.FindPath(start, goal, 200); err != nil {
		_ = a.SendChat("Path find failed: " + err.Error())
		return
	}
	_ = a.SendChat("Test path computed 5 blocks north")
}

func (a *agent) cmdFindPath(xs, ys, zs string) {
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	tx, err := parseFloat(xs)
	if err != nil {
		_ = a.SendChat("Invalid X coordinate")
		return
	}
	ty, err := parseFloat(ys)
	if err != nil {
		_ = a.SendChat("Invalid Y coordinate")
		return
	}
	tz, err := parseFloat(zs)
	if err != nil {
		_ = a.SendChat("Invalid Z coordinate")
		return
	}
	if a.pathfind == nil {
		_ = a.SendChat("Pathfinder not available")
		return
	}
	start := pathfinding.V3{X: x, Y: y, Z: z}
	goal := pathfinding.V3{X: tx, Y: ty, Z: tz}
	if _, err := a.pathfind.FindPath(start, goal, 200); err != nil {
		_ = a.SendChat("Path find failed: " + err.Error())
		return
	}
	_ = a.SendChat("Path computed")
}

// Tracking loop: look at nearest tracked entity periodically

// Tunables for testing and legacy parity
var (
	trackingTickDur           = time.Second / 20
	trackingStatsDur          = 15 * time.Second
	trackingNoPlayersInterval = 30 * time.Second
	bowHoldIterations         = 10
	bowHoldSleep              = time.Second
)

func (a *agent) cmdStartTracking() {
	a.trackMu.Lock()
	if a.trackActive {
		a.trackMu.Unlock()
		_ = a.SendChat("Tracking is already active!")
		return
	}
	if a.moveExec == nil {
		a.trackMu.Unlock()
		_ = a.SendChat("Movement not available")
		return
	}
	a.trackStop = make(chan struct{})
	a.trackActive = true
	a.lastNoPlayersMsg = time.Time{}
	a.trackMu.Unlock()

	_ = a.SendChat("Started tracking nearest player...")
	log.Println("Started tracking nearest player")

	tick := time.NewTicker(trackingTickDur)
	statsTick := time.NewTicker(trackingStatsDur)
	go func() {
		defer tick.Stop()
		defer statsTick.Stop()
		for {
			select {
			case <-a.trackStop:
				log.Println("Tracking stopped")
				return
			case <-statsTick.C:
				stats := a.getEntityStats()
				msg := a.formatEntityStats(stats)
				_ = a.SendChat(msg)
				log.Println(msg)
			case <-tick.C:
				nearest, ok := a.findNearestPlayer()
				if !ok {
					now := time.Now()
					if now.Sub(a.lastNoPlayersMsg) >= trackingNoPlayersInterval {
						stats := a.getEntityStats()
						_ = a.SendChat("No players nearby. " + a.formatEntityStats(stats))
						a.lastNoPlayersMsg = now
					}
					stats := a.getEntityStats()
					log.Printf("No players nearby. %s", a.formatEntityStats(stats))
					continue
				}
				// Look at nearest
				_ = a.moveExec.LookAt(nearest.X, nearest.Y, nearest.Z, true)
				bx, by, bz, okp := a.GetPositionSimple()
				if !okp {
					continue
				}
				pname := "Unknown"
				if a.nameByUUID != nil {
					if n, okn := a.nameByUUID(nearest.UUID); okn {
						pname = n
					}
				}
				msg := fmt.Sprintf("My Pos: (%.1f, %.1f, %.1f) | Nearest: %s | Distance: %.2f blocks | Pos: (%.1f, %.1f, %.1f)", bx, by, bz, pname, nearest.Distance, nearest.X, nearest.Y, nearest.Z)
				_ = a.SendChat(msg)
				log.Println(msg)
			}
		}
	}()
}

func (a *agent) cmdStopTracking() {
	a.trackMu.Lock()
	if !a.trackActive {
		a.trackMu.Unlock()
		_ = a.SendChat("Not tracking")
		return
	}
	close(a.trackStop)
	a.trackActive = false
	a.trackMu.Unlock()
	_ = a.SendChat("Tracking stopped")
}

// nearest player using tracked entities filtered by player list membership
type nearestInfo struct {
	EntityID int32
	UUID     [16]byte
	Distance float64
	X, Y, Z  float64
}

func (a *agent) findNearestPlayer() (nearestInfo, bool) {
	bx, by, bz, ok := a.GetPositionSimple()
	if !ok {
		return nearestInfo{}, false
	}
	ents := a.snapshotEntities()
	var res nearestInfo
	min := 0.0
	first := true
	for _, e := range ents {
		// Only consider if in player list when resolver present; else consider all
		if a.nameByUUID != nil {
			if _, okn := a.nameByUUID(e.UUID); !okn {
				continue
			}
		}
		dx, dy, dz := e.X-bx, e.Y-by, e.Z-bz
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if first || d < min {
			first = false
			min = d
			res = nearestInfo{EntityID: e.EntityID, UUID: e.UUID, Distance: d, X: e.X, Y: e.Y, Z: e.Z}
		}
	}
	if first {
		return nearestInfo{}, false
	}
	return res, true
}

// Entity stats for chat/log
func (a *agent) getEntityStats() map[string]int {
	ents := a.snapshotEntities()
	out := map[string]int{}
	for _, e := range ents {
		key := fmt.Sprintf("type:%d", e.EntityType)
		if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(e.EntityType); ok {
				key = name
			}
		}
		out[key]++
	}
	return out
}

func (a *agent) formatEntityStats(stats map[string]int) string {
	if len(stats) == 0 {
		return "No entities tracked"
	}
	total := 0
	parts := make([]string, 0, len(stats))
	for k, v := range stats {
		total += v
		parts = append(parts, fmt.Sprintf("%s: %d", k, v))
	}
	return fmt.Sprintf("Entities (Total: %d) - %s", total, strings.Join(parts, ", "))
}

// follow nearest: legacy behavior prints messages without starting follow
func (a *agent) cmdStartFollowingNearest() {
	n, ok := a.findNearestPlayer()
	if !ok {
		_ = a.SendChat("Failed to find nearest player")
		return
	}
	_ = a.SendChat(fmt.Sprintf("Found nearest player at %.1f blocks, starting to follow...", n.Distance))
	_ = a.SendChat("Note: Following nearest player requires name. Use 'follow <playername>' instead.")
}
