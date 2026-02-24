package models

// ContainerOperations provides high-level container interaction methods.
type ContainerOperations interface {
	SetContainerHelper(ch ContainerHelper)
	GetContainerHelper() ContainerHelper
	ContainerAccess
}
