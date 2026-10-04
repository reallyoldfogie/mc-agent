package actions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/structure"
)

func TestMaterialListSummary_EmptyStructure(t *testing.T) {
	got := materialListSummary(structure.MaterialList{}, false)
	want := "Material list: structure is empty (no non-air blocks)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMaterialListSummary_ListsItems(t *testing.T) {
	ml := structure.MaterialList{Items: []structure.MaterialEntry{
		{Item: "minecraft:oak_log", Count: 63},
		{Item: "minecraft:oak_planks", Count: 2},
	}}
	got := materialListSummary(ml, false)
	if !strings.Contains(got, "65 blocks, 2 item types") {
		t.Errorf("got %q, want it to mention \"65 blocks, 2 item types\"", got)
	}
	if !strings.Contains(got, "63x minecraft:oak_log") {
		t.Errorf("got %q, want it to list 63x minecraft:oak_log", got)
	}
	if strings.Contains(got, "written to file") {
		t.Errorf("got %q, want no export note when exported=false", got)
	}
}

func TestMaterialListSummary_NotesExport(t *testing.T) {
	ml := structure.MaterialList{Items: []structure.MaterialEntry{{Item: "minecraft:stone", Count: 1}}}
	got := materialListSummary(ml, true)
	if !strings.Contains(got, "written to file") {
		t.Errorf("got %q, want it to note the export", got)
	}
}

// A structure with more distinct item types than
// materialListSummaryMaxItems must still produce one bounded chat line -
// the same spam risk placeErrorChatAllowed/buildStructureSummaryMaxFailures
// guard against elsewhere, not one line per item type.
func TestMaterialListSummary_TruncatesManyItemTypes(t *testing.T) {
	var items []structure.MaterialEntry
	for i := 0; i < materialListSummaryMaxItems+4; i++ {
		items = append(items, structure.MaterialEntry{Item: fmt.Sprintf("minecraft:item_%d", i), Count: 1})
	}
	ml := structure.MaterialList{Items: items}
	got := materialListSummary(ml, false)

	wantTypeCount := fmt.Sprintf("%d item types", len(items))
	if !strings.Contains(got, wantTypeCount) {
		t.Errorf("got %q, want it to report the true total (%s)", got, wantTypeCount)
	}
	if !strings.Contains(got, "+4 more types") {
		t.Errorf("got %q, want it to note the 4 truncated entries", got)
	}
}
