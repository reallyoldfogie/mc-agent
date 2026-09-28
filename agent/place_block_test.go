package agent

import "testing"

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
