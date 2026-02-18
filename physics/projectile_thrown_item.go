package physics

import "github.com/reallyoldfogie/mc-agent/models"

// ThrownItemModel implements ProjectilePhysicsModel for ThrownEntity variants
// These are projectiles thrown by players: snowballs, eggs, enderpearls, potions, experience bottles
//
// Physics order: gravity → drag → position
// This matches ThrownEntity.java:48-67 from Minecraft 1.21.8
type ThrownItemModel struct {
	projectileType models.ProjectileType
	gravity        float64
	drag           float64
	waterDrag      float64
	initialSpeed   float64
}

// Type returns the projectile type
func (m *ThrownItemModel) Type() models.ProjectileType {
	return m.projectileType
}

// Gravity returns the gravitational acceleration
func (m *ThrownItemModel) Gravity() float64 {
	return m.gravity
}

// Drag returns the air drag coefficient
func (m *ThrownItemModel) Drag() float64 {
	return m.drag
}

// WaterDrag returns the water drag coefficient
func (m *ThrownItemModel) WaterDrag() float64 {
	return m.waterDrag
}

// InitialSpeed returns the default throw speed
func (m *ThrownItemModel) InitialSpeed() float64 {
	return m.initialSpeed
}

// UpdateOrder returns the physics update order for ThrownEntity
func (m *ThrownItemModel) UpdateOrder() models.UpdateOrder {
	return models.OrderGravityDragPosition // gravity → drag → position
}

// ApplyPhysics applies one tick of physics in the correct order
func (m *ThrownItemModel) ApplyPhysics(pos, vel models.V3, inWater bool) (models.V3, models.V3) {
	// Step 1: Apply gravity
	vel = m.ApplyVerticalForce(vel)

	// Step 2: Apply drag
	drag := m.Drag()
	if inWater {
		drag = m.WaterDrag()
	}
	vel = vel.Mul(drag)

	// Step 3: Update position
	pos = pos.Add(vel)

	return pos, vel
}

// ApplyVerticalForce applies gravity (standard gravity for thrown items)
func (m *ThrownItemModel) ApplyVerticalForce(vel models.V3) models.V3 {
	vel.Y -= m.Gravity()
	return vel
}

// NewThrownItemModel creates a new ThrownItemModel with the specified parameters
func NewThrownItemModel(pType models.ProjectileType, gravity, drag, waterDrag, initialSpeed float64) models.ProjectilePhysicsModel {
	return &ThrownItemModel{
		projectileType: pType,
		gravity:        gravity,
		drag:           drag,
		waterDrag:      waterDrag,
		initialSpeed:   initialSpeed,
	}
}

// Predefined models for each thrown item type
var (
	// SnowballModel: gravity 0.03, speed 1.5
	SnowballModel = NewThrownItemModel(models.Snowball, 0.03, 0.99, 0.8, 1.5)

	// EggModel: gravity 0.03, speed 1.5
	EggModel = NewThrownItemModel(models.Egg, 0.03, 0.99, 0.8, 1.5)

	// EnderPearlModel: gravity 0.03, speed 1.5
	EnderPearlModel = NewThrownItemModel(models.EnderPearl, 0.03, 0.99, 0.8, 1.5)

	// SplashPotionModel: gravity 0.05, speed 0.5 (slower arc)
	SplashPotionModel = NewThrownItemModel(models.SplashPotion, 0.05, 0.99, 0.8, 0.5)

	// ExperienceBottleModel: gravity 0.07 (heavier), speed 0.7
	ExperienceBottleModel = NewThrownItemModel(models.ExperienceBottle, 0.07, 0.99, 0.8, 0.7)
)
