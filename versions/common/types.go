package common

import "fmt"

// ErrUnsupportedVersion is returned when a version is not supported.
type ErrUnsupportedVersion struct {
	Version string
}

func (e ErrUnsupportedVersion) Error() string {
	return fmt.Sprintf("unsupported Minecraft version: %s", e.Version)
}

// ErrPacketParse is returned when a packet cannot be parsed.
type ErrPacketParse struct {
	PacketName string
	Cause      error
}

func (e ErrPacketParse) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("failed to parse %s packet: %v", e.PacketName, e.Cause)
	}
	return fmt.Sprintf("failed to parse %s packet", e.PacketName)
}

func (e ErrPacketParse) Unwrap() error {
	return e.Cause
}

// ErrPacketSend is returned when a packet cannot be sent.
type ErrPacketSend struct {
	PacketName string
	Cause      error
}

func (e ErrPacketSend) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("failed to send %s packet: %v", e.PacketName, e.Cause)
	}
	return fmt.Sprintf("failed to send %s packet", e.PacketName)
}

func (e ErrPacketSend) Unwrap() error {
	return e.Cause
}

// Player command action IDs (common across versions)
const (
	ActionStartSneaking     = 0
	ActionStopSneaking      = 1
	ActionLeaveBed          = 2
	ActionStartSprinting    = 3
	ActionStopSprinting     = 4
	ActionStartJumpHorse    = 5
	ActionStopJumpHorse     = 6
	ActionOpenVehicleInv    = 7
	ActionStartFlyingElytra = 8
)
