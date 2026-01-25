package models

// FollowManager manages follow behavior.
type FollowManager interface {
	Start(targetPlayerName string) error
	Stop() error
	IsActive() bool
	GetState() FollowState
	GetStatus() string
	GetPath() *Path
}
