package config

import (
	"flag"

	crlconfig "github.com/reallyoldfogie/cRL-go/pkg/config"
)

// DefaultConfigPath is this package's default -config flag value —
// registered directly by RegisterConfigPathFlag, or (for a command that
// also registers Train's flags via crlconfig.RegisterFlags, which
// registers its own "-config" flag) applied as an override afterward, to
// avoid a "flag redefined" panic from two competing -config
// registrations. See RegisterTrainFlags.
const DefaultConfigPath = "configs/config.json"

// Every RegisterXFlags/ApplyXFlags pair below follows the same contract:
// numeric/bool flags default to their zero value, and a flag only takes
// effect if the caller actually passed it (checked via fs.Visit, which
// only visits flags that were set) — so an explicitly-passed zero value
// (e.g. "-mine-search-radius=0") is still honored, while an un-passed flag
// never clobbers whatever JSON/ENV already set. This is CLI's own layer,
// the last and highest-precedence one in JSON -> ENV -> CLI.
//
// Each command composes only the registrars it actually needs — see
// docs/plans/UNIFIED_CONFIG_PLAN.md's per-command table (cmd/rl-train
// doesn't need -replay-out; cmd/agent doesn't need -mine-target-block).

// RegisterConfigPathFlag registers a plain "-config" flag for a command
// that does NOT also call RegisterTrainFlags (which registers its own,
// via crlconfig.RegisterFlags) — calling both on the same FlagSet would
// panic on the duplicate flag name.
func RegisterConfigPathFlag(fs *flag.FlagSet) *string {
	return fs.String("config", DefaultConfigPath, "path to a JSON config file (see Settings)")
}

// --- Connection ---

type ConnectionFlagOverrides struct {
	Address, Name, UUID, Version, Token, MCDataGenPath, MCProtocolGoPath string
	Offline                                                              bool
}

func RegisterConnectionFlags(fs *flag.FlagSet) *ConnectionFlagOverrides {
	o := &ConnectionFlagOverrides{}
	fs.StringVar(&o.Address, "address", "", "server address (overrides config file/env)")
	fs.StringVar(&o.Name, "name", "", "player name (overrides config file/env)")
	fs.StringVar(&o.UUID, "uuid", "", "player UUID, offline mode only (overrides config file/env)")
	fs.StringVar(&o.Version, "version", "", "target MC version, empty = auto-detect (overrides config file/env)")
	fs.BoolVar(&o.Offline, "offline", false, "use offline mode (overrides config file/env)")
	fs.StringVar(&o.Token, "token", "", "access token, offline mode only (overrides config file/env)")
	fs.StringVar(&o.MCDataGenPath, "data-path", "", "path to mc-data-gen data directory (overrides config file/env)")
	fs.StringVar(&o.MCProtocolGoPath, "protocol-path", "", "path to mc-protocol-go directory (overrides config file/env)")
	return o
}

func ApplyConnectionFlags(s *ConnectionSettings, fs *flag.FlagSet, o *ConnectionFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "address":
			s.Address = o.Address
		case "name":
			s.Name = o.Name
		case "uuid":
			s.UUID = o.UUID
		case "version":
			s.Version = o.Version
		case "offline":
			s.Offline = o.Offline
		case "token":
			s.Token = o.Token
		case "data-path":
			s.MCDataGenPath = o.MCDataGenPath
		case "protocol-path":
			s.MCProtocolGoPath = o.MCProtocolGoPath
		}
	})
}

// --- RCON ---

type RCONFlagOverrides struct{ Address, Password string }

func RegisterRCONFlags(fs *flag.FlagSet) *RCONFlagOverrides {
	o := &RCONFlagOverrides{}
	fs.StringVar(&o.Address, "rcon-address", "", "RCON host:port, e.g. localhost:25575 (overrides config file/env)")
	fs.StringVar(&o.Password, "rcon-password", "", "RCON password (overrides config file/env)")
	return o
}

func ApplyRCONFlags(s *RCONSettings, fs *flag.FlagSet, o *RCONFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "rcon-address":
			s.Address = o.Address
		case "rcon-password":
			s.Password = o.Password
		}
	})
}

// --- Replay ---

type ReplayFlagOverrides struct {
	Enable            bool
	Output, Generator string
}

func RegisterReplayFlags(fs *flag.FlagSet) *ReplayFlagOverrides {
	o := &ReplayFlagOverrides{}
	fs.BoolVar(&o.Enable, "replay", false, "enable ReplayMod recording (.mcpr) (overrides config file/env)")
	fs.StringVar(&o.Output, "replay-out", "", "replay output file path (overrides config file/env)")
	fs.StringVar(&o.Generator, "replay-generator", "", "replay generator string (overrides config file/env)")
	return o
}

func ApplyReplayFlags(s *ReplaySettings, fs *flag.FlagSet, o *ReplayFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "replay":
			s.Enable = o.Enable
		case "replay-out":
			s.Output = o.Output
		case "replay-generator":
			s.Generator = o.Generator
		}
	})
}

// --- Skin ---

type SkinFlagOverrides struct {
	CacheDir     string
	AllowNetwork bool
}

func RegisterSkinFlags(fs *flag.FlagSet) *SkinFlagOverrides {
	o := &SkinFlagOverrides{}
	fs.StringVar(&o.CacheDir, "skin-cache", "", "directory to cache player/default skins (overrides config file/env)")
	fs.BoolVar(&o.AllowNetwork, "skin-net", false, "allow network skin fetches from Mojang, default off (overrides config file/env)")
	return o
}

func ApplySkinFlags(s *SkinSettings, fs *flag.FlagSet, o *SkinFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "skin-cache":
			s.CacheDir = o.CacheDir
		case "skin-net":
			s.AllowNetwork = o.AllowNetwork
		}
	})
}

// --- Movement ---

type MovementFlagOverrides struct{ EnableClutch bool }

func RegisterMovementFlags(fs *flag.FlagSet) *MovementFlagOverrides {
	o := &MovementFlagOverrides{}
	fs.BoolVar(&o.EnableClutch, "clutch", false, "enable clutch assist during physics movement (overrides config file/env)")
	return o
}

func ApplyMovementFlags(s *MovementSettings, fs *flag.FlagSet, o *MovementFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "clutch" {
			s.EnableClutch = o.EnableClutch
		}
	})
}

// --- FollowCam ---

type FollowCamFlagOverrides struct {
	Target   string
	Distance float64
}

func RegisterFollowCamFlags(fs *flag.FlagSet) *FollowCamFlagOverrides {
	o := &FollowCamFlagOverrides{}
	fs.StringVar(&o.Target, "follow-cam", "", "player name to follow in spectator camera mode, requires RCON (overrides config file/env)")
	fs.Float64Var(&o.Distance, "follow-cam-distance", 0, "max distance in blocks to maintain from the followed player (overrides config file/env)")
	return o
}

func ApplyFollowCamFlags(s *FollowCamSettings, fs *flag.FlagSet, o *FollowCamFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "follow-cam":
			s.Target = o.Target
		case "follow-cam-distance":
			s.Distance = o.Distance
		}
	})
}

// --- Env (RL task/step config) ---

type EnvFlagOverrides struct {
	TargetOffsetX, TargetOffsetY, TargetOffsetZ float64
	ArrivalThreshold                            float64
	StepTimeoutSeconds                          float64
	MineTargetBlock                             string
	MineSearchRadius                            int
	CraftTargetItem                             string
	SeedEpisodes                                bool
}

func RegisterEnvFlags(fs *flag.FlagSet) *EnvFlagOverrides {
	o := &EnvFlagOverrides{}
	fs.Float64Var(&o.TargetOffsetX, "target-offset-x", 0, "GoToTarget/ReturnHome target offset, X (overrides config file/env)")
	fs.Float64Var(&o.TargetOffsetY, "target-offset-y", 0, "GoToTarget/ReturnHome target offset, Y (overrides config file/env)")
	fs.Float64Var(&o.TargetOffsetZ, "target-offset-z", 0, "GoToTarget/ReturnHome target offset, Z (overrides config file/env)")
	fs.Float64Var(&o.ArrivalThreshold, "arrival-threshold", 0, "distance in blocks counting as target arrival (overrides config file/env)")
	fs.Float64Var(&o.StepTimeoutSeconds, "step-timeout-seconds", 0, "per-Step dispatch timeout, seconds (overrides config file/env)")
	fs.StringVar(&o.MineTargetBlock, "mine-target-block", "", "block name the mine task targets, e.g. minecraft:stone (overrides config file/env)")
	fs.IntVar(&o.MineSearchRadius, "mine-search-radius", 0, "FindVisibleBlock search radius for the mine task (overrides config file/env)")
	fs.StringVar(&o.CraftTargetItem, "craft-target-item", "", "item name the craft task targets, e.g. minecraft:stick (overrides config file/env)")
	fs.BoolVar(&o.SeedEpisodes, "seed-episodes", false, "seed mine/craft tasks via RCON at Reset, requires -rcon-address (overrides config file/env)")
	return o
}

func ApplyEnvFlags(s *EnvSettings, fs *flag.FlagSet, o *EnvFlagOverrides) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "target-offset-x":
			s.TargetOffset[0] = o.TargetOffsetX
		case "target-offset-y":
			s.TargetOffset[1] = o.TargetOffsetY
		case "target-offset-z":
			s.TargetOffset[2] = o.TargetOffsetZ
		case "arrival-threshold":
			s.ArrivalThreshold = o.ArrivalThreshold
		case "step-timeout-seconds":
			s.StepTimeoutSeconds = o.StepTimeoutSeconds
		case "mine-target-block":
			s.MineTargetBlock = o.MineTargetBlock
		case "mine-search-radius":
			s.MineSearchRadius = o.MineSearchRadius
		case "craft-target-item":
			s.CraftTargetItem = o.CraftTargetItem
		case "seed-episodes":
			s.SeedEpisodes = o.SeedEpisodes
		}
	})
}

// --- Train (delegates to cRL-go's own crlconfig.RegisterFlags/Apply
// rather than reimplementing that merge logic — per this package's own
// precedent from rlconfig.RegisterFlags) ---

// RegisterTrainFlags registers cRL-go's own training-hyperparameter flags,
// AND the shared "-config" flag (crlconfig.RegisterFlags registers both
// together) — its default is overridden here to DefaultConfigPath, this
// package's merged-schema convention, not crlconfig's own bare
// "configs/config.json" default (same filename by coincidence, but a
// different schema — crlconfig's own default assumes a bare
// crlconfig.Settings file). Only cmd/rl-train needs this; commands that
// don't should call RegisterConfigPathFlag instead, never both (a second
// "-config" registration panics).
func RegisterTrainFlags(fs *flag.FlagSet) *crlconfig.FlagOverrides {
	o := crlconfig.RegisterFlags(fs)
	o.ConfigPath = DefaultConfigPath
	if f := fs.Lookup("config"); f != nil {
		f.DefValue = DefaultConfigPath
		f.Usage = "path to this run's JSON config file (see Settings)"
	}
	return o
}

// ApplyTrainFlags is crlconfig.Apply, exported under this package's
// naming convention for symmetry with the other ApplyXFlags functions.
func ApplyTrainFlags(s *crlconfig.Settings, fs *flag.FlagSet, o *crlconfig.FlagOverrides) {
	crlconfig.Apply(s, fs, o)
}
