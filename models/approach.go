package models

import (
	"context"
	"errors"
	"math"
	"time"
)

// ApproachAgent is what ApproachBlock needs: FindInteractPosition's
// capabilities plus the bot's position and a way to walk.
type ApproachAgent interface {
	InteractPositionAgent
	GetPositionSimple() (pos V3, initialized bool)
	MoveTo(ctx context.Context, x, y, z float64, notifyChat bool) error
}

// ApproachAttemptTimeout bounds each walk toward one candidate standing spot
// before ApproachBlock moves on to the next. A wedged approach never returns
// an error by itself, so without a bound the fallback to other spots would
// never be reached (found live: a bot 1.4 blocks from its crafting table
// spent ~17 minutes trying to step down into the exact cell beside it).
// A variable so tests can shorten it.
var ApproachAttemptTimeout = 5 * time.Second

// playerEyeHeight is a standing player's eye height above their feet.
const playerEyeHeight = 1.62

// ApproachOptions tunes ApproachBlock.
type ApproachOptions struct {
	// Announce narrates the first walk in chat (MoveTo's notifyChat); later
	// fallback attempts stay quiet so a run of them can't trip the server's
	// chat-spam kick.
	Announce bool
	// RequireSight makes "close enough" mean within reach *and* line of
	// sight - right for interactions mc-agent itself gates on sight (opening
	// containers). Digging only needs reach.
	RequireSight bool
}

// WithinInteractReach reports whether a bot standing at pos (feet) with its
// eyes eyeHeight higher is within InteractReachDistance of the center of the
// block at target (block coordinates).
func WithinInteractReach(pos V3, eyeHeight float64, target V3) bool {
	dx := target.X + 0.5 - pos.X
	dy := target.Y + 0.5 - (pos.Y + eyeHeight)
	dz := target.Z + 0.5 - pos.Z
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= InteractReachDistance
}

// CanInteractFromHere reports whether the bot can already act on the block
// at target from where it stands.
func CanInteractFromHere(ctx context.Context, agent ApproachAgent, target V3, requireSight bool) bool {
	pos, ok := agent.GetPositionSimple()
	if !ok || !WithinInteractReach(pos, playerEyeHeight, target) {
		return false
	}
	if !requireSight {
		return true
	}
	visible, err := agent.CanInteractFromPosition(ctx, pos.X, pos.Y, pos.Z, target.X, target.Y, target.Z)
	return err == nil && visible
}

// ApproachBlock gets the bot to where it can act on the block at target
// (block coordinates): a no-op if it already can - close enough is good
// enough, like a real player who doesn't shuffle to a "best" spot - and
// otherwise walks to the closest walkable, in-reach spot that works,
// trying each candidate closest-first with its own ApproachAttemptTimeout
// and failing only once every one has. If no standable spot with line of
// sight exists at all, it falls back to walking at the target itself.
func ApproachBlock(ctx context.Context, agent ApproachAgent, target V3, opts ApproachOptions) error {
	if CanInteractFromHere(ctx, agent, target, opts.RequireSight) {
		return nil
	}
	announce := opts.Announce
	err := TryInteractPositions(ctx, agent, target, func(pos V3) error {
		notify := announce
		announce = false
		moveCtx, cancel := context.WithTimeout(ctx, ApproachAttemptTimeout)
		defer cancel()
		return agent.MoveTo(moveCtx, pos.X, pos.Y, pos.Z, notify)
	})
	if errors.Is(err, ErrNoInteractPosition) {
		return agent.MoveTo(ctx, target.X, target.Y, target.Z, announce)
	}
	return err
}
