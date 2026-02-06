package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/physics"
)

// FireBow fires a bow with default hold time using version-specific handlers
// Deprecated: Use FireBowAt for targeted firing
func (a *agent) FireBow() error {
	if a.client == nil || a.versionHandler == nil {
		return fmt.Errorf("client or version handler not ready")
	}

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return fmt.Errorf("action handler not available")
	}

	// Get next sequence number
	sequence := a.getNextSequence()

	// Use main hand then simulate action
	yaw, pitch := a.getRotation()
	if err := actions.SendUseItem(a.client.Conn(), 0, sequence, yaw, pitch); err != nil {
		return fmt.Errorf("send use item: %w", err)
	}

	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(a.client.Conn(), 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(bowHoldSleep)
		}
		_ = actions.SendPlayerAction(a.client.Conn(), 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return nil
}

// cmdFireBow via UseItem + PlayerAction (legacy chat command)
func (a *agent) cmdFireBow() {
	_ = a.FireBow() // Delegate to the main implementation
}

// FireBowAt fires a bow at a specific target position using version-specific handlers
func (a *agent) FireBowAt(x, y, z float64) error {
	_, err := a.FireBowAtDebug(x, y, z)
	return err
}

func (a *agent) FireBowAtDebug(x, y, z float64) ([]physics.TrajectoryPoint, error) {
	if a.client == nil || a.versionHandler == nil {
		return nil, fmt.Errorf("client or version handler not ready")
	}

	if a.moveExec == nil {
		return nil, fmt.Errorf("movement executor not available")
	}

	if err := a.TurnTowards(context.Background(), x, y, z); err != nil {
		return nil, fmt.Errorf("turn towards target: %w", err)
	}

	time.Sleep(200 * time.Millisecond) // Small delay to ensure rotation is processed

	botX, botY, botZ, _, _, initialized := a.GetPosition()
	if !initialized {
		return nil, fmt.Errorf("bot position not initialized")
	}

	// Use physics package to calculate aiming
	// Note: arrows spawn at botY + 1.5, not at eye height
	botOrigin := physics.V3{X: botX, Y: botY + 1.5, Z: botZ}
	targetPos := physics.V3{X: x, Y: y, Z: z}

	// Get aiming parameters and trajectory
	dx := targetPos.X - botOrigin.X
	dy := targetPos.Y - botOrigin.Y
	dz := targetPos.Z - botOrigin.Z
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	yaw := math.Atan2(-dx, -dz) * 180 / math.Pi
	pitch, powerFactor, _, trajectory := physics.FindOptimalAiming(physics.Arrow, horizontalDist, dy)

	a.setPosition(botX, botY, botZ, float32(yaw), float32(pitch))

	// Visualize trajectory with display entities for debugging (uses RCON if available)
	log.Printf("[FireBowAtDebug] Calling visualizeArrowTrajectory with %d trajectory points", len(trajectory))
	a.visualizeArrowTrajectory(botOrigin, targetPos, trajectory)

	// Use power factor for hold duration calculation
	holdDurationSeconds := powerFactor * 1.0
	holdDuration := durationFromSeconds(holdDurationSeconds)

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return nil, fmt.Errorf("action handler not available")
	}

	// Get sequence and use action handler
	sequence := a.getNextSequence()
	if err := actions.SendUseItem(a.client.Conn(), 0, sequence, float32(yaw), float32(pitch)); err != nil {
		return nil, fmt.Errorf("send use item: %w", err)
	}

	// Hold for maximum duration to get full power
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(a.client.Conn(), 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(holdDuration / time.Duration(bowHoldIterations))
		}
		_ = actions.SendPlayerAction(a.client.Conn(), 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return trajectory, nil
}

// cmdFireBowAt via UseItem + PlayerAction (legacy chat command)
func (a *agent) cmdFireBowAt(x, y, z float64) {
	if err := a.FireBowAt(x, y, z); err != nil {
		_ = a.SendChat("Fire bow at error: " + err.Error())
	}
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// visualizeArrowTrajectory displays the arrow's trajectory using persistent markers via RCON
// Shows calculated trajectory in orange stained glass (1/8 scale) display entities
func (a *agent) visualizeArrowTrajectory(origin, target physics.V3, trajectory []physics.TrajectoryPoint) {
	log.Printf("[visualizeArrowTrajectory] Starting visualization (RCON available: %v, trajectory points: %d)",
		a.cfg.RCON != nil, len(trajectory))
	// Skip if we don't have RCON access (e.g., in non-testing scenarios)
	if a.cfg.RCON == nil {
		log.Printf("[visualizeArrowTrajectory] RCON not available, skipping visualization")
		return
	}

	if len(trajectory) == 0 {
		log.Printf("[visualizeArrowTrajectory] Empty trajectory, skipping visualization")
		return
	}

	log.Printf("[visualizeArrowTrajectory] Origin: (%.2f, %.2f, %.2f), Target: (%.2f, %.2f, %.2f)",
		origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z)
	log.Printf("[visualizeArrowTrajectory] Visualizing trajectory with %d points", len(trajectory))

	ctx := context.Background()

	// Build display entity commands for calculated trajectory (1/8 scale = 0.125)
	// Using orange_stained_glass for calculated path
	// Only visualize while arrow is in flight (has meaningful velocity > 0.05 blocks/tick)
	var trajectoryCommands []string
	for i, point := range trajectory {
		if i%2 == 0 { // Sample every 2 ticks for better visibility
			// Stop visualizing when arrow has essentially stopped moving
			horizontalVel := math.Sqrt(point.Vel.X*point.Vel.X + point.Vel.Z*point.Vel.Z)
			if horizontalVel < 0.05 && math.Abs(point.Vel.Y) < 0.05 {
				break // Arrow has landed
			}
			
			pos := point.Pos
			worldX := pos.X + origin.X
			worldY := pos.Y + origin.Y
			worldZ := pos.Z + origin.Z
			
			// Block display entity at 1/8 scale (0.125)
			nbt := `{Tags:["arrow_trajectory"],block_state:{Name:"minecraft:orange_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
			cmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", worldX, worldY, worldZ, nbt)
			trajectoryCommands = append(trajectoryCommands, cmd)
		}
	}

	// Send calculated trajectory display entities 
	for i, cmd := range trajectoryCommands {
		log.Printf("[visualizeArrowTrajectory] Sending trajectory point %d: %s", i, cmd)
		if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
			log.Printf("[visualizeArrowTrajectory] Error sending trajectory point %d: %v", i, err)
		}
	}

	// Mark the origin point with yellow glass
	originNBT := `{Tags:["arrow_trajectory"],block_state:{Name:"minecraft:yellow_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
	originCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", origin.X, origin.Y, origin.Z, originNBT)
	log.Printf("[visualizeArrowTrajectory] Origin marker: %s", originCmd)
	if _, err := a.cfg.RCON.Exec(ctx, originCmd); err != nil {
		log.Printf("[visualizeArrowTrajectory] Error sending origin marker: %v", err)
	}

	// Mark the target with bright green glass
	targetNBT := `{Tags:["arrow_trajectory"],block_state:{Name:"minecraft:lime_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
	targetCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", target.X, target.Y, target.Z, targetNBT)
	log.Printf("[visualizeArrowTrajectory] Target marker: %s", targetCmd)
	if _, err := a.cfg.RCON.Exec(ctx, targetCmd); err != nil {
		log.Printf("[visualizeArrowTrajectory] Error sending target marker: %v", err)
	}

	// Mark landing point of calculated trajectory with white glass
	if len(trajectory) > 0 {
		lastPoint := trajectory[len(trajectory)-1]
		landingX := lastPoint.Pos.X + origin.X
		landingY := lastPoint.Pos.Y + origin.Y
		landingZ := lastPoint.Pos.Z + origin.Z
		landingNBT := `{Tags:["arrow_trajectory"],block_state:{Name:"minecraft:white_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
		landingCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", landingX, landingY, landingZ, landingNBT)
		log.Printf("[visualizeArrowTrajectory] Landing marker: %s", landingCmd)
		if _, err := a.cfg.RCON.Exec(ctx, landingCmd); err != nil {
			log.Printf("[visualizeArrowTrajectory] Error sending landing marker: %v", err)
		}
	}

	log.Printf("[visualizeArrowTrajectory] Trajectory visualization complete (%d calculated points + 3 markers)", len(trajectoryCommands))
}

// visualizeActualArrowTrajectory displays the actual arrow path from server packets
// Uses cyan particles to show actual positions vs calculated trajectory (redstone)
func (a *agent) visualizeActualArrowTrajectory(positions []struct{ X, Y, Z float64 }) {
	if a.cfg.RCON == nil {
		log.Printf("[visualizeActualArrowTrajectory] RCON not available, skipping visualization")
		return
	}

	if len(positions) == 0 {
		log.Printf("[visualizeActualArrowTrajectory] No positions to visualize")
		return
	}

	log.Printf("[visualizeActualArrowTrajectory] Visualizing %d actual arrow positions (cyan)", len(positions))

	ctx := context.Background()

	// Build display entity commands for actual trajectory (1/8 scale = 0.125)
	// Using cyan_stained_glass for actual path
	var trajectoryCommands []string
	for i, pos := range positions {
		if i%2 == 0 { // Sample every other position for consistency with calculated trajectory
			// Block display entity at 1/8 scale
			nbt := `{Tags:["arrow_trajectory_actual"],block_state:{Name:"minecraft:cyan_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
			cmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", pos.X, pos.Y, pos.Z, nbt)
			trajectoryCommands = append(trajectoryCommands, cmd)
		}
	}

	// Send actual trajectory display entities 
	for i, cmd := range trajectoryCommands {
		log.Printf("[visualizeActualArrowTrajectory] Sending actual position %d: %s", i, cmd)
		if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
			log.Printf("[visualizeActualArrowTrajectory] Error sending position %d: %v", i, err)
		}
	}

	// Mark the actual landing position with blue glass
	if len(positions) > 0 {
		last := positions[len(positions)-1]
		landingNBT := `{Tags:["arrow_trajectory_actual"],block_state:{Name:"minecraft:blue_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.125f,0.125f,0.125f], right_rotation:[0f,0f,0f,1f]}}`
		landingCmd := fmt.Sprintf("/summon block_display %.2f %.2f %.2f %s", last.X, last.Y, last.Z, landingNBT)
		log.Printf("[visualizeActualArrowTrajectory] Actual landing marker: %s", landingCmd)
		if _, err := a.cfg.RCON.Exec(ctx, landingCmd); err != nil {
			log.Printf("[visualizeActualArrowTrajectory] Error sending landing marker: %v", err)
		}
	}

	log.Printf("[visualizeActualArrowTrajectory] Trajectory visualization complete (%d actual points + 1 marker)", len(trajectoryCommands))
}
