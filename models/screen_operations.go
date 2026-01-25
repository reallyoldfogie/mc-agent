package models

// ScreenOperations provides screen/inventory management functionality.
type ScreenOperations interface {
	ScreenAccess
	GetScreenManager() ScreenSubsystem
}
