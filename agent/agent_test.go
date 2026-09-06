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

type fakeEventBus struct{ handlers []bot.PacketHandler }

func (f *fakeEventBus) AddGeneric(listeners ...bot.PacketHandler) {}
func (f *fakeEventBus) AddListener(listeners ...bot.PacketHandler) {
	f.handlers = append(f.handlers, listeners...)
}
func (f *fakeEventBus) GetGenericListeners() []bot.PacketHandler { return nil }
func (f *fakeEventBus) GetListeners() [][]bot.PacketHandler      { return nil }

type fakeClient struct{ bus *fakeEventBus }

func (f *fakeClient) JoinServerWithOptions(context.Context, string, bot.JoinOptions) error {
	return nil
}
func (f *fakeClient) Events() bot.Events                                              { return f.bus }
func (f *fakeClient) Name() string                                                    { return "BOT" }
func (f *fakeClient) HandleGame(context.Context) error                                { return nil }
func (f *fakeClient) WritePacket(p pk.Packet) error                                   { return nil }
func (f *fakeClient) Close() error                                                    { return nil }
func (f *fakeClient) Conn() *bot.Conn                                                 { return nil }
func (f *fakeClient) SetAuth(bot.Auth)                                                {}
func (f *fakeClient) JoinServer(context.Context, string) error                        { return nil }
func (f *fakeClient) JoinServerWithDialer(context.Context, *net.Dialer, string) error { return nil }
func (f *fakeClient) UUID() uuid.UUID                                                 { return uuid.UUID{} }
func (f *fakeClient) Cookies() map[string][]byte                                      { return nil }
func (f *fakeClient) SetCookies(map[string][]byte)                                    {}
func (f *fakeClient) RegistryData() map[string]*bot.CustomRegistry                    { return nil }
func (f *fakeClient) RegistryTags() map[string]*bot.RegistryTags                      { return nil }
func (f *fakeClient) LoginPlugin() map[string]bot.CustomPayloadHandler                { return nil }
func (f *fakeClient) CustomReportDetails() map[string]string                          { return nil }
func (f *fakeClient) SetJoinLogin(func(*mcnet.Conn) error)                            {}
func (f *fakeClient) SetJoinConfiguration(func(*mcnet.Conn) error)                    {}
func (f *fakeClient) MovementMirror() bot.MovementMirror                              { return nil }
func (f *fakeClient) RegistryCallback() bot.RegistryDataCallback                      { return nil }
func (f *fakeClient) PacketMgr() protocol_models.PacketMgr                            { return nil }
func (f *fakeClient) EnableFeature([]pk.Identifier)                                   {}
func (f *fakeClient) PushResourcePack(bot.ResourcePack)                               {}
func (f *fakeClient) PopResourcePack(pk.UUID)                                         {}
func (f *fakeClient) PopAllResourcePack()                                             {}
func (f *fakeClient) SelectDataPacks([]bot.DataPack) []bot.DataPack                   { return nil }
func (f *fakeClient) SetVersionHandler(bot.VersionHandler)                            {}
func (f *fakeClient) VersionHandler() bot.VersionHandler                              { return nil }

func TestInitRegistersCoreHandlers(t *testing.T) {
	expectedPackets := []string{
		"ClientboundAddEntity",
		"ClientboundMoveEntityPosRot",
		"ClientboundMoveEntityPos",
		"ClientboundTeleportEntity",
		"ClientboundRemoveEntities",
	}

	for _, versionTest := range models.StandardVersionTests {
		t.Run(versionTest.Name, func(t *testing.T) {
			agentInt, err := New(models.AgentConfig{Version: versionTest.MCVersion, Address: "127.0.0.1:25565"})
			require.NoError(t, err)

			agent := agentInt.(*agent)

			// Use real packet manager for this version
			pktMgr := protocol_versions.GetPacketMgrForVersion(versionTest.MCVersion)
			require.NotNil(t, pktMgr, "packet manager for %s should not be nil", versionTest.MCVersion)
			agent.cfg.PacketMgr = pktMgr

			bus := &fakeEventBus{}
			agent.client = &fakeClient{bus: bus}

			if err := agent.Init(context.Background()); err != nil {
				t.Fatalf("Init failed: %v", err)
			}
			// We expect at least the core entity handlers to be registered
			require.NotEmpty(t, bus.handlers, "no handlers registered after Init")

			// Verify key entity packet handlers were registered by checking handler IDs
			gotIDs := map[protocol_models.ClientboundPacketID]bool{}
			for _, h := range bus.handlers {
				gotIDs[protocol_models.ClientboundPacketID(h.ID)] = true
			}

			for _, packetName := range expectedPackets {
				expectedID := pktMgr.GetClientboundPacketID(packetName)
				require.True(t, gotIDs[expectedID], "missing handler for packet %s (id %d)", packetName, expectedID)
			}
		})
	}
}

func TestCleanupRemovedEntities(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	agent.entities = map[int32]*trackedEntity{
		10: {EntityID: 10, Removed: true, RemovedAt: time.Now().Add(-EntityRemovalGracePeriod - time.Second)},
		11: {EntityID: 11, Removed: true, RemovedAt: time.Now()},
		12: {EntityID: 12, Removed: false},
	}
	agent.cleanupRemovedEntities()
	if _, ok := agent.entities[10]; ok {
		t.Errorf("entity 10 should be purged")
	}
	if _, ok := agent.entities[11]; !ok {
		t.Errorf("entity 11 should remain")
	}
	if _, ok := agent.entities[12]; !ok {
		t.Errorf("entity 12 should remain")
	}
}
