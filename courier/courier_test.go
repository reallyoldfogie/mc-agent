package courier

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// fakeAgent is a minimal AgentMessenger test double: SendPluginMessage
// forwards the encoded payload straight to whatever function the test wired
// up as "the other side", instead of a real network connection.
type fakeAgent struct {
	mu        sync.Mutex
	callbacks []models.PluginMessageCallback
	send      func(channel string, data []byte) error
}

func (f *fakeAgent) RegisterPluginMessageCallback(cb models.PluginMessageCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbacks = append(f.callbacks, cb)
}

func (f *fakeAgent) SendPluginMessage(channel string, data []byte) error {
	if f.send != nil {
		return f.send(channel, data)
	}
	return nil
}

// deliver simulates fakeAgent receiving an inbound CustomPayload, invoking
// every registered callback the way agent.onCustomPayload would.
func (f *fakeAgent) deliver(channel string, data []byte) {
	f.mu.Lock()
	callbacks := append([]models.PluginMessageCallback(nil), f.callbacks...)
	f.mu.Unlock()
	for _, cb := range callbacks {
		cb(channel, data)
	}
}

func TestHandshake(t *testing.T) {
	var sent []string
	src := &fakeAgent{send: func(channel string, data []byte) error {
		sent = append(sent, channel)
		return nil
	}}
	c := New(map[string]AgentMessenger{"source": src}, nil)
	c.Attach()

	if c.HandshakeComplete("source") {
		t.Fatal("handshake should not be complete before any reply")
	}
	if err := c.Handshake("source"); err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if len(sent) != 1 || sent[0] != ChannelHandshake {
		t.Fatalf("sent channels = %v, want [%s]", sent, ChannelHandshake)
	}

	// Simulate the mod replying on the same channel.
	reply, _ := HandshakePayload{ProtocolVersion: ProtocolVersion}.Encode()
	src.deliver(ChannelHandshake, reply)

	if !c.HandshakeComplete("source") {
		t.Fatal("handshake should be complete after a matching-version reply")
	}
}

func TestHandshakeVersionMismatchStillRecordsCompletion(t *testing.T) {
	src := &fakeAgent{}
	c := New(map[string]AgentMessenger{"source": src}, nil)
	c.Attach()

	reply, _ := HandshakePayload{ProtocolVersion: ProtocolVersion + 1}.Encode()
	src.deliver(ChannelHandshake, reply)

	// A version mismatch is logged, not silently dropped - but the courier
	// still knows a reply was received (a caller layer decides whether a
	// mismatch should block transfers; that's not this test's concern).
	if !c.HandshakeComplete("source") {
		t.Fatal("handshake completion should still be recorded on a version mismatch")
	}
}

// TestStartTransferHappyPath wires two fake agents together end-to-end
// through the full request->hold->claim->promised->commit sequence, with
// each fakeAgent's send function acting as "the mod", replying
// asynchronously the way a real server would.
func TestStartTransferHappyPath(t *testing.T) {
	var mu sync.Mutex
	var commitReceived bool
	var claimReceivedJWT string

	var source, dest *fakeAgent
	source = &fakeAgent{send: func(channel string, data []byte) error {
		switch channel {
		case ChannelRequest:
			req, err := DecodeRequestPayload(data)
			if err != nil {
				t.Errorf("decode request: %v", err)
				return nil
			}
			if req.SlotID != "5" {
				t.Errorf("request.SlotID = %q, want 5", req.SlotID)
			}
			go func() {
				hold, _ := HoldPayload{JWT: "signed.jwt.text", JTI: "jti-1"}.Encode()
				source.deliver(ChannelHold, hold)
			}()
		case ChannelCommit:
			commit, err := DecodeCommitPayload(data)
			if err != nil {
				t.Errorf("decode commit: %v", err)
				return nil
			}
			mu.Lock()
			commitReceived = commit.JTI == "jti-1"
			mu.Unlock()
		}
		return nil
	}}
	dest = &fakeAgent{send: func(channel string, data []byte) error {
		if channel == ChannelClaim {
			claim, err := DecodeClaimPayload(data)
			if err != nil {
				t.Errorf("decode claim: %v", err)
				return nil
			}
			mu.Lock()
			claimReceivedJWT = claim.JWT
			mu.Unlock()
			go func() {
				promised, _ := PromisedPayload{JTI: "jti-1"}.Encode()
				dest.deliver(ChannelPromised, promised)
			}()
		}
		return nil
	}}

	c := New(map[string]AgentMessenger{"source": source, "dest": dest}, nil)
	c.Attach()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	outcome, err := c.StartTransfer(ctx, "source", "dest", "5")
	if err != nil {
		t.Fatalf("StartTransfer: %v", err)
	}
	if outcome.JTI != "jti-1" {
		t.Fatalf("outcome.JTI = %q, want jti-1", outcome.JTI)
	}

	mu.Lock()
	defer mu.Unlock()
	if claimReceivedJWT != "signed.jwt.text" {
		t.Fatalf("dest never received the forwarded jwt, got %q", claimReceivedJWT)
	}
	if !commitReceived {
		t.Fatal("source never received commit")
	}
}

// TestStartTransferRequestError verifies a request-phase error (no jti yet)
// is correlated back to the right pending request via the FIFO queue and
// surfaces as an error from StartTransfer, not a hang.
func TestStartTransferRequestError(t *testing.T) {
	var source *fakeAgent
	source = &fakeAgent{send: func(channel string, data []byte) error {
		if channel == ChannelRequest {
			go func() {
				errPayload, _ := ErrorPayload{JTI: "", Code: "INVALID_SLOT", Message: "slot 99 out of range"}.Encode()
				source.deliver(ChannelError, errPayload)
			}()
		}
		return nil
	}}
	dest := &fakeAgent{}

	c := New(map[string]AgentMessenger{"source": source, "dest": dest}, nil)
	c.Attach()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.StartTransfer(ctx, "source", "dest", "99")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestStartTransferConcurrentSameSource verifies two concurrent transfers
// from the same source server are each matched to their own reply via FIFO
// ordering, not cross-wired.
func TestStartTransferConcurrentSameSource(t *testing.T) {
	var source, dest *fakeAgent
	var mu sync.Mutex
	slotToJTI := map[string]string{"1": "jti-a", "2": "jti-b"}

	source = &fakeAgent{send: func(channel string, data []byte) error {
		switch channel {
		case ChannelRequest:
			req, _ := DecodeRequestPayload(data)
			jti := slotToJTI[req.SlotID]
			go func() {
				hold, _ := HoldPayload{JWT: "jwt-" + jti, JTI: jti}.Encode()
				source.deliver(ChannelHold, hold)
			}()
		}
		return nil
	}}
	dest = &fakeAgent{send: func(channel string, data []byte) error {
		if channel == ChannelClaim {
			claim, _ := DecodeClaimPayload(data)
			go func() {
				// jwt is "jwt-<jti>" by construction above.
				jti := claim.JWT[len("jwt-"):]
				promised, _ := PromisedPayload{JTI: jti}.Encode()
				dest.deliver(ChannelPromised, promised)
			}()
		}
		return nil
	}}

	c := New(map[string]AgentMessenger{"source": source, "dest": dest}, nil)
	c.Attach()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(map[string]string) // slot -> jti
	for _, slot := range []string{"1", "2"} {
		slot := slot
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := c.StartTransfer(ctx, "source", "dest", slot)
			if err != nil {
				t.Errorf("StartTransfer(slot=%s): %v", slot, err)
				return
			}
			mu.Lock()
			results[slot] = outcome.JTI
			mu.Unlock()
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	for slot, wantJTI := range slotToJTI {
		if got := results[slot]; got != wantJTI {
			t.Errorf("slot %s got jti %q, want %q", slot, got, wantJTI)
		}
	}
}

func TestStartTransferUnknownServerLabel(t *testing.T) {
	c := New(map[string]AgentMessenger{"source": &fakeAgent{}}, nil)
	_, err := c.StartTransfer(context.Background(), "source", "nonexistent", "1")
	if err == nil {
		t.Fatal("expected an error for an unknown destination label")
	}
}
