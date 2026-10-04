package agent

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// fakeBlockWorld is a minimal blockReader for findSupportFace's tests: a
// sparse set of solid cells (everything else reads as loaded, passable
// air), plus an optional set of cells that report chunkLoaded=false.
// GetBlockAt floors its input, matching the real GetBlockAt convention
// (callers pass a cell's center, i.e. cell+0.5) - findSupportFace relies on
// this, so the fake must too, including for negative coordinates.
type fakeBlockWorld struct {
	solid    map[[3]int]bool
	unloaded map[[3]int]bool
}

func (w fakeBlockWorld) GetBlockAt(x, y, z float64) (uint32, bool) {
	cell := [3]int{int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))}
	if w.unloaded[cell] {
		return 0, false
	}
	if w.solid[cell] {
		return 1, true // any nonzero stateID; fakePassability below treats 0 as passable, nonzero as solid
	}
	return 0, true
}

// fakePassability treats stateID 0 as passable (air) and anything else as
// solid - paired with fakeBlockWorld's stateID convention above.
type fakePassability struct{}

func (fakePassability) IsPassable(stateID uint32) bool { return stateID == 0 }

func solidAt(x, y, z int) [3]int { return [3]int{x, y, z} }

func TestFindSupportFace_PrefersDown(t *testing.T) {
	target := models.V3{X: 5, Y: 5, Z: 5}
	world := fakeBlockWorld{solid: map[[3]int]bool{
		solidAt(5, 4, 5): true, // below
		solidAt(4, 5, 5): true, // west - also solid, should lose to "down"
		solidAt(6, 5, 5): true, // east - also solid
	}}

	support, face, ok := findSupportFace(world, fakePassability{}, target)
	if !ok {
		t.Fatal("expected a support face, got ok=false")
	}
	wantSupport := models.V3{X: 5, Y: 4, Z: 5}
	if support != wantSupport {
		t.Errorf("support = %+v, want %+v (down should win over west/east)", support, wantSupport)
	}
	if face != models.FaceUp {
		t.Errorf("face = %v, want FaceUp (clicking the top of the block below)", face)
	}
}

func TestFindSupportFace_FallsBackThroughOrder(t *testing.T) {
	target := models.V3{X: 0, Y: 0, Z: 0}
	tests := []struct {
		name        string
		solidCell   [3]int
		wantSupport models.V3
		wantFace    models.BlockFace
	}{
		{"north neighbor", solidAt(0, 0, -1), models.V3{X: 0, Y: 0, Z: -1}, models.FaceSouth},
		{"south neighbor", solidAt(0, 0, 1), models.V3{X: 0, Y: 0, Z: 1}, models.FaceNorth},
		{"east neighbor", solidAt(1, 0, 0), models.V3{X: 1, Y: 0, Z: 0}, models.FaceWest},
		{"west neighbor", solidAt(-1, 0, 0), models.V3{X: -1, Y: 0, Z: 0}, models.FaceEast},
		{"above (last resort)", solidAt(0, 1, 0), models.V3{X: 0, Y: 1, Z: 0}, models.FaceDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := fakeBlockWorld{solid: map[[3]int]bool{tt.solidCell: true}}
			support, face, ok := findSupportFace(world, fakePassability{}, target)
			if !ok {
				t.Fatalf("expected a support face, got ok=false")
			}
			if support != tt.wantSupport {
				t.Errorf("support = %+v, want %+v", support, tt.wantSupport)
			}
			if face != tt.wantFace {
				t.Errorf("face = %v, want %v", face, tt.wantFace)
			}
		})
	}
}

func TestFindSupportFace_NoSolidNeighbor(t *testing.T) {
	target := models.V3{X: 0, Y: 0, Z: 0}
	world := fakeBlockWorld{} // nothing solid anywhere

	_, _, ok := findSupportFace(world, fakePassability{}, target)
	if ok {
		t.Fatal("expected ok=false when every neighbor is passable")
	}
}

func TestFindSupportFace_UnloadedNeighborSkipped(t *testing.T) {
	// Down is unloaded (treated as "can't place against this, chunk not
	// in view" rather than crashing or false-positiving); north is solid
	// and loaded, so it should be picked instead.
	target := models.V3{X: 0, Y: 0, Z: 0}
	world := fakeBlockWorld{
		unloaded: map[[3]int]bool{solidAt(0, -1, 0): true},
		solid:    map[[3]int]bool{solidAt(0, 0, -1): true},
	}

	support, face, ok := findSupportFace(world, fakePassability{}, target)
	if !ok {
		t.Fatal("expected a support face from the loaded, solid north neighbor")
	}
	if want := (models.V3{X: 0, Y: 0, Z: -1}); support != want {
		t.Errorf("support = %+v, want %+v", support, want)
	}
	if face != models.FaceSouth {
		t.Errorf("face = %v, want FaceSouth", face)
	}
}

func TestFindSupportFace_NegativeCoordinates(t *testing.T) {
	// Regression guard for the floor-vs-truncate pitfall: a target at a
	// negative coordinate must still floor its neighbor cells correctly,
	// not truncate toward zero.
	target := models.V3{X: -5, Y: -60, Z: -5}
	world := fakeBlockWorld{solid: map[[3]int]bool{solidAt(-5, -61, -5): true}}

	support, face, ok := findSupportFace(world, fakePassability{}, target)
	if !ok {
		t.Fatal("expected a support face below the target")
	}
	if want := (models.V3{X: -5, Y: -61, Z: -5}); support != want {
		t.Errorf("support = %+v, want %+v", support, want)
	}
	if face != models.FaceUp {
		t.Errorf("face = %v, want FaceUp", face)
	}
}

func TestPlacementTookEffect(t *testing.T) {
	const table = "minecraft:crafting_table"
	tests := []struct {
		name                  string
		block                 string
		countNow, countBefore int
		want                  bool
	}{
		{"block visible", table, 1, 1, true},
		{"item consumed but the block not yet in view", "minecraft:air", 0, 1, true},
		{"both", table, 0, 1, true},
		{"nothing happened", "minecraft:air", 1, 1, false},
		{"chunk not loaded and nothing consumed", "<chunk not loaded>(1,2,3)", 2, 2, false},
	}
	for _, tt := range tests {
		if got := placementTookEffect(tt.block, table, tt.countNow, tt.countBefore); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
