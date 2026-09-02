package agent

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

func TestHelpListsLegacyCommands(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	capture := newCaptureChat()
	agent.SetChat(capture)
	agent.handleChatCommand("help")
	msgs := capture.GetMessages()
	if len(msgs) == 0 {
		t.Fatalf("no help message")
	}
	help := msgs[len(msgs)-1]
	required := []string{"testMove", "moveTo", "moveForward", "moveUp", "findPath", "testPath", "follow", "stopFollow", "followStatus", "startTracking", "stopTracking", "fireBow"}
	for _, r := range required {
		if !strings.Contains(strings.ToLower(help), strings.ToLower(r)) {
			t.Fatalf("help missing %q: %s", r, help)
		}
	}
}

type fakeSlotResolver struct {
	id, count int
	itemID    int32
	idx       int16
	ok        bool
}

func (f fakeSlotResolver) ResolveSlot(id int, index int16) (int32, int, bool) {
	return f.itemID, f.count, f.ok
}

type fakeItemMgr struct{ name string }

func (f fakeItemMgr) GetItemNameByID(id int32) string { return f.name }

func TestOnScreenSlotChange_DecodesItem(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.SetSlotResolver(fakeSlotResolver{itemID: 5, count: 3, ok: true})
	agent.SetItemManager(fakeItemMgr{name: "TestItem"})
	var buf bytes.Buffer
	agent.logWriter.setTarget(&buf)
	defer agent.logWriter.setTarget(os.Stdout)
	if err := agent.OnScreenSlotChange(0, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "TestItem") || !strings.Contains(out, "x3") || !strings.Contains(out, "id=5") {
		t.Fatalf("expected decoded item log, got: %s", out)
	}
}
