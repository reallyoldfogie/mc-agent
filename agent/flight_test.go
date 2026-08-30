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
