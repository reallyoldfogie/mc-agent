package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

const (
	// maxBowHoldDuration is the maximum time to hold a bow for full power
	maxBowHoldDuration = 1000 * time.Millisecond
	// projectileCallbackTimeout is the maximum time to wait for a projectile hit callback before firing with best-effort data
	projectileCallbackTimeout = 10 * time.Second
)

// projectileTypeFromEntityName resolves entity type name to ProjectileType.
func projectileTypeFromEntityName(name string) (models.ProjectileType, bool) {
	switch name {
	case "minecraft:arrow", "minecraft:spectral_arrow":
		return models.Arrow, true
	case "minecraft:trident":
		return models.Trident, true
	case "minecraft:snowball":
		return models.Snowball, true
	case "minecraft:egg":
		return models.Egg, true
	case "minecraft:ender_pearl":
		return models.EnderPearl, true
	case "minecraft:splash_potion", "minecraft:lingering_potion":
		return models.SplashPotion, true
	case "minecraft:experience_bottle":
		return models.ExperienceBottle, true
	case "minecraft:wind_charge":
		return models.WindCharge, true
	}
	return 0, false
}

// setPendingProjectileCallback queues callbacks to be fired when a matching projectile spawns.
// Multiple callbacks can be registered for the same projectile to allow multiple sub-systems to be notified.
// Multiple projectiles of different types can also be queued simultaneously.
// target may be nil if no specific target was specified for this shot.
func (a *agent) setPendingProjectileCallback(pt models.ProjectileType, target *models.V3, callbacks ...models.ProjectileHitCallback) {
	a.pendingProjectilesMu.Lock()
	info := pendingProjectileInfo{
		projectileType: pt,
		callbacks:      callbacks,
		hasTarget:      target != nil,
	}
	if target != nil {
		info.targetPos = *target
	}
	a.pendingProjectiles = append(a.pendingProjectiles, info)
	a.pendingProjectilesMu.Unlock()
}

// setCallbackRegistrationTime records when callbacks are registered on an active projectile.
// This is used to implement timeout-based callback firing if server packets don't arrive.
func (a *agent) setCallbackRegistrationTime(entityID int32) {
	a.activeProjectilesMu.Lock()
	if projInfo, exists := a.activeProjectiles[entityID]; exists {
		projInfo.callbackRegisteredAt = time.Now()
		log.Printf("[setCallbackRegistrationTime] Registered callbacks for projectile entityID=%d, will timeout in %.1fs if no server response",
			entityID, projectileCallbackTimeout.Seconds())
	}
	a.activeProjectilesMu.Unlock()
}

// getPacketWriter returns a PacketWriter for sending packets.
// In production, this uses the bot.Conn. In tests, it can be overridden.
// When a movement mirror is active, the writer is wrapped so that every
// serverbound packet it sends is also forwarded to the mirror. This lets the
// mirror observe non-movement actions (e.g. arm swings) for replay recording
// without requiring every action call site to notify the mirror explicitly.
func (a *agent) getPacketWriter() (models.PacketWriter, error) {
	var writer models.PacketWriter

	// Try to use client directly if it implements PacketWriter (common in tests)
	if pw, ok := a.client.(models.PacketWriter); ok {
		writer = pw
	} else {
		// Fall back to using Conn() for production
		conn := a.client.Conn()
		if conn == nil {
			return nil, fmt.Errorf("connection not established")
		}
		writer = conn
	}

	if a.moveMirror != nil {
		writer = mirroringPacketWriter{inner: writer, mirror: a.moveMirror}
	}
	return writer, nil
}

// mirroringPacketWriter forwards every packet written to an inner PacketWriter
// while also handing it to a MovementMirror so the replay mirror can react to
// outbound packets (e.g. arm swings) that don't flow through the movement
// executor's packet-callback pipeline.
type mirroringPacketWriter struct {
	inner  models.PacketWriter
	mirror models.MovementMirror
}

func (w mirroringPacketWriter) WritePacket(packet pk.Packet) error {
	err := w.inner.WritePacket(packet)
	if w.mirror != nil {
		w.mirror.HandleServerbound(packet)
	}
	return err
}

// FireBowWithPitch fires an arrow with a specific pitch and yaw, using predicted trajectory calculation
// without automatic pitch adjustment. This allows testing of specific trajectories for physics calibration.
// Returns the predicted trajectory as []models.TrajectoryPoint (cast from any) for comparison with actual observed trajectory.
func (a *agent) FireBowWithPitch(ctx context.Context, pitch, yaw float64, callbacks ...models.ProjectileHitCallback) ([]models.TrajectoryPoint, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if a.client == nil || a.versionHandler == nil {
		return nil, fmt.Errorf("client or version handler not ready")
	}

	if a.moveExec == nil {
		return nil, fmt.Errorf("movement executor not available")
	}

	botX, botY, botZ, _, _, initialized := a.GetPosition()
	if !initialized {
		return nil, fmt.Errorf("bot position not initialized")
	}

	// Set rotation to the specified pitch/yaw
	a.setPosition(botX, botY, botZ, yaw, pitch)

	// Arrow spawns at eye position minus 0.1 blocks (Minecraft PersistentProjectileEntity.java:113)
	// For standing player: eye height = 1.62, so spawn = Y + 1.62 - 0.1 = Y + 1.52
	botOrigin := models.V3{X: botX, Y: botY + a.getEyeHeight() - .1, Z: botZ}

	// Calculate velocity from pitch and yaw
	pitchRad := (pitch) * math.Pi / 180.0
	yawRad := (yaw) * math.Pi / 180.0

	// Get arrow physics constants
	arrowPhys := physics.GetProjectilePhysics(models.Arrow)
	initialSpeed := arrowPhys.InitialSpeed // Full power (power factor = 1.0)

	// Calculate velocity components using Minecraft conventions
	// This matches the client-side arrow rotation calculation from velocity vectors:
	// - yaw = atan2(velX, velZ) * 180/PI (calculated in rendering from velocity)
	// - pitch = atan2(velY, sqrt(velX² + velZ²)) * 180/PI (vertical angle from velocity)
	// For pitch: vY = -sin(pitch) where negative pitch = up, positive pitch = down
	// For yaw: 0° = North (+Z), 90° = West (-X), 180° = South (-Z), 270° = East (+X)
	velXZ := math.Cos(pitchRad) * initialSpeed
	velY := -math.Sin(pitchRad) * initialSpeed // Minecraft convention: negative sign
	velX := -math.Sin(yawRad) * velXZ
	velZ := math.Cos(yawRad) * velXZ // Changed from negative to positive
	// Validation: arrow rotation is calculated client-side based on velocity vectors
	// The arrow model in rendering uses atan2(vX, vZ) for yaw and atan2(vY, horizDist) for pitch

	// Simulate trajectory for prediction
	velocity := models.V3{X: velX, Y: velY, Z: velZ}
	trajectory := physics.SimulateProjectileTrajectory(models.Arrow, botOrigin, velocity, 400)

	log.Printf("[FireBowWithPitch] Pitch=%.1f, Yaw=%.1f, InitialVel=(%.3f, %.3f, %.3f), Predicted %d trajectory points",
		pitch, yaw, velX, velY, velZ, len(trajectory))

	// Visualize the predicted trajectory
	targetPos := botOrigin
	if len(trajectory) > 0 {
		lastPoint := trajectory[len(trajectory)-1]
		targetPos = models.V3{X: lastPoint.Pos.X, Y: lastPoint.Pos.Y, Z: lastPoint.Pos.Z}
	}
	a.visualizeTrajectory(botOrigin, targetPos, trajectory, yaw, true)

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return nil, fmt.Errorf("action handler not available")
	}

	// Register all callbacks for this arrow
	a.setPendingProjectileCallback(models.Arrow, nil, callbacks...)

	// Fire with full power (max hold duration)
	conn, err := a.getPacketWriter()
	if err != nil {
		return nil, err
	}
	sequence := a.getNextSequence()
	if err := actions.SendUseItem(conn, 0, sequence, yaw, pitch); err != nil {
		return nil, fmt.Errorf("send use item: %w", err)
	}

	// Hold for maximum duration to get full power
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(conn, 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(maxBowHoldDuration / time.Duration(bowHoldIterations))
		}
		_ = actions.SendPlayerAction(conn, 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return trajectory, nil
}

// FireBow fires a bow with default hold time using version-specific handlers
// Deprecated: Use FireBowAt for targeted firing
func (a *agent) FireBow(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if a.client == nil || a.versionHandler == nil {
		return fmt.Errorf("client or version handler not ready")
	}

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return fmt.Errorf("action handler not available")
	}

	// Get connection
	conn, err := a.getPacketWriter()
	if err != nil {
		return err
	}

	// Get next sequence number
	sequence := a.getNextSequence()

	// Use main hand then simulate action
	yaw, pitch := a.getRotation()
	if err := actions.SendUseItem(conn, 0, sequence, yaw, pitch); err != nil {
		return fmt.Errorf("send use item: %w", err)
	}

	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(conn, 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(bowHoldSleep)
		}
		_ = actions.SendPlayerAction(conn, 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return nil
}

// cmdFireBow via UseItem + PlayerAction (legacy chat command)
func (a *agent) cmdFireBow() {
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	_ = a.FireBow(ctx) // Delegate to the main implementation
}

// FireBowAt fires a bow at a specific target position using version-specific handlers
func (a *agent) FireBowAt(ctx context.Context, targetX, targetY, targetZ float64, callbacks ...models.ProjectileHitCallback) ([]models.TrajectoryPoint, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if a.client == nil || a.versionHandler == nil {
		return nil, fmt.Errorf("client or version handler not ready")
	}

	if a.moveExec == nil {
		return nil, fmt.Errorf("movement executor not available")
	}

	if err := a.TurnTowards(ctx, targetX, targetY, targetZ); err != nil {
		return nil, fmt.Errorf("turn towards target: %w", err)
	}

	time.Sleep(200 * time.Millisecond) // Small delay to ensure rotation is processed

	visible, _, _, _, err := a.hasLineOfSightForAccess(ctx, targetX, targetY, targetZ)
	if err != nil {
		return nil, err
	}

	if !visible {
		return nil, fmt.Errorf("Bot doesn't have line of sight to the target")
	}

	botX, botY, botZ, _, _, initialized := a.GetPosition()
	if !initialized {
		return nil, fmt.Errorf("bot position not initialized")
	}

	// Use physics package to calculate aiming
	// Arrows spawn at eye position minus 0.1 blocks
	// For standing player: eye height = 1.62, so spawn = Y + 1.62 - 0.1 = Y + 1.52
	// Source: PersistentProjectileEntity.java:113
	botOrigin := models.V3{X: botX, Y: botY + a.getEyeHeight() - .1, Z: botZ}
	targetPos := models.V3{X: targetX, Y: targetY, Z: targetZ}

	log.Printf("[Agent %s] FireBowAtDebug: INPUT CHECK - bot actual pos=(%.2f, %.2f, %.2f), target input=(%.2f, %.2f, %.2f)",
		a.cfg.Name, botX, botY, botZ, targetX, targetY, targetZ)

	// Use trajectory validation to find unobstructed path
	validSolution, err := a.FindValidTrajectory(models.Arrow, botOrigin, targetPos)
	if err != nil {
		log.Printf("[Agent %s] FireBowAtDebug: No valid trajectory: %v", a.cfg.Name, err)
		a.SendChat(fmt.Sprintf("Cannot fire at (%.1f, %.1f, %.1f): %v",
			targetPos.X, targetPos.Y, targetPos.Z, err))

		return nil, err
	}

	pitch := validSolution.Pitch
	powerFactor := 1.0 // Always use full power for debug mode
	trajectory := validSolution.Trajectory

	// Calculate yaw
	dx := targetPos.X - botOrigin.X
	dz := targetPos.Z - botOrigin.Z
	yaw := physics.YawForStartTarget(botOrigin, targetPos)

	log.Printf("[Agent %s] FireBowAtDebug: Yaw=%.10f°",
		a.cfg.Name, yaw)
	log.Printf("[Agent %s] FireBowAtDebug: Yaw calculation debug: dx=%.2f, dz=%.2f, atan2(dz,dx)_rad=%.4f, atan2(dz,dx)_deg=%.2f, yaw_final=%.2f°",
		a.cfg.Name, dx, dz, math.Atan2(dz, dx), math.Atan2(dz, dx)*180/math.Pi, yaw)
	log.Printf("[Agent %s] FireBowAtDebug: Trajectory validated for arrow: botOrigin=(%.2f,%.2f,%.2f), targetPos=(%.2f,%.2f,%.2f), yaw=%.2f°, pitch=%.2f°, power=%.3f, blocked=%v",
		a.cfg.Name, botOrigin.X, botOrigin.Y, botOrigin.Z, targetPos.X, targetPos.Y, targetPos.Z, yaw, pitch, powerFactor, validSolution.IsBlocked)
	log.Printf("[Agent %s] FireBowAtDebug: SendUseItem will send: yaw=%.2f°, pitch=%.2f°",
		a.cfg.Name, yaw, pitch)

	// CRITICAL: Send position packet with trajectory-verified pitch BEFORE using bow
	// This ensures server knows the correct player rotation matching the trajectory we calculated
	log.Printf("[Agent %s] FireBowAtDebug: Sending position with trajectory pitch=%.2f°", a.cfg.Name, pitch)
	if err := a.moveExec.SendPositionAndRotation(botX, botY, botZ, yaw, pitch, true); err != nil {
		return nil, fmt.Errorf("send position for bow: %w", err)
	}

	time.Sleep(50 * time.Millisecond) // Small delay to ensure position packet is processed

	a.setPosition(botX, botY, botZ, yaw, pitch)

	// Visualize trajectory with display entities for debugging (uses RCON if available)
	log.Printf("[Agent %s] FireBowAtDebug: Calling visualizeTrajectory with %d trajectory points", a.cfg.Name, len(trajectory))
	if len(trajectory) > 0 {
		a.visualizeTrajectory(botOrigin, targetPos, trajectory, yaw, true)
	} else {
		log.Printf("[Agent %s] FireBowAtDebug: WARNING: Target at (%.1f, %.1f, %.1f) is unreachable",
			a.cfg.Name, targetPos.X, targetPos.Y, targetPos.Z)
		a.SendChat(fmt.Sprintf("WARNING: Target at (%.1f, %.1f, %.1f) is unreachable",
			targetPos.X, targetPos.Y, targetPos.Z))

		return nil, fmt.Errorf("no valid trajectory found to target (%.1f, %.1f, %.1f)", targetPos.X, targetPos.Y, targetPos.Z)
	}

	// Use power factor for hold duration calculation
	holdDurationSeconds := powerFactor * 1.0
	holdDuration := durationFromSeconds(holdDurationSeconds)

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return nil, fmt.Errorf("action handler not available")
	}

	// Register all callbacks for this arrow
	a.setPendingProjectileCallback(models.Arrow, &targetPos, callbacks...)

	// Get sequence and use action handler
	conn, err := a.getPacketWriter()
	if err != nil {
		return nil, err
	}
	sequence := a.getNextSequence()
	// NOTE: UseItem yaw/pitch are not used by server for arrow direction.
	// The server uses the player's last known rotation from position packets.
	// TurnTowards now uses physics-consistent yaw calculation via YawForStartTarget.
	if err := actions.SendUseItem(conn, 0, sequence, yaw, pitch); err != nil {
		return nil, fmt.Errorf("send use item: %w", err)
	}

	// Hold for maximum duration to get full power
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(conn, 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(holdDuration / time.Duration(bowHoldIterations))
		}
		_ = actions.SendPlayerAction(conn, 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return trajectory, nil
}

// cmdFireBowAt via UseItem + PlayerAction (legacy chat command)
func (a *agent) cmdFireBowAt(x, y, z float64) {
	if _, err := a.FireBowAt(context.Background(), x, y, z); err != nil {
		_ = a.SendChat("Fire bow at error: " + err.Error())
	}
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// visualizeTrajectory displays the arrow's trajectory using persistent markers via RCON
// Shows calculated trajectory in orange stained glass (1/8 scale) display entities.
// yawDeg is the firing yaw in degrees, used to rotate the local-space trajectory into world coordinates.
func (a *agent) visualizeTrajectory(origin, target models.V3, trajectory []models.TrajectoryPoint, yawDeg float64, removePrevious bool) {
	log.Printf("[visualizeTrajectory] Starting visualization (RCON available: %v, trajectory points: %d, yaw: %.1f)",
		a.cfg.RCON != nil, len(trajectory), yawDeg)
	// Skip if we don't have RCON access (e.g., in non-testing scenarios)
	if a.cfg.RCON == nil {
		log.Printf("[visualizeTrajectory] RCON not available, skipping visualization")
		return
	}

	if removePrevious {
		log.Printf("[visualizeTrajectory] Removing previous trajectory markers")
		ctx := context.Background()
		removeCmd := `/kill @e[tag=projectile_trajectory]`
		if _, err := a.cfg.RCON.Exec(ctx, removeCmd); err != nil {
			log.Printf("[visualizeTrajectory] Error removing previous markers: %v", err)
		}
	}

	if len(trajectory) == 0 {
		log.Printf("[visualizeTrajectory] Empty trajectory, skipping visualization")
		return
	}

	log.Printf("[visualizeTrajectory] Origin: (%.2f, %.2f, %.2f), Target: (%.2f, %.2f, %.2f)",
		origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z)
	log.Printf("[visualizeTrajectory] Visualizing trajectory with %d points", len(trajectory))

	ctx := context.Background()

	// Build display entity commands for calculated trajectory (1/8 scale = 0.125)
	// Using orange_stained_glass for calculated path
	// Only visualize while projectile is in flight (has meaningful velocity > 0.05 blocks/tick)
	var trajectoryCommands []string
	for i, point := range trajectory {
		if i%2 == 0 { // Sample every 2 ticks for better visibility
			// Stop visualizing when projectile has essentially stopped moving
			horizontalVel := math.Sqrt(point.Vel.X*point.Vel.X + point.Vel.Z*point.Vel.Z)
			if horizontalVel < 0.05 && math.Abs(point.Vel.Y) < 0.05 {
				break // Projectile has landed
			}

			// Trajectory already in world coordinates, use directly
			worldX := point.Pos.X
			worldY := point.Pos.Y
			worldZ := point.Pos.Z

			// Block display entity at 1/8 scale (0.125)
			nbt := `{Tags:[projectile_trajectory],Glowing:1b,block_state:{Name:"minecraft:orange_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
			cmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", worldX, worldY, worldZ, nbt)
			trajectoryCommands = append(trajectoryCommands, cmd)
		}
	}

	// Send calculated trajectory display entities
	for i, cmd := range trajectoryCommands {
		log.Printf("[visualizeTrajectory] Sending trajectory point %d: %s", i, cmd)
		if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
			log.Printf("[visualizeTrajectory] Error sending trajectory point %d: %v", i, err)
		}
	}

	// Mark the origin point with green concrete
	originNBT := `{Tags:[projectile_trajectory],Glowing:1b,block_state:{Name:"minecraft:green_concrete"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
	originCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", origin.X, origin.Y, origin.Z, originNBT)
	log.Printf("[visualizeTrajectory] Origin marker: %s", originCmd)
	if _, err := a.cfg.RCON.Exec(ctx, originCmd); err != nil {
		log.Printf("[visualizeTrajectory] Error sending origin marker: %v", err)
	}

	// Mark the target with a target block
	targetNBT := `{Tags:[projectile_trajectory],Glowing:1b,block_state:{Name:"minecraft:target"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
	targetCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", target.X, target.Y, target.Z, targetNBT)
	log.Printf("[visualizeTrajectory] Target marker: %s", targetCmd)
	if _, err := a.cfg.RCON.Exec(ctx, targetCmd); err != nil {
		log.Printf("[visualizeTrajectory] Error sending target marker: %v", err)
	}

	// Mark landing point of calculated trajectory with white glass
	if len(trajectory) > 0 {
		lastPoint := trajectory[len(trajectory)-1]

		// Trajectory already in world coordinates
		landingX := lastPoint.Pos.X
		landingY := lastPoint.Pos.Y
		landingZ := lastPoint.Pos.Z

		landingNBT := `{Tags:[projectile_trajectory],Glowing:1b,block_state:{Name:"minecraft:white_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
		landingCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", landingX, landingY, landingZ, landingNBT)
		log.Printf("[visualizeTrajectory] Landing marker: %s", landingCmd)
		if _, err := a.cfg.RCON.Exec(ctx, landingCmd); err != nil {
			log.Printf("[visualizeTrajectory] Error sending landing marker: %v", err)
		}
	}

	log.Printf("[visualizeTrajectory] Trajectory visualization complete (%d calculated points + 3 markers)", len(trajectoryCommands))
}

// visualizeActualEntityTrajectory displays the actual projectile path from server packets
// For persistent projectiles (arrows, tridents), uses server position updates
// For non-persistent projectiles (wind charges, snowballs, etc.), uses simulated trajectory
// Uses magma_block for path (glowing orange/red) and crying_obsidian for landing marker
func (a *agent) visualizeActualEntityTrajectory(projType models.ProjectileType, positions []models.V3) {
	if a.cfg.RCON == nil {
		log.Printf("[visualizeActualEntityTrajectory] RCON not available, skipping visualization")
		return
	}

	if len(positions) == 0 {
		log.Printf("[visualizeActualEntityTrajectory] No positions to visualize for %s", projType)
		return
	}

	log.Printf("[visualizeActualEntityTrajectory] Visualizing %d actual %s positions (magma_block)", len(positions), projType)

	ctx := context.Background()

	// Kill previous entity track first
	killCmd := "/kill @e[tag=projectile_entity_track]"
	if _, err := a.cfg.RCON.Exec(ctx, killCmd); err != nil {
		log.Printf("[visualizeActualEntityTrajectory] Error killing previous entity track: %v", err)
	}

	// Build display entity commands for actual trajectory (1/8 scale = 0.125)
	// Using magma_block for actual path - distinctive glowing orange/red texture
	var trajectoryCommands []string
	for i, pos := range positions {
		if i%2 == 0 { // Sample every other position for consistency with calculated trajectory
			// Block display entity at 1/8 scale
			nbt := `{Tags:["projectile_entity_track"],Glowing:1b,block_state:{Name:"minecraft:magma_block"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
			cmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", pos.X, pos.Y, pos.Z, nbt)
			trajectoryCommands = append(trajectoryCommands, cmd)
		}
	}

	// Send actual trajectory display entities
	for i, cmd := range trajectoryCommands {
		log.Printf("[visualizeActualEntityTrajectory] Sending actual position %d: %s", i, cmd)
		if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
			log.Printf("[visualizeActualEntityTrajectory] Error sending position %d: %v", i, err)
		}
	}

	// Mark the actual landing position with crying_obsidian (dark purple with glowing drips)
	if len(positions) > 0 {
		last := positions[len(positions)-1]
		landingNBT := `{Tags:["projectile_entity_track"],Glowing:1b,block_state:{Name:"minecraft:crying_obsidian"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
		landingCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", last.X, last.Y, last.Z, landingNBT)
		log.Printf("[visualizeActualEntityTrajectory] Actual landing marker (%s): %s", projType, landingCmd)
		if _, err := a.cfg.RCON.Exec(ctx, landingCmd); err != nil {
			log.Printf("[visualizeActualEntityTrajectory] Error sending landing marker: %v", err)
		}
	}

	log.Printf("[visualizeActualEntityTrajectory] %s trajectory visualization complete (%d actual points + 1 marker)", projType, len(trajectoryCommands))
}

// ThrowProjectileAt throws/fires a projectile at a target location.
// Supports arrows (via bow), snowballs, eggs, ender pearls, and splash potions.
// projectileType values: 0=Arrow, 1=Snowball, 2=Egg, 3=EnderPearl, 4=SplashPotion
func (a *agent) ThrowProjectileAt(ctx context.Context, projectileType models.ProjectileType, x, y, z float64, callbacks ...models.ProjectileHitCallback) ([]models.TrajectoryPoint, error) {
	if a.client == nil || a.versionHandler == nil {
		return nil, fmt.Errorf("client or version handler not ready")
	}

	// For arrows, use the bow firing mechanism
	if projectileType == models.Arrow {
		return a.FireBowAt(ctx, x, y, z, callbacks...)
	}

	// For other projectiles, we need to:
	// 1. Find the item in hotbar
	// 2. Select it
	// 3. Aim at target
	// 4. Right-click air to throw

	// Get item name for this projectile type
	itemName := projectileType.GetID()
	if itemName == "" {
		return nil, fmt.Errorf("unsupported projectile type: %#v", projectileType)
	}

	// Ensure the item is equipped in the hotbar
	fullItemName := fmt.Sprintf("minecraft:%s", itemName)

	// First try using EquipItemByName if available (searches hotbar only)
	if equipMethod, ok := any(a).(interface {
		EquipItemByName(ctx context.Context, itemName string) error
	}); ok {
		err := equipMethod.EquipItemByName(ctx, fullItemName)
		if err == nil {
			// Successfully equipped from hotbar
			time.Sleep(50 * time.Millisecond)
		} else {
			log.Printf("[Agent %s] EquipItemByName failed for %s: %v. Attempting fallback to inventory search...", a.cfg.Name, fullItemName, err)
			// Not in hotbar, try to find in inventory and move to hotbar
			slot, found, err := a.FindSlotWith(ctx, itemName, 0)
			if err != nil || !found {
				log.Printf("[Agent %s] FindSlotWith also failed for %s in inventory. EquipError: %v, FindError: %v, Found: %v", a.cfg.Name, itemName, err, err, found)
				return nil, fmt.Errorf("%s not found in inventory: %v", itemName, err)
			}

			// Swap inventory slot with hotbar slot 0 (click and quick move)
			// This moves the item from inventory to hotbar
			if err := a.SwapInventoryWithHotbar(ctx, slot, 0); err != nil {
				return nil, fmt.Errorf("error moving %s to hotbar: %w", itemName, err)
			}

			// Now select hotbar slot 0
			if err := a.SelectHotbarSlot(ctx, 0); err != nil {
				return nil, fmt.Errorf("error selecting hotbar slot 0: %w", err)
			}

			time.Sleep(50 * time.Millisecond)
		}
	} else {
		return nil, fmt.Errorf("EquipItemByName method not implemented for this version, cannot equip %s", fullItemName)
	}

	// Aim at the target using physics-based aiming
	botX, botY, botZ, _, _, ok := a.GetPosition()
	if !ok {
		return nil, fmt.Errorf("unable to get bot position")
	}

	// Projectile spawns at eye position minus 0.1 blocks
	// For standing player: eye height = 1.62, so spawn = Y + 1.62 - 0.1 = Y + 1.52
	botOrigin := models.V3{X: botX, Y: botY + a.getEyeHeight() - .1, Z: botZ}
	targetPos := models.V3{X: x, Y: y, Z: z}

	// Use trajectory validation to find unobstructed path
	validSolution, err := a.FindValidTrajectory(projectileType, botOrigin, targetPos)
	if err != nil {
		log.Printf("[Agent %s] ThrowProjectileAt: No valid trajectory for %s: %v", a.cfg.Name, itemName, err)
		a.SendChat(fmt.Sprintf("Cannot throw %s at (%.1f, %.1f, %.1f): %v",
			itemName, x, y, z, err))
		return nil, err
	}

	pitch := validSolution.Pitch
	trajectory := validSolution.Trajectory

	log.Printf("[Agent %s] ThrowProjectileAt: Trajectory validated for %s: botOrigin=(%.2f,%.2f,%.2f), targetPos=(%.2f,%.2f,%.2f), pitch=%.2f°, trajectory points=%d, blocked=%v",
		a.cfg.Name, itemName, botOrigin.X, botOrigin.Y, botOrigin.Z, targetPos.X, targetPos.Y, targetPos.Z, pitch, len(trajectory), validSolution.IsBlocked)

	// Check if target is reachable
	if len(trajectory) == 0 {
		return nil, fmt.Errorf("%s cannot reach target at (%.1f, %.1f, %.1f)", itemName, x, y, z)
	}

	// Calculate yaw
	yaw := physics.YawForStartTarget(botOrigin, targetPos)

	a.visualizeTrajectory(botOrigin, targetPos, trajectory, yaw, true)

	log.Printf("[ThrowProjectileAt] Final aiming angles for %s: yaw=%.2f°, pitch=%.2f°", itemName, yaw, pitch)
	// Turn to face the target
	if err := a.moveExec.SendRotation(yaw, pitch, true); err != nil {
		return nil, fmt.Errorf("error setting rotation: %w", err)
	}

	// Brief delay for aim to register
	time.Sleep(50 * time.Millisecond)

	// Get the current rotation for the use item packet
	// _, _, _, currentYaw, currentPitch, ok := a.GetPosition()
	// if !ok {
	// 	return fmt.Errorf("unable to get bot rotation")
	// }

	// Use item (right-click air) - send use item packet
	// Get the action handler from the version handler
	actionHandler := a.versionHandler.Play().Actions()
	if actionHandler == nil {
		return nil, fmt.Errorf("action handler not available")
	}

	hand := models.MainHand

	// Register all callbacks before sending use item packet
	a.setPendingProjectileCallback(projectileType, &targetPos, callbacks...)

	log.Printf("[ThrowProjectileAt] Sending use item packet for %s with yaw=%.2f, pitch=%.2f", itemName, yaw, pitch)
	conn, err := a.getPacketWriter()
	if err != nil {
		return nil, err
	}
	if err := actionHandler.SendUseItem(conn, hand, 0, yaw, pitch); err != nil {
		return nil, fmt.Errorf("error throwing projectile: %w", err)
	}

	return trajectory, nil
}
