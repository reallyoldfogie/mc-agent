package agent

import (
	"context"
	"net"
	"testing"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	"github.com/reallyoldfogie/mc-agent/models"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/require"
)

// fakes for movement and pathfinding
type fakeMoveExec struct {
	posCalls       [][3]float64
	lookCalls      [][3]float64
	manualMode     bool
	manualThrottle [2]float64
	manualRotation [2]float64
	// For manual mode simulation
	currentPos [3]float64
	tickTimer  *time.Ticker
	stopChan   chan struct{}
	// Reference to agent for position updates in tests
	agent *agent
}

func (f *fakeMoveExec) SendPosition(x, y, z float64, onGround bool) error {
	f.posCalls = append(f.posCalls, [3]float64{x, y, z})
	f.currentPos = [3]float64{x, y, z}
	// Update agent position for manual mode testing
	if f.manualMode && f.agent != nil {
		f.agent.UpdatePosition(x, y, z, 0, 0)
	}
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
func (f *fakeMoveExec) SetTelemetryRecorder(recorder models.MovementTelemetryRecorder) {
	// no-op for fake
}

func (f *fakeMoveExec) SetVelocity(x, y, z float64) error {
	// no-op for fake
	return nil
}

// ManualMovementExecutor implementation for testing
func (f *fakeMoveExec) EnterManualMode() error {
	if f.manualMode {
		return nil // already in manual mode
	}
	f.manualMode = true
	f.stopChan = make(chan struct{})

	// Start a goroutine to simulate physics ticks
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Simulate movement based on current throttle
				// Walk speed: ~0.215 blocks/tick
				const speed = 0.215
				throttleX := f.manualThrottle[0] * speed
				throttleZ := f.manualThrottle[1] * speed

				// Update position (no gravity in fake mode, just horizontal movement)
				f.currentPos[0] += throttleX
				f.currentPos[2] += throttleZ

				// Report position to maintain ground contact
				_ = f.SendPosition(f.currentPos[0], f.currentPos[1], f.currentPos[2], true)

			case <-f.stopChan:
				return
			}
		}
	}()

	return nil
}

func (f *fakeMoveExec) ExitManualMode() error {
	if !f.manualMode {
		return nil
	}
	f.manualMode = false
	if f.stopChan != nil {
		close(f.stopChan)
		f.stopChan = nil
	}
	return nil
}

func (f *fakeMoveExec) SetManualThrottle(westEastThrottle, northSouthThrottle float64) error {
	f.manualThrottle = [2]float64{westEastThrottle, northSouthThrottle}
	return nil
}

func (f *fakeMoveExec) SetManualRotation(yaw, pitch float64) error {
	f.manualRotation = [2]float64{yaw, pitch}
	return nil
}

// SetMounted is a no-op for the fake executor
func (f *fakeMoveExec) SetMounted(vehicleEntityID int32) error {
	return nil
}

// SetDismounted is a no-op for the fake executor
func (f *fakeMoveExec) SetDismounted() error {
	return nil
}

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
func (f *fakeClientWriter) Events() bot.Events               { return nil }
func (f *fakeClientWriter) Name() string                     { return "BOT" }
func (f *fakeClientWriter) HandleGame(context.Context) error { return nil }
func (f *fakeClientWriter) WritePacket(p pk.Packet) error    { f.pkts = append(f.pkts, p); return nil }
func (f *fakeClientWriter) Close() error                     { return nil }
func (f *fakeClientWriter) Conn() *bot.Conn {
	// Return nil - getPacketWriter() will detect fakeClientWriter implements PacketWriter
	return nil
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
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(0, 0, 0, 0, 0)
	fm := &fakeMoveExec{agent: agent}
	agent.SetMovementExecutor(fm)
	agent.handleChatCommand("moveForward 0.1")
	time.Sleep(70 * time.Millisecond) // allow one step
	if len(fm.posCalls) == 0 {
		t.Fatalf("expected a SendPosition call")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	// With physics-based movement, expect forward movement (positive z) in the 0.1-0.3 range
	// (one tick of ~0.215 blocks/tick movement towards the target)
	if got[2] <= 0 || got[2] > 0.3 {
		t.Fatalf("expected forward movement z in (0, 0.3], got %#v", got)
	}
}

// Movement: moveTo with target in same block (floor(0.1)=0) should say "Already at target"
func TestCommand_MoveTo_SmallDelta(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.UpdatePosition(0, 0, 0, 0, 0)

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	// Need a fake pathfinder even though we won't use it
	agent.SetPathFinder(&fakePF{})
	// moveTo 0.1 0 0 from 0 0 0: floors to same block (0, 0, 0) -> (0, 0, 0)
	agent.handleChatCommand("moveTo 0.1 0 0")
	time.Sleep(20 * time.Millisecond)
	if !containsMsg(capture.GetMessages(), "Already at target position") {
		t.Fatalf("expected already-at-target message, got %v", capture.GetMessages())
	}
}

// Pathfinding: findPath calls FindPath with integerized coords
func TestCommand_FindPath(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
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
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "*********:25565"})
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
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	// Get real packet manager for version 1.21.5 to get correct packet IDs
	packetMgr := protocol_versions.GetPacketMgrForVersion("1.21.5")
	require.NotNil(t, packetMgr, "PacketMgr for 1.21.5 should not be nil")
	useItemID := int32(packetMgr.GetServerboundPacketID("ServerboundUseItem"))

	fc := newFakeClientWriter()
	agent.client = fc
	agent.handleChatCommand("fireBow")
	time.Sleep(20 * time.Millisecond)
	if len(fc.pkts) == 0 {
		t.Fatalf("expected at least one packet write")
	}
	if fc.pkts[0].ID != useItemID {
		t.Fatalf("expected first packet ID %d (UseItem), got %d", useItemID, fc.pkts[0].ID)
	}
}

func TestCommand_FireBow_ShootAfterHold(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	// Get real packet manager for version 1.21.5 to get correct packet IDs
	packetMgr := protocol_versions.GetPacketMgrForVersion("1.21.5")
	require.NotNil(t, packetMgr, "PacketMgr for 1.21.5 should not be nil")
	playerActionID := int32(packetMgr.GetServerboundPacketID("ServerboundPlayerAction"))

	fc := newFakeClientWriter()
	agent.client = fc
	bowHoldIterations = 0
	bowHoldSleep = 1 * time.Millisecond
	agent.handleChatCommand("fireBow")
	time.Sleep(50 * time.Millisecond)
	if len(fc.pkts) < 2 {
		t.Fatalf("expected multiple packets (use + shoot), got %d", len(fc.pkts))
	}
	foundPlayerAction := false
	for _, p := range fc.pkts {
		if p.ID == playerActionID {
			foundPlayerAction = true
			break
		}
	}
	if !foundPlayerAction {
		t.Fatalf("expected shoot action packet (ID %d) among writes", playerActionID)
	}
}
