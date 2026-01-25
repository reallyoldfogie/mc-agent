package models

import pk "github.com/Tnze/go-mc/net/packet"

// PacketSender interface for sending packets (implemented by client).
type PacketSender interface {
	WritePacket(packet pk.Packet) error
}
