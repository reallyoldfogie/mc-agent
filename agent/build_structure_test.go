package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/structure"
)

// fakeStructurePlacer is a minimal structurePlacer for buildStructureWith's
// tests: a fixed inventory, an optional set of item names that always fail
// to place, and a recording of every PlaceBlockAt call so tests can assert
// on order and arguments without a live agent.
type fakeStructurePlacer struct {
	inventory map[string]int
	failItem  map[string]bool
	calls     []placeCall
	onCall    func() // invoked at the start of each PlaceBlockAt call, e.g. to cancel ctx
}

type placeCall struct {
	pos  models.V3
	item string
}

func (f *fakeStructurePlacer) InventoryCount(itemName string) int { return f.inventory[itemName] }

func (f *fakeStructurePlacer) PlaceBlockAt(_ context.Context, pos models.V3, itemName string) error {
	if f.onCall != nil {
		f.onCall()
	}
	f.calls = append(f.calls, placeCall{pos: pos, item: itemName})
	if f.failItem[itemName] {
		return errors.New("placement failed")
	}
	return nil
}

// twoBlockStructure is a minimal fixture: a stone block at the structure's
// own (0,0,0) and another directly above it at (0,1,0) - just enough to
// exercise bottom-up ordering and origin offsetting.
func twoBlockStructure() *structure.Structure {
	return &structure.Structure{
		Size:    structure.Pos{X: 1, Y: 2, Z: 1},
		Palette: []structure.PaletteEntry{{Name: "minecraft:stone"}},
		Blocks: []structure.BlockEntry{
			{Pos: structure.Pos{X: 0, Y: 1, Z: 0}, PaletteIndex: 0},
			{Pos: structure.Pos{X: 0, Y: 0, Z: 0}, PaletteIndex: 0},
		},
	}
}

func TestMissingMaterials_ReportsShortfall(t *testing.T) {
	s := &structure.Structure{
		Palette: []structure.PaletteEntry{{Name: "minecraft:stone"}},
		Blocks: []structure.BlockEntry{
			{Pos: structure.Pos{X: 0, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: structure.Pos{X: 1, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: structure.Pos{X: 2, Y: 0, Z: 0}, PaletteIndex: 0},
		},
	}
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 1}}

	missing := missingMaterials(placer, s)
	if len(missing) != 1 || missing[0].Item != "minecraft:stone" || missing[0].Count != 2 {
		t.Errorf("missing = %+v, want [{minecraft:stone 2}] (need 3, have 1)", missing)
	}
}

func TestMissingMaterials_NilWhenSufficient(t *testing.T) {
	s := twoBlockStructure()
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 2}}

	if missing := missingMaterials(placer, s); missing != nil {
		t.Errorf("missingMaterials = %v, want nil", missing)
	}
}

func TestBuildStructureWith_AbortsBeforePlacingWhenMaterialsShort(t *testing.T) {
	s := twoBlockStructure()
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 1}} // needs 2

	result, err := buildStructureWith(context.Background(), placer, s, models.V3{}, nil)
	if err == nil {
		t.Fatal("expected an error when materials are short, got nil")
	}
	if len(placer.calls) != 0 {
		t.Errorf("PlaceBlockAt was called %d times, want 0 (should abort before placing anything)", len(placer.calls))
	}
	if result.Placed != 0 {
		t.Errorf("result.Placed = %d, want 0", result.Placed)
	}
}

func TestBuildStructureWith_PlacesBottomUpAtOriginOffset(t *testing.T) {
	s := twoBlockStructure()
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 2}}
	origin := models.V3{X: 10, Y: 20, Z: 30}

	result, err := buildStructureWith(context.Background(), placer, s, origin, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Placed != 2 {
		t.Errorf("result.Placed = %d, want 2", result.Placed)
	}
	if len(result.Failed) != 0 {
		t.Errorf("result.Failed = %v, want empty", result.Failed)
	}
	if len(placer.calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(placer.calls))
	}
	// Bottom-up: local (0,0,0) before local (0,1,0).
	want := []models.V3{
		{X: 10, Y: 20, Z: 30},
		{X: 10, Y: 21, Z: 30},
	}
	for i, call := range placer.calls {
		if call.pos != want[i] {
			t.Errorf("calls[%d].pos = %+v, want %+v", i, call.pos, want[i])
		}
		if call.item != "minecraft:stone" {
			t.Errorf("calls[%d].item = %q, want minecraft:stone", i, call.item)
		}
	}
}

func TestBuildStructureWith_ContinuesPastIndividualFailure(t *testing.T) {
	s := &structure.Structure{
		Palette: []structure.PaletteEntry{
			{Name: "minecraft:stone"},
			{Name: "minecraft:glass"},
		},
		Blocks: []structure.BlockEntry{
			{Pos: structure.Pos{X: 0, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: structure.Pos{X: 1, Y: 0, Z: 0}, PaletteIndex: 1}, // this one will fail
			{Pos: structure.Pos{X: 2, Y: 0, Z: 0}, PaletteIndex: 0},
		},
	}
	placer := &fakeStructurePlacer{
		inventory: map[string]int{"minecraft:stone": 2, "minecraft:glass": 1},
		failItem:  map[string]bool{"minecraft:glass": true},
	}

	result, err := buildStructureWith(context.Background(), placer, s, models.V3{}, nil)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if result.Placed != 2 {
		t.Errorf("result.Placed = %d, want 2", result.Placed)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("len(result.Failed) = %d, want 1", len(result.Failed))
	}
	if result.Failed[0].Item != "minecraft:glass" {
		t.Errorf("Failed[0].Item = %q, want minecraft:glass", result.Failed[0].Item)
	}
	if result.Failed[0].Reason == "" {
		t.Error("Failed[0].Reason is empty, want the placement error's message")
	}
	if len(placer.calls) != 3 {
		t.Errorf("len(calls) = %d, want 3 (all blocks attempted despite the middle failure)", len(placer.calls))
	}
}

func TestBuildStructureWith_StopsOnContextCancellation(t *testing.T) {
	s := &structure.Structure{
		Palette: []structure.PaletteEntry{{Name: "minecraft:stone"}},
		Blocks: []structure.BlockEntry{
			{Pos: structure.Pos{X: 0, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: structure.Pos{X: 1, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: structure.Pos{X: 2, Y: 0, Z: 0}, PaletteIndex: 0},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 3}}
	placer.onCall = func() {
		if len(placer.calls) == 0 {
			cancel() // cancel after the first call has been recorded
		}
	}

	result, err := buildStructureWith(ctx, placer, s, models.V3{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(placer.calls) != 1 {
		t.Errorf("len(calls) = %d, want 1 (should stop after cancellation, not attempt the rest)", len(placer.calls))
	}
	if result.Placed != 1 {
		t.Errorf("result.Placed = %d, want 1 (the call before cancellation was observed)", result.Placed)
	}
}

func TestBuildStructureWith_OnProgressCanBeNil(t *testing.T) {
	s := twoBlockStructure()
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 2}}

	if _, err := buildStructureWith(context.Background(), placer, s, models.V3{}, nil); err != nil {
		t.Fatalf("unexpected error with nil onProgress: %v", err)
	}
}

func TestBuildStructureWith_ReportsProgress(t *testing.T) {
	s := twoBlockStructure()
	placer := &fakeStructurePlacer{inventory: map[string]int{"minecraft:stone": 2}}

	var progressCalls [][2]int
	onProgress := func(placed, total int) {
		progressCalls = append(progressCalls, [2]int{placed, total})
	}

	if _, err := buildStructureWith(context.Background(), placer, s, models.V3{}, onProgress); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(progressCalls) != 2 {
		t.Fatalf("onProgress called %d times, want 2 (once per block)", len(progressCalls))
	}
	if progressCalls[1] != [2]int{2, 2} {
		t.Errorf("final onProgress call = %v, want {2 2}", progressCalls[1])
	}
}

func TestFormatMissingMaterials_SortedDeterministic(t *testing.T) {
	missing := []structure.MaterialEntry{
		{Item: "minecraft:chest", Count: 1},
		{Item: "minecraft:oak_planks", Count: 3},
	}
	got := formatMissingMaterials(missing)
	want := "1x minecraft:chest, 3x minecraft:oak_planks"
	if got != want {
		t.Errorf("formatMissingMaterials = %q, want %q", got, want)
	}
}
