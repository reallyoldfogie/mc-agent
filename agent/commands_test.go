package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeChat struct{ msgs []string }

func (f *fakeChat) SendMessage(s string) error { f.msgs = append(f.msgs, s); return nil }

func TestHandleChatCommand_Help(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.handleChatCommand("help")
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "Commands:") {
		t.Fatalf("expected help response, got %#v", fc.msgs)
	}
}

func TestHandleChatCommand_Say(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.handleChatCommand("say hello world")
	if len(fc.msgs) == 0 || fc.msgs[len(fc.msgs)-1] != "hello world" {
		t.Fatalf("expected echo, got %#v", fc.msgs)
	}
}

func TestHandleChatCommand_Pos_Uninitialized(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.handleChatCommand("pos")
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "not initialized") {
		t.Fatalf("expected not initialized, got %#v", fc.msgs)
	}
}

func TestHandleChatCommand_Pos_Initialized(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.UpdatePosition(1, 2, 3, 0, 0)
	agent.handleChatCommand("pos")
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "1.00") {
		t.Fatalf("expected position print, got %#v", fc.msgs)
	}
}
