package testing

import (
	"fmt"

	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/following"
	"github.com/reallyoldfogie/mc-agent/movement"
	pf "github.com/reallyoldfogie/mc-agent/pathfinding"
	agutils "github.com/reallyoldfogie/mc-agent/utils"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
	"github.com/reallyoldfogie/mc-bot-go/bot/msg"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-bot-go/bot/world"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// wireAgentSubsystems wires up all the subsystems needed for an agent to function.
// This mirrors the setup in cmd/agent/main.go.
func wireAgentSubsystems(agnt agent.Agent, botClient *bot.Client, packetMgr protocol_models.PacketMgr, blockMgr mc_versions.BlockMgr, cfg AgentConfig) error {
	// Create player subsystem with basic settings
	customSettings := basic.DefaultSettings
	customSettings.ViewDistance = 32
	customSettings.Locale = "en_us"

	player := basic.NewPlayer(botClient, customSettings, basic.EventsListener{
		GameStart:    agnt.HandleGameStart,
		Disconnect:   agnt.HandleDisconnect,
		HealthChange: agnt.HandleHealthChange,
		Death:        agnt.HandleDeath,
		Teleported:   agnt.HandleTeleported,
	}, packetMgr)

	// Expose teleport accepter to agent
	agnt.SetTeleportAccepter(player)

	// Player list for chat manager
	plist := playerlist.New(botClient, packetMgr)

	// Chat manager with agent event handlers
	chatMgr := msg.New(botClient, player, plist, msg.EventsHandler{
		SystemChat:        agnt.OnSystemChat,
		PlayerChatMessage: agnt.OnPlayerChat,
		DisguisedChat:     agnt.OnDisguisedChat,
	}, packetMgr)
	agnt.SetChat(agent.NewChatFromMsg(chatMgr))

	// World manager with chunk load/unload callbacks (created once, reused for pathfinding)
	wm := world.NewWorld(botClient, player, world.EventsListener{
		LoadChunk:   agnt.HandleChunkLoad,
		UnloadChunk: agnt.HandleChunkUnload,
	}, packetMgr)

	// Screen manager: wire slot change and provide slot resolver
	scr := screen.NewManager(botClient, screen.EventsListener{
		Open:    nil,
		SetSlot: agnt.OnScreenSlotChange,
		Close:   nil,
	}, packetMgr)
	agnt.SetSlotResolver(slotResolver{m: scr})
	agnt.SetItemManager(itemMgrAdapter{})

	// Provide player UUID resolver for following
	agnt.SetPlayerUUIDResolver(func(name string) ([16]byte, error) {
		for uuid, info := range plist.PlayerInfos {
			if info.Name == name {
				return uuid, nil
			}
		}
		return [16]byte{}, fmt.Errorf("player %s not found", name)
	})

	// Provide name-by-UUID resolver for tracking messages
	agnt.SetPlayerNameResolver(func(u [16]byte) (string, bool) {
		for uuid, info := range plist.PlayerInfos {
			var cu [16]byte
			copy(cu[:], uuid[:])
			if cu == u {
				return info.Name, true
			}
		}
		return "", false
	})

	// Wire up pathfinding and following if enabled
	if cfg.EnablePathfinding {
		dataBasePath, err := agutils.ResolveDataPath(cfg.MCDataGenPath, "../data/mc-data-gen-cache", "")
		if err != nil {
			return fmt.Errorf("resolve data path: %w", err)
		}

		shapeMgr, err := pf.NewBlockShapeManager(cfg.Version, dataBasePath)
		if err != nil {
			return fmt.Errorf("create block shape manager: %w", err)
		}

		var stateProps *pf.StatePropertyLoader
		if cfg.MCProtocolGoPath != "" {
			if spl, err := pf.NewStatePropertyLoader(cfg.MCProtocolGoPath, cfg.Version); err == nil {
				stateProps = spl
			}
		}

		// Reuse the world manager created above (do not recreate it)

		pathFinder := pf.NewPathFinder(wm, shapeMgr, blockMgr, stateProps)
		agnt.SetPathFinder(pathFinder)

		moveExec := movement.NewMovementExecutor(botClient, packetMgr, agnt.GetPosition, agnt.UpdatePosition, agnt.GetEntityID)
		agnt.SetMovementExecutor(moveExec)

		if cfg.EnableFollowing {
			targetSelector := following.NewTargetSelector(agnt.GetTrackedEntitiesForFollowing, agnt.ResolvePlayerUUIDByName, agnt.GetPositionSimple)
			followCfg := following.DefaultFollowConfig()
			followMgr := following.NewFollowManager(targetSelector, pathFinder, moveExec, agnt.GetPosition, agnt.SendChat, followCfg)
			agnt.SetFollowManager(followMgr)
		}
	}

	return nil
}

// slotResolver adapts screen.Manager slot data to the agent SlotResolver interface.
type slotResolver struct{ m *screen.Manager }

func (sr slotResolver) ResolveSlot(id, index int) (itemID int, count int, ok bool) {
	if id == -2 {
		if index >= 0 && index < len(sr.m.Inventory.Slots) {
			s := sr.m.Inventory.Slots[index]
			if s.ID >= 0 {
				return int(s.ID), int(s.Count), true
			}
		}
		return 0, 0, false
	}
	if id == -1 && index == -1 {
		s := sr.m.Cursor
		if s.ID >= 0 {
			return int(s.ID), int(s.Count), true
		}
		return 0, 0, false
	}
	if c, okc := sr.m.Screens[id]; okc {
		switch cont := c.(type) {
		case *screen.Inventory:
			if index >= 0 && index < len(cont.Slots) {
				s := cont.Slots[index]
				if s.ID >= 0 {
					return int(s.ID), int(s.Count), true
				}
			}
		}
	}
	return 0, 0, false
}

// itemMgrAdapter provides item names using registryid data as a fallback.
type itemMgrAdapter struct{}

func (itemMgrAdapter) GetItemNameByID(id int) string {
	// For tests, we don't need full item registry
	return fmt.Sprintf("item_%d", id)
}
