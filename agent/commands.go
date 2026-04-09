package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// handleChatCommand parses and executes simple chat commands.
// This initial set is minimal and safe; expand as more subsystems migrate.
func (a *agent) handleChatCommand(cmd string) {
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	a.handleChatCommandWithContext(ctx, cmd)
}

// handleChatCommandWithContext parses and executes simple chat commands with context support.
func (a *agent) handleChatCommandWithContext(ctx context.Context, cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	parts := strings.Fields(cmd)
	name := strings.ToLower(parts[0])
	args := parts[1:]
	if a.commandRegistry == nil {
		_ = a.SendChat("Command registry not initialized")
		return
	}
	if err := a.commandRegistry.Execute(ctx, name, a, args); err != nil {
		if errors.Is(err, models.ErrActionNotFound) {
			_ = a.SendChat("Unknown command. Try: help, pos, say")
			return
		}
		_ = a.SendChat(err.Error())
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
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.LineTo(ctx, tx, ty, tz, true); err != nil {
		_ = a.SendChat("Movement failed: " + err.Error())
	}
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
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.MoveTo(ctx, tx, ty, tz, true); err != nil {
		_ = a.SendChat(fmt.Sprintf("MoveTo - Pathfinding failed: %v", err))
	}
}

// followPath executes an already-computed path
func (a *agent) followPath(ctx context.Context, path *models.Path) error {
	log.Printf("[followPath] Executing cached path with %d steps", len(path.Steps))
	log.Printf("[followPath] %s", path.LogSummary())

	// Log first 5 steps to diagnose direction issues
	log.Printf("[followPath] Steps of path being executed:")
	for i := 0; i < len(path.Steps); i++ {
		step := path.Steps[i]
		log.Printf("[followPath]   Step %d: %s to (%.0f, %.0f, %.0f)",
			i+1, step.Movement.String(), step.Position.X, step.Position.Y, step.Position.Z)
	}

	if exec, ok := a.moveExec.(interface {
		ExecutePathWithContext(context.Context, *pathfinding.Path) error
	}); ok {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("[followPath] Exec implements ExecutePathWithContext, using it.")
		return exec.ExecutePathWithContext(ctx, path)
	}

	log.Printf("[followPath] Exec doesn't implement ExecutePath, using fallback")

	// Follow the path
	const stepDelay = 100 * time.Millisecond
	for i, step := range path.Steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stepX := float64(step.Position.X) + 0.5
		stepY := float64(step.Position.Y)
		stepZ := float64(step.Position.Z) + 0.5

		log.Printf("[followPath] Step %d/%d: %s to (%.1f, %.1f, %.1f)",
			i+1, len(path.Steps), step.Movement, stepX, stepY, stepZ)

		if err := a.moveExec.LookAt(stepX, stepY, stepZ, true); err != nil {
			log.Printf("[followPath] LookAt failed: %v", err)
		}

		if err := a.moveExec.SendPosition(stepX, stepY, stepZ, true); err != nil {
			log.Printf("[followPath] Movement failed: %v", err)
			return fmt.Errorf("movement failed at step %d: %w", i+1, err)
		}

		if err := sleepWithContext(ctx, stepDelay); err != nil {
			return err
		}
	}

	return nil
}

// pathfindAndFollow computes and follows a path from start to goal
func (a *agent) pathfindAndFollow(ctx context.Context, start, goal models.V3) error {
	log.Printf("[pathfindAndFollow] From (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z)

	distance := start.DistanceTo(goal)

	// Calculate step limit based on distance
	maxSteps := max(
		// Allow 200x distance for complex terrain (increased from 150 for water pathfinding)
		int(distance*200),
		// Minimum 20000 steps for short segments (increased from 10000 for water/swimming)
		20000)

	path, err := a.pathfind.FindPath(ctx, start, goal, maxSteps)
	if err != nil {
		log.Printf("[pathfindAndFollow] FindPath error: %v", err)
		return err
	}

	if !path.Found {
		log.Printf("[pathfindAndFollow] No path found")
		return fmt.Errorf("no path found")
	}

	return a.followPath(ctx, path)
}

func (a *agent) cmdMoveForward(ds string) {
	dist, err := parseFloat(ds)
	if err != nil {
		_ = a.SendChat("Invalid distance")
		return
	}
	_ = a.SendChat(fmt.Sprintf("Moving forward %.2f blocks", dist))
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.MoveForward(ctx, dist); err != nil {
		_ = a.SendChat("Movement failed: " + err.Error())
		return
	}
	_ = a.SendChat("Move forward complete")
}

func (a *agent) cmdMoveUp(ds string) {
	dist, err := parseFloat(ds)
	if err != nil {
		_ = a.SendChat("Invalid distance")
		return
	}
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.MoveUp(ctx, dist); err != nil {
		_ = a.SendChat("Movement failed: " + err.Error())
		return
	}
	_ = a.SendChat("Move up complete")
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
	start := models.V3{X: x, Y: y, Z: z}
	goal := models.V3{X: x, Y: y, Z: z + 5}
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := a.pathfind.FindPath(ctx, start, goal, 200); err != nil {
		_ = a.SendChat("Path find failed: " + err.Error())
		return
	}
	_ = a.SendChat("Test path computed 5 blocks north")
}

func (a *agent) cmdFindPath(xs, ys, zs string) {
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
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.FindPath(ctx, tx, ty, tz); err != nil {
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
				a.playerResolversMu.RLock()
				if a.playerNameByUUID != nil {
					if n, okn := a.playerNameByUUID(nearest.UUID); okn {
						pname = n
					}
				}
				a.playerResolversMu.RUnlock()
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

	// Snapshot the resolver to avoid repeated lock acquisitions
	a.playerResolversMu.RLock()
	resolver := a.playerNameByUUID
	a.playerResolversMu.RUnlock()

	var res nearestInfo
	min := 0.0
	first := true
	for _, e := range ents {
		// Only consider if in player list when resolver present; else consider all
		if resolver != nil {
			if _, okn := resolver(e.UUID); !okn {
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
