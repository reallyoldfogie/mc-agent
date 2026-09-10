// Command item-courier logs a single shared player identity into an
// arbitrary number of Minecraft servers simultaneously and relays
// mc-item-transfer-mod's item_transfer:* plugin messages between whichever
// pair a given transfer names. See
// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md.
//
// This command brings up one agent.Agent per configured server, attaches a
// courier.Courier, exchanges item_transfer:handshake with each, and
// registers courier.TransferAction on every one of them, so a transfer can
// be triggered by chat (">>>botName<<<transfer <slot> <destLabel>", per
// agent/commands.go's existing convention), by an RL/LLM executor driving
// the same action registry, or directly via Courier.StartTransfer from any
// Go caller — see the plan's Open Question 1, and courier.TransferAction's
// doc comment for why one implementation covers all three.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/config"
	"github.com/reallyoldfogie/mc-agent/courier"
	_ "github.com/reallyoldfogie/mc-agent/handler_versions"
	"github.com/reallyoldfogie/mc-agent/models"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"
)

// envPrefix is this command's ENV-override prefix (config.ApplyEnv's second
// layer, docs/plans/UNIFIED_CONFIG_PLAN.md) — e.g. MCCOURIER_CONNECTION_NAME.
const envPrefix = "MCCOURIER"

func main() {
	configPath := config.RegisterConfigPathFlag(flag.CommandLine)
	help := flag.Bool("help", false, "Display help")
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

	if len(settings.Courier.Servers) == 0 {
		log.Fatalf("courier.servers is empty — configure at least two servers (see docs/plans/ITEM_TRANSFER_COURIER_PLAN.md)")
	}
	if len(settings.Courier.Servers) < 2 {
		log.Printf("warning: only %d server configured — a transfer needs a distinct source and destination", len(settings.Courier.Servers))
	}
	labels := make(map[string]bool, len(settings.Courier.Servers))
	for _, s := range settings.Courier.Servers {
		if s.Label == "" {
			log.Fatalf("courier.servers: every entry needs a non-empty label (address %q)", s.Address)
		}
		if labels[s.Label] {
			log.Fatalf("courier.servers: duplicate label %q", s.Label)
		}
		labels[s.Label] = true
	}

	// Every server shares one player identity — see the plan's "same
	// identity on every server" assumption (Open Question 3).
	auth, err := agent.ResolveAuth(settings.Connection.Offline, settings.Connection.Name, settings.Connection.UUID, settings.Connection.Token, settings.Auth)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if settings.Connection.Offline {
		log.Printf("Offline mode => using shared identity name=%s uuid=%s", auth.Name, auth.UUID)
	} else {
		log.Printf("Authenticated as shared identity %s (%s)", auth.Name, auth.UUID)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	agents := make(map[string]models.Agent, len(settings.Courier.Servers))
	messengers := make(map[string]courier.AgentMessenger, len(settings.Courier.Servers))

	// Best-effort cleanup of whatever already started if a later server in
	// the list fails to connect — a partially-connected courier is more
	// confusing than no courier at all.
	closeAll := func() {
		for label, a := range agents {
			if err := a.Close(context.Background()); err != nil {
				log.Printf("close %s: %v", label, err)
			}
		}
	}

	for _, s := range settings.Courier.Servers {
		version := s.Version
		if version == "" {
			detected, _, err := rof_utils.CheckServerVersion(s.Address, 0)
			if err != nil {
				closeAll()
				log.Fatalf("auto-detect version for %s (%s): %v", s.Label, s.Address, err)
			}
			version = detected
		}

		cfg := models.AgentConfig{
			Name:             auth.Name,
			Address:          s.Address,
			Version:          version,
			Auth:             auth,
			MCDataGenPath:    settings.Connection.MCDataGenPath,
			MCProtocolGoPath: settings.Connection.MCProtocolGoPath,
			StopFilePath:     ".agentStop",
		}

		a, err := agent.New(cfg)
		if err != nil {
			closeAll()
			log.Fatalf("create agent for %s: %v", s.Label, err)
		}
		if err := a.Init(ctx); err != nil {
			closeAll()
			log.Fatalf("init %s (%s): %v", s.Label, s.Address, err)
		}
		if err := a.Start(ctx); err != nil {
			closeAll()
			log.Fatalf("start %s (%s): %v", s.Label, s.Address, err)
		}

		log.Printf("Connected to %s (%s, version %s)", s.Label, s.Address, version)
		agents[s.Label] = a
		messengers[s.Label] = a
	}

	c := courier.New(messengers, slog.Default())
	c.Attach()

	for label := range agents {
		if err := c.Handshake(label); err != nil {
			log.Printf("handshake with %s failed: %v", label, err)
		}
	}
	// Give in-flight handshake replies a moment to arrive before reporting
	// readiness — purely informational logging, not correctness-critical
	// (Courier.HandshakeComplete can always be checked later).
	time.Sleep(2 * time.Second)
	for label := range agents {
		if !c.HandshakeComplete(label) {
			log.Printf("warning: no handshake reply yet from %s — is item-transfer-mod installed there?", label)
		}
	}

	// Register the chat/RL/LLM-dispatchable "transfer" command on every
	// agent, one instance per server bound to that server as the implicit
	// source (see courier.TransferAction's doc comment on why this one
	// registration covers all three trigger surfaces named in
	// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md's Open Question 1). A
	// player addresses server X's bot with ">>>botName<<<transfer <slot>
	// <destLabel>" to move an item from X to destLabel.
	for label, a := range agents {
		a.RegisterAction(courier.TransferAction{Courier: c, SourceLabel: label})
	}

	log.Printf("item-courier ready: %d server(s) connected. Ctrl+C to stop.", len(agents))
	fmt.Println("Trigger a transfer by chatting \">>>" + auth.Name + "<<<transfer <slot> <destLabel>\" to any connected server, or by calling Courier.StartTransfer directly (API/RL/LLM use).")

	<-ctx.Done()
	log.Printf("shutting down...")
	closeAll()
}
