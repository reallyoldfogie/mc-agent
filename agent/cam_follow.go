package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// camFollowPollInterval is how often the follow loop checks distance to the
// target and repositions if needed.
const camFollowPollInterval = 1 * time.Second

// camFollowVerticalOffset is how far above the target's own position the
// cam is teleported to when repositioning - keeps it out of the target's
// hitbox while still framing them via the teleport command's "facing"
// clause.
const camFollowVerticalOffset = 3.0

// StartCamFollow switches this agent to spectator mode via RCON and begins
// periodically teleporting it (also via RCON) to stay within maxDistance
// blocks of targetName, snapping instantly if the target is itself
// teleported. See models.CommandAgent's doc comment for the overall
// contract: requires RCON to be configured on this agent, and returns an
// error immediately, without starting anything, if it isn't or the
// spectator-mode switch fails.
func (a *agent) StartCamFollow(ctx context.Context, targetName string, maxDistance float64) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("cam-follow requires RCON; none configured for this agent")
	}

	resp, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("gamemode spectator %s", a.cfg.Name))
	if err != nil {
		return fmt.Errorf("switch to spectator mode: %w", err)
	}
	// The command can "succeed" at the RCON transport level while vanilla
	// itself rejects it (e.g. no permission, or this player isn't
	// currently online from the server's point of view) - vanilla reports
	// that as a normal text response, not an RCON-level error, so check
	// for it explicitly rather than trusting a nil error alone.
	if looksLikeCommandFailure(resp) {
		return fmt.Errorf("switch to spectator mode: server rejected it: %s", resp)
	}

	a.camFollowMu.Lock()
	if a.camFollowCancel != nil {
		a.camFollowCancel()
	}
	loopCtx, cancel := context.WithCancel(ctx)
	a.camFollowCancel = cancel
	a.camFollowMu.Unlock()

	go a.runCamFollowLoop(loopCtx, targetName, maxDistance)

	return nil
}

// StopCamFollow stops any active StartCamFollow loop. No-op if not
// currently following.
func (a *agent) StopCamFollow() error {
	a.camFollowMu.Lock()
	defer a.camFollowMu.Unlock()
	if a.camFollowCancel != nil {
		a.camFollowCancel()
		a.camFollowCancel = nil
	}
	return nil
}

func (a *agent) runCamFollowLoop(ctx context.Context, targetName string, maxDistance float64) {
	ticker := time.NewTicker(camFollowPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.camFollowTick(ctx, targetName, maxDistance)
		}
	}
}

func (a *agent) camFollowTick(ctx context.Context, targetName string, maxDistance float64) {
	if ctx.Err() != nil {
		// StopCamFollow/agent shutdown raced the ticker - nothing to do,
		// and every RCON call below would otherwise fail noisily against
		// an already-closed connection.
		return
	}

	// Query the target's position via RCON rather than this agent's own
	// entity-tracking (models.TargetSelector/following.NewTargetSelector):
	// confirmed live that once the target moves beyond this agent's own
	// view/simulation distance (exactly the large-teleport case
	// StartCamFollow needs to handle), the server stops sending it position
	// updates at all, silently freezing FindPlayerByName's result at a
	// stale pre-move position. RCON queries the server's authoritative
	// state directly, with no view-distance concept at all.
	tx, ty, tz, ok := a.queryEntityPosViaRCON(ctx, targetName)
	if !ok {
		log.Printf("[Agent %s][CamFollow] target %q not currently locatable", a.cfg.Name, targetName)
		return
	}

	pos, ok := a.GetPositionSimple()
	if !ok {
		return
	}

	dx := tx - pos.X
	dy := ty - pos.Y
	dz := tz - pos.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if distance <= maxDistance {
		return
	}

	// "facing <entity>" alone only accepts a location; facing a specific
	// entity by name requires the "facing entity <target>" form.
	cmd := fmt.Sprintf("teleport %s %.2f %.2f %.2f facing entity %s",
		a.cfg.Name, tx, ty+camFollowVerticalOffset, tz, targetName)
	if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
		log.Printf("[Agent %s][CamFollow] teleport failed: %v", a.cfg.Name, err)
	}
}

// entityPosRe matches vanilla's "data get entity <target> Pos" response,
// e.g. "ChestAccess has the following entity data: [1.5d, 64.0d, -2.5d]".
var entityPosRe = regexp.MustCompile(`\[\s*(-?[0-9.]+)d?,\s*(-?[0-9.]+)d?,\s*(-?[0-9.]+)d?\s*\]`)

// queryEntityPosViaRCON resolves targetName's current position via RCON,
// bypassing this agent's own client-side entity tracking entirely.
func (a *agent) queryEntityPosViaRCON(ctx context.Context, targetName string) (x, y, z float64, ok bool) {
	resp, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("data get entity %s Pos", targetName))
	if err != nil {
		log.Printf("[Agent %s][CamFollow] query %q position: %v", a.cfg.Name, targetName, err)
		return 0, 0, 0, false
	}
	m := entityPosRe.FindStringSubmatch(resp)
	if m == nil {
		return 0, 0, 0, false
	}
	x, err1 := strconv.ParseFloat(m[1], 64)
	y, err2 := strconv.ParseFloat(m[2], 64)
	z, err3 := strconv.ParseFloat(m[3], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return x, y, z, true
}

// looksLikeCommandFailure heuristically detects vanilla's plain-text
// command-rejection responses (no permission, unknown/malformed command,
// target not found), which RCON delivers as a normal successful response
// string rather than a Go error.
func looksLikeCommandFailure(resp string) bool {
	lower := strings.ToLower(resp)
	for _, phrase := range []string{
		"unknown command",
		"incorrect argument",
		"no player was found",
		"no entity was found",
		"expected argument",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
