package physics

import "github.com/reallyoldfogie/mc-agent/models"

// WindChargeModel implements ProjectilePhysicsModel for wind charges
// Wind charges are player-thrown explosive projectiles introduced in Minecraft 1.21
//
// Physics order: drag → position (with custom vertical adjustment, no standard gravity)
// This matches ExplosiveProjectileEntity.java:66-94 from Minecraft 1.21.8
//
// IMPORTANT: Wind charges do NOT use standard gravity.
// Instead, they use a custom vertical adjustment of -0.02 per tick
type WindChargeModel struct {
	drag         float64 // Air drag coefficient
	waterDrag    float64 // Water drag coefficient
	initialSpeed float64 // Throw speed
	customAccel  float64 // Custom vertical adjustment (-0.02)
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
func (m *WindChargeModel) UpdateOrder() models.UpdateOrder {
	return models.OrderDragPositionCustomAccel // drag → position (custom acceleration)
}

// ApplyPhysics applies one tick of physics with custom acceleration
func (m *WindChargeModel) ApplyPhysics(pos, vel models.V3, inWater bool) (models.V3, models.V3) {
	// Step 1: Apply drag
	drag := m.Drag()
	if inWater {
		drag = m.WaterDrag()
	}
	vel = vel.Mul(drag)

	// Step 2: Apply custom vertical adjustment (not standard gravity)
	vel = m.ApplyVerticalForce(vel)

	// Step 3: Update position
	pos = pos.Add(vel)

	return pos, vel
}

// ApplyVerticalForce applies custom vertical adjustment instead of gravity
// Wind charges use -0.02 per tick instead of standard gravity
func (m *WindChargeModel) ApplyVerticalForce(vel models.V3) models.V3 {
	vel.Y -= m.customAccel // -0.02 per tick
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
// drag: 0.95 (slightly less drag than most projectiles)
// customAccel: 0.02 (custom vertical adjustment, not standard gravity)
var WindChargeModelInstance = NewWindChargeModel(0.95, 0.8, 1.5, 0.02)
