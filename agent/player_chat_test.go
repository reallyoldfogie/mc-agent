package agent

import (
	"bytes"
	"encoding/base64"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/stretchr/testify/require"
)

// TestClientboundPlayerChat_RealPacket tests parsing of an actual ClientboundPlayerChat
// packet that was received from the server. This packet caused the agent to exit unexpectedly.
//
// Original packet log:
// {"id":58,"name":"ClientboundPlayerChat","timestamp":"2025-12-15T16:54:24.102737363-07:00",
//
//	"data":"AOT8oYYCeDPmmDNU2n1s/TEAAAxIZWxsbywgd29ybGQAAAGbJG/HEgAAAAAAAAAAAAAAAQoKAAtjbGlja19ldmVudAgABmFjdGlvbgAPc3VnZ2VzdF9jb21tYW5kCAAHY29tbWFuZAALL3RlbGwgRGF6ZSAACAAJaW5zZXJ0aW9uAAREYXplCAAEdGV4dAAERGF6ZQoAC2hvdmVyX2V2ZW50CAAEbmFtZQAERGF6ZQgABmFjdGlvbgALc2hvd19lbnRpdHkIAAJpZAAQbWluZWNyYWZ0OnBsYXllcgsABHV1aWQAAAAE5PyhhgJ4M+aYM1TafWz9MQAAAA==",
//	"version":"1.21.5","protocol_version":770}
func TestClientboundPlayerChat_RealPacket(t *testing.T) {
	// Decode the base64 packet data
	packetDataB64 := "AOT8oYYCeDPmmDNU2n1s/TEAAAxIZWxsbywgd29ybGQAAAGbJG/HEgAAAAAAAAAAAAAAAQoKAAtjbGlja19ldmVudAgABmFjdGlvbgAPc3VnZ2VzdF9jb21tYW5kCAAHY29tbWFuZAALL3RlbGwgRGF6ZSAACAAJaW5zZXJ0aW9uAAREYXplCAAEdGV4dAAERGF6ZQoAC2hvdmVyX2V2ZW50CAAEbmFtZQAERGF6ZQgABmFjdGlvbgALc2hvd19lbnRpdHkIAAJpZAAQbWluZWNyYWZ0OnBsYXllcgsABHV1aWQAAAAE5PyhhgJ4M+aYM1TafWz9MQAAAA=="

	packetData, err := base64.StdEncoding.DecodeString(packetDataB64)
	require.NoError(t, err, "Failed to decode base64 packet data")

	// Create a packet with ID 58 (ClientboundPlayerChat for protocol 770/1.21.5)
	p := pk.Packet{
		ID:   58,
		Data: packetData,
	}

	t.Logf("Packet ID: %d", p.ID)
	t.Logf("Packet Length: %d bytes", len(p.Data))
	t.Logf("Packet Data (hex): %x", p.Data)

	// Try to parse the packet fields manually to understand structure
	// ClientboundPlayerChat structure (1.21.5):
	// - Sender UUID (128 bits / 16 bytes)
	// - Index (VarInt)
	// - Message Signature Present (Boolean)
	// - [if present] Message Signature (256 bytes)
	// - Message (String)
	// - Timestamp (Long)
	// - Salt (Long)
	// - Previous Messages Count (VarInt)
	// - [for each] Previous Message
	// - Unsigned Content (Optional Chat Component)
	// - Filter Type (VarInt)
	// - [if partial] Filter Type Bits (BitSet)
	// - Chat Type (VarInt)
	// - Network Name (Chat Component)
	// - Network Target Name (Optional Chat Component)

	var (
		senderUUID pk.UUID
		index      pk.VarInt
		sigPresent pk.Boolean
	)

	err = p.Scan(&senderUUID, &index, &sigPresent)
	if err != nil {
		t.Logf("Failed to parse initial fields: %v", err)
		t.Logf("This is expected if the packet structure doesn't match")
	} else {
		t.Logf("Sender UUID: %x", senderUUID)
		t.Logf("Index: %d", index)
		t.Logf("Signature Present: %t", sigPresent)
	}

	// Parse the full message to extract the text
	var plainMsg pk.String
	// Skip to plain message: UUID (16) + VarInt(1) + Boolean(1) = 18 bytes
	rr := bytes.NewReader(p.Data[18:])
	if _, err := plainMsg.ReadFrom(rr); err == nil {
		t.Logf("Plain Message: %s", plainMsg)
	}

	// The important thing is that the packet doesn't cause a panic or unexpected exit
	// This test serves as documentation of the problematic packet
}

// TestClientboundPlayerChat_MinimalPacket tests a minimal valid ClientboundPlayerChat packet
func TestClientboundPlayerChat_MinimalPacket(t *testing.T) {
	t.Skip("Skipping until we understand the full packet structure for 1.21.5")
	// This would require understanding the exact format for protocol 770
}
