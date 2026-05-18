package models

import (
	"io"

	"github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// AgentConfig contains all inputs required to construct and run an Agent.
// It is intentionally decoupled from concrete implementations to support testing.
type AgentConfig struct {
	Name string

	// Connection
	Address         string
	Version         string
	ProtocolVersion uint

	// Authentication
	Auth Auth

	// Optional: external managers. If nil, they can be derived from Version.
	PacketMgr protocol_models.PacketMgr
	BlockMgr  mc_versions.BlockMgr
	SoundMgr  mc_versions.SoundMgr

	// Version-specific packet handler.
	// When set, enables version-aware packet construction and parsing via the versions/ package.
	VersionHandler VersionHandler

	// Optional: prebuilt client (useful for tests). If nil, Agent may construct one.
	Client bot.Client

	// Optional: subsystems
	Chat Chat

	// Optional: movement mode
	EnableClutchAssist     bool    // Enable clutch planning/actions during physics movement
	PathfinderGoalRadius   float64 // Treat goals within this radius as reached (defaults to 0.5 when 0)

	// Data paths (optional)
	MCDataGenPath    string // Path to mc-data-gen data (for block collision shapes)
	MCProtocolGoPath string // Path to mc-protocol-go (for state properties)
	RegistriesPath   string // Path to Minecraft registries.json directory (contains data_generator/reports/registries.json)

	// Logging output (optional)
	LogWriter io.Writer

	// Graceful shutdown
	StopFilePath string // optional path to stop file; when this file exists, agent shuts down gracefully

	// Replay recording
	EnableReplay    bool   // when true, record clientbound packets to an .mcpr
	ReplayOutput    string // output path, defaults to "session.mcpr" if empty
	ReplayGenerator string // optional generator string; defaults to "mc-agent"
	ReplayAutoCamera *bool // when non-nil, overrides default (true) for auto-camera timelines in replays

	// Optional: skins provider for replay embedding (pass NewSkinFetcher result)
	SkinProvider SkinProvider

	// Optional: initial plan to execute after Start.
	InitialPlan Plan

	// Optional: rcon controller for world maniputation (should only be used for debugging)
	RCON testenv.RCONHelper

	// Optional: HPA* debug visualization styling (path steps only)
	HPADebugPathBlock string // Explicit block name to use (expects <color>_stained_glass)
	HPADebugPathColor string // Color name to use when block is not specified
}

// Validate performs basic configuration checks.
func (c *AgentConfig) Validate() error {
	if c.Address == "" {
		return ErrInvalidConfig("Address must be set")
	}
	return nil
}
