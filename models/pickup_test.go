package models

import (
	"context"
	"errors"
	"math"
	"testing"
)

// The vanilla box test: the player's box grows 1.0 sideways and 0.5 up and
// down; an item just inside that (1.425 from centre: 0.3 + 1.0 + 0.125) is
// collected, one just outside is not.
func TestItemInPickupRangeMatchesTheVanillaBox(t *testing.T) {
	player := V3{X: 10.5, Y: -60, Z: 10.5}
	cases := []struct {
		name string
		item V3
		want bool
	}{
		{"at the player's feet", V3{X: 10.5, Y: -60, Z: 10.5}, true},
		{"just inside sideways", V3{X: 11.9, Y: -60, Z: 10.5}, true},
		{"just outside sideways", V3{X: 12.0, Y: -60, Z: 10.5}, false},
		{"just outside on z", V3{X: 10.5, Y: -60, Z: 8.9}, false},
		{"a block above the head", V3{X: 10.5, Y: -57.5, Z: 10.5}, false},
		{"level with the head", V3{X: 10.5, Y: -58.5, Z: 10.5}, true},
		{"well below the feet", V3{X: 10.5, Y: -61.5, Z: 10.5}, false},
	}
	for _, c := range cases {
		if got := ItemInPickupRange(player, c.item); got != c.want {
			t.Errorf("%s: ItemInPickupRange = %v, want %v", c.name, got, c.want)
		}
	}
}

// Every offered position is really in range, the item's own cell (when
// standable) comes first, and nothing is offered further than a block out.
func TestPickupCandidatesAreInRangeNearestFirst(t *testing.T) {
	item := V3{X: -295.6, Y: -60, Z: -299.8}
	cands := pickupCandidates(item)
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	first := cands[0]
	if first.X != -295.5 || first.Z != -299.5 || first.Y != -60 {
		t.Errorf("first candidate = %v, want the item's own cell centre (-295.5,-60,-299.5)", first)
	}
	prev := -1.0
	for _, c := range cands {
		if !ItemInPickupRange(c, item) {
			t.Errorf("candidate %v is not in pickup range of %v", c, item)
		}
		d := math.Hypot(c.X-item.X, c.Z-item.Z)
		if d > 1.5 {
			t.Errorf("candidate %v is %.2f from the item, want within a block or so", c, d)
		}
		if d+1e-9 < prev {
			t.Errorf("candidates not ordered nearest first: %.3f after %.3f", d, prev)
		}
		prev = d
	}
}

type noPickupAgent struct{}

func (noPickupAgent) GetWorld() World                      { return nil }
func (noPickupAgent) BlockShapeManager() BlockShapeManager { return nil }

func TestTryPickupPositionsWithoutAWorldReportsNoPosition(t *testing.T) {
	err := TryPickupPositions(context.Background(), noPickupAgent{}, V3{}, func(V3) error { return nil })
	if !errors.Is(err, ErrNoPickupPosition) {
		t.Fatalf("err = %v, want ErrNoPickupPosition", err)
	}
}
