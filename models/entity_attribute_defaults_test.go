package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wireMapper1211 is the full ClientboundEntityUpdateAttributes VarInt->name
// mapper table for Minecraft 1.21.1, transcribed verbatim from
// mc-protocol-go/data/1.21.1/play/clientbound/packet_entityupdateattributes.go.
// See PHASE_7_PLAN.md §1.5.
var wireMapper1211 = []string{
	"generic.armor", "generic.armor_toughness", "generic.attack_damage",
	"generic.attack_knockback", "generic.attack_speed", "player.block_break_speed",
	"player.block_interaction_range", "player.entity_interaction_range",
	"generic.fall_damage_multiplier", "generic.flying_speed", "generic.follow_range",
	"generic.gravity", "generic.jump_strength", "generic.knockback_resistance",
	"generic.luck", "generic.max_absorption", "generic.max_health",
	"generic.movement_speed", "generic.safe_fall_distance", "generic.scale",
	"zombie.spawn_reinforcements", "generic.step_height",
}

// wireMapper1216 is the same table for 1.21.6, which has grown to 31 entries
// (Mojang added attributes between versions) and introduces the first
// prefix-less wire names (burning_time, camera_distance, ...). Transcribed
// verbatim from mc-protocol-go/data/1.21.6/play/clientbound/packet_entityupdateattributes.go.
var wireMapper1216 = []string{
	"generic.armor", "generic.armor_toughness", "generic.attack_damage",
	"generic.attack_knockback", "generic.attack_speed", "player.block_break_speed",
	"player.block_interaction_range", "burning_time", "camera_distance",
	"explosion_knockback_resistance", "player.entity_interaction_range",
	"generic.fall_damage_multiplier", "generic.flying_speed", "generic.follow_range",
	"generic.gravity", "generic.jump_strength", "generic.knockback_resistance",
	"generic.luck", "generic.max_absorption", "generic.max_health",
	"generic.movement_speed", "generic.safe_fall_distance", "generic.scale",
	"zombie.spawn_reinforcements", "generic.step_height", "submerged_mining_speed",
	"sweeping_damage_ratio", "tempt_range", "water_movement_efficiency",
	"waypoint_transmit_range", "waypoint_receive_range",
}

func TestWireAttributeNameToDataGenName_RealMapperTables(t *testing.T) {
	// Every wire name from both real tables must translate to a plausible
	// "minecraft:<bare-name>" identifier with the category prefix (if any)
	// stripped — not just the four examples in the doc, the whole table.
	for _, table := range [][]string{wireMapper1211, wireMapper1216} {
		for _, wireName := range table {
			got := wireAttributeNameToDataGenName(wireName)
			assert.True(t, len(got) > len("minecraft:"), "translation of %q produced too-short result %q", wireName, got)
			assert.Equal(t, "minecraft:", got[:len("minecraft:")], "translation of %q must be minecraft:-prefixed, got %q", wireName, got)
			assert.NotContains(t, got[len("minecraft:"):], ".", "translated name %q for wire key %q should have no leftover category prefix", got, wireName)
		}
	}
}

func TestWireAttributeNameToDataGenName_AllFourCategoryShapes(t *testing.T) {
	// The doc's translation rule table (PHASE_7_PLAN.md §1.5) covers four
	// shapes; make sure each one is exercised explicitly, not just implied by
	// the full-table sweep above.
	cases := []struct {
		wireName string
		want     string
	}{
		{"generic.movement_speed", "minecraft:movement_speed"},            // generic. prefix
		{"player.block_break_speed", "minecraft:block_break_speed"},       // player. prefix
		{"zombie.spawn_reinforcements", "minecraft:spawn_reinforcements"}, // zombie. prefix
		{"burning_time", "minecraft:burning_time"},                        // no prefix
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, wireAttributeNameToDataGenName(tc.wireName), "wire name %q", tc.wireName)
	}
}

func TestStripNamespace(t *testing.T) {
	assert.Equal(t, "happy_ghast", stripNamespace("minecraft:happy_ghast"))
	assert.Equal(t, "no_namespace", stripNamespace("no_namespace"))
	assert.Equal(t, "", stripNamespace(""))
}

func TestEntityAttributeDefaultsRegistry_Fallback(t *testing.T) {
	reg := NewEntityAttributeDefaultsRegistryFromFallback()
	require.NotNil(t, reg)
	assert.True(t, reg.UsedFallback())

	speed, ok := reg.Get("donkey", "generic.movement_speed")
	require.True(t, ok, "donkey must have its own fallback, not inherit horse's")
	assert.Equal(t, 0.175, speed)

	horseSpeed, ok := reg.Get("horse", "generic.movement_speed")
	require.True(t, ok)
	assert.Equal(t, 0.225, horseSpeed)

	_, ok = reg.Get("camel", "generic.movement_speed")
	assert.False(t, ok, "camel is intentionally not in the built-in fallback table (see fallbackAttributeDefaults doc comment); its own constant is the fallback of the fallback at the call site")
}

func TestEntityAttributeDefaultsRegistry_NilSafe(t *testing.T) {
	var reg *EntityAttributeDefaultsRegistry
	_, ok := reg.Get("horse", "generic.movement_speed")
	assert.False(t, ok)
	assert.False(t, reg.UsedFallback())
	assert.Equal(t, 0, reg.Count())
}

func TestLoadEntityAttributeDefaultsRegistry_MissingDirUsesFallback(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadEntityAttributeDefaultsRegistry(dir, "1.21.99")
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.True(t, reg.UsedFallback())
}

func TestLoadEntityAttributeDefaultsRegistry_ReadsRealExportShape(t *testing.T) {
	dir := t.TempDir()
	entitiesDir := filepath.Join(dir, "1.21.6", "entities", "minecraft")
	require.NoError(t, os.MkdirAll(entitiesDir, 0o755))

	// Mirrors the real mc-data-gen per-entity file shape (EntityFile /
	// EntityRecordSlim in mc-data-gen/loader/entities.go): a top-level
	// "entity_id" plus a "data" object whose "attributes" array is a list of
	// {name, base_value} pairs. Deliberately includes an untracked attribute
	// (movement_efficiency) alongside a networked one, mirroring the real
	// happy_ghast.json quoted in PHASE_7_PLAN.md §1.3.
	payload := []byte(`{
		"entity_id": "minecraft:happy_ghast",
		"data": {
			"attributes": [
				{"name": "minecraft:movement_speed", "base_value": 0.05},
				{"name": "minecraft:flying_speed", "base_value": 0.05},
				{"name": "minecraft:movement_efficiency", "base_value": 0}
			]
		}
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(entitiesDir, "happy_ghast.json"), payload, 0o644))

	reg, err := LoadEntityAttributeDefaultsRegistry(dir, "1.21.6")
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.False(t, reg.UsedFallback())
	assert.Equal(t, 1, reg.Count())

	speed, ok := reg.Get("happy_ghast", "generic.movement_speed")
	require.True(t, ok)
	assert.Equal(t, 0.05, speed)

	flyingSpeed, ok := reg.Get("happy_ghast", "generic.flying_speed")
	require.True(t, ok)
	assert.Equal(t, 0.05, flyingSpeed)

	// The untracked attribute (movement_efficiency) is still present in the
	// loaded data and answerable if queried by its translated name — this
	// registry doesn't filter it out. What actually makes it unreachable in
	// practice (§1.6) is that no real wire packet ever contains a matching
	// key for GetEntityAttribute's live map to begin with, which is a
	// property of the live path, not this one.
	untrackedValue, ok := reg.Get("happy_ghast", "generic.movement_efficiency")
	require.True(t, ok)
	assert.Equal(t, 0.0, untrackedValue)

	// An entity type with no data at all falls through cleanly.
	_, ok = reg.Get("nonexistent_entity", "generic.movement_speed")
	assert.False(t, ok)
}

func TestLoadEntityAttributeDefaultsRegistry_EntityWithNoAttributesOmitted(t *testing.T) {
	// A non-LivingEntity (e.g. arrow) legitimately has an empty attributes
	// array in the real export (PHASE_6_PLAN.md §1.7's arrow.json check).
	// Such entities should not appear in the registry at all, so a lookup
	// against them reports found=false rather than "found an empty set."
	dir := t.TempDir()
	entitiesDir := filepath.Join(dir, "1.21.6", "entities", "minecraft")
	require.NoError(t, os.MkdirAll(entitiesDir, 0o755))
	payload := []byte(`{"entity_id": "minecraft:arrow", "data": {"attributes": []}}`)
	require.NoError(t, os.WriteFile(filepath.Join(entitiesDir, "arrow.json"), payload, 0o644))

	reg, err := LoadEntityAttributeDefaultsRegistry(dir, "1.21.6")
	require.NoError(t, err)
	// No entity type carried any attributes, so this degrades to the
	// built-in fallback rather than an empty-but-"loaded" registry.
	assert.True(t, reg.UsedFallback())
}
