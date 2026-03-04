package agent

import (
	"testing"
)

func TestHandleChatCommand_Help(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("help")
	if !capture.ContainsMessage("Commands:") {
		t.Fatalf("expected help response, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_Say(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("say hello world")
	if capture.GetLastMessage() != "hello world" {
		t.Fatalf("expected echo, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_Pos_Uninitialized(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("pos")
	if !capture.ContainsMessage("not initialized") {
		t.Fatalf("expected not initialized, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_Pos_Initialized(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.UpdatePosition(1, 2, 3, 0, 0)
	agent.handleChatCommand("pos")
	if !capture.ContainsMessage("1.00") {
		t.Fatalf("expected position print, got %#v", capture.GetMessages())
	}
}
