package courier

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// ProtocolVersion is this courier's supported item_transfer protocol
// version, exchanged via item_transfer:handshake. Must track
// mc-item-transfer-mod's ItemTransferMod.PROTOCOL_VERSION - see
// docs/protocol.md.
const ProtocolVersion int32 = 1

// DefaultTimeout is how long StartTransfer waits for each stage's reply
// before giving up, when the caller's context has no earlier deadline.
const DefaultTimeout = 30 * time.Second

// AgentMessenger is the narrow slice of models.Agent this package actually
// depends on - just the plugin-messaging surface (models.PluginMessaging).
// Any models.Agent value already satisfies this. Depending on this instead
// of the full models.Agent interface keeps courier testable against small
// fakes and makes the real dependency explicit - courier has no business
// touching movement, containers, chat, etc.
type AgentMessenger interface {
	RegisterPluginMessageCallback(models.PluginMessageCallback)
	SendPluginMessage(channel string, data []byte) error
}

// Courier owns one AgentMessenger per configured server, keyed by a label
// matching mc-item-transfer-mod's serverId concept (see
// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md's Design section), and relays
// item_transfer:* plugin messages between whichever pair a given transfer
// names. It is a stateless black box with respect to the JWT itself - it
// never parses or decodes jwt/jti fields, only relays them.
type Courier struct {
	logger *slog.Logger

	serversMu sync.RWMutex
	servers   map[string]AgentMessenger

	handshakeMu   sync.Mutex
	handshakeDone map[string]bool

	// pendingRequests holds, per source server label, the FIFO queue of
	// in-flight "request" calls still waiting for their "hold" (or an
	// error with no jti yet). See the doc comment on requestFIFO below for
	// why this is a queue rather than a map keyed by something sturdier.
	pendingMu       sync.Mutex
	pendingRequests map[string][]*pendingRequest

	// byJTI holds every transfer from the moment its jti becomes known
	// (the "hold" reply) until it completes (commit) or fails
	// (error/abort).
	byJTIMu sync.Mutex
	byJTI   map[string]*pendingTransfer
}

// pendingRequest is one in-flight "request" awaiting its "hold" (success) or
// an "error" with no jti yet (failure before a jti was assigned).
//
// requestFIFO's correlation assumption: item_transfer:request carries no
// request/correlation ID (see docs/protocol.md §2) - the wire protocol
// relies on requests to one server being answered in the order they were
// sent, on that same connection. That's a reasonable assumption for a
// single Netty channel processed on a single server tick thread, but it is
// NOT verified against the real mod yet - Phase 4's integration test
// against real servers should confirm holds/errors actually arrive in
// send order before this is trusted beyond "reasonable engineering
// assumption, flagged clearly."
type pendingRequest struct {
	resultCh chan requestResult
}

type requestResult struct {
	hold *HoldPayload
	err  *ErrorPayload
}

// pendingTransfer is one in-flight transfer from the moment its jti is
// known (a "hold" reply) onward.
type pendingTransfer struct {
	sourceLabel string
	destLabel   string
	resultCh    chan transferResult
}

type transferResult struct {
	promised *PromisedPayload
	err      *ErrorPayload
	abort    *AbortPayload
}

// New constructs a Courier over the given servers (label -> already
// constructed, but not yet necessarily started, agent). logger may be nil,
// in which case slog.Default() is used.
func New(servers map[string]AgentMessenger, logger *slog.Logger) *Courier {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Courier{
		logger:          logger,
		servers:         make(map[string]AgentMessenger, len(servers)),
		handshakeDone:   make(map[string]bool, len(servers)),
		pendingRequests: make(map[string][]*pendingRequest),
		byJTI:           make(map[string]*pendingTransfer),
	}
	for label, a := range servers {
		c.servers[label] = a
	}
	return c
}

// Attach registers this courier's dispatch as a plugin-message callback on
// every configured agent. Call once, after all agents are constructed but
// before (or immediately after) they connect - a callback registered before
// Init/Start will simply see no traffic until the connection exists.
func (c *Courier) Attach() {
	c.serversMu.RLock()
	defer c.serversMu.RUnlock()
	for label, a := range c.servers {
		label := label
		a.RegisterPluginMessageCallback(func(channel string, data []byte) {
			c.dispatch(label, channel, data)
		})
	}
}

// Handshake sends item_transfer:handshake on the named server. Call once per
// server, after Init/Start succeeds, before allowing any transfer request
// involving that server - see docs/plans/ITEM_TRANSFER_COURIER_PLAN.md
// Phase 2.
func (c *Courier) Handshake(label string) error {
	a, err := c.agent(label)
	if err != nil {
		return err
	}
	payload := HandshakePayload{ProtocolVersion: ProtocolVersion}
	data, err := payload.Encode()
	if err != nil {
		return fmt.Errorf("encode handshake for %q: %w", label, err)
	}
	return a.SendPluginMessage(ChannelHandshake, data)
}

// dispatch routes one inbound CustomPayload to the right handler by channel.
// It knows nothing about any channel outside item_transfer:* - anything else
// is logged and ignored, not an error (another mod's plugin channel may
// share the connection).
func (c *Courier) dispatch(fromLabel, channel string, data []byte) {
	switch channel {
	case ChannelHandshake:
		c.handleHandshake(fromLabel, data)
	case ChannelHold:
		c.handleHold(fromLabel, data)
	case ChannelPromised:
		c.handlePromised(fromLabel, data)
	case ChannelError:
		c.handleError(fromLabel, data)
	case ChannelAbort:
		c.handleAbort(fromLabel, data)
	default:
		c.logger.Debug("courier: ignoring unrecognized channel", "server", fromLabel, "channel", channel)
	}
}

func (c *Courier) handleHandshake(fromLabel string, data []byte) {
	hs, err := DecodeHandshakePayload(data)
	if err != nil {
		c.logger.Warn("courier: malformed handshake payload", "server", fromLabel, "error", err)
		return
	}
	c.handshakeMu.Lock()
	c.handshakeDone[fromLabel] = true
	c.handshakeMu.Unlock()

	if hs.ProtocolVersion != ProtocolVersion {
		c.logger.Warn("courier: protocol version mismatch",
			"server", fromLabel, "server_version", hs.ProtocolVersion, "courier_version", ProtocolVersion)
		return
	}
	c.logger.Info("courier: handshake complete", "server", fromLabel, "protocol_version", hs.ProtocolVersion)
}

// HandshakeComplete reports whether a handshake reply has been received from
// the named server yet.
func (c *Courier) HandshakeComplete(label string) bool {
	c.handshakeMu.Lock()
	defer c.handshakeMu.Unlock()
	return c.handshakeDone[label]
}

func (c *Courier) handleHold(fromLabel string, data []byte) {
	hold, err := DecodeHoldPayload(data)
	if err != nil {
		c.logger.Warn("courier: malformed hold payload", "server", fromLabel, "error", err)
		return
	}
	pr := c.popPendingRequest(fromLabel)
	if pr == nil {
		c.logger.Warn("courier: received hold with no matching pending request", "server", fromLabel, "jti", hold.JTI)
		return
	}
	pr.resultCh <- requestResult{hold: &hold}
}

func (c *Courier) handlePromised(fromLabel string, data []byte) {
	promised, err := DecodePromisedPayload(data)
	if err != nil {
		c.logger.Warn("courier: malformed promised payload", "server", fromLabel, "error", err)
		return
	}
	pt := c.popTransfer(promised.JTI)
	if pt == nil {
		c.logger.Warn("courier: received promised with no matching transfer", "server", fromLabel, "jti", promised.JTI)
		return
	}
	pt.resultCh <- transferResult{promised: &promised}
}

func (c *Courier) handleError(fromLabel string, data []byte) {
	errPayload, err := DecodeErrorPayload(data)
	if err != nil {
		c.logger.Warn("courier: malformed error payload", "server", fromLabel, "error", err)
		return
	}
	if errPayload.JTI == "" {
		// Failed before a jti was assigned - it's an answer to a pending
		// request, correlated the same way "hold" is (see pendingRequest's
		// FIFO doc comment).
		pr := c.popPendingRequest(fromLabel)
		if pr == nil {
			c.logger.Warn("courier: received request-phase error with no matching pending request", "server", fromLabel, "code", errPayload.Code)
			return
		}
		pr.resultCh <- requestResult{err: &errPayload}
		return
	}
	pt := c.popTransfer(errPayload.JTI)
	if pt == nil {
		c.logger.Warn("courier: received error with no matching transfer", "server", fromLabel, "jti", errPayload.JTI, "code", errPayload.Code)
		return
	}
	pt.resultCh <- transferResult{err: &errPayload}
}

func (c *Courier) handleAbort(fromLabel string, data []byte) {
	abort, err := DecodeAbortPayload(data)
	if err != nil {
		c.logger.Warn("courier: malformed abort payload", "server", fromLabel, "error", err)
		return
	}
	pt := c.popTransfer(abort.JTI)
	if pt == nil {
		// Not necessarily an error: the other server may have already
		// completed/aborted this transfer independently.
		c.logger.Debug("courier: received abort with no matching transfer", "server", fromLabel, "jti", abort.JTI, "reason", abort.Reason)
		return
	}
	pt.resultCh <- transferResult{abort: &abort}
}

func (c *Courier) agent(label string) (AgentMessenger, error) {
	c.serversMu.RLock()
	defer c.serversMu.RUnlock()
	a, ok := c.servers[label]
	if !ok {
		return nil, fmt.Errorf("courier: unknown server label %q", label)
	}
	return a, nil
}

func (c *Courier) pushPendingRequest(label string, pr *pendingRequest) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.pendingRequests[label] = append(c.pendingRequests[label], pr)
}

// popPendingRequest removes and returns the oldest still-pending request for
// label (FIFO), or nil if there is none.
func (c *Courier) popPendingRequest(label string) *pendingRequest {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	q := c.pendingRequests[label]
	if len(q) == 0 {
		return nil
	}
	pr := q[0]
	c.pendingRequests[label] = q[1:]
	return pr
}

func (c *Courier) registerTransfer(jti string, pt *pendingTransfer) {
	c.byJTIMu.Lock()
	defer c.byJTIMu.Unlock()
	c.byJTI[jti] = pt
}

func (c *Courier) popTransfer(jti string) *pendingTransfer {
	c.byJTIMu.Lock()
	defer c.byJTIMu.Unlock()
	pt, ok := c.byJTI[jti]
	if !ok {
		return nil
	}
	delete(c.byJTI, jti)
	return pt
}

// TransferOutcome is StartTransfer's result on success.
type TransferOutcome struct {
	JTI string
}

// StartTransfer drives one transfer's full request -> hold -> claim ->
// promised -> commit sequence between sourceLabel and destLabel for the
// given inventory slot, blocking until it completes, fails, or ctx is done.
// It is deliberately trigger-surface-agnostic - any trigger (chat command,
// agent action, RL/LLM executor) can call this directly; see
// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md Open Question 1 for what still
// needs deciding about *how* a player invokes it, which this function does
// not assume or depend on.
//
// Multiple concurrent calls - including ones sharing the same sourceLabel or
// destLabel - are safe and independent: pre-jti correlation is FIFO per
// source server (see pendingRequest's doc comment), and post-jti correlation
// uses the jti itself, which is globally unique.
func (c *Courier) StartTransfer(ctx context.Context, sourceLabel, destLabel, slotID string) (TransferOutcome, error) {
	sourceAgent, err := c.agent(sourceLabel)
	if err != nil {
		return TransferOutcome{}, err
	}
	destAgent, err := c.agent(destLabel)
	if err != nil {
		return TransferOutcome{}, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	// --- request -> hold ---
	pr := &pendingRequest{resultCh: make(chan requestResult, 1)}
	c.pushPendingRequest(sourceLabel, pr)

	reqData, err := RequestPayload{SlotID: slotID}.Encode()
	if err != nil {
		return TransferOutcome{}, fmt.Errorf("encode request: %w", err)
	}
	if err := sourceAgent.SendPluginMessage(ChannelRequest, reqData); err != nil {
		return TransferOutcome{}, fmt.Errorf("send request to %q: %w", sourceLabel, err)
	}

	var hold HoldPayload
	select {
	case res := <-pr.resultCh:
		if res.err != nil {
			return TransferOutcome{}, fmt.Errorf("request to %q failed: %s: %s", sourceLabel, res.err.Code, res.err.Message)
		}
		hold = *res.hold
	case <-ctx.Done():
		return TransferOutcome{}, fmt.Errorf("waiting for hold from %q: %w", sourceLabel, ctx.Err())
	}

	jti := hold.JTI
	pt := &pendingTransfer{sourceLabel: sourceLabel, destLabel: destLabel, resultCh: make(chan transferResult, 1)}
	c.registerTransfer(jti, pt)

	// --- claim -> promised ---
	claimData, err := ClaimPayload{JWT: hold.JWT}.Encode()
	if err != nil {
		c.popTransfer(jti)
		return TransferOutcome{}, fmt.Errorf("encode claim: %w", err)
	}
	if err := destAgent.SendPluginMessage(ChannelClaim, claimData); err != nil {
		c.popTransfer(jti)
		return TransferOutcome{}, fmt.Errorf("send claim to %q: %w", destLabel, err)
	}

	select {
	case res := <-pt.resultCh:
		switch {
		case res.err != nil:
			c.abort(sourceLabel, jti, "destination reported: "+res.err.Code)
			return TransferOutcome{}, fmt.Errorf("claim to %q failed: %s: %s", destLabel, res.err.Code, res.err.Message)
		case res.abort != nil:
			return TransferOutcome{}, fmt.Errorf("transfer %s aborted: %s", jti, res.abort.Reason)
		}
	case <-ctx.Done():
		c.popTransfer(jti)
		c.abort(sourceLabel, jti, "courier timed out waiting for promised")
		c.abort(destLabel, jti, "courier timed out waiting for promised")
		return TransferOutcome{}, fmt.Errorf("waiting for promised from %q: %w", destLabel, ctx.Err())
	}

	// --- commit ---
	commitData, err := CommitPayload{JTI: jti}.Encode()
	if err != nil {
		return TransferOutcome{}, fmt.Errorf("encode commit: %w", err)
	}
	if err := sourceAgent.SendPluginMessage(ChannelCommit, commitData); err != nil {
		return TransferOutcome{}, fmt.Errorf("send commit to %q: %w", sourceLabel, err)
	}

	return TransferOutcome{JTI: jti}, nil
}

// abort best-effort sends item_transfer:abort to label for jti, logging (not
// returning) any send failure - it's already being called from an
// already-failing path.
func (c *Courier) abort(label, jti, reason string) {
	a, err := c.agent(label)
	if err != nil {
		return
	}
	data, err := AbortPayload{JTI: jti, Reason: reason}.Encode()
	if err != nil {
		c.logger.Warn("courier: encode abort failed", "server", label, "jti", jti, "error", err)
		return
	}
	if err := a.SendPluginMessage(ChannelAbort, data); err != nil {
		c.logger.Warn("courier: send abort failed", "server", label, "jti", jti, "error", err)
	}
}
