package agent

import pk "github.com/Tnze/go-mc/net/packet"

// PacketBuilder is a minimal subset of the generated packet marshaller API.
type PacketBuilder interface {
	GetFields() map[string]pk.FieldEncoder
	SetFields(map[string]pk.FieldEncoder)
	Marshal() pk.Packet
	Scan(pk.Packet) error
}
