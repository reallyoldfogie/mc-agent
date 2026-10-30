package world

import "testing"

func TestManager_GetHeightBounds_DefaultsToOverworld(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	minY, maxY := m.GetHeightBounds()
	if minY != -64 || maxY != 319 {
		t.Fatalf("GetHeightBounds() = (%d, %d), want (-64, 319)", minY, maxY)
	}
}

func TestManager_SetHeightBounds_RoundTrip(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)

	// Nether/End: min_y=0, height=256 -> world-Y bounds [0, 255].
	m.SetHeightBounds(0, 255)
	minY, maxY := m.GetHeightBounds()
	if minY != 0 || maxY != 255 {
		t.Fatalf("GetHeightBounds() after SetHeightBounds(0, 255) = (%d, %d), want (0, 255)", minY, maxY)
	}

	// Back to the Overworld's own bounds.
	m.SetHeightBounds(-64, 319)
	minY, maxY = m.GetHeightBounds()
	if minY != -64 || maxY != 319 {
		t.Fatalf("GetHeightBounds() after SetHeightBounds(-64, 319) = (%d, %d), want (-64, 319)", minY, maxY)
	}
}

// TestManager_HandleChunkLoad_UsesCurrentHeightBounds confirms a chunk
// loaded after SetHeightBounds gets the new dimension's MinSectionY/
// SectionCount, not whatever the Manager started with - this is the
// actual wiring onRespawn depends on, not just the getter/setter pair in
// isolation.
func TestManager_HandleChunkLoad_UsesCurrentHeightBounds(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	m.SetHeightBounds(0, 255) // Nether/End shape: 16 sections

	if err := m.HandleChunkLoad(0, 0, buildRawChunk(16)); err != nil {
		t.Fatalf("HandleChunkLoad failed: %v", err)
	}

	m.mu.RLock()
	chunk := m.Columns[ChunkPos{X: 0, Z: 0}]
	m.mu.RUnlock()
	if chunk == nil {
		t.Fatal("chunk not stored")
	}
	if chunk.MinSectionY != 0 || chunk.SectionCount != 16 {
		t.Fatalf("chunk.MinSectionY/SectionCount = %d/%d, want 0/16", chunk.MinSectionY, chunk.SectionCount)
	}

	// And it should actually decode correctly end to end through the
	// Manager's own GetBlockAt, not just ChunkData's.
	if got, loaded := m.GetBlockAt(0, 240, 0); !loaded || got != 16 {
		t.Fatalf("GetBlockAt(0, 240, 0) = (%d, %v), want (16, true)", got, loaded)
	}
}
