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

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/config"
	_ "github.com/reallyoldfogie/mc-agent/handler_versions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
	// _ "github.com/reallyoldfogie/mc-agent/handler_versions/common"
)

// envPrefix is this command's ENV-override prefix (config.ApplyEnv's
// second layer, docs/plans/UNIFIED_CONFIG_PLAN.md) — e.g.
// MCAGENT_CONNECTION_ADDRESS.
const envPrefix = "MCAGENT"

var (
	configPath  *string
	connOv      *config.ConnectionFlagOverrides
	rconOv      *config.RCONFlagOverrides
	replayOv    *config.ReplayFlagOverrides
	skinOv      *config.SkinFlagOverrides
	movementOv  *config.MovementFlagOverrides
	followCamOv *config.FollowCamFlagOverrides
	loggingOv   *config.LoggingFlagOverrides

	help = flag.Bool("help", false, "Display help")
)

func init() {
	configPath = config.RegisterConfigPathFlag(flag.CommandLine)
	connOv = config.RegisterConnectionFlags(flag.CommandLine)
	rconOv = config.RegisterRCONFlags(flag.CommandLine)
	replayOv = config.RegisterReplayFlags(flag.CommandLine)
	skinOv = config.RegisterSkinFlags(flag.CommandLine)
	movementOv = config.RegisterMovementFlags(flag.CommandLine)
	followCamOv = config.RegisterFollowCamFlags(flag.CommandLine)
	loggingOv = config.RegisterLoggingFlags(flag.CommandLine)
}

func main() {
	flag.Parse()

	if *help {
		flag.PrintDefaults()
		return
	}

	settings, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if err := config.ApplyEnv(&settings, envPrefix); err != nil {
		log.Fatalf("%v", err)
	}
	config.ApplyConnectionFlags(&settings.Connection, flag.CommandLine, connOv)
	config.ApplyRCONFlags(&settings.RCON, flag.CommandLine, rconOv)
	config.ApplyReplayFlags(&settings.Replay, flag.CommandLine, replayOv)
	config.ApplySkinFlags(&settings.Skin, flag.CommandLine, skinOv)
	config.ApplyMovementFlags(&settings.Movement, flag.CommandLine, movementOv)
	config.ApplyFollowCamFlags(&settings.FollowCam, flag.CommandLine, followCamOv)
	config.ApplyLoggingFlags(&settings.Logging, flag.CommandLine, loggingOv)

	logLevel, err := utils.ParseLevel(settings.Logging.Level)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Build Auth from flags (offline mode) or Microsoft authentication
	// (cached under settings.Auth.CacheDir) — see agent.ResolveAuth's doc
	// comment.
	auth, err := agent.ResolveAuth(settings.Connection.Offline, settings.Connection.Name, settings.Connection.UUID, settings.Connection.Token, settings.Auth)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if settings.Connection.Offline {
		fmt.Printf("Offline mode => using name=%s uuid=%s\n", auth.Name, auth.UUID)
	} else {
		log.Printf("Authenticated as %s (%s)", auth.Name, auth.UUID)
	}

	// Create skin provider for replay texture embedding
	skinProvider := agent.NewSkinFetcher(agent.SkinFetcherConfig{
		AllowNetwork: settings.Skin.AllowNetwork,
		CacheRoot:    settings.Skin.CacheDir,
		HTTPClient:   &http.Client{Timeout: 3 * time.Second},
	})

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
	if settings.Connection.Version == "" {
		detectedVersion, _, err := rof_utils.CheckServerVersion(settings.Connection.Address, 0)
		if err != nil {
			panic(fmt.Sprintf("auto-detect version from %s failed: %v", settings.Connection.Address, err))
		}
		settings.Connection.Version = detectedVersion
	}

	if settings.Replay.Output == "" {
		replayDir := filepath.Join(cacheDir, "replays", settings.Connection.Version)
		settings.Replay.Output = filepath.Join(replayDir, auth.Name+"_"+time.Now().Format("20060102_150405")+".mcpr")
	}

	// Dial RCON if configured - only needed for -follow-cam.
	camRCON, err := agent.DialRCON(context.Background(), settings.RCON.Address, settings.RCON.Password)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if camRCON != nil {
		log.Printf("Connected to RCON at %s", settings.RCON.Address)
	}

	// Build agent config - version detection, manager resolution, and client creation
	// are now handled automatically by agent.Init() if not provided
	cfg := models.AgentConfig{
		Name:               auth.Name, // Use authenticated name
		Address:            settings.Connection.Address,
		Version:            settings.Connection.Version, // Empty = auto-detect from server
		Auth:               auth,
		MCDataGenPath:      settings.Connection.MCDataGenPath,
		MCProtocolGoPath:   settings.Connection.MCProtocolGoPath,
		EnableClutchAssist: settings.Movement.EnableClutch,
		StopFilePath:       ".agentStop", // Enable graceful shutdown via stop file
		EnableReplay:       settings.Replay.Enable,
		ReplayOutput:       settings.Replay.Output,
		ReplayGenerator:    settings.Replay.Generator,
		SkinProvider:       skinProvider,
		LogWriter:          packetLogWriter,
		LogLevel:           logLevel,
		RCON:               camRCON,
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

	if settings.FollowCam.Target != "" {
		// A cam-follow failure isn't fatal to the agent process itself -
		// log it clearly and keep running normally, just without the
		// follow behavior. It's the operator's responsibility to ensure
		// RCON access and the necessary server permissions are in place.
		if err := a.StartCamFollow(ctx, settings.FollowCam.Target, settings.FollowCam.Distance); err != nil {
			log.Printf("cam-follow disabled: %v", err)
		}
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
