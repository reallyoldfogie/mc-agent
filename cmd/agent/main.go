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

	msauth "github.com/maxsupermanhd/go-mc-ms-auth"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/reallyoldfogie/mc-agent/agent"
)

var (
	address        = flag.String("address", "127.0.0.1:25565", "The server address")
	name           = flag.String("name", "Daze", "The player's name")
	playerID       = flag.String("uuid", "", "The player's UUID")
	mcVersion      = flag.String("version", "", "target MC version (empty = auto-detect from server)")
	offline        = flag.Bool("offline", false, "use offline mode")
	accessToken    = flag.String("token", "", "AccessToken - offline mode only")
	mcDataPath     = flag.String("data-path", "", "Path to mc-data-gen data directory")
	protoGoPath    = flag.String("protocol-path", "", "Path to mc-protocol-go directory")
	disablePhysics = flag.Bool("disable-physics", false, "Disable physics-based movement (defaults to physics enabled)")
	enableClutch   = flag.Bool("clutch", false, "Enable clutch assist during physics movement")

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
		auth = agent.Auth{AccessToken: *accessToken, Name: *name, UUID: *playerID}
		fmt.Printf("Offline mode => using name=%s uuid=%s\n", auth.Name, auth.UUID)
	}

	// Create skin provider for replay texture embedding
	skinProvider := agent.NewSkinFetcher(agent.SkinFetcherConfig{
		AllowNetwork: *skinNetEnabled,
		CacheRoot:    *skinCacheDir,
		HTTPClient:   &http.Client{Timeout: 3 * time.Second},
	})

	cfg := agent.Config{
		Address:                *address,
		Version:                *mcVersion,
		Auth:                   auth,
		MCDataGenPath:          *mcDataPath,
		MCProtocolGoPath:       *protoGoPath,
		DisablePhysicsMovement: *disablePhysics,
		EnableClutchAssist:     *enableClutch,

		// Managers (PacketMgr/BlockMgr/SoundMgr) can be injected later when migrated.
		StopFilePath:    ".agentStop", // Enable graceful shutdown via stop file
		EnableReplay:    *enableReplay,
		ReplayOutput:    *replayOut,
		ReplayGenerator: *replayGenerator,
		SkinProvider:    skinProvider,
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
	var client bot.Client
	if packetMgr != nil {
		client = bot.NewClient(packetMgr)
		// Apply auth
		if *offline {
			client.SetAuth(bot.Auth{AccessToken: auth.AccessToken, Name: auth.Name, UUID: auth.UUID})
		} else {
			cid := "88650e7e-efee-4857-b9a9-cf580a00ef43"
			mauth, err := msauth.GetMCcredentials(".credCacheFile", cid)
			if err != nil {
				log.Fatalf("auth failed: %v", err)
			}
			log.Printf("Authenticated as %s (%s)", mauth.Name, mauth.UUID)
			client.SetAuth(bot.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID})
			cfg.Auth = agent.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}
		}
		cfg.Client = client // agent.NewClientFromBot(client)
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
