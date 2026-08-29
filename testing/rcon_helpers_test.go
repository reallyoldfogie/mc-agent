package testing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSplitTopLevelBraceEntries guards against the regression found live
// while testing elytra unequip: an item carrying component data (e.g. an
// elytra with accumulated durability wear reports as
// `{..., components: {"minecraft:damage": 1}, id: "minecraft:elytra"}`)
// has a NESTED {...} inside its entry, which the previous naive
// \{([^}]*)\} regex matched as its own bogus entry and stopped at,
// silently dropping the real item from GetInventoryItems/GetChestContents
// results.
func TestSplitTopLevelBraceEntries(t *testing.T) {
	t.Run("flat entries with no nested braces", func(t *testing.T) {
		resp := `ChestAccess has the following entity data: [{count: 4, Slot: 0b, id: "minecraft:firework_rocket"}, {count: 1, Slot: 36b, id: "minecraft:stone"}]`
		got := splitTopLevelBraceEntries(resp)
		assert.Equal(t, []string{
			`{count: 4, Slot: 0b, id: "minecraft:firework_rocket"}`,
			`{count: 1, Slot: 36b, id: "minecraft:stone"}`,
		}, got)
	})

	t.Run("entry with nested components object is not split apart", func(t *testing.T) {
		resp := `ChestAccess has the following entity data: [{count: 4, Slot: 0b, id: "minecraft:firework_rocket"}, {count: 1, Slot: 9b, components: {"minecraft:damage": 1}, id: "minecraft:elytra"}]`
		got := splitTopLevelBraceEntries(resp)
		assert.Equal(t, []string{
			`{count: 4, Slot: 0b, id: "minecraft:firework_rocket"}`,
			`{count: 1, Slot: 9b, components: {"minecraft:damage": 1}, id: "minecraft:elytra"}`,
		}, got)
	})

	t.Run("braces inside quoted strings do not confuse depth tracking", func(t *testing.T) {
		resp := `[{count: 1, Slot: 0b, id: "minecraft:written_book", components: {"minecraft:custom_name": "a {weird} title"}}]`
		got := splitTopLevelBraceEntries(resp)
		assert.Equal(t, []string{
			`{count: 1, Slot: 0b, id: "minecraft:written_book", components: {"minecraft:custom_name": "a {weird} title"}}`,
		}, got)
	})
}
