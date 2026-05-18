package agent

import (
	"bytes"
	"context"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"

	"github.com/stretchr/testify/require"
)

// helper to append field bytes to a buffer
func appendField(buf *bytes.Buffer, f pk.FieldEncoder) {
	_, _ = f.WriteTo(buf)
}

func TestOnClientboundPosition_Absolute(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err, "agent.Init failed")

	// Inject mock player after Init to override the auto-created player
	mockConn := &MockConn{}
	mockPlayer := NewMockPlayer(mockConn)
	agent.player = mockPlayer

	var (
		TeleportID pk.VarInt              = 5
		X          pk.Double              = 1
		Y          pk.Double              = 2
		Z          pk.Double              = 3
		DX         pk.Double              = 0
		DY         pk.Double              = 0
		DZ         pk.Double              = 0
		Yaw        pk.Float               = 10
		Pitch      pk.Float               = 20
		Flags      protocol_models.UInt32 = 0
	)

	// Manually construct a Position packet by marshalling the fields
	var buf bytes.Buffer
	appendField(&buf, TeleportID)
	appendField(&buf, X)
	appendField(&buf, Y)
	appendField(&buf, Z)
	appendField(&buf, DX)
	appendField(&buf, DY)
	appendField(&buf, DZ)
	appendField(&buf, Yaw)
	appendField(&buf, Pitch)
	appendField(&buf, Flags)
	p := pk.Packet{ID: int32(agent.packetMgr.GetClientboundPacketID("ClientboundPosition")), Data: buf.Bytes()}

	if err := agent.onClientboundPosition(p); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	x, y, z, yaw, pitch, ok := agent.GetPosition()
	if !ok {
		t.Fatalf("position not initialized")
	}
	if x != 1 || y != 2 || z != 3 {
		t.Fatalf("unexpected pos: %v %v %v", x, y, z)
	}
	if yaw != 10 || pitch != 20 {
		t.Fatalf("unexpected rot: %v %v", yaw, pitch)
	}
}

func TestOnRegistryData(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())

	var (
		id    pk.String  = "minecraft:entity_type"
		count pk.VarInt  = 2
		e0    pk.String  = "minecraft:foo"
		e1    pk.String  = "minecraft:bar"
		has0  pk.Boolean = false
		has1  pk.Boolean = false
	)

	var buf bytes.Buffer
	appendField(&buf, id)
	appendField(&buf, count)
	appendField(&buf, e0)
	appendField(&buf, has0)
	appendField(&buf, e1)
	appendField(&buf, has1)
	p := pk.Packet{ID: int32(agent.packetMgr.GetClientboundPacketID("ClientboundConfigRegistryData")), Data: buf.Bytes()}

	if err := agent.onRegistryData(p); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	reg := agent.GetRegistry("minecraft:entity_type")
	if reg == nil || !reg.IsReady() {
		t.Fatalf("registry not ready")
	}
	zeroName, ok := reg.GetNameByID(0)
	if !ok || zeroName != "minecraft:foo" {
		t.Fatalf("unexpected id 0 name: %v %t", zeroName, ok)
	}
	oneName, ok := reg.GetNameByID(1)
	if !ok || oneName != "minecraft:bar" {
		t.Fatalf("unexpected id 1 name: %v %t", oneName, ok)
	}
}
