package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// oreDropItemsOnce/oreDropItemsCache/oreDropItemsCacheErr cache the result
// of parsing every data/minecraft/loot_table/blocks/*.json loot table for
// the connected version into a block-name -> normal-drop-item-name map,
// loaded lazily on first use and reused for the rest of the session -
// mirrors loadCraftingRecipes' own caching reasoning (agent/craft.go):
// re-reading hundreds of small JSON files from a tight loop (here,
// collectDrops' own diagnostic log - see rlenv.Environment's call site)
// caused real I/O contention once found live under multi-agent training
// (agent/craft.go's own doc comment on loadCraftingRecipes), so this is
// loaded once per process, not once per call.
var (
	oreDropItemsOnce     sync.Once
	oreDropItemsCache    map[string]string
	oreDropItemsCacheErr error
)

// OreDropItem is rlenv.OreDropItemResolver's implementation - see that
// interface's own doc comment for why this exists: collectDrops' own
// diagnostic log (rlenv/environment.go) needs to know a mined block's
// actual normal drop item, not a hand-guessed one. Resolves blockName
// (e.g. "minecraft:coal_ore") against the connected version's own loot
// table data (data/minecraft/loot_table/blocks/<name>.json, the same
// cached data_generator tree agent.craftingRecipeDataDir already reads
// recipes from), picking the entry that doesn't require Silk Touch. ok is
// false if that file doesn't exist or its shape isn't one
// parseLootTableNormalDrop recognizes - loot tables can nest arbitrarily;
// this only follows the shape every ore block in 1.21.5 actually uses, not
// a general interpreter, and reports false rather than guessing wrong.
func (a *agent) OreDropItem(blockName string) (string, bool) {
	oreDropItemsOnce.Do(func() {
		oreDropItemsCache, oreDropItemsCacheErr = a.loadOreDropItems()
	})
	if oreDropItemsCacheErr != nil {
		return "", false
	}
	name, ok := oreDropItemsCache[blockName]
	return name, ok
}

// loadOreDropItems parses every blocks/*.json loot table once - see
// oreDropItemsOnce's own doc comment for why this isn't done per call.
func (a *agent) loadOreDropItems() (map[string]string, error) {
	dir, err := a.craftingRecipeDataDir()
	if err != nil {
		return nil, err
	}
	lootDir := filepath.Join(dir, "minecraft", "loot_table", "blocks")
	entries, err := os.ReadDir(lootDir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		blockName := "minecraft:" + strings.TrimSuffix(entry.Name(), ".json")
		raw, err := os.ReadFile(filepath.Join(lootDir, entry.Name()))
		if err != nil {
			continue
		}
		if dropName, ok := parseLootTableNormalDrop(raw); ok {
			result[blockName] = dropName
		}
	}
	return result, nil
}

// lootTable mirrors just enough of the vanilla loot table JSON schema
// (data/minecraft/loot_table/blocks/*.json) to answer "what does this
// block drop without Silk Touch" - see parseLootTableNormalDrop.
type lootTable struct {
	Pools []struct {
		Entries []lootTableEntry `json:"entries"`
	} `json:"pools"`
}

type lootTableEntry struct {
	Type       string           `json:"type"`
	Name       string           `json:"name"`
	Conditions []lootTableCond  `json:"conditions"`
	Children   []lootTableEntry `json:"children"`
}

type lootTableCond struct {
	Condition string `json:"condition"`
	Predicate struct {
		Predicates struct {
			Enchantments []struct {
				Enchantment string `json:"enchantments"`
			} `json:"minecraft:enchantments"`
		} `json:"predicates"`
	} `json:"predicate"`
}

// requiresSilkTouch reports whether entry only drops when the tool has
// Silk Touch - vanilla ore loot tables gate the block's own-item drop on
// exactly this condition (see e.g. coal_ore.json's minecraft:match_tool
// condition), making it the one entry to skip in favor of the normal drop.
func (e lootTableEntry) requiresSilkTouch() bool {
	for _, c := range e.Conditions {
		if c.Condition != "minecraft:match_tool" {
			continue
		}
		for _, ench := range c.Predicate.Predicates.Enchantments {
			if strings.Contains(ench.Enchantment, "silk_touch") {
				return true
			}
		}
	}
	return false
}

// parseLootTableNormalDrop walks raw (one block's loot table JSON) for its
// normal (non-Silk-Touch) drop item name. Only follows the
// pools->entries->(minecraft:item | minecraft:alternatives->children)
// shape every ore block in 1.21.5 actually uses; anything else reports
// ok=false rather than guessing.
func parseLootTableNormalDrop(raw []byte) (name string, ok bool) {
	var table lootTable
	if err := json.Unmarshal(raw, &table); err != nil {
		return "", false
	}
	for _, pool := range table.Pools {
		for _, entry := range pool.Entries {
			if name, ok := firstNormalDrop(entry); ok {
				return name, true
			}
		}
	}
	return "", false
}

func firstNormalDrop(entry lootTableEntry) (string, bool) {
	switch entry.Type {
	case "minecraft:item":
		if entry.requiresSilkTouch() {
			return "", false
		}
		return entry.Name, entry.Name != ""
	case "minecraft:alternatives":
		for _, child := range entry.Children {
			if name, ok := firstNormalDrop(child); ok {
				return name, true
			}
		}
	}
	return "", false
}
