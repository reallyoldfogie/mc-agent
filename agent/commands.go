package agent

import (
	"fmt"
	"math"
	"strings"
	"time"

	"log"
	"strconv"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// handleChatCommand parses and executes simple chat commands.
// This initial set is minimal and safe; expand as more subsystems migrate.
func (a *Agent) handleChatCommand(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	parts := strings.Fields(cmd)
	name := strings.ToLower(parts[0])
	args := parts[1:]

	switch name {
	case "help":
		_ = a.SendChat("Commands: help, pos, say <text>, testMove, moveTo <x> <y> <z>, moveForward <distance>, moveUp <distance>, findPath <x> <y> <z>, testPath, follow [<player>], stopFollow, followStatus, startTracking, stopTracking, fireBow")
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
	default:
		_ = a.SendChat("Unknown command. Try: help, pos, say")
	}
}

// parse helpers
func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// testMove: move +1 on X in small steps
func (a *Agent) cmdTestMove() {
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

func (a *Agent) cmdMoveTo(xs, ys, zs string) {
	if a.moveExec == nil {
		_ = a.SendChat("Movement not available")
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
	_ = a.SendChat(fmt.Sprintf("Moving from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [%.2f blocks]", x, y, z, tx, ty, tz, total))
	_ = a.moveExec.LookAt(tx, ty, tz, true)
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(total / stepSize))
	if steps == 0 {
		_ = a.SendChat("Already at target position")
		return
	}
	for i := 0; i < steps; i++ {
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

func (a *Agent) cmdMoveForward(ds string) {
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

func (a *Agent) cmdMoveUp(ds string) {
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

func (a *Agent) cmdTestPath() {
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

func (a *Agent) cmdFindPath(xs, ys, zs string) {
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

func (a *Agent) cmdStartTracking() {
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

func (a *Agent) cmdStopTracking() {
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

// fireBow via UseItem + PlayerAction
func (a *Agent) cmdFireBow() {
	if a.client == nil || a.packetMgr == nil {
		_ = a.SendChat("Client not ready")
		return
	}
	// Use main hand then simulate action
	useID := a.packetMgr.GetServerboundPacketID("ServerboundUseItem")
	_ = a.client.WritePacket(pk.Marshal(useID, pk.VarInt(0), pk.VarInt(1), pk.Float(0), pk.Float(0)))
	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			actID := a.packetMgr.GetServerboundPacketID("ServerboundPlayerAction")
			_ = a.client.WritePacket(pk.Marshal(actID, pk.VarInt(0), pk.Position{X: 0, Y: 0, Z: 0}, pk.Byte(0), pk.VarInt(0)))
			time.Sleep(bowHoldSleep)
		}
		actID := a.packetMgr.GetServerboundPacketID("ServerboundPlayerAction")
		_ = a.client.WritePacket(pk.Marshal(actID, pk.VarInt(5), pk.Position{X: 0, Y: 0, Z: 0}, pk.Byte(0), pk.VarInt(0)))
	}()
}

// nearest player using tracked entities filtered by player list membership
type nearestInfo struct {
	EntityID int32
	UUID     [16]byte
	Distance float64
	X, Y, Z  float64
}

func (a *Agent) findNearestPlayer() (nearestInfo, bool) {
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
func (a *Agent) getEntityStats() map[string]int {
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

func (a *Agent) formatEntityStats(stats map[string]int) string {
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
func (a *Agent) cmdStartFollowingNearest() {
	n, ok := a.findNearestPlayer()
	if !ok {
		_ = a.SendChat("Failed to find nearest player")
		return
	}
	_ = a.SendChat(fmt.Sprintf("Found nearest player at %.1f blocks, starting to follow...", n.Distance))
	_ = a.SendChat("Note: Following nearest player requires name. Use 'follow <playername>' instead.")
}
