package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	loader "github.com/reallyoldfogie/mc-data-gen/loader"
)

// EntityAttributeDefaultsRegistry answers "what is entity type X's default
// value for attribute Y" from mc-data-gen's per-version entity export
// (data/<version>/entities/**/*.json). It exists so riding handlers (and any
// future consumer) don't have to hand-transcribe vanilla attribute defaults
// from decompiled source one mount at a time — see
// docs/plans/physics_and_movement_engine_enhancement/PHASE_7_PLAN.md.
//
// This is only ever a fallback. Every call site already prefers the live,
// per-entity value from the server's ClientboundEntityUpdateAttributes packet
// (GetEntityAttribute) when one has arrived; this registry only fills the gap
// before that first packet lands for a freshly-mounted entity.
type EntityAttributeDefaultsRegistry struct {
	// byEntityType is keyed by the entity type's bare name (no "minecraft:"
	// prefix, e.g. "camel" — matching the EntityType constants), then by
	// mc-data-gen's export attribute name (e.g. "minecraft:movement_speed").
	byEntityType map[string]map[string]float64
	usedFallback bool
}

// LoadEntityAttributeDefaultsRegistry reads data/<version>/entities under
// dataBasePath (mc-data-gen's per-entity JSON files, walked recursively via
// loader.LoadEntitiesDir). If the directory is missing, it returns a registry
// seeded with the built-in fallback table and usedFallback=true — mirroring
// LoadEntityPoseRegistry's behavior so a missing/stale data directory
// degrades gracefully instead of failing agent startup. Other read/parse
// errors are returned rather than silently swallowed, the same distinction
// LoadEntityPoseRegistry draws between "absent" and "broken."
func LoadEntityAttributeDefaultsRegistry(dataBasePath, version string) (*EntityAttributeDefaultsRegistry, error) {
	root := filepath.Join(dataBasePath, version, "entities")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		if err == nil || os.IsNotExist(err) {
			return newAttributeDefaultsRegistryFromFallback(), nil
		}
		return nil, fmt.Errorf("stat entities dir %s: %w", root, err)
	}

	entities, err := loader.LoadEntitiesDir(root)
	if err != nil {
		return nil, fmt.Errorf("load entities dir %s: %w", root, err)
	}

	byEntityType := make(map[string]map[string]float64, len(entities))
	for entityID, info := range entities {
		if len(info.Attributes) == 0 {
			continue
		}
		attrs := make(map[string]float64, len(info.Attributes))
		for _, attr := range info.Attributes {
			attrs[attr.Name] = attr.BaseValue
		}
		byEntityType[stripNamespace(entityID)] = attrs
	}
	if len(byEntityType) == 0 {
		return newAttributeDefaultsRegistryFromFallback(), nil
	}
	return &EntityAttributeDefaultsRegistry{byEntityType: byEntityType}, nil
}

// NewEntityAttributeDefaultsRegistryFromFallback returns a registry seeded
// with the built-in fallback table. Intended for tests and for callers that
// haven't loaded per-version data yet.
func NewEntityAttributeDefaultsRegistryFromFallback() *EntityAttributeDefaultsRegistry {
	return newAttributeDefaultsRegistryFromFallback()
}

func newAttributeDefaultsRegistryFromFallback() *EntityAttributeDefaultsRegistry {
	return &EntityAttributeDefaultsRegistry{
		byEntityType: fallbackAttributeDefaults(),
		usedFallback: true,
	}
}

// fallbackAttributeDefaults seeds the small set of entity types that shared a
// single hardcoded fallback constant before this registry existed and so had
// no way to differentiate species: handleRidingModeHorse serves horse,
// skeleton_horse, zombie_horse, donkey, and mule alike, but only ever had one
// literal (0.225, the horse value) to fall back to. Donkey and mule in
// particular had no fallback of their own at all — see PHASE_7_PLAN.md §1.2.
//
// Camel/camel_husk and nautilus/zombie_nautilus are deliberately not
// duplicated here: they already differentiate correctly without this
// registry via models.CamelDefaultMovementSpeed and NautilusState's
// IsZombie-gated DefaultMovementSpeed(), so there is no gap for this table to
// fill for them. Pig and strider are single-species handlers with their own
// physics-package constants already serving as their fallback; adding entries
// here would just duplicate those numbers in a second place.
//
// Values are vanilla's generic.movement_speed defaults, cited the same way
// elsewhere in this codebase (e.g. models.CamelDefaultMovementSpeed): horse
// 0.225, donkey/mule ~0.175, skeleton_horse/zombie_horse 0.2.
func fallbackAttributeDefaults() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"horse":          {"minecraft:movement_speed": 0.225},
		"skeleton_horse": {"minecraft:movement_speed": 0.2},
		"zombie_horse":   {"minecraft:movement_speed": 0.2},
		"donkey":         {"minecraft:movement_speed": 0.175},
		"mule":           {"minecraft:movement_speed": 0.175},
	}
}

// Get returns the data-driven default value for a wire-format attribute name
// (e.g. "generic.movement_speed", exactly what GetEntityAttribute already
// accepts) on the given entity type (bare name, no "minecraft:" prefix).
//
// found is false when the entity type isn't in the loaded data, or when it
// doesn't carry that attribute — either because vanilla doesn't register it
// for that type, or because it's one of the attributes never sent over the
// wire at all (see PHASE_7_PLAN.md §1.6: movement_efficiency, oxygen_bonus,
// and similar enchantment-only attributes are real and exported, but
// GetEntityAttribute's live path can never populate them either, so this
// registry correctly missing them too is not a regression).
func (r *EntityAttributeDefaultsRegistry) Get(entityType EntityType, wireAttributeName string) (float64, bool) {
	if r == nil {
		return 0, false
	}
	attrs, ok := r.byEntityType[string(entityType)]
	if !ok {
		return 0, false
	}
	value, ok := attrs[wireAttributeNameToDataGenName(wireAttributeName)]
	return value, ok
}

// UsedFallback reports whether the registry was loaded from the built-in
// fallback rather than real mc-data-gen data. Useful for one-time startup
// warnings.
func (r *EntityAttributeDefaultsRegistry) UsedFallback() bool {
	return r != nil && r.usedFallback
}

// Count returns the number of entity types the registry has attribute data
// for.
func (r *EntityAttributeDefaultsRegistry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.byEntityType)
}

// wireAttributeNameToDataGenName converts a wire-format attribute name (as
// produced by ParseEntityUpdateAttributes and passed to GetEntityAttribute,
// e.g. "generic.movement_speed" or the prefix-less "burning_time") into
// mc-data-gen's export convention ("minecraft:movement_speed" /
// "minecraft:burning_time"). See PHASE_7_PLAN.md §1.5 for the full mapping
// table (four wire category prefixes: generic., player., zombie., and none)
// this rule is derived from and tested against.
func wireAttributeNameToDataGenName(wireName string) string {
	if _, bareName, found := strings.Cut(wireName, "."); found {
		wireName = bareName
	}
	return "minecraft:" + wireName
}

// stripNamespace removes a leading "<namespace>:" from a registry-style
// identifier (e.g. "minecraft:happy_ghast" -> "happy_ghast"). Identifiers
// with no namespace segment are returned unchanged.
func stripNamespace(id string) string {
	if _, bareName, found := strings.Cut(id, ":"); found {
		return bareName
	}
	return id
}
