// Command rl-train trains a REINFORCE policy (github.com/reallyoldfogie/cRL-go/pkg/reinforce)
// against a live mc-agent bot session, through rlenv.Environment — closing
// docs/plans/RL_POLICY_INTEGRATION_PLAN.md item 6 (the training entrypoint)
// per docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 3.
//
// Structurally, this mirrors cRL-go's own cmd/train/main.go closely (the
// per-epoch RunEpoch loop, checkpoint-dir resume/save, metrics CSV) — the
// difference is the environment: instead of a cheap, stateless toy
// environment constructed fresh per rollout (reinforce.EnvFactory), this
// command connects exactly one live bot session and drives it via
// reinforce.NewWithPersistentEnv (one long-lived rlenv.Environment reused
// across every episode, Settings.Train.Workers forced to 1 — see
// rlenv/environment.go's own doc comment on why).
//
// Algorithm choice: this command uses pkg/reinforce (plain REINFORCE), not
// pkg/ppo — RL_TRAINING_LOOP_PLAN.md Phase 3 flagged this as an open
// decision given how expensive a live-bot episode is compared to a toy
// environment (sample efficiency likely matters more here). REINFORCE was
// chosen for this first implementation because it's the simpler of the two
// and rlenv/doc.go's own header comment names it first; a pkg/ppo-based
// alternative (mirroring cRL-go's own cmd/train-ppo) is a reasonable
// follow-up if REINFORCE's sample efficiency proves too poor against real
// episode costs — swapping is a new command, not a rewrite of this one,
// since rlenv.Environment itself is algorithm-agnostic.
//
// Configuration is the shared config.Settings (docs/plans/UNIFIED_CONFIG_PLAN.md):
// JSON config file -> environment variables (MCAGENT_ prefix) -> CLI flags,
// each layer overriding the last. Only the sections this command actually
// uses are registered as CLI flags (Connection, RCON, Env, Train) — see
// config/flags.go's own per-section doc comment.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/reallyoldfogie/cRL-go/pkg/checkpoint"
	"github.com/reallyoldfogie/cRL-go/pkg/metrics"
	"github.com/reallyoldfogie/cRL-go/pkg/policy"
	"github.com/reallyoldfogie/cRL-go/pkg/reinforce"
	"github.com/reallyoldfogie/cRL-go/pkg/rl"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/config"
	_ "github.com/reallyoldfogie/mc-agent/handler_versions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	"github.com/reallyoldfogie/mc-agent/utils"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
)

// envPrefix is this command's ENV-override prefix (config.ApplyEnv's
// second layer) — e.g. MCAGENT_CONNECTION_ADDRESS.
const envPrefix = "MCAGENT"

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rl-train", flag.ContinueOnError)
	trainOverrides := config.RegisterTrainFlags(fs) // also registers -config
	connOverrides := config.RegisterConnectionFlags(fs)
	rconOverrides := config.RegisterRCONFlags(fs)
	envOverrides := config.RegisterEnvFlags(fs)
	checkpointIn := fs.String("checkpoint-in", "", "path to a checkpoint (see -checkpoint-out) to resume training from, instead of a fresh policy (optional)")
	checkpointOut := fs.String("checkpoint-out", "", "path to save the trained policy weights to after training completes (optional)")
	checkpointDir := fs.String("checkpoint-dir", "", "directory to auto-resume the latest checkpoint from, and periodically save numbered checkpoints into (optional; independent of -checkpoint-in/-checkpoint-out)")
	checkpointInterval := fs.Int("checkpoint-interval", 50, "save a checkpoint to -checkpoint-dir every N epochs (only used if -checkpoint-dir is set)")
	metricsOut := fs.String("metrics-out", "", "path to write one CSV row of per-epoch metrics to (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	settings, err := config.Load(trainOverrides.ConfigPath)
	if err != nil {
		return err
	}
	if err := config.ApplyEnv(&settings, envPrefix); err != nil {
		return err
	}
	config.ApplyTrainFlags(&settings.Train, fs, trainOverrides)
	config.ApplyConnectionFlags(&settings.Connection, fs, connOverrides)
	config.ApplyRCONFlags(&settings.RCON, fs, rconOverrides)
	config.ApplyEnvFlags(&settings.Env, fs, envOverrides)
	if err := validate(settings); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := connectAgent(ctx, settings)
	if err != nil {
		return err
	}
	defer func() {
		if err := a.Close(context.Background()); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// A live bot session can end on its own (e.g. the .agentStop file —
	// see models.AgentConfig.StopFilePath) independent of this process's
	// own signal handling; treat that the same as SIGINT/SIGTERM by
	// canceling the training context so the epoch loop below stops
	// gracefully (and still saves a final checkpoint) instead of the next
	// Step erroring against a session that already tore itself down.
	if done := a.Done(); done != nil {
		go func() {
			select {
			case <-ctx.Done():
			case <-done:
				stop()
			}
		}()
	}

	liveAgent, ok := a.(rlenv.LiveAgent)
	if !ok {
		return fmt.Errorf("agent does not satisfy rlenv.LiveAgent (missing InventoryCount/Craftable/BlockNameAt/HealthProvider?)")
	}
	registry := actions.NewRegistry()
	envCfg := settings.Env.ToRlenvConfig()

	// One Environment instance, constructed once and reused for the whole
	// run: PersistentEnvFactory is only ever invoked once by
	// reinforce.NewWithPersistentEnv (see its own doc comment — rollout
	// collection then resets this same instance between episodes rather
	// than rebuilding it), so there's no reason to defer construction into
	// the closure the way a per-episode EnvFactory would.
	env, err := rlenv.New(liveAgent, registry, envCfg)
	if err != nil {
		return fmt.Errorf("constructing environment: %w", err)
	}
	persistentFactory := func(*rand.Rand) (rl.Environment, error) {
		return env, nil
	}

	// environmentID identifies the observation/action-space shape a
	// checkpoint was trained against (see rlenv/observation.go's
	// observationSize, rlenv/action.go's NumActions), so a checkpoint
	// saved against one rlenv version can't be silently loaded into an
	// incompatible one — mirrors cRL-go/cmd/train's own
	// "%s:%d" (envName, gridSize) environmentID exactly, adapted since
	// rlenv has neither.
	environmentID := fmt.Sprintf("mc-agent-rlenv:actions=%d:obs=%d", env.ActionSpace(), env.ObservationSize())

	resume, err := resumeFromCheckpointDir(*checkpointDir, environmentID)
	if err != nil {
		return err
	}

	initialParams := resume.Params
	if initialParams != nil && *checkpointIn != "" {
		return fmt.Errorf("both -checkpoint-in and an existing checkpoint in -checkpoint-dir were found; use only one to resume from")
	}
	if initialParams == nil {
		initialParams, err = loadInitialParams(*checkpointIn, environmentID)
		if err != nil {
			return err
		}
	}

	trainer, err := reinforce.NewWithPersistentEnv(settings.Train, persistentFactory, initialParams)
	if err != nil {
		return err
	}

	var metricsWriter *metrics.CSVWriter
	if *metricsOut != "" {
		metricsWriter, err = metrics.NewCSVWriter(*metricsOut, []string{"epoch", "average_return", "sample_count", "return_std"})
		if err != nil {
			return err
		}
	}

	bestReturn := resume.BestReturn
	totalUpdates := resume.TotalUpdates
	lastCompletedEpoch := resume.StartEpoch - 1

	for epoch := resume.StartEpoch; epoch < settings.Train.Epochs; epoch++ {
		if err := ctx.Err(); err != nil {
			log.Printf("stopping before epoch %d: %v", epoch, err)
			break
		}

		stats, err := trainer.RunEpoch(ctx, epoch)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("training interrupted during epoch %d: %v", epoch, err)
				break
			}
			return fmt.Errorf("epoch %d: %w", epoch, err)
		}
		lastCompletedEpoch = epoch
		totalUpdates += stats.UpdateCount
		if stats.AverageReturn > bestReturn {
			bestReturn = stats.AverageReturn
		}

		log.Printf("Epoch %d | Average return: %.3f | Samples: %d | Return std: %.3f",
			stats.Epoch, stats.AverageReturn, stats.SampleCount, stats.ReturnStd)

		if metricsWriter != nil {
			if err := metricsWriter.WriteRow(stats.Epoch, stats.AverageReturn, stats.SampleCount, stats.ReturnStd); err != nil {
				return err
			}
		}

		interval := *checkpointInterval
		if *checkpointDir != "" && interval > 0 && (epoch+1)%interval == 0 {
			if err := saveCheckpointToDir(*checkpointDir, trainer.Params(), environmentID, epoch, bestReturn, totalUpdates); err != nil {
				return fmt.Errorf("saving periodic checkpoint: %w", err)
			}
		}
	}

	if metricsWriter != nil {
		if err := metricsWriter.Close(); err != nil {
			return err
		}
	}

	if *checkpointDir != "" && lastCompletedEpoch >= resume.StartEpoch {
		if err := saveCheckpointToDir(*checkpointDir, trainer.Params(), environmentID, lastCompletedEpoch, bestReturn, totalUpdates); err != nil {
			return fmt.Errorf("saving final checkpoint: %w", err)
		}
	}

	if *checkpointOut != "" {
		metadata := checkpoint.Metadata{Epoch: lastCompletedEpoch, BestReturn: bestReturn, TotalUpdates: totalUpdates}
		if err := policy.SaveFile(*checkpointOut, trainer.Params(), environmentID, metadata); err != nil {
			return fmt.Errorf("saving checkpoint: %w", err)
		}
	}
	return nil
}

// validate checks exactly what this command needs from the shared
// Settings type — deliberately not a method on config.Settings itself,
// since e.g. "Train.Workers must be 1" only applies to a single live bot
// session, not to every command that happens to load a Train section (see
// docs/plans/UNIFIED_CONFIG_PLAN.md).
func validate(s config.Settings) error {
	if err := s.Train.Validate(); err != nil {
		return err
	}
	if s.Train.Workers != 1 {
		return fmt.Errorf("rl-train: train.workers must be 1 for a single live bot session, got %d", s.Train.Workers)
	}
	if s.Env.ArrivalThreshold <= 0 {
		return fmt.Errorf("rl-train: env.arrival_threshold must be > 0")
	}
	if s.Env.StepTimeoutSeconds <= 0 {
		return fmt.Errorf("rl-train: env.step_timeout_seconds must be > 0")
	}
	if s.Connection.Address == "" {
		return fmt.Errorf("rl-train: connection.address must be set")
	}
	if s.Connection.Offline && s.Connection.Name == "" {
		// UUID is deliberately not required: an empty UUID in offline
		// mode is an established pattern elsewhere (testing/framework.go)
		// — the server generates one.
		return fmt.Errorf("rl-train: connection.name must be set when connection.offline is true")
	}
	if s.Env.SeedEpisodes && s.RCON.Address == "" {
		return fmt.Errorf("rl-train: rcon.address must be set when env.seed_episodes is true")
	}
	return nil
}

// loadInitialParams loads the checkpoint at checkpointPath if one was
// given, returning nil (meaning "initialize fresh") if checkpointPath is
// empty — identical to cRL-go/cmd/train's own helper of the same name.
func loadInitialParams(checkpointPath, expectedEnvironmentID string) (*policy.Params, error) {
	if checkpointPath == "" {
		return nil, nil
	}
	params, _, err := policy.LoadFile(checkpointPath, expectedEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("loading checkpoint: %w", err)
	}
	return params, nil
}

// connectAgent establishes one live bot session from settings, following
// cmd/agent/main.go's own connection bootstrap (auth resolution, packet
// logging, version auto-detect, RCON dial, agent.New/Init/Start) — see
// agent.ResolveAuth/agent.DialRCON's doc comments for the two pieces
// factored out of that command so this one doesn't duplicate them.
func connectAgent(ctx context.Context, settings config.Settings) (models.Agent, error) {
	conn := settings.Connection

	auth, err := agent.ResolveAuth(conn.Offline, conn.Name, conn.UUID, conn.Token, settings.Auth)
	if err != nil {
		return nil, err
	}
	if conn.Offline {
		log.Printf("Offline mode => using name=%s uuid=%s", auth.Name, auth.UUID)
	} else {
		log.Printf("Authenticated as %s (%s)", auth.Name, auth.UUID)
	}

	cacheDir, err := utils.FindOrCreateCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find cache directory: %w", err)
	}
	packetLogsDir := filepath.Join(cacheDir, "logs", "packets")
	_ = os.MkdirAll(packetLogsDir, 0760)
	packetLogWriter := &lumberjack.Logger{
		Filename:   filepath.Join(packetLogsDir, auth.Name+"_rl-train_"+time.Now().Format("20060102_150405")+".log"),
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
		LocalTime:  true,
	}
	log.Printf("Packet log: %s", packetLogWriter.Filename)

	version := conn.Version
	if version == "" {
		detectedVersion, _, err := rof_utils.CheckServerVersion(conn.Address, 0)
		if err != nil {
			return nil, fmt.Errorf("auto-detect version from %s: %w", conn.Address, err)
		}
		version = detectedVersion
	}

	rcon, err := agent.DialRCON(ctx, settings.RCON.Address, settings.RCON.Password)
	if err != nil {
		return nil, err
	}
	if rcon != nil {
		log.Printf("Connected to RCON at %s", settings.RCON.Address)
	}

	cfg := models.AgentConfig{
		Name:             auth.Name,
		Address:          conn.Address,
		Version:          version,
		Auth:             auth,
		MCDataGenPath:    conn.MCDataGenPath,
		MCProtocolGoPath: conn.MCProtocolGoPath,
		StopFilePath:     ".agentStop",
		LogWriter:        packetLogWriter,
		RCON:             rcon,
		SkinProvider: agent.NewSkinFetcher(agent.SkinFetcherConfig{
			AllowNetwork: settings.Skin.AllowNetwork,
			CacheRoot:    settings.Skin.CacheDir,
			HTTPClient:   &http.Client{Timeout: 3 * time.Second},
		}),
	}

	a, err := agent.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}
	if err := a.Init(ctx); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if err := a.Start(ctx); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return a, nil
}
