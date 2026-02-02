package agent

import (
	"context"
	"math"
	"net"
	"testing"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	"github.com/reallyoldfogie/mc-agent/models"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/require"
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
func (f *fakeMoveExec) StartSprinting() error { return nil }
func (f *fakeMoveExec) StopSprinting() error  { return nil }
func (f *fakeMoveExec) IsSprinting() bool     { return false }
func (f *fakeMoveExec) StartSneaking() error  { return nil }
func (f *fakeMoveExec) StopSneaking() error   { return nil }
func (f *fakeMoveExec) IsSneaking() bool      { return false }

type fakePF struct {
	start, pathGoal models.V3
	called          bool
}

func (f *fakePF) FindPath(ctx context.Context, s, g models.V3, _ int) (*models.Path, error) {
	f.start, f.pathGoal, f.called = s, g, true
	return &models.Path{}, nil
}
func (f *fakePF) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return startY
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

// fakeConn is a minimal fake that implements WritePacket
type fakeConn struct {
	pkts *[]pk.Packet
}

func (f *fakeConn) WritePacket(p pk.Packet) error {
	if f.pkts != nil {
		*f.pkts = append(*f.pkts, p)
	}
	return nil
}

type fakeClientWriter struct {
	pkts     []pk.Packet
	fakeConn *fakeConn
}

func newFakeClientWriter() *fakeClientWriter {
	f := &fakeClientWriter{}
	f.fakeConn = &fakeConn{pkts: &f.pkts}
	return f
}

func (f *fakeClientWriter) JoinServerWithOptions(context.Context, string, bot.JoinOptions) error {
	return nil
}
func (f *fakeClientWriter) Events() bot.Events                       { return nil }
func (f *fakeClientWriter) Name() string                             { return "BOT" }
func (f *fakeClientWriter) HandleGame(context.Context) error         { return nil }
func (f *fakeClientWriter) WritePacket(p pk.Packet) error            { f.pkts = append(f.pkts, p); return nil }
func (f *fakeClientWriter) Close() error                             { return nil }
func (f *fakeClientWriter) Conn() *bot.Conn {
	// We can't properly fake bot.Conn as it's a concrete struct with many private fields.
	// However, Go will allow us to call WritePacket on the returned value via duck typing
	// if we return an interface{} cast to *bot.Conn.
	// This is hacky but works for testing purposes.
	var conn interface{} = f.fakeConn
	return (*bot.Conn)(conn.(*fakeConn))
}
func (f *fakeClientWriter) SetAuth(bot.Auth)                         {}
func (f *fakeClientWriter) JoinServer(context.Context, string) error { return nil }
func (f *fakeClientWriter) JoinServerWithDialer(context.Context, *net.Dialer, string) error {
	return nil
}
func (f *fakeClientWriter) UUID() uuid.UUID                                  { return uuid.UUID{} }
func (f *fakeClientWriter) Cookies() map[string][]byte                       { return nil }
func (f *fakeClientWriter) SetCookies(map[string][]byte)                     {}
func (f *fakeClientWriter) RegistryData() map[string]*bot.CustomRegistry     { return nil }
func (f *fakeClientWriter) RegistryTags() map[string]*bot.RegistryTags       { return nil }
func (f *fakeClientWriter) LoginPlugin() map[string]bot.CustomPayloadHandler { return nil }
func (f *fakeClientWriter) CustomReportDetails() map[string]string           { return nil }
func (f *fakeClientWriter) SetJoinLogin(func(*mcnet.Conn) error)             {}
func (f *fakeClientWriter) SetJoinConfiguration(func(*mcnet.Conn) error)     {}
func (f *fakeClientWriter) MovementMirror() bot.MovementMirror               { return nil }
func (f *fakeClientWriter) RegistryCallback() bot.RegistryDataCallback       { return nil }
func (f *fakeClientWriter) PacketMgr() protocol_models.PacketMgr             { return nil }
func (f *fakeClientWriter) EnableFeature([]pk.Identifier)                    {}
func (f *fakeClientWriter) PushResourcePack(bot.ResourcePack)                {}
func (f *fakeClientWriter) PopResourcePack(pk.UUID)                          {}
func (f *fakeClientWriter) PopAllResourcePack()                              {}
func (f *fakeClientWriter) SelectDataPacks([]bot.DataPack) []bot.DataPack    { return nil }
func (f *fakeClientWriter) SetVersionHandler(bot.VersionHandler)             {}

// Movement: moveForward 0.1 should send one position packet forward (yaw=0 => +Z)
func TestCommand_MoveForward_SmallStep(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(0, 0, 0, 0, 0)
	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	agent.handleChatCommand("moveForward 0.1")
	time.Sleep(70 * time.Millisecond) // allow one step
	if len(fm.posCalls) == 0 {
		t.Fatalf("expected a SendPosition call")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	if math.Abs(got[2]-0.1) > 1e-6 {
		t.Fatalf("expected z≈0.1, got %#v", got)
	}
}

// Movement: moveTo with target in same block (floor(0.1)=0) should say "Already at target"
func TestCommand_MoveTo_SmallDelta(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "*********:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(0, 0, 0, 0, 0)
	fc := &fakeChat{}
	agent.SetChat(fc)
	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	// Need a fake pathfinder even though we won't use it
	agent.SetPathFinder(&fakePF{})
	// moveTo 0.1 0 0 from 0 0 0: floors to same block (0, 0, 0) -> (0, 0, 0)
	agent.handleChatCommand("moveTo 0.1 0 0")
	time.Sleep(20 * time.Millisecond)
	if !containsMsg(fc.msgs, "Already at target position") {
		t.Fatalf("expected already-at-target message, got %v", fc.msgs)
	}
}

// Pathfinding: findPath calls FindPath with integerized coords
func TestCommand_FindPath(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(0, 0, 0, 0, 0)
	pf := &fakePF{}
	agent.SetPathFinder(pf)
	agent.handleChatCommand("findPath 1 0 0")
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
	agentInt, err := New(Config{Version: "1.21.5", Address: "*********:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(0, 0, 0, 0, 0)
	// seed entities
	agent.entities = map[int32]*trackedEntity{
		1: {EntityID: 1, X: 0, Y: 0, Z: 1}, // near
		2: {EntityID: 2, X: 0, Y: 0, Z: 10},
	}
	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	agent.handleChatCommand("startTracking")
	time.Sleep(250 * time.Millisecond)
	if len(fm.lookCalls) == 0 {
		t.Fatalf("expected at least one LookAt call")
	}
	agent.handleChatCommand("stopTracking")
	n := len(fm.lookCalls)
	// Allow for one more tick that may have been in flight
	time.Sleep(250 * time.Millisecond)
	if len(fm.lookCalls) > n+1 {
		t.Fatalf("expected LookAt calls to stop after stopTracking, had %d, now have %d", n, len(fm.lookCalls))
	}
}

// FireBow: immediately sends a UseItem packet
func TestCommand_FireBow_UseItemFirst(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.packetMgr = fakeSBPacketMgr{srv: map[string]protocol_models.ServerboundPacketID{"ServerboundUseItem": 123, "ServerboundPlayerAction": 456}}
	fc := newFakeClientWriter()
	agent.client = fc
	// shorten
	agent.handleChatCommand("fireBow")
	time.Sleep(20 * time.Millisecond)
	if len(fc.pkts) == 0 {
		t.Fatalf("expected at least one packet write")
	}
	if int(fc.pkts[0].ID) != 123 {
		t.Fatalf("expected first packet ID 123, got %d", fc.pkts[0].ID)
	}
}

func TestCommand_FireBow_ShootAfterHold(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.packetMgr = fakeSBPacketMgr{srv: map[string]protocol_models.ServerboundPacketID{"ServerboundUseItem": 123, "ServerboundPlayerAction": 456}}
	fc := &fakeClientWriter{}
	agent.client = fc
	// shorten
	bowHoldIterations = 0
	bowHoldSleep = 1 * time.Millisecond
	agent.handleChatCommand("fireBow")
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
