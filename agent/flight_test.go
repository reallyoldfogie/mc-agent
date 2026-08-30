package agent

import (
	"testing"
)

// These are lightweight dispatch/argument-validation tests (no live server,
// no movement executor configured) proving the chat commands are correctly
// registered and route to the right agent method with the right arguments.
// The underlying flight/equip behavior itself is covered end to end against
// real servers in testing/elytra_commands_test.go.

func TestHandleChatCommand_Equip_MissingArg(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("equip")
	if !capture.ContainsMessage("Usage: equip") {
		t.Fatalf("expected usage message, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_Equip_NotFound(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("equip elytra")
	if !capture.ContainsMessage("Equip failed") {
		t.Fatalf("expected equip failure (no inventory/network), got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_UseItem_Offhand(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	// No version handler wired up without a live connection, so this
	// exercises argument parsing (offhand) and dispatch reaching UseItem,
	// which then fails cleanly rather than panicking.
	agent.handleChatCommand("useItem offhand")
	if !capture.ContainsMessage("Use item failed") {
		t.Fatalf("expected use item failure (no connection), got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_FlyTo_InvalidCoordinates(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("flyTo notanumber 0 0")
	if !capture.ContainsMessage("Invalid X coordinate") {
		t.Fatalf("expected invalid X coordinate message, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_FlyTo_MissingArgs(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("flyTo 1 2")
	if !capture.ContainsMessage("Usage: flyTo") {
		t.Fatalf("expected usage message, got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_Mount_NumericID(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	// No entities tracked without a live connection, so this exercises
	// argument parsing (numeric -> MountEntity) and dispatch, which then
	// fails cleanly rather than panicking.
	agent.handleChatCommand("mount 5")
	if !capture.ContainsMessage("Mount failed") {
		t.Fatalf("expected mount failure (entity not found), got %#v", capture.GetMessages())
	}
}

func TestHandleChatCommand_Mount_EntityType(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	// Non-numeric argument routes to MountNearest instead of MountEntity;
	// no nearby entities without a live connection, so this fails cleanly.
	agent.handleChatCommand("mount horse")
	if !capture.ContainsMessage("Mount failed") {
		t.Fatalf("expected mount failure (no nearby horse), got %#v", capture.GetMessages())
	}
}

// TestHandleChatCommand_Fly_NotAllowed exercises the "fly" command's
// dispatch and its AllowFlying gate (SetFlying refuses to enable flying
// without server-granted permission) without a live connection - abilities
// are never received in this harness, so AllowFlying defaults to false,
// giving a clean failure. Full end-to-end coverage (creative granting
// AllowFlying, the real toggle taking effect) is in
// testing/flying_command_test.go against a real server; "land" isn't
// tested here at all since, unlike "fly", it has no validation gate that
// runs before reaching the network layer - calling it with no live
// connection would reach a real packet write and panic, not fail cleanly.
func TestHandleChatCommand_Fly_NotAllowed(t *testing.T) {
	agent, capture := setupChatCapture(t, "1.21.5")

	agent.handleChatCommand("fly")
	if !capture.ContainsMessage("Fly failed") {
		t.Fatalf("expected fly failure (no server-granted AllowFlying), got %#v", capture.GetMessages())
	}
}
