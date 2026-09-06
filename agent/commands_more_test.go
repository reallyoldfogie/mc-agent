package agent

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	"github.com/reallyoldfogie/mc-agent/models"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakes for movement and pathfinding
//
// mu guards every field below: EnterManualMode's background ticker
// goroutine reads/writes manualThrottle/currentPos/manualMode concurrently
// with the test goroutine's SetManualThrottle/ExitManualMode/PosCalls
// calls - found live via `go test -race` (WARNING: DATA RACE between
// ExitManualMode and the ticker goroutine's reads), not hypothetical.
type fakeMoveExec struct {
	mu             sync.Mutex
	posCalls       [][3]float64
	lookCalls      [][3]float64
	manualMode     bool
	manualThrottle [2]float64
	manualRotation [2]float64
	manualJump     bool
	// For manual mode simulation
	currentPos [3]float64
	tickTimer  *time.Ticker
	stopChan   chan struct{}
	// Reference to agent for position updates in tests
	agent *agent
}

// PosCalls returns a snapshot of every position recorded via SendPosition
// so far. Safe to call concurrently with the manual-mode ticker goroutine
// (see EnterManualMode) - unlike reading the posCalls field directly, which
// is exactly the pattern that used to race.
func (f *fakeMoveExec) PosCalls() [][3]float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][3]float64, len(f.posCalls))
	copy(out, f.posCalls)
	return out
}

// LookCalls returns a snapshot of every target recorded via LookAt so far -
// see PosCalls' doc comment.
func (f *fakeMoveExec) LookCalls() [][3]float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][3]float64, len(f.lookCalls))
	copy(out, f.lookCalls)
	return out
}

// Compile-time assertions that the fake satisfies the executor interfaces the
// agent commands require. Without these, a missing method silently downgrades
// the fake to a plain MovementExecutor and manual-mode commands fail at runtime.
var (
	_ models.MovementExecutor       = (*fakeMoveExec)(nil)
	_ models.ManualMovementExecutor = (*fakeMoveExec)(nil)
)

func (f *fakeMoveExec) SendPosition(x, y, z float64, onGround bool) error {
	f.mu.Lock()
	f.posCalls = append(f.posCalls, [3]float64{x, y, z})
	f.currentPos = [3]float64{x, y, z}
	manual := f.manualMode
	f.mu.Unlock()
	// Update agent position for manual mode testing - outside the lock:
	// agent.UpdatePosition takes the agent's own locks, and this method is
	// also called from EnterManualMode's ticker goroutine, so holding
	// fakeMoveExec's lock across an unrelated subsystem call is worth
	// avoiding on general principle even though nothing calls back in.
	if manual && f.agent != nil {
		f.agent.UpdatePosition(models.V3{X: x, Y: y, Z: z}, 0, 0)
	}
	return nil
}
func (f *fakeMoveExec) SendPositionAndRotation(x, y, z float64, yaw, pitch float64, onGround bool) error {
	return f.SendPosition(x, y, z, onGround)
}
func (f *fakeMoveExec) SendRotation(yaw, pitch float64, onGround bool) error { return nil }
func (f *fakeMoveExec) MoveTowards(tx, ty, tz float64, d float64, og bool) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (f *fakeMoveExec) LookAt(x, y, z float64, onGround bool) error {
	f.mu.Lock()
	f.lookCalls = append(f.lookCalls, [3]float64{x, y, z})
	f.mu.Unlock()
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

func (f *fakeMoveExec) GetVelocity() (float64, float64, float64) {
	// no-op for fake
	return 0, 0, 0
}

func (f *fakeMoveExec) IsGliding() bool {
	// no-op for fake
	return false
}

func (f *fakeMoveExec) SetMovementHandler(models.MovementHandler) {}

// ManualMovementExecutor implementation for testing
func (f *fakeMoveExec) EnterManualMode() error {
	f.mu.Lock()
	if f.manualMode {
		f.mu.Unlock()
		return nil // already in manual mode
	}
	f.manualMode = true
	f.stopChan = make(chan struct{})
	// Captured locally rather than read as f.stopChan inside the select
	// below: select re-evaluates its case expressions every loop
	// iteration, so reading the field directly would race
	// ExitManualMode's write of f.stopChan = nil. A local copy, read once,
	// sidesteps that entirely.
	stop := f.stopChan
	f.mu.Unlock()

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
				f.mu.Lock()
				throttleX := f.manualThrottle[0] * speed
				throttleZ := f.manualThrottle[1] * speed
				// Update position (no gravity in fake mode, just horizontal movement)
				f.currentPos[0] += throttleX
				f.currentPos[2] += throttleZ
				pos := f.currentPos
				f.mu.Unlock()

				// Report position to maintain ground contact
				_ = f.SendPosition(pos[0], pos[1], pos[2], true)

			case <-stop:
				return
			}
		}
	}()

	return nil
}

func (f *fakeMoveExec) ExitManualMode() error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	f.manualThrottle = [2]float64{westEastThrottle, northSouthThrottle}
	f.mu.Unlock()
	return nil
}

func (f *fakeMoveExec) SetManualRotation(yaw, pitch float64) error {
	f.mu.Lock()
	f.manualRotation = [2]float64{yaw, pitch}
	f.mu.Unlock()
	return nil
}

func (f *fakeMoveExec) SetManualJump(enabled bool) error {
	f.mu.Lock()
	f.manualJump = enabled
	f.mu.Unlock()
	return nil
}

func (f *fakeMoveExec) SetManualSprint(enabled bool) error {
	return nil
}

func (f *fakeMoveExec) SetManualSneak(enabled bool) error {
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

// fakePF's FindPath is called from the goroutine FindPath.Execute spawns
// for the async chat command (actions/commands.go), while the test reads
// called/pathGoal from its own goroutine after a fixed sleep - mu guards
// both sides; use Called()/PathGoal() from tests rather than the raw
// fields.
type fakePF struct {
	mu              sync.Mutex
	start, pathGoal models.V3
	called          bool
}

func (f *fakePF) FindPath(ctx context.Context, s, g models.V3, _ int) (*models.Path, error) {
	f.mu.Lock()
	f.start, f.pathGoal, f.called = s, g, true
	f.mu.Unlock()
	return &models.Path{}, nil
}
func (f *fakePF) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return startY
}

// Called reports whether FindPath has been invoked yet.
func (f *fakePF) Called() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called
}

// PathGoal returns the most recent goal FindPath was called with.
func (f *fakePF) PathGoal() models.V3 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pathGoal
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

// fakeConn is a minimal fake that implements WritePacket. It shares
// fakeClientWriter's mutex (not just its pkts slice) since both can be
// invoked from the goroutine an async chat command (e.g. FireBow) spawns,
// concurrently with a test reading pkts after a fixed sleep.
type fakeConn struct {
	mu   *sync.Mutex
	pkts *[]pk.Packet
}

func (f *fakeConn) WritePacket(p pk.Packet) error {
	if f.pkts == nil {
		return nil
	}
	f.mu.Lock()
	*f.pkts = append(*f.pkts, p)
	f.mu.Unlock()
	return nil
}

type fakeClientWriter struct {
	mu       sync.Mutex
	pkts     []pk.Packet
	fakeConn *fakeConn
}

func newFakeClientWriter() *fakeClientWriter {
	f := &fakeClientWriter{}
	f.fakeConn = &fakeConn{mu: &f.mu, pkts: &f.pkts}
	return f
}

// Pkts returns a snapshot of every packet written so far - safe to call
// concurrently with WritePacket, unlike reading the pkts field directly.
func (f *fakeClientWriter) Pkts() []pk.Packet {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]pk.Packet, len(f.pkts))
	copy(out, f.pkts)
	return out
}

func (f *fakeClientWriter) JoinServerWithOptions(context.Context, string, bot.JoinOptions) error {
	return nil
}
func (f *fakeClientWriter) Events() bot.Events               { return nil }
func (f *fakeClientWriter) Name() string                     { return "BOT" }
func (f *fakeClientWriter) HandleGame(context.Context) error { return nil }
func (f *fakeClientWriter) WritePacket(p pk.Packet) error {
	f.mu.Lock()
	f.pkts = append(f.pkts, p)
	f.mu.Unlock()
	return nil
}
func (f *fakeClientWriter) Close() error { return nil }
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
func (f *fakeClientWriter) VersionHandler() bot.VersionHandler               { return nil }

// Movement: moveForward 0.1 should send one position packet forward (yaw=0 => +Z)
func TestCommand_MoveForward_SmallStep(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(models.V3{}, 0, 0)
	fm := &fakeMoveExec{agent: agent}
	agent.SetMovementExecutor(fm)
	agent.handleChatCommand("moveForward 0.1")
	time.Sleep(70 * time.Millisecond) // allow one step
	posCalls := fm.PosCalls()
	if len(posCalls) == 0 {
		t.Fatalf("expected a SendPosition call")
	}
	got := posCalls[len(posCalls)-1]
	// With physics-based movement, expect forward movement (positive z) in the 0.1-0.3 range
	// (one tick of ~0.215 blocks/tick movement towards the target)
	if got[2] <= 0 || got[2] > 0.3 {
		t.Fatalf("expected forward movement z in (0, 0.3], got %#v", got)
	}
}

// Movement: moveTo with target in same block (floor(0.1)=0) should say "Already at target"
func TestCommand_MoveTo_SmallDelta(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.UpdatePosition(models.V3{}, 0, 0)

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

	agent.UpdatePosition(models.V3{}, 0, 0)
	pf := &fakePF{}
	agent.SetPathFinder(pf)
	agent.handleChatCommand("findPath 1 0 0")
	time.Sleep(10 * time.Millisecond)
	if !pf.Called() {
		t.Fatalf("expected FindPath call")
	}
	if pathGoal := pf.PathGoal(); pathGoal.X != 1 || pathGoal.Y != 0 || pathGoal.Z != 0 {
		t.Fatalf("unexpected goal: %#v", pathGoal)
	}
}

// Tracking: startTracking should invoke LookAt on nearest at least once, and stopTracking should stop it
func TestCommand_StartStopTracking(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "*********:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.UpdatePosition(models.V3{}, 0, 0)

	// Create unique UUIDs for test entities
	uuid1 := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	uuid2 := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}

	// Seed entities with proper UUIDs
	agent.entities = map[int32]*trackedEntity{
		1: {EntityID: 1, UUID: uuid1, X: 0, Y: 0, Z: 1}, // near
		2: {EntityID: 2, UUID: uuid2, X: 0, Y: 0, Z: 10},
	}

	// Set up player resolver to recognize test entity UUIDs as players
	agent.SetPlayerNameResolver(func(u [16]byte) (string, bool) {
		if u == uuid1 {
			return "Player1", true
		}
		if u == uuid2 {
			return "Player2", true
		}
		return "", false
	})

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	agent.handleChatCommand("startTracking")
	time.Sleep(250 * time.Millisecond)
	assert.Greater(t, len(fm.LookCalls()), 0, "expected at least one LookAt call")

	agent.handleChatCommand("stopTracking")
	n := len(fm.LookCalls())

	// Allow for one more tick that may have been in flight
	time.Sleep(250 * time.Millisecond)
	finalLookCalls := len(fm.LookCalls())
	assert.LessOrEqual(t, finalLookCalls, n+1, "expected LookAt calls to stop after stopTracking, had %d, now have %d", n, finalLookCalls)
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
	pkts := fc.Pkts()
	if len(pkts) == 0 {
		t.Fatalf("expected at least one packet write")
	}
	if pkts[0].ID != useItemID {
		t.Fatalf("expected first packet ID %d (UseItem), got %d", useItemID, pkts[0].ID)
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
	setBowHoldTestTunables(0, 1*time.Millisecond)
	agent.handleChatCommand("fireBow")
	time.Sleep(50 * time.Millisecond)
	pkts := fc.Pkts()
	if len(pkts) < 2 {
		t.Fatalf("expected multiple packets (use + shoot), got %d", len(pkts))
	}
	foundPlayerAction := false
	for _, p := range pkts {
		if p.ID == playerActionID {
			foundPlayerAction = true
			break
		}
	}
	if !foundPlayerAction {
		t.Fatalf("expected shoot action packet (ID %d) among writes", playerActionID)
	}
}
