package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	registryid "github.com/Tnze/go-mc/data/registryid"
	msauth "github.com/maxsupermanhd/go-mc-ms-auth"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
	"github.com/reallyoldfogie/mc-bot-go/bot/msg"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-bot-go/bot/world"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/following"
	"github.com/reallyoldfogie/mc-agent/movement"
	pf "github.com/reallyoldfogie/mc-agent/pathfinding"
	agutils "github.com/reallyoldfogie/mc-agent/utils"
)

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
	if id >= 0 && id < len(registryid.Item) {
		return registryid.Item[id]
	}
	return ""
}

var (
	address     = flag.String("address", "127.0.0.1:25565", "The server address")
	name        = flag.String("name", "Daze", "The player's name")
	playerID    = flag.String("uuid", "", "The player's UUID")
	mcVersion   = flag.String("version", "", "target MC version (empty = auto-detect from server)")
	offline     = flag.Bool("offline", false, "use offline mode")
	accessToken = flag.String("token", "", "AccessToken - offline mode only")
	mcDataPath  = flag.String("data-path", "", "Path to mc-data-gen data directory")
	protoGoPath = flag.String("protocol-path", "", "Path to mc-protocol-go directory")

	// replay flags
	enableReplay    = flag.Bool("replay", false, "Enable ReplayMod recording (.mcpr)")
	replayOut       = flag.String("replay-out", "session.mcpr", "Replay output file path")
	replayGenerator = flag.String("replay-generator", "mc-agent", "Replay generator string")

	skinCacheDir   = flag.String("skin-cache", "skins", "Directory to cache player/default skins")
	skinNetEnabled = flag.Bool("skin-net", false, "Allow network skin fetches from Mojang (default off)")
)

func main() {
	flag.Parse()

	// Build Auth from flags (offline support - online auth is handled later, when bot client is created)
	auth := agent.Auth{}
	if *offline {
		auth = agent.Auth{AsTk: *accessToken, Name: *name, UUID: *playerID}
		fmt.Printf("Offline mode => using name=%s uuid=%s\n", auth.Name, auth.UUID)
	}

	// Create skin provider for replay texture embedding
	skinProvider := agent.NewSkinFetcher(agent.SkinFetcherConfig{
		AllowNetwork: *skinNetEnabled,
		CacheRoot:    *skinCacheDir,
		HTTPClient:   &http.Client{Timeout: 3 * time.Second},
	})

	cfg := agent.Config{
		Address:          *address,
		Version:          *mcVersion,
		Auth:             auth,
		MCDataGenPath:    *mcDataPath,
		MCProtocolGoPath: *protoGoPath,

		// Managers (PacketMgr/BlockMgr/SoundMgr) can be injected later when migrated.
		EnablePathfinding: true,
		EnableFollowing:   true,
		StopFilePath:      ".agentStop", // Enable graceful shutdown via stop file
		EnableReplay:      *enableReplay,
		ReplayOutput:      *replayOut,
		ReplayGenerator:   *replayGenerator,
		SkinProvider:      skinProvider,
	}

	// Auto-detect server version if not specified, otherwise resolve protocol from version
	if cfg.Version == "" {
		v, proto, err := rof_utils.CheckServerVersion(cfg.Address, 0)
		if err != nil {
			log.Fatalf("version detect failed: %v", err)
		}
		cfg.Version = v
		cfg.ProtocolVersion = proto
		log.Printf("Auto-detected server version %s (protocol %d)", v, proto)
	} else {
		// Version specified: resolve protocol from version string
		if proto, ok := mc_versions.VersionProtocol[cfg.Version]; ok {
			cfg.ProtocolVersion = proto
			log.Printf("Using specified version %s (protocol %d)", cfg.Version, proto)
		} else {
			log.Fatalf("unsupported version: %s", cfg.Version)
		}
	}

	// Resolve packet manager for version (best effort).
	// This enables handler ID resolution and constructing the bot client.
	packetMgr := mc_versions.GetPacketMgrForVersion(cfg.Version)
	if packetMgr != nil {
		cfg.PacketMgr = packetMgr
	}
	// Resolve sound manager for version (best effort) and inject for logging.
	soundMgr := mc_versions.GetSoundMgrForVersion(cfg.Version)
	if soundMgr != nil {
		cfg.SoundMgr = soundMgr
	}

	// Prepare rotating log for packet logging
	_ = os.MkdirAll("./logs", 0760)
	cfg.LogWriter = &lumberjack.Logger{
		Filename:   "./logs/" + time.Now().Format(time.RFC3339) + "_receiver.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
		LocalTime:  true,
	}

	// Construct a real bot client and inject it.
	var client *bot.Client
	if packetMgr != nil {
		client = bot.NewClient(packetMgr)
		// Apply auth
		if *offline {
			client.Auth = bot.Auth{AsTk: auth.AsTk, Name: auth.Name, UUID: auth.UUID}
		} else {
			cid := "88650e7e-efee-4857-b9a9-cf580a00ef43"
			mauth, err := msauth.GetMCcredentials(".credCacheFile", cid)
			if err != nil {
				log.Fatalf("auth failed: %v", err)
			}
			log.Printf("Authenticated as %s (%s)", mauth.Name, mauth.UUID)
			client.Auth = bot.Auth{AsTk: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}
			cfg.Auth = agent.Auth{AsTk: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}
		}
		cfg.Client = agent.NewClientFromBot(client)
	}

	a, err := agent.New(cfg)
	if err != nil {
		log.Fatalf("failed creating agent: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := a.Init(ctx); err != nil {
		log.Fatalf("init failed: %v", err)
	}

	// If we have a concrete client, wire minimal subsystems for chat and chunk events.
	if client != nil && packetMgr != nil {
		// Player with basic events; keep most callbacks nil for now.
		// Create custom settings with increased render distance
		customSettings := basic.DefaultSettings
		customSettings.ViewDistance = 32 // Maximum render distance (2-32)
		customSettings.Locale = "en_us"

		player := basic.NewPlayer(client, customSettings, basic.EventsListener{
			GameStart:    a.HandleGameStart,
			Disconnect:   a.HandleDisconnect,
			HealthChange: a.HandleHealthChange,
			Death:        a.HandleDeath,
			Teleported:   a.HandleTeleported,
		}, packetMgr)

		// Expose teleport accepter to agent.
		a.SetTeleportAccepter(player)
		// Also expose respawner through same object
		// (basic.Player implements Respawn())

		// Player list for chat manager
		plist := playerlist.New(client, packetMgr)

		// Chat manager with agent event handlers
		chatMgr := msg.New(client, player, plist, msg.EventsHandler{
			SystemChat:        a.OnSystemChat,
			PlayerChatMessage: a.OnPlayerChat,
			DisguisedChat:     a.OnDisguisedChat,
		}, packetMgr)
		a.SetChat(agent.NewChatFromMsg(chatMgr))

		// World manager with chunk load/unload callbacks pointing to agent methods
		wm := world.NewWorld(client, player, world.EventsListener{LoadChunk: a.HandleChunkLoad, UnloadChunk: a.HandleChunkUnload}, packetMgr)

		// Screen manager: wire slot change and provide slot resolver
		scr := screen.NewManager(client, screen.EventsListener{Open: nil, SetSlot: a.OnScreenSlotChange, Close: nil}, packetMgr)
		a.SetSlotResolver(slotResolver{m: scr})
		a.SetItemManager(itemMgrAdapter{})

		// Provide player UUID resolver for following
		a.SetPlayerUUIDResolver(func(name string) ([16]byte, error) {
			for uuid, info := range plist.PlayerInfos {
				if info.Name == name {
					return uuid, nil
				}
			}
			return [16]byte{}, fmt.Errorf("player %s not found", name)
		})

		// Provide name-by-UUID resolver for tracking messages
		a.SetPlayerNameResolver(func(u [16]byte) (string, bool) {
			for uuid, info := range plist.PlayerInfos {
				var cu [16]byte
				copy(cu[:], uuid[:])
				if cu == u {
					return info.Name, true
				}
			}
			return "", false
		})

		// Optional: follow system wiring if pathfinding enabled
		if cfg.EnablePathfinding {
			dataBasePath, err := agutils.ResolveDataPath(cfg.MCDataGenPath, "./data/mc-data-gen-cache", "../mc-data-gen/data")
			if err != nil {
				log.Printf("Warning: Failed to resolve mc-data-gen data path: %v", err)
			} else {
				shapeMgr, err := pf.NewBlockShapeManager(cfg.Version, dataBasePath)
				if err != nil {
					log.Printf("Warning: Failed to initialize BlockShapeManager: %v", err)
				} else {
					var stateProps *pf.StatePropertyLoader
					if cfg.MCProtocolGoPath != "" {
						if spl, err := pf.NewStatePropertyLoader(cfg.MCProtocolGoPath, cfg.Version); err == nil {
							stateProps = spl
						} else {
							log.Printf("Warning: state props load failed: %v", err)
						}
					}

					blockMgr := mc_versions.GetBlockMgrForVersion(cfg.Version)
					pathFinder := pf.NewPathFinder(wm, shapeMgr, blockMgr, stateProps)
					a.SetPathFinder(pathFinder)

					moveExec := movement.NewMovementExecutor(client, packetMgr, a.GetPosition, a.UpdatePosition, a.GetEntityID)
					a.SetMovementExecutor(moveExec)

					targetSelector := following.NewTargetSelector(a.GetTrackedEntitiesForFollowing, a.ResolvePlayerUUIDByName, a.GetPositionSimple)
					followCfg := following.DefaultFollowConfig()
					followMgr := following.NewFollowManager(targetSelector, pathFinder, moveExec, a.GetPosition, a.SendChat, followCfg)
					a.SetFollowManager(followMgr)
					a.SetTargetSelector(targetSelector)
				}
			}
		}
	}

	if err := a.Start(ctx); err != nil {
		log.Fatalf("start failed: %v", err)
	}

	agentDone := a.Done()
	if agentDone == nil {
		<-ctx.Done()
	} else {
		select {
		case <-ctx.Done():
		case <-agentDone:
		}
	}

	if err := a.Close(context.Background()); err != nil {
		log.Printf("close error: %v", err)
	}
}
