// Package v1_21_6 provides version-specific packet handling for Minecraft 1.21.6.
package v1_21_6

import (
	"bytes"
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
	agent_models "github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.6/basetypes"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.6/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.6/play/serverbound"
	"github.com/reallyoldfogie/mc-protocol-go/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

const (
	// Version is the Minecraft version string
	Version = "1.21.6"
	// ProtocolVersion is the protocol version number
	ProtocolVersion = 771
)

// Handler implements common.VersionHandler for Minecraft 1.21.6.
type Handler struct {
	packetMgr protocol_models.PacketMgr

	// Sub-handlers (lazily initialized)
	loginHandler  *loginHandler
	configHandler *configurationHandler
	playHandler   *playHandler
}

// NewHandler creates a new version handler for 1.21.6.
// The packetMgr parameter should be the mc-protocol-go PacketMgr for 1.21.6.
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

// playHandler implements common.PlayHandler for 1.21.6.
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
// In 1.21.6, CommonSettings has 9 fields (including ParticleStatus).
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
	pkt.ParticleStatus = basetypes.CommonSettingsParticleStatus{Value: "all"} // Default to "all"

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

	// Find event type ID from reason string using GameStateChangeReasonMappings
	var eventID int
	for id, reason := range cb.GameStateChangeReasonMappings {
		if reason == gameStateChange.Reason.Value {
			eventID = int(id)
			break
		}
	}

	return eventID, 0, 0, 0, float64(gameStateChange.GameMode), nil
}

// ParseUpdateRecipes parses a ClientboundDeclareRecipes packet for 1.21.6.
// Uses protocol structs to parse property sets and stonecutter recipes.
func (p *playHandler) ParseUpdateRecipes(pkt pk.Packet) (*agent_models.UpdateRecipesPayload, error) {
	declareRecipes := cb.NewDeclareRecipes()
	if err := declareRecipes.Scan(pkt); err != nil {
		return nil, common.ErrPacketParse{PacketName: "DeclareRecipes", Cause: fmt.Errorf("failed to scan packet: %w", err)}
	}
	var payload agent_models.UpdateRecipesPayload

	// Parse property sets (recipes)
	recipes := declareRecipes.Recipes.Get()
	for _, recipe := range recipes {
		items := recipe.Items.Get()
		var itemList []int32
		for _, itemID := range items {
			itemList = append(itemList, int32(itemID))
		}
		payload.PropertySets = append(payload.PropertySets, agent_models.PropertySet{
			ID:    string(recipe.Name),
			Items: itemList,
		})
	}

	// Parse stonecutter entries
	stonecutterRecipes := declareRecipes.StoneCutterRecipes.Get()
	for _, recipe := range stonecutterRecipes {
		// Convert IDSet to SlotDisplay
		var input agent_models.SlotDisplay
		if recipe.Input.IsTagList {
			// Tag representation - not used in recipe inputs
			input = agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
		} else {
			// IDs list representation
			idsAry := recipe.Input.IDs.Get()
			if len(idsAry) == 0 {
				input = agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
			} else if len(idsAry) == 1 {
				input = agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeItem, Item: &agent_models.SlotDisplayItem{ItemID: int32(idsAry[0])}}
			} else {
				// Multiple items - create composite
				options := make([]agent_models.SlotDisplay, len(idsAry))
				for optIdx, itemID := range idsAry {
					options[optIdx] = agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeItem, Item: &agent_models.SlotDisplayItem{ItemID: int32(itemID)}}
				}
				input = agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeComposite, Composite: options}
			}
		}

		// Convert protocol SlotDisplay to agent model
		result := p.convertProtocolSlotDisplay(recipe.SlotDisplay)

		payload.StonecutterEntries = append(payload.StonecutterEntries, agent_models.StonecutterEntry{
			Input:   input,
			Results: []agent_models.SlotDisplay{result},
		})
	}

	return &payload, nil
}

// convertProtocolSlotDisplay converts protocol SlotDisplay to agent model SlotDisplay.
func (p *playHandler) convertProtocolSlotDisplay(slot cb.SlotDisplay) agent_models.SlotDisplay {
	switch slot.Type.Value {
	case "empty", "any_fuel":
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	case "item":
		if itemID, ok := slot.Data.(*pk.VarInt); ok && itemID != nil {
			return agent_models.SlotDisplay{
				Type: agent_models.SlotDisplayTypeItem,
				Item: &agent_models.SlotDisplayItem{ItemID: int32(*itemID)},
			}
		}
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	case "item_stack":
		if slotData, ok := slot.Data.(*basetypes.Slot); ok && slotData != nil && slotData.ItemCount > 0 {
			return agent_models.SlotDisplay{
				Type: agent_models.SlotDisplayTypeItem,
				Item: &agent_models.SlotDisplayItem{ItemID: int32(slotData.ItemCount)},
			}
		}
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	case "with_remainder":
		if wrData, ok := slot.Data.(*cb.SlotDisplayDataWithRemainder); ok && wrData != nil {
			ingredient := p.convertProtocolSlotDisplay(wrData.Input)
			remainder := p.convertProtocolSlotDisplay(wrData.Remainder)
			return agent_models.SlotDisplay{
				Type: agent_models.SlotDisplayTypeWithRemainder,
				WithRemainder: &agent_models.SlotDisplayWithRemainder{
					Ingredient: ingredient,
					Remainder:  remainder,
				},
			}
		}
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	default:
		return agent_models.SlotDisplay{Type: agent_models.SlotDisplayTypeEmpty}
	}
}

// entityHandler is implemented in entities.go

// containerHandler is implemented in containers.go

// chatHandler is implemented in chat.go

// worldHandler is implemented in world.go
