package models

import pk "github.com/Tnze/go-mc/net/packet"

// MovementMirror consumes serverbound packets and may synthesize clientbound packets.
type MovementMirror interface {
	HandleServerbound(pk.Packet)
	SetEntityMeta(entityID int32, name string, uuid [16]byte)
	SetEntityType(entityType int32)
	HandlePlayerInfo(pk.Packet)
	NotifyLoginSeen()
}
