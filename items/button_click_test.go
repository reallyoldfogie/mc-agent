package items

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			clicker := NewButtonClicker(client, packetMgr)

			err := clicker.ClickButton(tt.windowID, tt.buttonID)

			require.NoError(t, err)
			require.Len(t, client.packets, 1, "Should send exactly one packet")

			// Verify packet structure
			packet := client.packets[0]

			// Packet ID should be for ServerboundContainerButtonClick
			expectedID := packetMgr.GetServerboundPacketID("ServerboundContainerButtonClick")
			assert.Equal(t, int32(expectedID), packet.ID, "Packet ID should match ServerboundContainerButtonClick")

			// Decode and verify fields
			var windowID, buttonID pk.Byte
			err = packet.Scan(&windowID, &buttonID)
			require.NoError(t, err, "Should decode packet fields")

			assert.Equal(t, tt.windowID, byte(windowID), "Window ID should match")
			assert.Equal(t, tt.buttonID, byte(buttonID), "Button ID should match")
		})
	}
}

func TestClickButton_Integration(t *testing.T) {
	// Test that the packet structure matches what we expect
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	clicker := NewButtonClicker(client, packetMgr)

	// Click button 7 in window 3
	err := clicker.ClickButton(3, 7)
	require.NoError(t, err)

	// Verify the packet
	require.Len(t, client.packets, 1)
	packet := client.packets[0]

	var windowID, buttonID pk.Byte
	err = packet.Scan(&windowID, &buttonID)
	require.NoError(t, err)

	assert.Equal(t, byte(3), byte(windowID))
	assert.Equal(t, byte(7), byte(buttonID))
}
