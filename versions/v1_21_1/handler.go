// Package v1_21_1 provides version-specific packet handling for Minecraft 1.21.1.
package v1_21_1

import (
	"bytes"
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
	agent_models "github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/basetypes"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/serverbound"
	"github.com/reallyoldfogie/mc-protocol-go/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

const (
	// Version is the Minecraft version string
	Version = "1.21.1"
	// ProtocolVersion is the protocol version number
	ProtocolVersion = 769
)

// Handler implements common.VersionHandler for Minecraft 1.21.1.
type Handler struct {
	packetMgr protocol_models.PacketMgr

	// Sub-handlers (lazily initialized)
	loginHandler  *loginHandler
	configHandler *configurationHandler
	playHandler   *playHandler
}

// NewHandler creates a new version handler for 1.21.1.
// The packetMgr parameter should be the mc-protocol-go PacketMgr for 1.21.1.
func NewHandler(packetMgr protocol_models.PacketMgr) *Handler {
	return &Handler{
		packetMgr: packetMgr,
	}
}

// Version returns the Minecraft version string.
func (h *Handler) Version() string {
	return Version
}

// ProtocolVersion returns the protocol version number.
func (h *Handler) ProtocolVersion() uint {
	return ProtocolVersion
}

// PacketMgr returns the underlying packet manager.
func (h *Handler) PacketMgr() protocol_models.PacketMgr {
	return h.packetMgr
}

// Login returns the login phase handler.
func (h *Handler) Login() common.LoginHandler {
	if h.loginHandler == nil {
		h.loginHandler = &loginHandler{packetMgr: h.packetMgr}
	}
	return h.loginHandler
}

// Configuration returns the configuration phase handler.
func (h *Handler) Configuration() common.ConfigurationHandler {
	if h.configHandler == nil {
		h.configHandler = &configurationHandler{packetMgr: h.packetMgr}
	}
	return h.configHandler
}

// Play returns the play phase handler.
func (h *Handler) Play() common.PlayHandler {
	if h.playHandler == nil {
		h.playHandler = &playHandler{packetMgr: h.packetMgr}
	}
	return h.playHandler
}

// loginHandler is implemented in login.go

// configurationHandler is implemented in configuration.go

// playHandler implements common.PlayHandler for 1.21.1.
type playHandler struct {
	packetMgr protocol_models.PacketMgr

	// Sub-handlers (lazily initialized)
	movement   *movementHandler
	entities   *entityHandler
	containers *containerHandler
	chat       *chatHandler
	world      *worldHandler
	actions    *actionHandler
}

func (p *playHandler) Movement() common.MovementHandler {
	if p.movement == nil {
		p.movement = &movementHandler{packetMgr: p.packetMgr}
	}
	return p.movement
}

func (p *playHandler) Entities() common.EntityHandler {
	if p.entities == nil {
		p.entities = &entityHandler{packetMgr: p.packetMgr}
	}
	return p.entities
}

func (p *playHandler) Containers() common.ContainerHandler {
	if p.containers == nil {
		p.containers = &containerHandler{packetMgr: p.packetMgr}
	}
	return p.containers
}

func (p *playHandler) Chat() common.ChatHandler {
	if p.chat == nil {
		p.chat = &chatHandler{packetMgr: p.packetMgr}
	}
	return p.chat
}

func (p *playHandler) World() common.WorldHandler {
	if p.world == nil {
		p.world = &worldHandler{packetMgr: p.packetMgr}
	}
	return p.world
}

func (p *playHandler) Actions() common.ActionHandler {
	if p.actions == nil {
		p.actions = &actionHandler{packetMgr: p.packetMgr}
	}
	return p.actions
}

// SendClientInformation sends client settings/information.
// In 1.21.1, CommonSettings has 8 fields (no ParticleStatus).
func (p *playHandler) SendClientInformation(conn common.PacketWriter, info common.ClientInfo) error {
	pkt := basetypes.NewCommonSettings()
	pkt.SetPacketID(int32(p.packetMgr.GetServerboundPacketID("ServerboundCommonSettings")))
	pkt.Locale = pk.String(info.Locale)
	pkt.ViewDistance = pk.Byte(info.ViewDistance)
	pkt.ChatFlags = pk.VarInt(info.ChatMode)
	pkt.ChatColors = pk.Boolean(info.ChatColors)
	pkt.SkinParts = pk.UnsignedByte(info.DisplayedSkinParts)
	pkt.MainHand = pk.VarInt(info.MainHand)
	pkt.EnableTextFiltering = pk.Boolean(info.EnableTextFiltering)
	pkt.EnableServerListing = pk.Boolean(info.AllowServerListings)
	// Note: ParticleStatus field added in 1.21.4+, not present in 1.21.1

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "CommonSettings", Cause: err}
	}
	return nil
}

// SendCustomPayload sends a custom payload packet (plugin channels).
func (p *playHandler) SendCustomPayload(conn common.PacketWriter, channel string, payload string) error {
	pkt := sb.NewCustomPayload()
	pkt.Channel = pk.String(channel)
	var buf bytes.Buffer
	if _, err := pk.String(payload).WriteTo(&buf); err != nil {
		return common.ErrPacketSend{PacketName: "CustomPayload", Cause: err}
	}
	pkt.Data = models.RestBuffer{Data: buf.Bytes()}

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "CustomPayload", Cause: err}
	}
	return nil
}

// ParseLogin parses the ClientboundLogin packet to extract entity ID.
func (p *playHandler) ParseLogin(pkt pk.Packet) (entityID int32, err error) {
	var id pk.Int
	if err = pkt.Scan(&id); err != nil {
		return 0, common.ErrPacketParse{PacketName: "Login", Cause: err}
	}
	return int32(id), nil
}

// ParseSound parses a ClientboundSound packet.
func (p *playHandler) ParseSound(pkt pk.Packet) (soundID, category int32, x, y, z int32, volume, pitch float32, seed int64, err error) {
	var (
		soundIDVar          pk.VarInt
		categoryVar         pk.VarInt
		xVar, yVar, zVar    pk.Int
		volumeVar, pitchVar pk.Float
		seedVar             pk.Long
	)
	if err = pkt.Scan(&soundIDVar, &categoryVar, &xVar, &yVar, &zVar, &volumeVar, &pitchVar, &seedVar); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, common.ErrPacketParse{PacketName: "Sound", Cause: err}
	}
	return int32(soundIDVar), int32(categoryVar), int32(xVar), int32(yVar), int32(zVar), float32(volumeVar), float32(pitchVar), int64(seedVar), nil
}

// ParseViewDistance parses a ClientboundSetChunkCacheRadius packet.
func (p *playHandler) ParseViewDistance(pkt pk.Packet) (viewDistance int32, err error) {
	var vd pk.VarInt
	if err = pkt.Scan(&vd); err != nil {
		return 0, common.ErrPacketParse{PacketName: "SetChunkCacheRadius", Cause: err}
	}
	return int32(vd), nil
}

// ParseSimulationDistance parses a ClientboundSetSimulationDistance packet.
func (p *playHandler) ParseSimulationDistance(pkt pk.Packet) (simulationDistance int32, err error) {
	var sd pk.VarInt
	if err = pkt.Scan(&sd); err != nil {
		return 0, common.ErrPacketParse{PacketName: "SetSimulationDistance", Cause: err}
	}
	return int32(sd), nil
}

// ParseDisconnect parses a ClientboundDisconnect packet.
func (p *playHandler) ParseDisconnect(pkt pk.Packet) (reason string, err error) {
	var r pk.String
	if err = pkt.Scan(&r); err != nil {
		return "", common.ErrPacketParse{PacketName: "Disconnect", Cause: err}
	}
	return string(r), nil
}

// ParseGameEvent parses a ClientboundGameStateChange packet (Game Event in 1.20 and earlier).
func (p *playHandler) ParseGameEvent(pkt pk.Packet) (eventType int, x, y, z, value float64, err error) {
	gameStateChange := cb.NewGameStateChange()
	if err = gameStateChange.Scan(pkt); err != nil {
		return 0, 0, 0, 0, 0, common.ErrPacketParse{PacketName: "GameStateChange", Cause: err}
	}

	return int(gameStateChange.Reason), 0, 0, 0, float64(gameStateChange.GameMode), nil
}

// ParseUpdateRecipes parses the ClientboundDeclareRecipes packet for 1.21.1.
// 1.21.1 uses the legacy recipe format with many recipe types.
// We extract stonecutter recipes and convert them to our SlotDisplay format.
func (p *playHandler) ParseUpdateRecipes(pkt pk.Packet) (*agent_models.UpdateRecipesPayload, error) {
	declareRecipes := cb.NewDeclareRecipes()
	if err := declareRecipes.Scan(pkt); err != nil {
		return nil, common.ErrPacketParse{PacketName: "DeclareRecipes", Cause: fmt.Errorf("failed to scan packet: %w", err)}
	}

	var payload agent_models.UpdateRecipesPayload

	// Get the recipes array
	recipes := declareRecipes.Recipes.Get()
	if recipes == nil || len(recipes) == 0 {
		return &payload, nil
	}

	// Process recipes from the recipes array
	// In 1.21.1, stonecutter recipes are embedded with type="minecraft:stonecutting"
	for _, recipe := range recipes {
		// Check if this is a stonecutter recipe
		if recipe.Type.Value == "minecraft:stonecutting" {
			// Extract the stonecutter data
			if stonecutterData, ok := recipe.Data.(*cb.DeclareRecipesRecipesArrayTypeDataMinecraftStonecutting); ok {
				// Convert ingredient (Array[VarInt, Slot]) to SlotDisplay
				inputDisplay := p.convertIngredientToSlotDisplay(&stonecutterData.Ingredient)

				// Convert result Slot to SlotDisplay
				resultDisplay := p.convertSlotToSlotDisplay(&stonecutterData.Result)

				payload.StonecutterEntries = append(payload.StonecutterEntries, agent_models.StonecutterEntry{
					Input:   inputDisplay,
					Results: []agent_models.SlotDisplay{resultDisplay},
				})
			}
		}
	}

	return &payload, nil
}

// convertIngredientToSlotDisplay converts basetypes.Ingredient (Array[VarInt, Slot]) to SlotDisplay.
func (p *playHandler) convertIngredientToSlotDisplay(ingredient *basetypes.Ingredient) agent_models.SlotDisplay {
	if ingredient == nil {
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	}

	slots := ingredient.Get()

	if slots == nil || len(slots) == 0 {
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	}

	if len(slots) == 1 {
		// Single item
		return p.convertSlotToSlotDisplay(&(slots)[0])
	}

	// Multiple options - create composite
	options := make([]agent_models.SlotDisplay, len(slots))
	for i, slotItem := range slots {
		options[i] = p.convertSlotToSlotDisplay(&slotItem)
	}
	return agent_models.SlotDisplay{
		Type:      agent_models.SlotDisplayTypeComposite,
		Composite: options,
	}
}

// convertSlotToSlotDisplay converts basetypes.Slot to SlotDisplay.
func (p *playHandler) convertSlotToSlotDisplay(slot *basetypes.Slot) agent_models.SlotDisplay {
	if slot == nil || slot.ItemCount == 0 {
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	}

	// Extract ItemId from the switch field
	var itemID int32
	if unnamedField, ok := slot.UnnamedType0002.(*basetypes.SlotUnnamedType0002Default); ok {
		itemID = int32(unnamedField.ItemId)
	}

	return agent_models.SlotDisplay{
		Type: agent_models.SlotDisplayTypeItem,
		Item: &agent_models.SlotDisplayItem{
			ItemID: itemID,
		},
	}
}

// entityHandler is implemented in entities.go

// containerHandler is implemented in containers.go

// chatHandler is implemented in chat.go

// worldHandler is implemented in world.go
