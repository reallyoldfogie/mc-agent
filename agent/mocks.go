package agent

import (
	"sync"

	pk "github.com/Tnze/go-mc/net/packet"
)

// MockConn is a mock implementation of bot.Conn for testing
type MockConn struct {
	packets []pk.Packet
	err     error
}

func (m *MockConn) WritePacket(packet pk.Packet) error {
	if m.err != nil {
		return m.err
	}
	m.packets = append(m.packets, packet)
	return nil
}

func (m *MockConn) ReadPacket(packetIDLen int) (packet pk.Packet, err error) {
	return pk.Packet{}, nil
}

func (m *MockConn) GetPackets() []pk.Packet {
	return m.packets
}

func (m *MockConn) Clear() {
	m.packets = nil
}

// MockPlayer is a mock implementation of a player for testing
type MockPlayer struct {
	teleportConfirmed bool
	conn              *MockConn
	packetMgr         interface{} // Minimal packet manager mock

	// respawnMu guards respawnCalls - Respawn() runs from a background
	// goroutine in real usage (onDeath/onGameEvent's win-game branch both
	// call it after a time.Sleep), so a test reading RespawnCalls()
	// concurrently needs this synchronized.
	respawnMu    sync.Mutex
	respawnCalls int
}

func NewMockPlayer(conn *MockConn) *MockPlayer {
	return &MockPlayer{
		conn: conn,
	}
}

func (m *MockPlayer) AcceptTeleportation(teleportID pk.VarInt) error {
	if m.conn == nil {
		return nil
	}
	m.teleportConfirmed = true
	return m.conn.WritePacket(pk.Marshal(int32(0), teleportID))
}

func (m *MockPlayer) WasTeleportConfirmed() bool {
	return m.teleportConfirmed
}

// Respawn implements models.Respawner, so a *MockPlayer assigned to
// agent.player satisfies the type assertion getRespawner() makes.
func (m *MockPlayer) Respawn() error {
	m.respawnMu.Lock()
	m.respawnCalls++
	m.respawnMu.Unlock()
	return nil
}

// RespawnCalls returns how many times Respawn has been called so far.
func (m *MockPlayer) RespawnCalls() int {
	m.respawnMu.Lock()
	defer m.respawnMu.Unlock()
	return m.respawnCalls
}

// MockClient wraps client functionality for testing
type MockClient struct {
	conn   *MockConn
	player *MockPlayer
}

func NewMockClient(conn *MockConn) *MockClient {
	return &MockClient{
		conn:   conn,
		player: NewMockPlayer(conn),
	}
}

func (m *MockClient) Conn() *MockConn {
	return m.conn
}

func (m *MockClient) Player() *MockPlayer {
	return m.player
}
