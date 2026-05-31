package models

// TrackedEntityInfo exposes entity tracking data for external use.
type TrackedEntityInfo struct {
	EntityID   int32
	EntityType int32
	UUID       [16]byte
	X, Y, Z    float64
	Yaw        int8
	Pitch      int8
	Health     float32 // Current health (0 = dead)
	MaxHealth  float32 // Maximum health (typically 20.0 for mobs)
	Removed    bool

	// Pose metadata from EntityPose wire value (e.g., standing, sitting, sleeping)
	// HasPose is false until the server sends a pose update
	Pose     int32  // Raw wire ordinal from protocol
	PoseName string // Lowercased Java enum name (e.g., "standing", "sitting")
	HasPose  bool   // True if at least one pose update has been received
}
