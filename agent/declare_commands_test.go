package agent

import (
	"bytes"
	"encoding/base64"
	"testing"

	v1_21_5 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
)

// TestDeclareCommandsPacketParsing tests parsing of a real DeclareCommands packet
// that was captured from a live server. This packet was causing EOF errors during
// MCPR replay validation.
func TestDeclareCommandsPacketParsing(t *testing.T) {
	// This is the actual packet data from the log entry:
	// {"id":16,"name":"ClientboundDeclareCommands","timestamp":"2025-12-16T07:22:49.119073133-07:00",
	//  "data":"GQAKAQIDBAUGBwgJCgEBCwJtZQUBDARoZWxwBQENBGxpc3QBAQ4DbXNnCQAEBHRlbGwJAAQBdwECDxAGcmFuZG9t...",
	//  "version":"1.21.5","protocol_version":770}
	packetData := "GQAKAQIDBAUGBwgJCgEBCwJtZQUBDARoZWxwBQENBGxpc3QBAQ4DbXNnCQAEBHRlbGwJAAQBdwECDxAGcmFuZG9tAQERB3RlYW1tc2cJAAgCdG0BARIHdHJpZ2dlcgYABmFjdGlvbhMGAAdjb21tYW5kBQIFAAV1dWlkcwIBEwd0YXJnZXRzBgIBARQFdmFsdWUBARUEcm9sbAYAB21lc3NhZ2UTFgIWFwlvYmplY3RpdmUXFG1pbmVjcmFmdDphc2tfc2VydmVyBgAHbWVzc2FnZRMGAAVyYW5nZSYGAAVyYW5nZSYBARgDYWRkAQEYA3NldAYABXZhbHVlAwAA"

	// Decode the base64 data
	data, err := base64.StdEncoding.DecodeString(packetData)
	if err != nil {
		t.Fatalf("Failed to decode base64 packet data: %v", err)
	}

	t.Logf("Packet data: %d bytes", len(data))
	t.Logf("First 32 bytes: % x", data[:min(32, len(data))])

	// Create a reader from the data
	reader := bytes.NewReader(data)

	// Create a new DeclareCommands packet
	packet := v1_21_5.NewDeclareCommands()

	// Try to parse the packet
	bytesRead, err := packet.ReadFrom(reader)
	if err != nil {
		t.Fatalf("Failed to parse DeclareCommands packet: %v (read %d/%d bytes)", err, bytesRead, len(data))
	}

	// Verify we read all the data
	if bytesRead != int64(len(data)) {
		t.Errorf("Expected to read %d bytes, but read %d bytes", len(data), bytesRead)
	}

	// Get the parsed data
	nodes := packet.GetNodes().Get()
	rootIndex := packet.GetRootIndex()

	// Basic validation
	if nodes == nil {
		t.Fatal("Nodes array is nil")
	}

	nodeCount := len(*nodes)
	t.Logf("Successfully parsed %d command nodes, root index: %d", nodeCount, rootIndex)

	// Verify we have a reasonable number of nodes
	if nodeCount == 0 {
		t.Error("Expected at least one command node")
	}

	// Verify root index is valid
	if int(rootIndex) >= nodeCount {
		t.Errorf("Root index %d is out of bounds for %d nodes", rootIndex, nodeCount)
	}

	// Log details about the first few nodes for debugging
	for i := 0; i < min(5, nodeCount); i++ {
		node := (*nodes)[i]
		childCount := len(*node.Children.Get())
		t.Logf("  Node[%d]: Type=%d, HasCommand=%d, HasRedirect=%d, Children=%d",
			i, node.Flags.CommandNodeType, node.Flags.HasCommand,
			node.Flags.HasRedirectNode, childCount)
	}
}

// TestDeclareCommandsEmptyPacket tests parsing an empty commands packet
func TestDeclareCommandsEmptyPacket(t *testing.T) {
	// A packet with 0 nodes and root index 0
	// VarInt 0 (nodes count) + VarInt 0 (root index) = [0x00, 0x00]
	data := []byte{0x00, 0x00}

	reader := bytes.NewReader(data)
	packet := v1_21_5.NewDeclareCommands()

	bytesRead, err := packet.ReadFrom(reader)
	if err != nil {
		t.Fatalf("Failed to parse empty DeclareCommands packet: %v", err)
	}

	if bytesRead != int64(len(data)) {
		t.Errorf("Expected to read %d bytes, but read %d bytes", len(data), bytesRead)
	}

	nodes := packet.GetNodes().Get()
	if len(*nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(*nodes))
	}

	if packet.GetRootIndex() != 0 {
		t.Errorf("Expected root index 0, got %d", packet.GetRootIndex())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
