package agent

import (
	"io"

	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// Config contains all inputs required to construct and run an Agent.
// It is intentionally decoupled from concrete implementations to support testing.
type Config struct {
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

	// Optional: prebuilt client (useful for tests). If nil, Agent may construct one.
	Client Client

	// Optional: subsystems
	Chat Chat

	// Data paths (optional)
	MCDataGenPath    string
	MCProtocolGoPath string

	// Logging output (optional)
	LogWriter io.Writer

	// Feature toggles
	EnablePathfinding bool
	EnableFollowing   bool

	// Graceful shutdown
	StopFilePath string // optional path to stop file; when this file exists, agent shuts down gracefully

	// Replay recording
	EnableReplay    bool   // when true, record clientbound packets to an .mcpr
	ReplayOutput    string // output path, defaults to "session.mcpr" if empty
	ReplayGenerator string // optional generator string; defaults to "mc-agent"

	// Optional: skins provider for replay embedding (pass NewSkinFetcher result)
	SkinProvider SkinProvider
}

// Validate performs basic configuration checks.
func (c *Config) Validate() error {
	if c.Address == "" {
		return ErrInvalidConfig("Address must be set")
	}
	return nil
}
