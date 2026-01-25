package models

import pk "github.com/Tnze/go-mc/net/packet"

// TeleportAccepter accepts teleports (provided by player subsystem when available).
type TeleportAccepter interface {
	AcceptTeleportation(id pk.VarInt) error
}
