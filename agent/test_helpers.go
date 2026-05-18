package agent

import (
	"context"
	"strings"
	"sync"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
)

// captureChat is a Chat implementation that captures messages sent via SendMessage.
// This is the standard test chat for verifying command output without a network connection.
type captureChat struct {
	mu   sync.Mutex
	msgs []string
}

func newCaptureChat() *captureChat {
	return &captureChat{
		msgs: []string{},
	}
}

func (c *captureChat) SendMessage(msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, msg)
	return nil
}

func (c *captureChat) GetMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.msgs))
	copy(result, c.msgs)
	return result
}

func (c *captureChat) GetLastMessage() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.msgs) == 0 {
		return ""
	}
	return c.msgs[len(c.msgs)-1]
}

func (c *captureChat) ContainsMessage(substring string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, msg := range c.msgs {
		if containsCI(msg, substring) {
			return true
		}
	}
	return false
}

// containsCI checks if a substring is in a string (case-insensitive)
func containsCI(s, substr string) bool {
	sLower := strings.ToLower(s)
	subLower := strings.ToLower(substr)
	return strings.Contains(sLower, subLower)
}

// containsMsg checks if any message in a slice contains a substring (case-insensitive).
// This is a helper for tests to check captured messages.
func containsMsg(msgs []string, sub string) bool {
	subLower := strings.ToLower(sub)
	for _, m := range msgs {
		if containsCI(strings.ToLower(m), subLower) {
			return true
		}
	}
	return false
}

// setupChatCapture is a simple helper to create an agent and return a captureChat for testing.
// This is the preferred way for tests that need to capture chat output.
func setupChatCapture(t interface{ Fatalf(string, ...interface{}) }, version string) (*agent, *captureChat) {
	agentInt, err := New(models.AgentConfig{Version: version, Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	a := agentInt.(*agent)

	// Initialize with a background context
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init agent: %v", err)
	}

	// Create and inject capture chat
	capture := newCaptureChat()
	a.SetChat(capture)

	return a, capture
}

// extractChatMessageFromPacket extracts the message from a Serverbound Chat packet.
// This is used by tests to verify that SendChat sent the expected message via the version handler.
func extractChatMessageFromPacket(a *agent, pkt *pk.Packet) string {
	if pkt == nil {
		return ""
	}

	// The packet contains an encoded chat message.
	// For now, we'll use a simple heuristic: scan the packet data for strings.
	// A more robust approach would be to use the version handler to decode it,
	// but that's complex since we'd need to reconstruct the packet structure.
	//
	// For test purposes, we can rely on the fact that the message text is
	// often embedded in the packet data. This is a pragmatic approach that works
	// for most cases without requiring full packet decoding.

	data := pkt.Data
	if len(data) == 0 {
		return ""
	}

	// Scan for null-terminated UTF-8 strings in the packet data
	// (Minecraft uses a variant of this for string encoding)
	var result string
	for i := 0; i < len(data); i++ {
		// Look for reasonable ASCII ranges (32-126) followed by null or next length byte
		if data[i] >= 32 && data[i] <= 126 {
			s := ""
			for i < len(data) && data[i] >= 32 && data[i] <= 126 {
				s += string(data[i])
				i++
			}
			if len(s) > 2 { // Filter out very short matches
				result = s
			}
		}
	}
	return result
}
