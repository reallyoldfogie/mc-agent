package models

// ContainerHelper provides high-level container interaction methods.
type ContainerHelper interface {
	SetEntityIDProvider(provider EntityIDProvider)
	ContainerAccess
	GetContainerSlotCount(windowID byte) int
}
