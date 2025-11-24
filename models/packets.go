//go:build none

package models

type (
	ClientboundPacketID int32
	ServerboundPacketID int32
)

type PacketParse struct {
	Configuration PacketMap `json:"configuration,omitempty"`
	Handshake     PacketMap `json:"handshake,omitempty"`
	Login         PacketMap `json:"login,omitempty"`
	Play          PacketMap `json:"play,omitempty"`
	Status        PacketMap `json:"status,omitempty"`
}

type PacketMap struct {
	Clientbound map[string]ProtocolID `json:"clientbound,omitempty"`
	Serverbound map[string]ProtocolID `json:"serverbound,omitempty"`
}
