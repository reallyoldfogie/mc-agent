package models

import "sync"

// HappyGhastState tracks the client-predicted vehicle yaw for a ridden happy
// ghast — the only per-tick state its movement needs beyond what
// PhysicsMovementExecutor's ridingVelX/Y/Z already carries. Mirrors
// NautilusState's role but is much smaller: the happy ghast has no dash or
// charge, and its pitch is derived fresh each tick (half the pilot's current
// pitch) rather than stored.
//
// Field access is synchronized with mu so the physics tick loop can write
// while tests read.
type HappyGhastState struct {
	mu sync.RWMutex

	// VehicleYaw is the ghast's own yaw in degrees. Java tickControlled eases
	// it toward the pilot's look yaw by a small fraction (0.08) each tick —
	// much slower than a nautilus's 0.5.
	VehicleYaw float64
}

// NewHappyGhastState creates a HappyGhastState seeded with the mount's
// current yaw.
func NewHappyGhastState(initialYaw float64) *HappyGhastState {
	return &HappyGhastState{VehicleYaw: initialYaw}
}

// GetVehicleYaw returns the ghast's current (eased) yaw in degrees.
func (s *HappyGhastState) GetVehicleYaw() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.VehicleYaw
}

// EaseYawToward eases the vehicle yaw toward the pilot's look yaw and
// returns the updated value. Mirrors Java HappyGhastEntity.tickControlled:
//
//	f += MathHelper.wrapDegrees(pilotYaw - f) * 0.08F
func (s *HappyGhastState) EaseYawToward(riderYaw float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	delta := wrapDegrees(riderYaw - s.VehicleYaw)
	s.VehicleYaw += delta * 0.08
	return s.VehicleYaw
}
