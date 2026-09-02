package movement

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// applyMovementState applies sprint/sneak state changes based on inputs.
func (pe *PhysicsMovementExecutor) applyMovementState(inputs physics.Inputs) {
	// Apply sprint state. canSprint gates both starting a new sprint and
	// continuing one already in progress (Phase 4: Blindness, §4.9) —
	// mirrors Java's canStartSprinting()/shouldStopSprinting(), both of
	// which key off the same canSprint() check.
	canSprint := true
	if pe.entityPositionGetter != nil {
		_, hasBlindness := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:blindness")
		canSprint = physics.CanSprint(hasBlindness)
	}

	if inputs.Sprint && canSprint && !pe.IsSprinting() {
		if err := pe.StartSprinting(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to start sprinting: %v", err)
		}
	} else if (!inputs.Sprint || !canSprint) && pe.IsSprinting() {
		if err := pe.StopSprinting(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to stop sprinting: %v", err)
		}
	}

	// Apply sneak state
	if inputs.Sneak && !pe.IsSneaking() {
		if err := pe.StartSneaking(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to start sneaking: %v", err)
		}
	} else if !inputs.Sneak && pe.IsSneaking() {
		if err := pe.StopSneaking(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to stop sneaking: %v", err)
		}
	}
}

// syncActiveEffects reads the agent's own currently-active status effects
// (Phase 4a: Slow Falling, Levitation) via entityPositionGetter and pushes
// them into physicsState before Tick() runs, mirroring how other
// externally-sourced per-tick state (e.g. mounted-entity position) is synced
// in rather than pulled by physics.State itself, which has no agent/registry
// access.
func (pe *PhysicsMovementExecutor) syncActiveEffects() {
	if pe.entityPositionGetter == nil {
		return
	}

	var effects models.ActiveEffects
	if _, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:slow_falling"); found {
		effects.HasSlowFalling = true
	}
	if amplifier, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:levitation"); found {
		effects.HasLevitation = true
		effects.LevitationAmplifier = amplifier
	}
	if amplifier, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:speed"); found {
		effects.HasSpeed = true
		effects.SpeedAmplifier = amplifier
	}
	if amplifier, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:slowness"); found {
		effects.HasSlowness = true
		effects.SlownessAmplifier = amplifier
	}
	if amplifier, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:jump_boost"); found {
		effects.HasJumpBoost = true
		effects.JumpBoostAmplifier = amplifier
	}
	if _, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:dolphins_grace"); found {
		effects.HasDolphinsGrace = true
	}
	if _, found := pe.entityPositionGetter.GetOwnActiveEffect("minecraft:weaving"); found {
		effects.HasWeaving = true
	}

	pe.physicsState.SetActiveEffects(effects)
}

// syncEquipment reads the agent's own currently-equipped chest and feet
// items via entityPositionGetter and pushes elytra-equipped/leather-boots
// state into physicsState before Tick() runs, mirroring syncActiveEffects's
// pattern — physics.State has no inventory access of its own.
func (pe *PhysicsMovementExecutor) syncEquipment() {
	if pe.entityPositionGetter == nil {
		return
	}

	chestItemName, _ := pe.entityPositionGetter.GetOwnEquippedChestItem()
	pe.physicsState.SetElytraEquipped(chestItemName == "elytra")

	feetItemName, _ := pe.entityPositionGetter.GetOwnEquippedFeetItem()
	pe.physicsState.SetLeatherBootsEquipped(feetItemName == "leather_boots")
}

// syncFireworkBoost reads whether a firework rocket is currently attached
// to (boosting) the agent's own entity via entityPositionGetter and pushes
// that into physicsState before Tick() runs, mirroring syncEquipment's
// pattern — physics.State has no entity-tracking access of its own.
func (pe *PhysicsMovementExecutor) syncFireworkBoost() {
	if pe.entityPositionGetter == nil {
		return
	}

	pe.physicsState.SetFireworkBoosting(pe.entityPositionGetter.HasActiveFireworkBoost())
}

// syncFlying reads the agent's own tracked flying ability state via
// entityPositionGetter and pushes it into physicsState before Tick() runs,
// mirroring syncEquipment's pattern — physics.State has no ability-tracking
// access of its own.
func (pe *PhysicsMovementExecutor) syncFlying() {
	if pe.entityPositionGetter == nil {
		return
	}

	flying, flySpeed := pe.entityPositionGetter.GetOwnFlying()
	pe.physicsState.SetFlying(flying, flySpeed)
}

// syncNoClip reads the agent's own tracked game mode via
// entityPositionGetter and pushes spectator-noclip state into physicsState
// before Tick() runs, mirroring syncEquipment's pattern — physics.State
// has no game-mode access of its own.
func (pe *PhysicsMovementExecutor) syncNoClip() {
	if pe.entityPositionGetter == nil {
		return
	}

	pe.physicsState.SetNoClip(pe.entityPositionGetter.IsSpectator())
}

// recordTelemetry records telemetry data if a recorder is set.
func (pe *PhysicsMovementExecutor) recordTelemetry(inputs physics.Inputs) {
	if pe.telemetryRecorder == nil {
		return
	}

	pos, _, _, onGround := pe.physicsState.GetPosition()

	// Record jump if jumping
	if inputs.Jump && onGround {
		pe.telemetryRecorder.RecordJump()
	}

	// Check if on climbable block
	blockAtPlayer, _ := pe.world.GetBlockStatus(
		int(pos.X),
		int(pos.Y),
		int(pos.Z),
	)
	climbing := pe.shapeProvider.IsClimbable(blockAtPlayer)

	// Record tick
	pe.telemetryRecorder.RecordTick(
		pos.X, pos.Y, pos.Z,
		onGround,
		climbing,
		inputs.Sneak,
	)
}

// checkClutch checks for clutch opportunities if enabled.
func (pe *PhysicsMovementExecutor) checkClutch() {
	if pe.clutchCallback == nil {
		return
	}

	if time.Since(pe.lastClutchTime) < pe.clutchCooldown {
		return
	}

	if plan, ok := physics.PlanClutch(pe.physicsState, pe.world, pe.shapeProvider); ok {
		pe.lastClutchTime = time.Now()
		pe.clutchCallback(plan)
	}
}

// SetPositionUpdateCallback sets an optional callback invoked after every
// position update is sent to the server.
func (pe *PhysicsMovementExecutor) SetPositionUpdateCallback(callback func(x, y, z float64, yaw, pitch float64)) {
	pe.onPositionUpdate = callback
}

// sendPositionUpdate sends the current physics state position to the server.
func (pe *PhysicsMovementExecutor) sendPositionUpdate() {
	if pe.isDead.Load() {
		return
	}
	pos, yaw, pitch, onGround := pe.physicsState.GetPosition()

	if err := pe.movementPacketSender.SendPositionAndRotation(pos.X, pos.Y, pos.Z, yaw, pitch, onGround); err != nil {
		// Don't spam logs on errors
	}

	if pe.onPositionUpdate != nil {
		pe.onPositionUpdate(pos.X, pos.Y, pos.Z, yaw, pitch)
	}
}

// SetPath sets a new navigation path for the physics executor to follow.
func (pe *PhysicsMovementExecutor) SetPath(path *pathfinding.Path) error {
	if path == nil {
		return fmt.Errorf("path cannot be nil")
	}

	if !path.Found {
		return fmt.Errorf("path not found")
	}

	pe.pathMu.Lock()
	defer pe.pathMu.Unlock()

	pe.currentPath = path
	pe.currentStep = 0

	pos, _, _, _ := pe.physicsState.GetPosition()
	pe.stepStartTime = time.Now()
	pe.stepStartPos = models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
	pe.lastProgressPos = pe.stepStartPos
	pe.lastProgressTime = time.Now()
	pe.isRecovering = false

	if pe.pathDone != nil {
		close(pe.pathDone)
	}
	pe.pathDone = make(chan struct{}, 1)

	pe.modeMu.Lock()
	oldMode := pe.mode
	pe.mode = PhysicsModeNavigating
	pe.modeMu.Unlock()

	agentName := "<unknown>"
	if pe.movementPacketSender != nil && pe.movementPacketSender.client != nil {
		agentName = pe.movementPacketSender.client.Name()
	}

	log.Printf("[PhysicsExecutor %s] Mode changed: %s → Navigating", agentName, oldMode)
	log.Printf("[PhysicsExecutor %s] Path set: %d steps, cost=%.2f", agentName, len(path.Steps), path.TotalCost)

	if len(path.Steps) <= 5 {
		log.Printf("[PhysicsExecutor %s] Path steps:", agentName)
		for i, step := range path.Steps {
			log.Printf("[PhysicsExecutor %s]   Step %d: %s to (%.0f, %.0f, %.0f)",
				agentName, i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
		}
	} else {
		log.Printf("[PhysicsExecutor %s] First 3 steps:", agentName)
		for i := 0; i < 3 && i < len(path.Steps); i++ {
			step := path.Steps[i]
			log.Printf("[PhysicsExecutor %s]   Step %d: %s to (%.0f, %.0f, %.0f)",
				agentName, i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
		}
		log.Printf("[PhysicsExecutor %s] ... (%d steps omitted)", agentName, len(path.Steps)-6)
		log.Printf("[PhysicsExecutor %s] Last 3 steps:", agentName)
		for i := len(path.Steps) - 3; i < len(path.Steps); i++ {
			step := path.Steps[i]
			log.Printf("[PhysicsExecutor %s]   Step %d: %s to (%.0f, %.0f, %.0f)",
				agentName, i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
		}
	}

	return nil
}

// WaitForPathCompletion blocks until the current path is complete, context is cancelled, or timeout expires.
func (pe *PhysicsMovementExecutor) WaitForPathCompletion(ctx context.Context, timeout time.Duration) (bool, error) {
	pe.pathMu.RLock()
	pathDone := pe.pathDone
	pe.pathMu.RUnlock()

	if pathDone == nil {
		return true, nil
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	select {
	case <-pathDone:
		log.Printf("[PhysicsExecutor] Path completion signaled")
		return true, nil
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[PhysicsExecutor] Path completion timeout")
			return false, nil
		}
		log.Printf("[PhysicsExecutor] Path completion cancelled by context")
		return false, ctx.Err()
	}
}

// WaitForPathCompletionLegacy is deprecated. Use WaitForPathCompletion with timeout=0 instead.
func (pe *PhysicsMovementExecutor) WaitForPathCompletionLegacy(ctx context.Context) error {
	completed, err := pe.WaitForPathCompletion(ctx, 0)
	if !completed && err == nil {
		return nil
	}
	return err
}

// ClearPath clears the current path and switches to idle mode.
func (pe *PhysicsMovementExecutor) ClearPath() {
	pe.pathMu.Lock()
	defer pe.pathMu.Unlock()

	pe.currentPath = nil
	pe.currentStep = 0

	if pe.pathDone != nil {
		select {
		case pe.pathDone <- struct{}{}:
		default:
		}
		close(pe.pathDone)
		pe.pathDone = nil
	}

	pe.modeMu.Lock()
	pe.mode = PhysicsModeIdle
	pe.modeMu.Unlock()

	log.Printf("[PhysicsExecutor] Path cleared, switched to idle mode")
}
