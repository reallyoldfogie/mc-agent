package models

type Position interface {
	// Position update (for movement executor wiring)
	UpdatePosition(pos V3, yaw, pitch float64)
	GetPosition() (pos V3, yaw, pitch float64, initialized bool)
	GetPositionSimple() (pos V3, initialized bool)
}
