package agent

import (
	"context"
	"testing"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
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

type fakeEventBus struct{ handlers []PacketHandler }

func (f *fakeEventBus) AddGeneric(listeners ...PacketHandler) {}
func (f *fakeEventBus) AddListener(listeners ...PacketHandler) {
	f.handlers = append(f.handlers, listeners...)
}

type fakeClient struct{ bus *fakeEventBus }

func (f *fakeClient) JoinServerWithOptions(context.Context, string, JoinOptions) error { return nil }
func (f *fakeClient) Events() EventBus                                                 { return f.bus }
func (f *fakeClient) Name() string                                                     { return "BOT" }
func (f *fakeClient) HandleGame(context.Context) error                                 { return nil }
func (f *fakeClient) WritePacket(p pk.Packet) error                                    { return nil }

func TestInitRegistersCoreHandlers(t *testing.T) {
	a, err := New(Config{Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}

	// Inject fakes
	a.packetMgr = fakeCBPacketMgr{ids: map[string]protocol_models.ClientboundPacketID{
		"ClientboundAddEntity":        1,
		"ClientboundMoveEntityPosRot": 2,
		"ClientboundMoveEntityPos":    3,
		"ClientboundTeleportEntity":   4,
		"ClientboundRemoveEntities":   5,
	}}
	bus := &fakeEventBus{}
	a.client = &fakeClient{bus: bus}

	if err := a.Init(context.Background()); err != nil {
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
	a, err := New(Config{Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	a.entities = map[int32]*trackedEntity{
		10: {EntityID: 10, Removed: true, RemovedAt: time.Now().Add(-EntityRemovalGracePeriod - time.Second)},
		11: {EntityID: 11, Removed: true, RemovedAt: time.Now()},
		12: {EntityID: 12, Removed: false},
	}
	a.cleanupRemovedEntities()
	if _, ok := a.entities[10]; ok {
		t.Errorf("entity 10 should be purged")
	}
	if _, ok := a.entities[11]; !ok {
		t.Errorf("entity 11 should remain")
	}
	if _, ok := a.entities[12]; !ok {
		t.Errorf("entity 12 should remain")
	}
}
