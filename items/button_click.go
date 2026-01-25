package items

import (
	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// ButtonClicker handles clicking buttons in container screens
type ButtonClicker struct {
	client    models.PacketSender
	packetMgr protocol_models.PacketMgr
}

// NewButtonClicker creates a new ButtonClicker
func NewButtonClicker(client models.PacketSender, packetMgr protocol_models.PacketMgr) *ButtonClicker {
	return &ButtonClicker{
		client:    client,
		packetMgr: packetMgr,
	}
}

// ClickButton sends a ServerboundContainerButtonClick packet to the server
// Used for:
// - Enchanting table: selecting enchantment (buttonID 0-2)
// - Stonecutter: selecting recipe (buttonID = recipe index)
// - Loom: selecting pattern (buttonID = pattern index)
// - Cartography table: selecting operation (buttonID = operation index)
// - Beacon: confirming effect selection (buttonID 0)
// - Lectern: turning pages (buttonID = page navigation)
//
// windowID: The container window ID (from OpenContainer)
// buttonID: The button/option to select (meaning depends on container type)
func (bc *ButtonClicker) ClickButton(windowID byte, buttonID byte) error {
	packetID := bc.packetMgr.GetServerboundPacketID("ServerboundContainerButtonClick")

	// Marshal packet: Window ID (Byte), Button ID (Byte)
	packet := pk.Marshal(
		packetID,
		pk.Byte(windowID),
		pk.Byte(buttonID),
	)

	return bc.client.WritePacket(packet)
}
