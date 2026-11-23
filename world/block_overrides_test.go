package world

import "testing"

func TestSetGetBlockOverrideRoundTrips(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	m.setBlockOverride(5, 64, -3, 42)

	got, ok := m.getBlockOverride(5, 64, -3)
	if !ok || got != 42 {
		t.Fatalf("getBlockOverride(5, 64, -3) = %d, %v, want 42, true", got, ok)
	}

	if _, ok := m.getBlockOverride(5, 64, -4); ok {
		t.Fatalf("getBlockOverride for an untouched position reported ok=true")
	}
}

// TestChunkAndLocalPosHandlesNegativeCoordinates confirms -1 lands in
// chunk -1 at local offset 15, not chunk 0 at local offset -1 - see
// chunkAndLocalPos's own doc comment for why its >>4/&15 split already
// gets this right without any extra floor-division handling.
func TestChunkAndLocalPosHandlesNegativeCoordinates(t *testing.T) {
	chunkPos, local := chunkAndLocalPos(-1, 64, -17)
	if want := (ChunkPos{X: -1, Z: -2}); chunkPos != want {
		t.Fatalf("chunkAndLocalPos(-1, 64, -17) chunk = %+v, want %+v", chunkPos, want)
	}
	if want := (localBlockPos{X: 15, Z: 15, Y: 64}); local != want {
		t.Fatalf("chunkAndLocalPos(-1, 64, -17) local = %+v, want %+v", local, want)
	}
}

func TestSetBlockOverrideRoundTripsAcrossNegativeChunkBoundary(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	m.setBlockOverride(-1, 64, -17, 7)

	got, ok := m.getBlockOverride(-1, 64, -17)
	if !ok || got != 7 {
		t.Fatalf("getBlockOverride(-1, 64, -17) = %d, %v, want 7, true", got, ok)
	}
}

func TestClearOverridesForChunkOnlyClearsThatChunk(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	m.setBlockOverride(5, 64, 5, 1)  // chunk (0,0)
	m.setBlockOverride(20, 64, 5, 2) // chunk (1,0)

	m.clearOverridesForChunk(0, 0)

	if _, ok := m.getBlockOverride(5, 64, 5); ok {
		t.Fatalf("override in cleared chunk (0,0) still present")
	}
	got, ok := m.getBlockOverride(20, 64, 5)
	if !ok || got != 2 {
		t.Fatalf("override in untouched chunk (1,0) = %d, %v, want 2, true", got, ok)
	}
}

func TestClearAllOverridesRemovesEverything(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	m.setBlockOverride(5, 64, 5, 1)
	m.setBlockOverride(200, 64, 200, 2)

	m.clearAllOverrides()

	if _, ok := m.getBlockOverride(5, 64, 5); ok {
		t.Fatalf("override present after clearAllOverrides")
	}
	if _, ok := m.getBlockOverride(200, 64, 200); ok {
		t.Fatalf("override present after clearAllOverrides")
	}
}

// TestGetBlockAtPrefersOverrideOverLoadedChunkData exercises the real,
// exported call path (HandleBlockUpdate -> GetBlockAt) every agent
// actually uses, on top of the two-level map internals the tests above
// cover directly.
func TestGetBlockAtPrefersOverrideOverLoadedChunkData(t *testing.T) {
	m := NewManager(nil, EventsListener{}, nil)
	m.HandleBlockUpdate(5, 64, 5, 99)

	got, ok := m.GetBlockAt(5, 64, 5)
	if !ok || got != 99 {
		t.Fatalf("GetBlockAt(5, 64, 5) = %d, %v, want 99, true", got, ok)
	}
}
