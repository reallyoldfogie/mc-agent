package items

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockContainerHandler implements common.ContainerHandler for testing
type mockContainerHandler struct {
	buttonClicks []buttonClickCall
}

type buttonClickCall struct {
	windowID int8
	buttonID int8
}

func (m *mockContainerHandler) SendContainerButtonClick(conn common.PacketWriter, windowID int8, buttonID int8) error {
	m.buttonClicks = append(m.buttonClicks, buttonClickCall{windowID: windowID, buttonID: buttonID})
	return nil
}

// Stub implementations for other ContainerHandler methods
func (m *mockContainerHandler) SendContainerClick(conn common.PacketWriter, windowID int8, stateID, slot int32, button int8, mode int32, changedSlots map[int16]common.Slot, carriedItem common.Slot) error {
	return nil
}
func (m *mockContainerHandler) SendContainerClose(conn common.PacketWriter, windowID int8) error {
	return nil
}
func (m *mockContainerHandler) SendSetCreativeModeSlot(conn common.PacketWriter, slot int16, item common.Slot) error {
	return nil
}
func (m *mockContainerHandler) SendPickItem(conn common.PacketWriter, slot int32) error {
	return nil
}
func (m *mockContainerHandler) SendSetCarriedItem(conn common.PacketWriter, slot int16) error {
	return nil
}
func (m *mockContainerHandler) SendUseItemOn(conn common.PacketWriter, hand models.Hand, x, y, z int, face int32, cursorX, cursorY, cursorZ float32, insideBlock bool, sequence int32) error {
	return nil
}
func (m *mockContainerHandler) ParseOpenScreen(p pk.Packet) (windowID int8, windowType int32, title string, err error) {
	return 0, 0, "", nil
}
func (m *mockContainerHandler) ParseContainerSetContent(p pk.Packet) (windowID int8, stateID int32, slots []common.Slot, carriedItem common.Slot, err error) {
	return 0, 0, nil, common.Slot{}, nil
}
func (m *mockContainerHandler) ParseContainerSetSlot(p pk.Packet) (windowID int8, stateID int32, slot int16, item common.Slot, err error) {
	return 0, 0, 0, common.Slot{}, nil
}
func (m *mockContainerHandler) ParseHeldItemSlot(p pk.Packet) (int16, error) {
	return 0, nil
}

func TestClickButton(t *testing.T) {
	tests := []struct {
		name     string
		windowID byte
		buttonID byte
	}{
		{
			name:     "Enchanting table - first enchantment",
			windowID: 1,
			buttonID: 0,
		},
		{
			name:     "Enchanting table - second enchantment",
			windowID: 1,
			buttonID: 1,
		},
		{
			name:     "Stonecutter - select recipe 5",
			windowID: 2,
			buttonID: 5,
		},
		{
			name:     "Loom - select pattern 10",
			windowID: 3,
			buttonID: 10,
		},
		{
			name:     "Beacon - confirm effect",
			windowID: 4,
			buttonID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockPacketSender{}
			packetMgr := newMockPacketManager()
			mockHandler := &mockContainerHandler{}

			clicker := NewButtonClicker(client, packetMgr)
			clicker.SetContainerHandler(mockHandler)

			err := clicker.ClickButton(tt.windowID, tt.buttonID)

			require.NoError(t, err)
			require.Len(t, mockHandler.buttonClicks, 1, "Should record exactly one button click")

			// Verify the call was made with correct parameters
			call := mockHandler.buttonClicks[0]
			assert.Equal(t, int8(tt.windowID), call.windowID, "Window ID should match")
			assert.Equal(t, int8(tt.buttonID), call.buttonID, "Button ID should match")
		})
	}
}

func TestClickButton_NoHandler(t *testing.T) {
	// Test that ClickButton returns an error when no handler is set
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()

	clicker := NewButtonClicker(client, packetMgr)
	// Don't set container handler

	err := clicker.ClickButton(1, 0)
	require.Error(t, err, "Should return error when handler not set")
	assert.Contains(t, err.Error(), "ContainerHandler not set")
}

func TestClickButton_Integration(t *testing.T) {
	// Test that the handler method is called with correct parameters
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	mockHandler := &mockContainerHandler{}

	clicker := NewButtonClicker(client, packetMgr)
	clicker.SetContainerHandler(mockHandler)

	// Click button 7 in window 3
	err := clicker.ClickButton(3, 7)
	require.NoError(t, err)

	// Verify the call
	require.Len(t, mockHandler.buttonClicks, 1)
	call := mockHandler.buttonClicks[0]
	assert.Equal(t, int8(3), call.windowID)
	assert.Equal(t, int8(7), call.buttonID)
}
