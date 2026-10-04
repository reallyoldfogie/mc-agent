package structure

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// MaterialEntry is one line of a MaterialList: an item name and how many
// of it are needed.
type MaterialEntry struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
}

// MaterialList is everything a Structure needs to build, as a flat,
// serializable shopping list - deliberately independent of any particular
// build (no positions, no palette indices) so it's useful on its own: handed
// to a human, written to disk for later, or compared against whatever the
// bot currently has access to, today or down the line. Items is sorted by
// item name, so two lists computed from the same Structure always compare
// and serialize identically.
type MaterialList struct {
	// Source, if set, records where this list was computed from (typically
	// the .nbt file path) - informational only, so an exported file is
	// still self-describing if it's looked at later on its own, away from
	// the structure that produced it.
	Source string          `json:"source,omitempty"`
	Items  []MaterialEntry `json:"items"`
}

// ComputeMaterialList counts how many of each item a Structure needs,
// resolved from its PlacementOrder - the same bottom-up, air/structure_void-
// filtered list BuildStructure actually places from, so this always counts
// exactly what a real build would need, never more (raw s.Blocks would
// overcount by every air/void cell a structure file's bounding box is full
// of).
func ComputeMaterialList(s *Structure) MaterialList {
	need := map[string]int{}
	for _, b := range PlacementOrder(s) {
		entry, ok := s.Block(b)
		if !ok {
			continue
		}
		need[entry.Name]++
	}

	items := make([]MaterialEntry, 0, len(need))
	for name, count := range need {
		items = append(items, MaterialEntry{Item: name, Count: count})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Item < items[j].Item })

	return MaterialList{Items: items}
}

// Count returns how many of item ml calls for, or 0 if it isn't in the
// list at all.
func (ml MaterialList) Count(item string) int {
	for _, e := range ml.Items {
		if e.Item == item {
			return e.Count
		}
	}
	return 0
}

// Total returns the sum of every entry's Count - the total number of
// blocks the list represents, across every item type.
func (ml MaterialList) Total() int {
	total := 0
	for _, e := range ml.Items {
		total += e.Count
	}
	return total
}

// Missing returns the subset of ml's items not fully covered by have - a
// per-item count source, deliberately a plain function rather than a
// concrete inventory type, so the exact same comparison works against the
// bot's own current inventory (today - see agent/build_structure.go's
// missingMaterials) and, later, against whatever a long-term-memory system
// reports as available across chests/shulker boxes/barrels/etc., without
// this function - or its caller - needing to change. Each returned entry's
// Count is the shortfall (needed - have), not the original needed amount.
// Returns nil if nothing is short.
func (ml MaterialList) Missing(have func(item string) int) []MaterialEntry {
	var missing []MaterialEntry
	for _, e := range ml.Items {
		if got := have(e.Item); got < e.Count {
			missing = append(missing, MaterialEntry{Item: e.Item, Count: e.Count - got})
		}
	}
	return missing
}

// WriteJSON writes ml as indented JSON to path - the persistent form of the
// list, readable by a human, re-loadable via LoadMaterialListJSON, or
// consumable by anything else that just wants "what does this build need"
// without parsing the original structure file at all.
func (ml MaterialList) WriteJSON(path string) error {
	data, err := json.MarshalIndent(ml, "", "  ")
	if err != nil {
		return fmt.Errorf("encode material list: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write material list to %s: %w", path, err)
	}
	return nil
}

// LoadMaterialListJSON reads a MaterialList previously written by
// MaterialList.WriteJSON.
func LoadMaterialListJSON(path string) (MaterialList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MaterialList{}, fmt.Errorf("read material list from %s: %w", path, err)
	}
	var ml MaterialList
	if err := json.Unmarshal(data, &ml); err != nil {
		return MaterialList{}, fmt.Errorf("decode material list from %s: %w", path, err)
	}
	return ml, nil
}
