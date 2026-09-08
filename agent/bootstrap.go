package agent

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"time"

	msauth "github.com/maxsupermanhd/go-mc-ms-auth"
	"github.com/reallyoldfogie/mc-agent/config"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

// ResolveAuth resolves models.Auth for a new bot session: offline mode
// (name/uuid/token supplied directly, no network call) or Microsoft
// authentication, cached at auth.CredCacheFile (or, if that's unset,
// filepath.Join(auth.CacheDir, ".credCacheFile")). Factored out of
// cmd/agent/main.go's original inline logic so every entry point
// (cmd/agent, cmd/ollama, cmd/rl-train) can reuse it without duplicating
// the Microsoft-auth/offline-mode branching. Takes an already-loaded
// config.AuthSettings rather than a file path — the caller loads Settings
// once, via the unified JSON->ENV->CLI pipeline
// (docs/plans/UNIFIED_CONFIG_PLAN.md), and this function shouldn't load a
// second, independent copy of the same file.
//
// CredCacheFile is deliberately an explicit path, not derived from name —
// switching Microsoft accounts/bot identities is "point -config at a
// different file" (each with its own cred_cache_file), matching the
// workflow the pre-unification configs/config.yaml supported and this
// package's own JSON config replaced it with, not a naming convention this
// function would need to reverse-engineer from name.
func ResolveAuth(offline bool, name, uuid, token string, auth config.AuthSettings) (models.Auth, error) {
	if offline {
		return models.Auth{AccessToken: token, Name: name, UUID: uuid}, nil
	}

	mauth, err := msauth.GetMCcredentials(credCachePath(auth), auth.ClientID)
	if err != nil {
		return models.Auth{}, fmt.Errorf("auth failed: %w", err)
	}
	return models.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}, nil
}

// credCachePath resolves where ResolveAuth reads/writes the cached
// Microsoft credentials for auth — a plain function, not inlined into
// ResolveAuth, so this resolution logic is unit-testable without a real
// device-auth network round trip (which msauth.GetMCcredentials would
// otherwise force on any test exercising it).
func credCachePath(auth config.AuthSettings) string {
	if auth.CredCacheFile != "" {
		return auth.CredCacheFile
	}
	return filepath.Join(auth.CacheDir, ".credCacheFile")
}

// DialRCON parses "host:port" and dials RCON via testenv.DialRCON, bounded
// by a 10s timeout — factored out of cmd/agent/main.go's original inline
// dial-on-startup logic (eager dialing surfaces a bad address/password
// immediately rather than after the agent has already connected) so
// cmd/rl-train can reuse it for episode-seeding RCON access
// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4) without duplicating it.
// Returns a nil RCONHelper and no error if address is empty — matches the
// original call site's "only dial if an address was actually configured"
// behavior.
func DialRCON(ctx context.Context, address, password string) (testenv.RCONHelper, error) {
	if address == "" {
		return nil, nil
	}
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid rcon address %q: %w", address, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid rcon address port %q: %w", portStr, err)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rcon, err := testenv.DialRCON(dialCtx, host, port, password)
	if err != nil {
		return nil, fmt.Errorf("dial RCON at %s: %w", address, err)
	}
	return rcon, nil
}
