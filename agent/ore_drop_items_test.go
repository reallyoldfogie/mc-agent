package agent

import "testing"

// These test parseLootTableNormalDrop in isolation against synthetic JSON
// matching the real cached loot table schema (data_generator/data/minecraft/
// loot_table/blocks/*.json, verified against the real cached 1.21.5 output
// for each shape below) - mirrors craft_test.go's own reasoning for testing
// recipe parsing this way. loadOreDropItems/OreDropItem themselves need a
// live cache directory and aren't covered here.

// coalOreLootTableJSON is the real shape for an ore whose Silk Touch
// alternative is listed first (verified against the cached
// blocks/coal_ore.json).
const coalOreLootTableJSON = `{
  "type": "minecraft:block",
  "pools": [
    {
      "bonus_rolls": 0.0,
      "entries": [
        {
          "type": "minecraft:alternatives",
          "children": [
            {
              "type": "minecraft:item",
              "conditions": [
                {
                  "condition": "minecraft:match_tool",
                  "predicate": {
                    "predicates": {
                      "minecraft:enchantments": [
                        {"enchantments": "minecraft:silk_touch", "levels": {"min": 1}}
                      ]
                    }
                  }
                }
              ],
              "name": "minecraft:coal_ore"
            },
            {
              "type": "minecraft:item",
              "functions": [
                {"enchantment": "minecraft:fortune", "formula": "minecraft:ore_drops", "function": "minecraft:apply_bonus"},
                {"function": "minecraft:explosion_decay"}
              ],
              "name": "minecraft:coal"
            }
          ]
        }
      ],
      "rolls": 1.0
    }
  ],
  "random_sequence": "minecraft:blocks/coal_ore"
}`

// unconditionedLootTableJSON is the real shape for a block with a single,
// unconditioned drop (verified against the cached blocks/ancient_debris.json
// - a "survives_explosion" pool condition, irrelevant to the item's own
// identity, is deliberately omitted here since parseLootTableNormalDrop
// doesn't need to look at pool-level conditions to answer "what drops").
const unconditionedLootTableJSON = `{
  "type": "minecraft:block",
  "pools": [
    {
      "bonus_rolls": 0.0,
      "entries": [
        {"type": "minecraft:item", "name": "minecraft:ancient_debris"}
      ],
      "rolls": 1.0
    }
  ],
  "random_sequence": "minecraft:blocks/ancient_debris"
}`

func TestParseLootTableNormalDrop_SkipsSilkTouchAlternative(t *testing.T) {
	name, ok := parseLootTableNormalDrop([]byte(coalOreLootTableJSON))
	if !ok {
		t.Fatal("expected a normal drop to be found")
	}
	if name != "minecraft:coal" {
		t.Errorf("got %q, want minecraft:coal (the fortune-eligible alternative, not the silk-touch one)", name)
	}
}

func TestParseLootTableNormalDrop_UnconditionedSingleEntry(t *testing.T) {
	name, ok := parseLootTableNormalDrop([]byte(unconditionedLootTableJSON))
	if !ok {
		t.Fatal("expected a normal drop to be found")
	}
	if name != "minecraft:ancient_debris" {
		t.Errorf("got %q, want minecraft:ancient_debris", name)
	}
}

func TestParseLootTableNormalDrop_MalformedJSON(t *testing.T) {
	if _, ok := parseLootTableNormalDrop([]byte("not json")); ok {
		t.Error("expected ok=false for malformed JSON, not a guess")
	}
}

func TestParseLootTableNormalDrop_UnrecognizedShape(t *testing.T) {
	// A shape this parser deliberately doesn't follow (e.g. a bare empty
	// pool list) should report ok=false, not a wrong guess.
	if _, ok := parseLootTableNormalDrop([]byte(`{"type":"minecraft:block","pools":[]}`)); ok {
		t.Error("expected ok=false for a loot table with no entries")
	}
}
