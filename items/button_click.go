package items

import (
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// ButtonClicker handles clicking buttons in container screens
type ButtonClicker struct {
	client           models.PacketSender
	packetMgr        protocol_models.PacketMgr
	containerHandler models.ContainerHandler
}

// NewButtonClicker creates a new ButtonClicker
func NewButtonClicker(client models.PacketSender, packetMgr protocol_models.PacketMgr) *ButtonClicker {
	return &ButtonClicker{
		client:    client,
		packetMgr: packetMgr,
	}
}

// SetContainerHandler sets the version-specific container handler.
// This must be called before using ClickButton for proper version-specific packet handling.
func (bc *ButtonClicker) SetContainerHandler(handler models.ContainerHandler) {
	bc.containerHandler = handler
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
	if bc.containerHandler == nil {
		return common.ErrHandlerNotSet{HandlerName: "ContainerHandler"}
	}

	return bc.containerHandler.SendContainerButtonClick(bc.client, int8(windowID), int8(buttonID))
}
