package physics

import "github.com/reallyoldfogie/mc-agent/models"

// PersistentProjectileModel implements ProjectilePhysicsModel for PersistentProjectileEntity variants
// These are projectiles that stick to surfaces: arrows and tridents
//
// Physics order: position/collision → drag → gravity
// This matches PersistentProjectileEntity.java:163-256 from Minecraft 1.21.8
//
// IMPORTANT: Arrow gravity is 0.05 (not 0.03 like snowballs!)
// This is overridden from the parent ProjectileEntity class
type PersistentProjectileModel struct {
	projectileType models.ProjectileType
	gravity        float64
	drag           float64
	waterDrag      float64
	initialSpeed   float64
}

// Type returns the projectile type
func (m *PersistentProjectileModel) Type() models.ProjectileType {
	return m.projectileType
}

// Gravity returns the gravitational acceleration
func (m *PersistentProjectileModel) Gravity() float64 {
	return m.gravity
}

// Drag returns the air drag coefficient
func (m *PersistentProjectileModel) Drag() float64 {
	return m.drag
}

// WaterDrag returns the water drag coefficient
func (m *PersistentProjectileModel) WaterDrag() float64 {
	return m.waterDrag
}

// InitialSpeed returns the default shoot speed
func (m *PersistentProjectileModel) InitialSpeed() float64 {
	return m.initialSpeed
}

// UpdateOrder returns the physics update order for PersistentProjectileEntity
func (m *PersistentProjectileModel) UpdateOrder() models.UpdateOrder {
	return models.OrderCollisionDragGravity // collision/position → drag → gravity
}

// ApplyPhysics applies one tick of physics in the correct order
// Note: In real Minecraft, position is updated during collision detection.
// For trajectory simulation, we treat it as a simple position update.
func (m *PersistentProjectileModel) ApplyPhysics(pos, vel models.V3, inWater bool) (models.V3, models.V3) {
	// Step 1: Update position (in real Minecraft, this is via collision detection)
	pos = pos.Add(vel)

	// Step 2: Apply drag
	drag := m.Drag()
	if inWater {
		drag = m.WaterDrag()
	}
	vel = vel.Mul(drag)

	// Step 3: Apply gravity
	vel = m.ApplyVerticalForce(vel)

	return pos, vel
}

// ApplyVerticalForce applies gravity (standard gravity for persistent projectiles)
func (m *PersistentProjectileModel) ApplyVerticalForce(vel models.V3) models.V3 {
	vel.Y -= m.Gravity()
	return vel
}

// NewPersistentProjectileModel creates a new PersistentProjectileModel
func NewPersistentProjectileModel(pType models.ProjectileType, gravity, drag, waterDrag, initialSpeed float64) models.ProjectilePhysicsModel {
	return &PersistentProjectileModel{
		projectileType: pType,
		gravity:        gravity,
		drag:           drag,
		waterDrag:      waterDrag,
		initialSpeed:   initialSpeed,
	}
}

// Predefined models for persistent projectiles
var (
	// ArrowModel: gravity 0.05 (IMPORTANT: different from snowballs!), speed 3.0
	// Source: PersistentProjectileEntity.java:302 in Minecraft 1.21.8
	ArrowModel = NewPersistentProjectileModel(models.Arrow, 0.05, 0.99, 0.99, 3.0)

	// TridentModel: gravity 0.05, speed 3.0 (similar to arrows)
	TridentModel = NewPersistentProjectileModel(models.Trident, 0.05, 0.99, 0.99, 3.0)
)
