package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"

	"github.com/reallyoldfogie/mc-agent/utils"
)

// packetLogger returns a generic packet handler that writes JSON logs to a.logw.
func (a *agent) packetLogger() bot.PacketHandler {
	ver := a.cfg.Version
	proto := a.cfg.ProtocolVersion
	return bot.PacketHandler{
		Priority: 0,
		F: func(p pk.Packet) error {
			if a.packetLogWriter != nil {
				name := ""
				if a.packetMgr != nil {
					name = a.packetMgr.ClientboundToString(protocol_models.ClientboundPacketID(p.ID))
				}
				pl := protocol_models.PacketLog{
					ID:              p.ID,
					Data:            make([]byte, len(p.Data)),
					Name:            name,
					Timestamp:       time.Now(),
					Version:         ver,
					ProtocolVersion: proto,
					Direction:       "clientbound",
					State:           "play",
					Source:          "agent",
				}
				copy(pl.Data, p.Data)
				if err := json.NewEncoder(a.packetLogWriter).Encode(pl); err != nil {
					log.Printf("[ERROR] failed to write packet log: %v", err)
					return err
				}
			}
			return nil
		},
	}
}

// Agent log setup (separate from packet logging)
var (
	agentLogFile *os.File
	logSetupMu   sync.Mutex
)

// setupAgentLogging initializes the global log package to write to both a file and stdout.
// This is called automatically from agent.Init() and should not be called by clients.
func setupAgentLogging() error {
	logSetupMu.Lock()
	defer logSetupMu.Unlock()

	// Already setup?
	if agentLogFile != nil {
		return nil
	}

	// Find or create cache directory
	cacheDir, err := utils.FindOrCreateCacheDir()
	if err != nil {
		return fmt.Errorf("find cache directory: %w", err)
	}

	// Create log directory
	agentLogsDir := filepath.Join(cacheDir, "logs", "agents")
	if err := os.MkdirAll(agentLogsDir, 0760); err != nil {
		return fmt.Errorf("create agent logs directory: %w", err)
	}

	// Create log file with timestamp
	logFileName := filepath.Join(agentLogsDir, "agents_"+time.Now().Format("20060102_150405")+".log")
	file, err := os.Create(logFileName)
	if err != nil {
		return fmt.Errorf("create agent log file: %w", err)
	}

	// Create MultiWriter to write to both file and stdout
	multiWriter := io.MultiWriter(file, os.Stdout)

	// Set global log output
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	// Store for cleanup
	agentLogFile = file

	fmt.Printf("Agent logging enabled: %s (also to console)\n", logFileName)
	return nil
}

// closeAgentLog closes the agent log file and restores the log package to stdout only.
// This is called automatically from agent.Close() and should not be called by clients.
func closeAgentLog() {
	logSetupMu.Lock()
	defer logSetupMu.Unlock()

	if agentLogFile != nil {
		agentLogFile.Close()
		agentLogFile = nil
		// Restore log to stdout only
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags)
	}
}
