package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/structure"
)

// buildProgressChatEvery throttles BuildStructure's "Placed N/Total" chat
// updates - same reasoning as placeErrorChatEvery (commands.go): a build
// with hundreds of blocks sending one line per block risks a spam kick.
const buildProgressChatEvery = 5 * time.Second

// structurePlacer is the minimal capability buildStructureWith needs -
// narrow enough to fake in tests instead of requiring a full live *agent.
// Same rationale as blockReader/passabilityChecker (place_block.go) and
// models.InteractPositionAgent.
type structurePlacer interface {
	InventoryCount(itemName string) int
	PlaceBlockAt(ctx context.Context, pos models.V3, itemName string) error
}

// BuildStructure implements models.CommandAgent. See that interface's doc
// comment for the contract. This loads the file, then delegates to
// buildStructureWith with a throttled chat-progress callback.
func (a *agent) BuildStructure(ctx context.Context, path string, origin models.V3) (models.BuildStructureResult, error) {
	s, err := structure.LoadFile(path)
	if err != nil {
		return models.BuildStructureResult{}, fmt.Errorf("build structure: %w", err)
	}

	var lastProgress time.Time
	onProgress := func(placed, total int) {
		now := time.Now()
		if now.Sub(lastProgress) < buildProgressChatEvery {
			return
		}
		lastProgress = now
		_ = a.SendChat(fmt.Sprintf("Build: placed %d/%d", placed, total))
	}

	return buildStructureWith(ctx, a, s, origin, onProgress)
}

// buildStructureWith is BuildStructure's core: an upfront material check
// (missingMaterials), then a bottom-up placement loop
// (structure.PlacementOrder) that continues past an individual placement
// failure rather than aborting the whole build - a wall missing one block
// because of a transient server hiccup is recoverable and worth reporting
// precisely; an aborted half-built structure with no record of what's left
// is not. Factored out of BuildStructure so it's testable against a fake
// structurePlacer. onProgress may be nil (tests pass nil); real callers get
// a throttled chat update.
func buildStructureWith(ctx context.Context, placer structurePlacer, s *structure.Structure, origin models.V3, onProgress func(placed, total int)) (models.BuildStructureResult, error) {
	order := structure.PlacementOrder(s)

	if missing := missingMaterials(placer, s); len(missing) > 0 {
		return models.BuildStructureResult{}, fmt.Errorf("build structure: missing materials: %s", formatMissingMaterials(missing))
	}

	var result models.BuildStructureResult
	total := len(order)
	for _, b := range order {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		entry, ok := s.Block(b)
		if !ok {
			continue // PlacementOrder already filters unresolvable entries; defensive only
		}
		target := models.V3{
			X: origin.X + float64(b.Pos.X),
			Y: origin.Y + float64(b.Pos.Y),
			Z: origin.Z + float64(b.Pos.Z),
		}
		if err := placer.PlaceBlockAt(ctx, target, entry.Name); err != nil {
			result.Failed = append(result.Failed, models.BuildStructureFailure{Pos: target, Item: entry.Name, Reason: err.Error()})
		} else {
			result.Placed++
		}
		if onProgress != nil {
			onProgress(result.Placed, total)
		}
	}
	return result, nil
}

// missingMaterials reports which of s's required materials placer's
// inventory is short on, and by how much. Built on structure.MaterialList -
// the same computation (and, via MaterialList.Missing, the same have-count-
// as-a-function abstraction) is reusable outside the agent package entirely
// (see docs/STRUCTURE_LOADER.md) - placer.InventoryCount is just today's
// "have" source; a long-term-memory system checking chests/shulker boxes/
// barrels later is a different "have" function, not a different
// comparison. Returns nil if nothing is short. See
// BuildStructure's doc comment for the known block-name-vs-item-name
// limitation this inherits.
func missingMaterials(placer structurePlacer, s *structure.Structure) []structure.MaterialEntry {
	return structure.ComputeMaterialList(s).Missing(placer.InventoryCount)
}

// formatMissingMaterials renders missing as "3x minecraft:oak_planks, 1x
// minecraft:chest" - already sorted by name, since MaterialList.Missing
// preserves ComputeMaterialList's own sorted order.
func formatMissingMaterials(missing []structure.MaterialEntry) string {
	parts := make([]string, 0, len(missing))
	for _, e := range missing {
		parts = append(parts, fmt.Sprintf("%dx %s", e.Count, e.Item))
	}
	return strings.Join(parts, ", ")
}
