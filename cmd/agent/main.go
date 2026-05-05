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
	_ "github.com/reallyoldfogie/mc-agent/handler_versions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
	// _ "github.com/reallyoldfogie/mc-agent/handler_versions/common"
)

var (
	address      = flag.String("address", "127.0.0.1:25565", "The server address")
	name         = flag.String("name", "Daze", "The player's name")
	playerID     = flag.String("uuid", "", "The player's UUID")
	mcVersion    = flag.String("version", "", "target MC version (empty = auto-detect from server)")
	offline      = flag.Bool("offline", false, "use offline mode")
	accessToken  = flag.String("token", "", "AccessToken - offline mode only")
	mcDataPath   = flag.String("data-path", "", "Path to mc-data-gen data directory (empty=auto-detect; use 'build/cache/mc-data-gen' for centralized cache)")
	protoGoPath  = flag.String("protocol-path", "", "Path to mc-protocol-go directory")
	enableClutch = flag.Bool("clutch", false, "Enable clutch assist during physics movement")

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
	auth := models.Auth{}
	if *offline {
		auth = models.Auth{AccessToken: *accessToken, Name: *name, UUID: *playerID}
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
		auth = models.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}
	}

	// Prepare rotating log for packet logging
	cacheDir, err := utils.FindOrCreateCacheDir()
	if err != nil {
		log.Fatalf("find cache directory: %v", err)
	}
	packetLogsDir := filepath.Join(cacheDir, "logs", "packets")
	_ = os.MkdirAll(packetLogsDir, 0760)
	packetLogWriter := &lumberjack.Logger{
		Filename:   filepath.Join(packetLogsDir, auth.Name+"_"+time.Now().Format("20060102_150405")+".log"),
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
		replayDir := filepath.Join(cacheDir, "replays", *mcVersion)
		*replayOut = filepath.Join(replayDir, auth.Name+"_"+time.Now().Format("20060102_150405")+".mcpr")
	}

	// Build agent config - version detection, manager resolution, and client creation
	// are now handled automatically by agent.Init() if not provided
	cfg := models.AgentConfig{
		Name:               auth.Name, // Use authenticated name
		Address:            *address,
		Version:            *mcVersion, // Empty = auto-detect from server
		Auth:               auth,
		MCDataGenPath:      *mcDataPath,
		MCProtocolGoPath:   *protoGoPath,
		EnableClutchAssist: *enableClutch,
		StopFilePath:       ".agentStop", // Enable graceful shutdown via stop file
		EnableReplay:       *enableReplay,
		ReplayOutput:       *replayOut,
		ReplayGenerator:    *replayGenerator,
		SkinProvider:       skinProvider,
		LogWriter:          packetLogWriter,
	}

	a, err := agent.New(cfg)
	if err != nil {
		log.Fatalf("failed creating agent: %v", err)
	}

	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := a.Init(ctx); err != nil {
		log.Printf("init failed: %v", err)
		exitCode = 1
		return
	}

	if err := a.Start(ctx); err != nil {
		log.Printf("start failed: %v", err)
		exitCode = 1
		return
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
		exitCode = 1
	}

	if critErr := a.CriticalError(); critErr != nil {
		log.Printf("Critical error: %v", critErr)
		exitCode = 1
	}
}
