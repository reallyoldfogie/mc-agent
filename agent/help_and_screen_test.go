package agent

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelpListsLegacyCommands(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.handleChatCommand("help")
	if len(fc.msgs) == 0 {
		t.Fatalf("no help message")
	}
	help := fc.msgs[len(fc.msgs)-1]
	required := []string{"testMove", "moveTo", "moveForward", "moveUp", "findPath", "testPath", "follow", "stopFollow", "followStatus", "startTracking", "stopTracking", "fireBow"}
	for _, r := range required {
		if !strings.Contains(strings.ToLower(help), strings.ToLower(r)) {
			t.Fatalf("help missing %q: %s", r, help)
		}
	}
}

type fakeSlotResolver struct {
	id, idx, itemID, count int
	ok                     bool
}

func (f fakeSlotResolver) ResolveSlot(id, index int) (int, int, bool) { return f.itemID, f.count, f.ok }

type fakeItemMgr struct{ name string }

func (f fakeItemMgr) GetItemNameByID(id int) string { return f.name }

func TestOnScreenSlotChange_DecodesItem(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	agent.SetSlotResolver(fakeSlotResolver{itemID: 5, count: 3, ok: true})
	agent.SetItemManager(fakeItemMgr{name: "TestItem"})
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)
	if err := agent.OnScreenSlotChange(0, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "TestItem") || !strings.Contains(out, "x3") || !strings.Contains(out, "id=5") {
		t.Fatalf("expected decoded item log, got: %s", out)
	}
}
