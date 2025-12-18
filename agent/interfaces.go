package agent

import (
	"context"

	pk "github.com/Tnze/go-mc/net/packet"
)

// Narrow interfaces to decouple from concrete implementations and enable fakes in tests.

// Client encapsulates the network client and event bus.
type Client interface {
	JoinServerWithOptions(ctx context.Context, address string, opts JoinOptions) error
	Events() EventBus
	Name() string
	HandleGame(ctx context.Context) error
	WritePacket(p pk.Packet) error
}

// JoinOptions mirrors the options needed by the client to connect.
type JoinOptions struct {
	ProtocolVersion      uint
	ReplayRecorder       PacketRecorder
	MovementMirror       MovementMirror
	SkinProvider         SkinProvider
	RegistryDataCallback func(registryID string, entries map[string]int32)
}

// PacketRecorder is a minimal recorder interface used for replay capture.
type PacketRecorder interface {
	RecordNow(id int32, payload []byte) error
	SetSelfID(id int)
	AddPlayer(uuid string)
}

// SkinProvider returns skin/texture properties for a given player UUID/name.
type SkinProvider interface {
	Get(uuid [16]byte, name string) []profileProperty
}

// MovementMirror consumes serverbound packets and may synthesize clientbound
// packets (e.g., to mirror the bot's own movement into a replay).
type MovementMirror interface {
	HandleServerbound(pk.Packet)
	SetEntityMeta(entityID int32, name string, uuid [16]byte)
	SetEntityType(entityType int32)
	HandlePlayerInfo(pk.Packet)
	NotifyLoginSeen() // signals that LOGIN packet has been recorded
}

// EventBus registers and dispatches packet handlers.
type EventBus interface {
	AddGeneric(listeners ...PacketHandler)
	AddListener(listeners ...PacketHandler)
}

// PacketHandler represents a packet listener.
type PacketHandler struct {
	ID       int32
	Priority int
	F        func(pk.Packet) error
}

// Auth mirrors the authentication details required by the underlying client.
type Auth struct {
	AsTk string
	Name string
	UUID string
}

// Optional: accept teleports (provided by player subsystem when available)
type TeleportAccepter interface {
	AcceptTeleportation(id pk.VarInt) error
}

// Respawner triggers a player respawn.
type Respawner interface {
	Respawn() error
}

// Chat provides a way to send messages to the server.
type Chat interface {
	SendMessage(string) error
}

// ItemManager provides item name lookups by ID.
type ItemManager interface {
	GetItemNameByID(id int) string
}

// SlotResolver resolves a slot into an item ID and count.
type SlotResolver interface {
	ResolveSlot(id, index int) (itemID int, count int, ok bool)
}
