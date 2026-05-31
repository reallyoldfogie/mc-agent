package models

// PhysicsState is an interface for physics state needed by input generators and movement executors.
type PhysicsState interface {
	Position

	Position() V3
	Velocity() V3
	Yaw() float64
	Pitch() float64
	OnGround() bool
	IsSneaking() bool
	IsSwimming() bool
	IsInWater() bool
	FallDistance() float64
	GetDimensions() (width, height, eyeHeight float64)
	GetAABB() AABB
	SetPosition(pos V3, yaw, pitch float64, onGround bool)
	SetPositionSimple(pos V3)
	SetYaw(yaw float64)
	SetPitch(pitch float64)
	SetVelocity(vel V3)
	SetOnGround(onGround bool)
	SetSneaking(sneaking bool)
	SetFallDistance(distance float64)
	GetVelocity() V3
	Tick(input Inputs, w PhysicsWorld) error
	PredictMovement(inputs []Inputs, maxTicks int, w PhysicsWorld) []PhysicsState
	PredictPosition(vel V3, ticks int, w PhysicsWorld) V3
	WillCollide(targetPos V3, w PhysicsWorld) bool
	HasGroundSupportAt(pos V3, w PhysicsWorld) bool
	IsLookingAtTarget(targetYaw, targetPitch float64) bool
	GetSurroundingBoxes(queryBB AABB, w PhysicsWorld) []AABB
}
