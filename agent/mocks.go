package agent

import (
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
