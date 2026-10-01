package agent

import (
	"context"
	"fmt"
	"sort"
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

	if missing := missingMaterials(placer, s, order); len(missing) > 0 {
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

// missingMaterials counts, across order, how many more of each palette item
// name the inventory needs than it currently has. Returns nil if nothing is
// short. See BuildStructure's doc comment for the known block-name-vs-
// item-name limitation this inherits.
func missingMaterials(placer structurePlacer, s *structure.Structure, order []structure.BlockEntry) map[string]int {
	need := map[string]int{}
	for _, b := range order {
		entry, ok := s.Block(b)
		if !ok {
			continue
		}
		need[entry.Name]++
	}
	missing := map[string]int{}
	for name, count := range need {
		if have := placer.InventoryCount(name); have < count {
			missing[name] = count - have
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return missing
}

// formatMissingMaterials renders missing as "3x minecraft:oak_planks, 1x
// minecraft:chest", sorted by name for deterministic error messages.
func formatMissingMaterials(missing map[string]int) string {
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%dx %s", missing[name], name))
	}
	return strings.Join(parts, ", ")
}
