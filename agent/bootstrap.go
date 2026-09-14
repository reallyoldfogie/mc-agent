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
//
// Validates the resolved name against usernameMaxLength before returning
// it — this is deliberately the single choke point for that check (rather
// than each caller/connectAgent-style helper remembering to check its own
// config's name) precisely because it's already "every entry point reuses
// this," per the paragraph above. See usernameMaxLength's own doc comment
// for why this exists: a name over the limit reaches a real server today
// as a generic, unhelpful decode-failure disconnect instead of a clear
// local error.
func ResolveAuth(offline bool, name, uuid, token string, auth config.AuthSettings) (models.Auth, error) {
	if offline {
		if err := validateUsernameLength(name); err != nil {
			return models.Auth{}, err
		}
		return models.Auth{AccessToken: token, Name: name, UUID: uuid}, nil
	}

	mauth, err := msauth.GetMCcredentials(credCachePath(auth), auth.ClientID)
	if err != nil {
		return models.Auth{}, fmt.Errorf("auth failed: %w", err)
	}
	if err := validateUsernameLength(mauth.Name); err != nil {
		return models.Auth{}, err
	}
	return models.Auth{AccessToken: mauth.AsTk, Name: mauth.Name, UUID: mauth.UUID}, nil
}

// usernameMaxLength is Minecraft's protocol-enforced maximum length, in
// characters, for a player username. The serverbound `hello`/LoginStart
// packet's Username field
// (mc-protocol-go's generated data/1.21.5/login/serverbound/packet_loginstart.go)
// is a bare pk.String on the wire, with no length bound in mc-agent's own
// client-side encoder — but a real server's decoder enforces this limit
// and rejects the whole packet outright if it's violated, not with a
// clear "username too long" error but a generic
// `DecoderException: Failed to decode packet 'serverbound/minecraft:hello'`
// that just disconnects the client. Confirmed live: this is
// docs/bugs/offline-login-hello-packet-decode-disconnect.md's root cause
// — its second occurrence's bot name ("RSITrainSmokeTest") is 17
// characters, exactly one over this limit.
const usernameMaxLength = 16

// validateUsernameLength returns an error naming exactly what's wrong
// (rather than letting a too-long name reach a real server, where it
// becomes the opaque decode-failure disconnect usernameMaxLength's own
// doc comment describes) if name exceeds usernameMaxLength. Counts runes,
// not bytes, matching the protocol field's own character-count semantics
// (mirrors agent/game_chat.go's splitChatMessage taking the same care for
// the serverbound chat packet's own length limit) — doesn't matter for a
// valid Minecraft username, which is ASCII-only by Mojang's own rules, but
// is the correct check to write regardless of what a caller actually
// passes.
func validateUsernameLength(name string) error {
	if length := len([]rune(name)); length > usernameMaxLength {
		return fmt.Errorf("username %q is %d characters, exceeds Minecraft's %d-character limit — a real server would reject this with an unhelpful decode-failure disconnect rather than a clear error, so ResolveAuth checks it first (see docs/bugs/offline-login-hello-packet-decode-disconnect.md)", name, length, usernameMaxLength)
	}
	return nil
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
