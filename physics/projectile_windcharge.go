package physics

import "github.com/reallyoldfogie/mc-agent/models"

// WindChargeModel implements ProjectilePhysicsModel for wind charges
// Wind charges are player-thrown explosive projectiles introduced in Minecraft 1.21
//
// Physics based on AbstractWindChargeEntity from Minecraft 1.21.8:
// - Drag coefficient: 1.0F (no reduction, constant velocity)
// - Acceleration power: 0.0 (no acceleration, purely ballistic flight)
// - Update order: drag → position (no gravity or custom acceleration)
//
// Wind charges travel in STRAIGHT LINES with NO VERTICAL DROP,
// maintaining constant velocity until collision/despawn.
//
// IMPORTANT: Randomness in Player-Fired Projectiles
// In Minecraft, player-fired projectiles include randomness to simulate inaccuracy:
// - Formula: deviation = random.nextGaussian() * 0.0075 * inaccuracy
// - Expected spread: ~0.3 blocks (pre-1.21.6) or delayed spread (1.21.6+)
// - 1.21.6+ provides zero spread for first 2 ticks, then gradual increase (0.05/tick)
// - This RANDOMNESS is NOT part of the physics model itself, but applied at firing time
// - Testing should account for this inherent inaccuracy (±0.3-0.5 block tolerance)
// See: docs/PROJECTILE_RANDOMNESS.md for details
type WindChargeModel struct {
	drag         float64 // Air drag coefficient (1.0 = constant velocity)
	waterDrag    float64 // Water drag coefficient (1.0 = no attenuation in liquids per Java)
	initialSpeed float64 // Throw speed
	customAccel  float64 // Custom vertical adjustment (0.0 = no acceleration)
}

// Type returns the projectile type
func (m *WindChargeModel) Type() models.ProjectileType {
	return models.WindCharge
}

// Gravity returns 0 (wind charges don't use standard gravity)
func (m *WindChargeModel) Gravity() float64 {
	return 0
}

// Drag returns the air drag coefficient
func (m *WindChargeModel) Drag() float64 {
	return m.drag
}

// WaterDrag returns the water drag coefficient
func (m *WindChargeModel) WaterDrag() float64 {
	return m.waterDrag
}

// InitialSpeed returns the throw speed
func (m *WindChargeModel) InitialSpeed() float64 {
	return m.initialSpeed
}

// UpdateOrder returns the physics update order for wind charges
// Note: While OrderDragPositionCustomAccel is used, customAccel=0.0 makes vertical force a no-op
func (m *WindChargeModel) UpdateOrder() models.UpdateOrder {
	return models.OrderDragPositionCustomAccel // drag → position → (no acceleration)
}

// ApplyPhysics applies one tick of physics to a wind charge
// Wind charges maintain constant velocity with optional liquid attenuation
func (m *WindChargeModel) ApplyPhysics(pos, vel models.V3, inWater bool) (models.V3, models.V3) {
	// Step 1: Apply drag (1.0 in air and water = constant velocity everywhere)
	drag := m.Drag()
	if inWater {
		drag = m.WaterDrag()
	}
	vel = vel.Mul(drag)

	// Step 2: Update position (straight-line flight)
	pos = pos.Add(vel)

	// Step 3: Apply vertical force (0.0 = no acceleration, purely ballistic)
	vel = m.ApplyVerticalForce(vel)

	return pos, vel
}

// ApplyVerticalForce applies vertical acceleration (0.0 for wind charges)
// Wind charges have NO vertical acceleration, maintaining level flight
func (m *WindChargeModel) ApplyVerticalForce(vel models.V3) models.V3 {
	vel.Y -= m.customAccel // 0.0 = no vertical acceleration
	return vel
}

// NewWindChargeModel creates a wind charge model with custom parameters
func NewWindChargeModel(drag, waterDrag, initialSpeed, customAccel float64) models.ProjectilePhysicsModel {
	return &WindChargeModel{
		drag:         drag,
		waterDrag:    waterDrag,
		initialSpeed: initialSpeed,
		customAccel:  customAccel,
	}
}

// WindChargeModelInstance is the predefined wind charge model
// Based on AbstractWindChargeEntity from Minecraft 1.21.8:
// drag: 1.0 (no reduction in air per Java source: "Drag coefficient: 1.0F")
// waterDrag: 1.0 (no reduction in water per Java source: "no reduction in water")
// initialSpeed: 1.5 (high velocity for projectile)
// customAccel: 0.0 (no acceleration - purely ballistic per Java source)
var WindChargeModelInstance = NewWindChargeModel(1.0, 1.0, 1.5, 0.0)
