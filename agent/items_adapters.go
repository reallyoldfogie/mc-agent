package agent

import (
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

type itemPacketSender struct {
	conn *bot.Conn
}

func (s itemPacketSender) WritePacket(packet pk.Packet) error {
	if s.conn == nil {
		return nil
	}
	return s.conn.WritePacket(packet)
}

type itemPacketManager struct {
	pm protocol_models.PacketMgr
}

func (m itemPacketManager) GetServerboundPacketID(name string) protocol_models.ServerboundPacketID {
	if m.pm == nil {
		return 0
	}
	return m.pm.GetServerboundPacketID(name)
}

func (m itemPacketManager) GetClientboundLoginPacketID(name string) protocol_models.ClientboundPacketID {
	if m.pm == nil {
		return 0
	}
	return m.pm.GetClientboundLoginPacketID(name)
}

func (m itemPacketManager) GetClientboundConfigPacketID(name string) protocol_models.ClientboundPacketID {
	if m.pm == nil {
		return 0
	}
	return m.pm.GetClientboundConfigPacketID(name)
}

func (m itemPacketManager) GetClientboundPacketID(name string) protocol_models.ClientboundPacketID {
	if m.pm == nil {
		return 0
	}
	return m.pm.GetClientboundPacketID(name)
}

func (m itemPacketManager) ClientboundLoginToString(id protocol_models.ClientboundPacketID) string {
	if m.pm == nil {
		return ""
	}
	return m.pm.ClientboundLoginToString(id)
}

func (m itemPacketManager) ClientboundConfigToString(id protocol_models.ClientboundPacketID) string {
	if m.pm == nil {
		return ""
	}
	return m.pm.ClientboundConfigToString(id)
}

func (m itemPacketManager) ClientboundToString(id protocol_models.ClientboundPacketID) string {
	if m.pm == nil {
		return ""
	}
	return m.pm.ClientboundToString(id)
}

func (m itemPacketManager) GetClientboundLoginPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetClientboundLoginPacketByID(id)
}

func (m itemPacketManager) GetClientboundConfigPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetClientboundConfigPacketByID(id)
}

func (m itemPacketManager) GetClientboundPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetClientboundPacketByID(id)
}

func (m itemPacketManager) GetClientboundHandshakingPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetClientboundHandshakingPacketByID(id)
}

func (m itemPacketManager) GetClientboundStatusPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetClientboundStatusPacketByID(id)
}

func (m itemPacketManager) GetServerboundLoginPacketID(name string) protocol_models.ServerboundPacketID {
	if m.pm == nil {
		return 0
	}
	return m.pm.GetServerboundLoginPacketID(name)
}

func (m itemPacketManager) GetServerboundConfigPacketID(name string) protocol_models.ServerboundPacketID {
	if m.pm == nil {
		return 0
	}
	return m.pm.GetServerboundConfigPacketID(name)
}

func (m itemPacketManager) ServerboundConfigToString(id protocol_models.ServerboundPacketID) string {
	if m.pm == nil {
		return ""
	}
	return m.pm.ServerboundConfigToString(id)
}

func (m itemPacketManager) ServerboundToString(id protocol_models.ServerboundPacketID) string {
	if m.pm == nil {
		return ""
	}
	return m.pm.ServerboundToString(id)
}

func (m itemPacketManager) GetServerboundLoginPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetServerboundLoginPacketByID(id)
}

func (m itemPacketManager) GetServerboundConfigPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetServerboundConfigPacketByID(id)
}

func (m itemPacketManager) GetServerboundPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetServerboundPacketByID(id)
}

func (m itemPacketManager) GetServerboundHandshakingPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetServerboundHandshakingPacketByID(id)
}

func (m itemPacketManager) GetServerboundStatusPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	if m.pm == nil {
		return nil, fmt.Errorf("packet manager not set")
	}
	return m.pm.GetServerboundStatusPacketByID(id)
}

func (m itemPacketManager) Name() string {
	if m.pm == nil {
		return ""
	}
	return m.pm.Name()
}

func (m itemPacketManager) VersionProtocol() uint64 {
	if m.pm == nil {
		return 0
	}
	return m.pm.VersionProtocol()
}

func (m itemPacketManager) GetEntityTypeID(name string) int32 {
	if m.pm == nil {
		return -1
	}
	return m.pm.GetEntityTypeID(name)
}

type registryItemManager struct {
	registryGetter func(string) models.CustomRegistry
}

func (m registryItemManager) GetItemNameByID(id int) string {
	if m.registryGetter == nil {
		return ""
	}
	reg := m.registryGetter("minecraft:item")
	if reg == nil || !reg.IsReady() {
		return ""
	}
	if name, ok := reg.GetNameByID(int32(id)); ok {
		return name
	}
	return ""
}

// RegistryItemManager returns an ItemManager backed by the agent's registries.
func (a *agent) RegistryItemManager() ItemManager {
	return registryItemManager{registryGetter: a.GetRegistry}
}
