package agent

import (
	"testing"
)

// These are lightweight dispatch/argument-validation tests (no live server,
// no RCON configured) proving the followCam/stopFollowCam chat commands are
// correctly registered and route to the right agent method. Full end-to-end
// coverage (RCON forcing spectator mode, the teleport-snap loop actually
// keeping distance, including across a simulated teleport) is in
// testing/cam_follow_test.go against a real server.

func TestHandleChatCommand_FollowCam_MissingArg(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("followcam")
	if !capture.ContainsMessage("Usage: followCam") {
		t.Fatalf("expected usage message, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_FollowCam_NoRCON(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	// setupChatCapture never configures RCON, so this exercises argument
	// parsing and dispatch to StartCamFollow, which then fails cleanly
	// (RCON is checked before anything network-related is touched) rather
	// than reaching a live connection.
	agent.handleChatCommand("followcam SomePlayer")
	if !capture.ContainsMessage("FollowCam failed") {
		t.Fatalf("expected followCam failure (no RCON configured), got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_FollowCam_InvalidDistance(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("followcam SomePlayer notanumber")
	if !capture.ContainsMessage("Invalid maxDistance") {
		t.Fatalf("expected invalid distance message, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_StopFollowCam(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	// StopCamFollow is a no-op when nothing is following, so this should
	// always succeed cleanly regardless of RCON/connection state.
	agent.handleChatCommand("stopfollowcam")
	if !capture.ContainsMessage("Stopped following") {
		t.Fatalf("expected stop confirmation, got %#v", capture.GetMessages())
	}
}
