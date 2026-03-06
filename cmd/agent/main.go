package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	msauth "github.com/maxsupermanhd/go-mc-ms-auth"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/config"
	_ "github.com/reallyoldfogie/mc-agent/versions"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
	// _ "github.com/reallyoldfogie/mc-agent/versions/common"
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
	replayOut       = flag.String("replay-out", "", "Replay output file path")
	replayGenerator = flag.String("replay-generator", "mc-agent", "Replay generator string")

	skinCacheDir   = flag.String("skin-cache", "skins", "Directory to cache player/default skins")
	skinNetEnabled = flag.Bool("skin-net", false, "Allow network skin fetches from Mojang (default off)")

	help = flag.Bool("help", false, "Display help")
)

func main() {
	flag.Parse()

	if *help {
		flag.PrintDefaults()
		return
	}

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

	// Handle Microsoft authentication if not in offline mode
	// This must happen before agent creation to get player name/UUID
	if !*offline {
		cfg, err := config.Load("configs/config.yaml")
		if err != nil {
			log.Fatalf("config load failed: %v", err)
		}

		credCachePath := filepath.Join(cfg.CacheDir, ".credCacheFile")
		fmt.Printf("Using credential cache path: %s\n", credCachePath)

		mauth, err := msauth.GetMCcredentials(credCachePath, cfg.ClientID)
		if err != nil {
			log.Fatalf("auth failed: %v", err)
		}
		log.Printf("Authenticated as %s (%s)", mauth.Name, mauth.UUID)
		auth = agent.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}
	}

	// Prepare rotating log for packet logging
	_ = os.MkdirAll("./logs", 0760)
	packetLogWriter := &lumberjack.Logger{
		Filename:   filepath.Join(".", "logs", "packets", auth.Name+"_"+time.Now().Format("20060102_150405")+".log"),
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
		LocalTime:  true,
	}

	log.Printf("Packet log: %s", packetLogWriter.Filename)

	// auto-detect version (if not provided)
	if *mcVersion == "" {
		detectedVersion, _, err := rof_utils.CheckServerVersion(*address, 0)
		if err != nil {
			panic(fmt.Sprintf("auto-detect version from %s failed: %v", *address, err))
		}
		*mcVersion = detectedVersion
	}

	if *replayOut == "" {
		*replayOut = filepath.Join(".", "replays", *mcVersion, auth.Name+"_"+time.Now().Format("20060102_150405")+".mcpr")
	}

	// Build agent config - version detection, manager resolution, and client creation
	// are now handled automatically by agent.Init() if not provided
	cfg := agent.Config{
		Name:                   auth.Name, // Use authenticated name
		Address:                *address,
		Version:                *mcVersion, // Empty = auto-detect from server
		Auth:                   auth,
		MCDataGenPath:          *mcDataPath,
		MCProtocolGoPath:       *protoGoPath,
		DisablePhysicsMovement: *disablePhysics,
		EnableClutchAssist:     *enableClutch,
		StopFilePath:           ".agentStop", // Enable graceful shutdown via stop file
		EnableReplay:           *enableReplay,
		ReplayOutput:           *replayOut,
		ReplayGenerator:        *replayGenerator,
		SkinProvider:           skinProvider,
		LogWriter:              packetLogWriter,
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
