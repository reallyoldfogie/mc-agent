package agent

import (
	"context"
	"math"
	"testing"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// fakes for movement and pathfinding
type fakeMoveExec struct {
	posCalls  [][3]float64
	lookCalls [][3]float64
}

func (f *fakeMoveExec) SendPosition(x, y, z float64, onGround bool) error {
	f.posCalls = append(f.posCalls, [3]float64{x, y, z})
	return nil
}
func (f *fakeMoveExec) SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error {
	return f.SendPosition(x, y, z, onGround)
}
func (f *fakeMoveExec) SendRotation(yaw, pitch float32, onGround bool) error { return nil }
func (f *fakeMoveExec) MoveTowards(tx, ty, tz float64, d float64, og bool) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (f *fakeMoveExec) LookAt(x, y, z float64, onGround bool) error {
	f.lookCalls = append(f.lookCalls, [3]float64{x, y, z})
	return nil
}

type fakePF struct {
	start, pathGoal pathfinding.V3
	called          bool
}

func (f *fakePF) FindPath(s, g pathfinding.V3, _ int) (*pathfinding.Path, error) {
	f.start, f.pathGoal, f.called = s, g, true
	return &pathfinding.Path{}, nil
}

type fakeSBPacketMgr struct {
	srv map[string]protocol_models.ServerboundPacketID
}

func (f fakeSBPacketMgr) GetClientboundPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (f fakeSBPacketMgr) GetClientboundConfigPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (f fakeSBPacketMgr) GetClientboundLoginPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (f fakeSBPacketMgr) Name() string            { return "test" }
func (f fakeSBPacketMgr) VersionProtocol() uint64 { return 0 }
func (f fakeSBPacketMgr) ClientboundToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (f fakeSBPacketMgr) ClientboundConfigToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (f fakeSBPacketMgr) ClientboundLoginToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (f fakeSBPacketMgr) GetServerboundPacketID(name string) protocol_models.ServerboundPacketID {
	if id, ok := f.srv[name]; ok {
		return id
	}
	return 0
}
func (f fakeSBPacketMgr) GetServerboundConfigPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (f fakeSBPacketMgr) GetServerboundLoginPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (f fakeSBPacketMgr) ServerboundToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (f fakeSBPacketMgr) ServerboundConfigToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (f fakeSBPacketMgr) GetClientboundPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetClientboundConfigPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetClientboundLoginPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetClientboundHandshakingPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetClientboundStatusPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetServerboundPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetServerboundConfigPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetServerboundLoginPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetServerboundHandshakingPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetServerboundStatusPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeSBPacketMgr) GetEntityTypeID(name string) int32 {
	return 0
}

type fakeClientWriter struct{ pkts []pk.Packet }

func (f *fakeClientWriter) JoinServerWithOptions(context.Context, string, JoinOptions) error {
	return nil
}
func (f *fakeClientWriter) Events() EventBus                 { return nil }
func (f *fakeClientWriter) Name() string                     { return "BOT" }
func (f *fakeClientWriter) HandleGame(context.Context) error { return nil }
func (f *fakeClientWriter) WritePacket(p pk.Packet) error    { f.pkts = append(f.pkts, p); return nil }

// Movement: moveForward 0.1 should send one position packet forward (yaw=0 => +Z)
func TestCommand_MoveForward_SmallStep(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	a.UpdatePosition(0, 0, 0, 0, 0)
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.handleChatCommand("moveForward 0.1")
	time.Sleep(70 * time.Millisecond) // allow one step
	if len(fm.posCalls) == 0 {
		t.Fatalf("expected a SendPosition call")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	if math.Abs(got[2]-0.1) > 1e-6 {
		t.Fatalf("expected z≈0.1, got %#v", got)
	}
}

// Movement: moveTo small delta should LookAt and SendPosition once
func TestCommand_MoveTo_SmallDelta(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	a.UpdatePosition(0, 0, 0, 0, 0)
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.handleChatCommand("moveTo 0.1 0 0")
	time.Sleep(70 * time.Millisecond)
	if len(fm.lookCalls) == 0 {
		t.Fatalf("expected LookAt call")
	}
	if len(fm.posCalls) == 0 {
		t.Fatalf("expected SendPosition call")
	}
}

// Pathfinding: findPath calls FindPath with integerized coords
func TestCommand_FindPath(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	a.UpdatePosition(0, 0, 0, 0, 0)
	pf := &fakePF{}
	a.SetPathFinder(pf)
	a.handleChatCommand("findPath 1 0 0")
	time.Sleep(10 * time.Millisecond)
	if !pf.called {
		t.Fatalf("expected FindPath call")
	}
	if pf.pathGoal.X != 1 || pf.pathGoal.Y != 0 || pf.pathGoal.Z != 0 {
		t.Fatalf("unexpected goal: %#v", pf.pathGoal)
	}
}

// Tracking: startTracking should invoke LookAt on nearest at least once, and stopTracking should stop it
func TestCommand_StartStopTracking(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	a.UpdatePosition(0, 0, 0, 0, 0)
	// seed entities
	a.entities = map[int32]*trackedEntity{
		1: {EntityID: 1, X: 0, Y: 0, Z: 1}, // near
		2: {EntityID: 2, X: 0, Y: 0, Z: 10},
	}
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.handleChatCommand("startTracking")
	time.Sleep(250 * time.Millisecond)
	if len(fm.lookCalls) == 0 {
		t.Fatalf("expected at least one LookAt call")
	}
	a.handleChatCommand("stopTracking")
	n := len(fm.lookCalls)
	time.Sleep(250 * time.Millisecond)
	if len(fm.lookCalls) > n {
		t.Fatalf("expected LookAt calls to stop after stopTracking")
	}
}

// FireBow: immediately sends a UseItem packet
func TestCommand_FireBow_UseItemFirst(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	a.packetMgr = fakeSBPacketMgr{srv: map[string]protocol_models.ServerboundPacketID{"ServerboundUseItem": 123, "ServerboundPlayerAction": 456}}
	fc := &fakeClientWriter{}
	a.client = fc
	a.handleChatCommand("fireBow")
	time.Sleep(20 * time.Millisecond)
	if len(fc.pkts) == 0 {
		t.Fatalf("expected at least one packet write")
	}
	if int(fc.pkts[0].ID) != 123 {
		t.Fatalf("expected first packet ID 123, got %d", fc.pkts[0].ID)
	}
}

func TestCommand_FireBow_ShootAfterHold(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	a.packetMgr = fakeSBPacketMgr{srv: map[string]protocol_models.ServerboundPacketID{"ServerboundUseItem": 123, "ServerboundPlayerAction": 456}}
	fc := &fakeClientWriter{}
	a.client = fc
	// shorten
	bowHoldIterations = 0
	bowHoldSleep = 1 * time.Millisecond
	a.handleChatCommand("fireBow")
	time.Sleep(20 * time.Millisecond)
	if len(fc.pkts) < 2 {
		t.Fatalf("expected multiple packets (use + shoot), got %d", len(fc.pkts))
	}
	foundShoot := false
	for _, p := range fc.pkts {
		if int(p.ID) == 456 {
			foundShoot = true
			break
		}
	}
	if !foundShoot {
		t.Fatalf("expected shoot action packet (456) among writes")
	}
}
