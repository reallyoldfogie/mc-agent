// Package config is mc-agent's single unified settings surface: one
// Settings type with independently-optional sections (auth, connection,
// RCON, replay, skin, movement, follow-cam, RL training/env), loaded with
// JSON as the base, environment variables as the next override layer, and
// CLI flags as the final, highest-precedence layer — JSON -> ENV -> CLI.
//
// Replaces two narrower predecessors that had each independently grown a
// "how do I connect to a server" field set (address/name/uuid/version/
// offline/token/data-path/protocol-path) duplicating what cmd/agent's and
// cmd/ollama's own CLI flags already declared: the old auth-only
// config.AppConfig (configs/config.yaml) and rlconfig.Settings
// (configs/rl_training.json, docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 2).
// That drift risk was flagged directly and is what this package fixes —
// see docs/plans/UNIFIED_CONFIG_PLAN.md.
//
// Config *code* lives here; example config *data* lives in
// configs/config.example.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	crlconfig "github.com/reallyoldfogie/cRL-go/pkg/config"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// DefaultClientID is the Microsoft app ID from
// github.com/maxsupermanhd/go-mc-ms-auth, used when Auth.ClientID is unset.
const DefaultClientID = "88650e7e-efee-4857-b9a9-cf580a00ef43"

// Settings is the full app configuration surface. Every section is
// independently optional in a JSON config file — a command only reads the
// sections it actually needs (see docs/plans/UNIFIED_CONFIG_PLAN.md's
// per-command table); adding a new setting means adding one field to the
// relevant section once, not touching every command's own flag list.
type Settings struct {
	Auth       AuthSettings       `json:"auth"`
	Connection ConnectionSettings `json:"connection"`
	RCON       RCONSettings       `json:"rcon"`
	Replay     ReplaySettings     `json:"replay"`
	Skin       SkinSettings       `json:"skin"`
	Movement   MovementSettings   `json:"movement"`
	FollowCam  FollowCamSettings  `json:"follow_cam"`
	Logging    LoggingSettings    `json:"logging"`
	// Train is cRL-go's own training hyperparameters, used as-is — see
	// docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 2 on why this can't be
	// forked into an mc-agent-local type. Only relevant to cmd/rl-train.
	Train crlconfig.Settings `json:"train"`
	// Env configures the rlenv.Environment task(s) posed each training
	// episode. Only relevant to cmd/rl-train.
	Env EnvSettings `json:"env"`
	// Courier configures the set of servers cmd/item-courier logs into
	// simultaneously — see docs/plans/ITEM_TRANSFER_COURIER_PLAN.md. Only
	// relevant to cmd/item-courier; Connection/Auth still supply the single
	// shared player identity used to log into every one of them.
	Courier CourierSettings `json:"courier"`
}

// AuthSettings configures Microsoft authentication — see
// agent.ResolveAuth.
type AuthSettings struct {
	ClientID string `json:"client_id"`
	CacheDir string `json:"cache_dir"`
	// CredCacheFile, if set, is the exact path to the cached Microsoft
	// auth credentials file (the account's device-auth token/refresh
	// token, in plaintext JSON) — overrides the default
	// filepath.Join(CacheDir, ".credCacheFile"). Making this an explicit
	// path, not derived from the player/agent name, is deliberate: it's
	// what lets switching between Microsoft accounts/bot identities be
	// "point -config at a different file" rather than needing per-name
	// derivation logic — restores the behavior configs/config.yaml had
	// before the JSON config unification (docs/plans/UNIFIED_CONFIG_PLAN.md).
	CredCacheFile string `json:"cred_cache_file"`
}

// ConnectionSettings names the live server a command connects to.
type ConnectionSettings struct {
	Address          string `json:"address"`
	Name             string `json:"name"`
	UUID             string `json:"uuid"`
	Version          string `json:"version"`
	Offline          bool   `json:"offline"`
	Token            string `json:"token"`
	MCDataGenPath    string `json:"mc_data_gen_path"`
	MCProtocolGoPath string `json:"mc_protocol_go_path"`
}

// RCONSettings configures optional RCON access — follow-cam
// (agent.StartCamFollow) or RL episode seeding
// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4).
type RCONSettings struct {
	Address  string `json:"address"`
	Password string `json:"password"`
}

// ReplaySettings configures ReplayMod (.mcpr) recording.
type ReplaySettings struct {
	Enable    bool   `json:"enable"`
	Output    string `json:"output"`
	Generator string `json:"generator"`
}

// SkinSettings configures player-skin fetching/caching.
type SkinSettings struct {
	CacheDir     string `json:"cache_dir"`
	AllowNetwork bool   `json:"allow_network"`
}

// MovementSettings configures physics-based movement behavior.
type MovementSettings struct {
	EnableClutch bool `json:"enable_clutch"`
}

// FollowCamSettings configures spectator-mode camera auto-follow
// (agent.StartCamFollow) — requires RCON.
type FollowCamSettings struct {
	Target   string  `json:"target"`
	Distance float64 `json:"distance"`
}

// LoggingSettings configures the per-agent slog.Logger every *agent builds
// in New (agent/logging.go) and threads down into pathfinding/movement/
// physics/handler_versions — see
// docs/bugs/global-log-output-not-per-agent.md Phase 3. Replaces the old
// MC_AGENT_VERBOSE_LOG environment variable as the primary way to opt into
// high-frequency diagnostic logging: Level is read from the config file
// like every other setting here, with the usual ENV (e.g.
// MCAGENT_LOGGING_LEVEL) and CLI (-log-level) override layers on top,
// rather than a bespoke env-only reader.
type LoggingSettings struct {
	// Level is the minimum severity written to an agent's log file/stdout:
	// "debugverbose" (utils.LevelDebugVerbose — below debug, the old
	// MC_AGENT_VERBOSE_LOG behavior), "debug", "info" (default), "warn", or
	// "error". Case-insensitive; parsed by utils.ParseLevel.
	Level string `json:"level"`
}

// CourierSettings configures cmd/item-courier: the set of Minecraft servers
// a single courier process logs into simultaneously, under one shared player
// identity (Connection.Name/UUID/Offline/Token, Auth — see
// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md's "same identity on every server"
// assumption), relaying item_transfer:* plugin messages between whichever
// pair of them a given transfer names. Deliberately a list, not per-server
// CLI flags — the whole point of this section is that the count is
// arbitrary.
type CourierSettings struct {
	Servers []CourierServerSettings `json:"servers"`
}

// CourierServerSettings is one server a courier logs into.
type CourierServerSettings struct {
	// Label identifies this server and MUST match the mod-side serverId
	// concept from mc-item-transfer-mod's docs/protocol.md/trust-store
	// config — the same string an operator already types into that mod's
	// `/itemtransfer trust add <serverId> <pubkey>` on whichever other
	// server trusts this one, not an independently invented local name
	// (see the plan's Open Question 5).
	Label   string `json:"label"`
	Address string `json:"address"`
	// Version, if empty, is auto-detected — same convention as
	// ConnectionSettings.Version.
	Version string `json:"version"`
}

// EnvSettings is a JSON-friendly mirror of rlenv.Config — StepTimeout
// there is a time.Duration, awkward to hand-edit as raw nanoseconds, so
// this uses a plain float64 seconds count instead (see ToRlenvConfig).
// Migrated as-is from rlconfig.EnvSettings
// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phases 2 and 4).
type EnvSettings struct {
	TargetOffset       [3]float64 `json:"target_offset"`
	ArrivalThreshold   float64    `json:"arrival_threshold"`
	StepTimeoutSeconds float64    `json:"step_timeout_seconds"`
	MineTargetBlock    string     `json:"mine_target_block"`
	MineSearchRadius   int        `json:"mine_search_radius"`
	CraftTargetItem    string     `json:"craft_target_item"`
	// SeedEpisodes enables rlenv.DefaultEpisodeSeeder as this run's
	// rlenv.Config.Seeder — requires RCON.Address to be set (checked by
	// whichever command uses this, not here).
	SeedEpisodes bool `json:"seed_episodes"`

	// UseResetOrigin/ResetOrigin together mirror rlenv.Config.ResetOrigin's
	// *[3]float64 (JSON has no natural "unset" for a bare array — a plain
	// [0,0,0] is itself a plausible, valid origin, e.g. world spawn — so
	// this uses SeedEpisodes/Seeder's own bool-gate pattern instead of
	// overloading the zero value). Requires RCON.Address to be set (checked
	// by whichever command uses this, not here) — rlenv.Environment.Reset
	// also requires the live agent to implement rlenv.ResetAgent, which
	// *agent already satisfies unconditionally.
	UseResetOrigin bool       `json:"use_reset_origin"`
	ResetOrigin    [3]float64 `json:"reset_origin"`

	// StuckTimeout mirrors rlenv.Config.StuckTimeout directly — 0 (the
	// default) disables it, matching that field's own zero-value contract,
	// so no separate bool gate is needed here the way ResetOrigin needs one.
	StuckTimeout int `json:"stuck_timeout"`

	// Jitter/JitterSeed mirror rlenv.Config.Jitter/JitterSeed directly —
	// [3]float64{} (the default) disables jitter, matching that field's
	// own zero-value contract, so no separate bool gate is needed here
	// either.
	Jitter     [3]float64 `json:"jitter"`
	JitterSeed int64      `json:"jitter_seed"`
}

// ToRlenvConfig converts e into an rlenv.Config, ready to pass to
// rlenv.New. Seeder is set to rlenv.DefaultEpisodeSeeder iff
// e.SeedEpisodes.
func (e EnvSettings) ToRlenvConfig() rlenv.Config {
	cfg := rlenv.Config{
		TargetOffset:     e.TargetOffset,
		ArrivalThreshold: e.ArrivalThreshold,
		StepTimeout:      time.Duration(e.StepTimeoutSeconds * float64(time.Second)),
		MineTargetBlock:  e.MineTargetBlock,
		MineSearchRadius: e.MineSearchRadius,
		CraftTargetItem:  e.CraftTargetItem,
		StuckTimeout:     e.StuckTimeout,
		Jitter:           e.Jitter,
		JitterSeed:       e.JitterSeed,
	}
	if e.SeedEpisodes {
		cfg.Seeder = rlenv.DefaultEpisodeSeeder
	}
	if e.UseResetOrigin {
		origin := e.ResetOrigin
		cfg.ResetOrigin = &origin
	}
	return cfg
}

// Default returns out-of-the-box Settings. Fields with no sensible default
// (server address, player name, RL task targets) are left at their zero
// value — every command that needs one requires it be set explicitly via
// JSON/ENV/CLI.
func Default() Settings {
	return Settings{
		Auth:      AuthSettings{ClientID: DefaultClientID, CacheDir: "."},
		Replay:    ReplaySettings{Generator: "mc-agent"},
		Skin:      SkinSettings{CacheDir: "skins"},
		FollowCam: FollowCamSettings{Distance: 8.0},
		Train:     crlconfig.Default(),
		Env: EnvSettings{
			ArrivalThreshold:   1.5,
			StepTimeoutSeconds: 10,
			MineSearchRadius:   32,
		},
	}
}

// Load reads Settings from a JSON file at path, starting from Default()
// and overwriting only the fields present in the file. A missing file is
// not an error — every config file in this package is optional.
func Load(path string) (Settings, error) {
	settings := Default()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("config: reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	if settings.Auth.CacheDir == "" {
		settings.Auth.CacheDir = "."
	}
	if settings.Auth.CacheDir != "." {
		if err := os.MkdirAll(settings.Auth.CacheDir, 0700); err != nil {
			return Settings{}, fmt.Errorf("config: create cache directory %s: %w", settings.Auth.CacheDir, err)
		}
	}
	if settings.Auth.ClientID == "" {
		settings.Auth.ClientID = DefaultClientID
	}
	return settings, nil
}
