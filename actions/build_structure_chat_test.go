package actions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestBuildStructureSummary_AllPlacedNoFailures(t *testing.T) {
	got := buildStructureSummary(models.BuildStructureResult{Placed: 42})
	want := "Build complete: placed 42 blocks"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildStructureSummary_ListsFailures(t *testing.T) {
	result := models.BuildStructureResult{
		Placed: 2,
		Failed: []models.BuildStructureFailure{
			{Pos: models.V3{X: 1, Y: 2, Z: 3}, Item: "minecraft:glass", Reason: "no solid neighbor"},
		},
	}
	got := buildStructureSummary(result)
	if !strings.Contains(got, "placed 2 blocks, 1 failed") {
		t.Errorf("got %q, want it to mention \"placed 2 blocks, 1 failed\"", got)
	}
	if !strings.Contains(got, "minecraft:glass at (1, 2, 3)") {
		t.Errorf("got %q, want it to name the failed item and position", got)
	}
}

// A build with more failures than buildStructureSummaryMaxFailures must
// still produce one bounded chat line, not one line per failure - the same
// spam risk placeErrorChatAllowed guards against for Place.
func TestBuildStructureSummary_TruncatesManyFailures(t *testing.T) {
	var failed []models.BuildStructureFailure
	for i := 0; i < buildStructureSummaryMaxFailures+3; i++ {
		failed = append(failed, models.BuildStructureFailure{
			Pos: models.V3{X: float64(i)}, Item: "minecraft:stone", Reason: "placement failed",
		})
	}
	got := buildStructureSummary(models.BuildStructureResult{Placed: 10, Failed: failed})

	wantFailedCount := fmt.Sprintf("%d failed", len(failed))
	if !strings.Contains(got, wantFailedCount) {
		t.Errorf("got %q, want it to report the true total (%s)", got, wantFailedCount)
	}
	if !strings.Contains(got, "+3 more") {
		t.Errorf("got %q, want it to note the 3 truncated entries", got)
	}
}
