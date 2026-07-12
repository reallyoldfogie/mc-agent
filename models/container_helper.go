package models

// ContainerManager provides high-level container interaction methods.
type ContainerManager interface {
	SetEntityIDProvider(provider EntityIDProvider)
	ContainerAccess
	GetContainerSlotCount(windowID byte) int
}
