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

	// Handle Microsoft authentication if not in offline mode
	// This must happen before agent creation to get player name/UUID
	if !*offline {
		cid := "88650e7e-efee-4857-b9a9-cf580a00ef43"
		mauth, err := msauth.GetMCcredentials(".credCacheFile", cid)
		if err != nil {
			log.Fatalf("auth failed: %v", err)
		}
		log.Printf("Authenticated as %s (%s)", mauth.Name, mauth.UUID)
		auth = agent.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}
	}

	// Prepare rotating log for packet logging
	_ = os.MkdirAll("./logs", 0760)
	logWriter := &lumberjack.Logger{
		Filename:   "./logs/" + time.Now().Format(time.RFC3339) + "_receiver.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
		LocalTime:  true,
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
		LogWriter:              logWriter,
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
