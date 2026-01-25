package models

// PacketManager interface for getting packet IDs.
type PacketManager interface {
	GetServerboundPacketID(name string) int32
}
