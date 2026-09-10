package courier

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeChatResponder is TransferAction.run's minimal test double - it only
// needs SendChat, unlike the full models.CommandAgent Execute is forced to
// accept (see ChatResponder's doc comment).
type fakeChatResponder struct {
	mu       sync.Mutex
	messages []string
}

func (f *fakeChatResponder) SendChat(message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, message)
	return nil
}

func (f *fakeChatResponder) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func TestTransferActionUsageError(t *testing.T) {
	action := TransferAction{SourceLabel: "source"}
	chat := &fakeChatResponder{}
	if _, err := action.run(context.Background(), chat, []string{"only-one-arg"}); err == nil {
		t.Fatal("expected a usage error for the wrong number of args")
	}
	if _, err := action.run(context.Background(), chat, nil); err == nil {
		t.Fatal("expected a usage error for zero args")
	}
}

func TestTransferActionHappyPath(t *testing.T) {
	var source, dest *fakeAgent
	source = &fakeAgent{send: func(channel string, data []byte) error {
		if channel == ChannelRequest {
			go func() {
				hold, _ := HoldPayload{JWT: "signed.jwt.text", JTI: "jti-1"}.Encode()
				source.deliver(ChannelHold, hold)
			}()
		}
		return nil
	}}
	dest = &fakeAgent{send: func(channel string, data []byte) error {
		if channel == ChannelClaim {
			go func() {
				promised, _ := PromisedPayload{JTI: "jti-1"}.Encode()
				dest.deliver(ChannelPromised, promised)
			}()
		}
		return nil
	}}

	c := New(map[string]AgentMessenger{"source": source, "dest": dest}, nil)
	c.Attach()

	action := TransferAction{Courier: c, SourceLabel: "source"}
	chat := &fakeChatResponder{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	completion, err := action.run(ctx, chat, []string{"5", "dest"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := completion.Wait(ctx); err != nil {
		t.Fatalf("completion.Wait: %v", err)
	}

	msgs := chat.snapshot()
	if len(msgs) != 2 {
		t.Fatalf("chat messages = %v, want 2 messages (starting + complete)", msgs)
	}
	if !strings.Contains(msgs[0], "Starting transfer") {
		t.Errorf("first message = %q, want a Starting transfer message", msgs[0])
	}
	if !strings.Contains(msgs[1], "Transfer complete") || !strings.Contains(msgs[1], "jti-1") {
		t.Errorf("second message = %q, want a Transfer complete message naming jti-1", msgs[1])
	}
}

func TestTransferActionFailure(t *testing.T) {
	source := &fakeAgent{send: func(channel string, data []byte) error {
		return nil // request never answered -> StartTransfer times out
	}}
	dest := &fakeAgent{}

	c := New(map[string]AgentMessenger{"source": source, "dest": dest}, nil)
	c.Attach()

	action := TransferAction{Courier: c, SourceLabel: "source"}
	chat := &fakeChatResponder{}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	completion, err := action.run(ctx, chat, []string{"5", "dest"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if err := completion.Wait(waitCtx); err == nil {
		t.Fatal("expected the transfer to fail (request never answered)")
	}

	msgs := chat.snapshot()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "Transfer failed") {
		t.Fatalf("chat messages = %v, want a Transfer failed message second", msgs)
	}
}

func TestTransferActionUnknownDestLabel(t *testing.T) {
	c := New(map[string]AgentMessenger{"source": &fakeAgent{}}, nil)
	action := TransferAction{Courier: c, SourceLabel: "source"}
	chat := &fakeChatResponder{}

	completion, err := action.run(context.Background(), chat, []string{"5", "nonexistent"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := completion.Wait(waitCtx); err == nil {
		t.Fatal("expected failure for an unknown destination label")
	}
}
