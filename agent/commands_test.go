package agent

import (
	"context"
	"strings"
	"testing"
)

type fakeChat struct{ msgs []string }

func (f *fakeChat) SendMessage(s string) error { f.msgs = append(f.msgs, s); return nil }

func TestHandleChatCommand_Help(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	a.handleChatCommand("help")
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "Commands:") {
		t.Fatalf("expected help response, got %#v", fc.msgs)
	}
}

func TestHandleChatCommand_Say(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	a.handleChatCommand("say hello world")
	if len(fc.msgs) == 0 || fc.msgs[len(fc.msgs)-1] != "hello world" {
		t.Fatalf("expected echo, got %#v", fc.msgs)
	}
}

func TestHandleChatCommand_Pos_Uninitialized(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	a.handleChatCommand("pos")
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "not initialized") {
		t.Fatalf("expected not initialized, got %#v", fc.msgs)
	}
}

func TestHandleChatCommand_Pos_Initialized(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	a.UpdatePosition(1, 2, 3, 0, 0)
	a.handleChatCommand("pos")
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "1.00") {
		t.Fatalf("expected position print, got %#v", fc.msgs)
	}
}
