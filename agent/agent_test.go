package agent

import (
	"context"
	"net"
	"testing"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/require"
)

type fakeCBPacketMgr struct {
	ids map[string]protocol_models.ClientboundPacketID
}

func (f fakeCBPacketMgr) GetClientboundPacketID(name string) protocol_models.ClientboundPacketID {
	return f.ids[name]
}
func (f fakeCBPacketMgr) GetClientboundConfigPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (f fakeCBPacketMgr) GetClientboundLoginPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (f fakeCBPacketMgr) Name() string            { return "test" }
func (f fakeCBPacketMgr) VersionProtocol() uint64 { return 0 }
func (f fakeCBPacketMgr) ClientboundToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (f fakeCBPacketMgr) ClientboundConfigToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (f fakeCBPacketMgr) ClientboundLoginToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (f fakeCBPacketMgr) GetServerboundPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (f fakeCBPacketMgr) GetServerboundConfigPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (f fakeCBPacketMgr) GetServerboundLoginPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (f fakeCBPacketMgr) ServerboundToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (f fakeCBPacketMgr) ServerboundConfigToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (f fakeCBPacketMgr) GetClientboundPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetClientboundConfigPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetClientboundLoginPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetClientboundHandshakingPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetClientboundStatusPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetServerboundPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetServerboundConfigPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetServerboundLoginPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetServerboundHandshakingPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetServerboundStatusPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (f fakeCBPacketMgr) GetEntityTypeID(name string) int32 {
	return 0
}

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

func TestInitRegistersCoreHandlers(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	// Inject fakes
	agent.packetMgr = fakeCBPacketMgr{ids: map[string]protocol_models.ClientboundPacketID{
		"ClientboundAddEntity":        1,
		"ClientboundMoveEntityPosRot": 2,
		"ClientboundMoveEntityPos":    3,
		"ClientboundTeleportEntity":   4,
		"ClientboundRemoveEntities":   5,
	}}
	bus := &fakeEventBus{}
	agent.client = &fakeClient{bus: bus}

	if err := agent.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	// We expect at least the core entity handlers to be registered (others may exist)
	gotIDs := map[protocol_models.ClientboundPacketID]bool{}
	for _, h := range bus.handlers {
		gotIDs[protocol_models.ClientboundPacketID(h.ID)] = true
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if !gotIDs[protocol_models.ClientboundPacketID(id)] {
			t.Errorf("missing handler id %d", id)
		}
	}
}

func TestCleanupRemovedEntities(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
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
