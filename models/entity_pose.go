package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// EntityPoseRegistry maps the protocol's pose varint to the Java EntityPose
// enum name (lowercased). The wire value is the explicit map key in the
// poses.json file emitted by the mc-data-gen Fabric exporter — a JSON object
// of the form {"0": "standing", "1": "fall_flying", ...}.
//
// When poses.json is missing for a version, the registry falls back to a
// built-in table that matches Java EntityPose ordering as of 1.21.10.
// Unknown ordinals (server sends a value we don't have a name for) are
// reported as "unknown_<n>" so callers can still log meaningfully.
type EntityPoseRegistry struct {
	// ordToName is keyed by the wire varint. We use a map rather than a slice
	// so the registry stays correct even if a future MC version leaves gaps
	// in the enum (e.g. removes a pose without renumbering the survivors).
	ordToName  map[int32]string
	nameToOrd  map[string]int32
	usedFallbk bool
}

// fallbackPoseNames is the EntityPose enum order as of Java 1.21.10/1.21.11.
// Used when data/<ver>/poses.json is not available.
var fallbackPoseNames = []string{
	"standing",
	"fall_flying",
	"sleeping",
	"swimming",
	"spin_attack",
	"crouching",
	"long_jumping",
	"dying",
	"croaking",
	"using_tongue",
	"sitting",
	"roaring",
	"sniffing",
	"emerging",
	"digging",
	"sliding",
	"shooting",
	"inhaling",
}

// LoadEntityPoseRegistry reads data/<version>/poses.json under dataBasePath.
// The file is expected to be a JSON object mapping the wire ordinal (as a
// string) to the lowercased EntityPose name, e.g. {"0":"standing","10":"sitting"}.
// If the file is absent or unreadable, it returns a registry seeded with the
// built-in fallback table and usedFallback=true.
func LoadEntityPoseRegistry(dataBasePath, version string) (*EntityPoseRegistry, error) {
	path := filepath.Join(dataBasePath, version, "poses.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newRegistryFromFallbackNames(), nil
		}
		return nil, fmt.Errorf("read poses.json: %w", err)
	}

	parsed, err := parsePosesPayload(raw)
	if err != nil {
		return nil, fmt.Errorf("parse poses.json: %w", err)
	}
	if len(parsed) == 0 {
		return newRegistryFromFallbackNames(), nil
	}
	return newRegistryFromOrdinalMap(parsed, false), nil
}

// NewEntityPoseRegistryFromFallback returns a registry seeded with the built-in
// 1.21.10 enum order. Intended for tests and for callers that haven't loaded
// per-version data yet.
func NewEntityPoseRegistryFromFallback() *EntityPoseRegistry {
	return newRegistryFromFallbackNames()
}

// parsePosesPayload accepts the on-disk format (an ordinal->name object).
// For backwards-compatibility with the very first revision of the exporter
// (which emitted a positional array), it also accepts a JSON array and
// converts it using the array index as the ordinal.
func parsePosesPayload(raw []byte) (map[int32]string, error) {
	// Try the canonical object form first.
	var asMap map[string]string
	if err := json.Unmarshal(raw, &asMap); err == nil {
		out := make(map[int32]string, len(asMap))
		for k, v := range asMap {
			ord, parseErr := strconv.Atoi(k)
			if parseErr != nil {
				return nil, fmt.Errorf("non-integer pose key %q", k)
			}
			out[int32(ord)] = strings.ToLower(v)
		}
		return out, nil
	}

	// Legacy array form.
	var asArr []string
	if err := json.Unmarshal(raw, &asArr); err != nil {
		return nil, err
	}
	out := make(map[int32]string, len(asArr))
	for i, n := range asArr {
		out[int32(i)] = strings.ToLower(n)
	}
	return out, nil
}

func newRegistryFromFallbackNames() *EntityPoseRegistry {
	ords := make(map[int32]string, len(fallbackPoseNames))
	for i, n := range fallbackPoseNames {
		ords[int32(i)] = n
	}
	return newRegistryFromOrdinalMap(ords, true)
}

func newRegistryFromOrdinalMap(ords map[int32]string, fallback bool) *EntityPoseRegistry {
	normalized := make(map[int32]string, len(ords))
	lookup := make(map[string]int32, len(ords))
	for ord, name := range ords {
		lower := strings.ToLower(name)
		normalized[ord] = lower
		lookup[lower] = ord
	}
	return &EntityPoseRegistry{ordToName: normalized, nameToOrd: lookup, usedFallbk: fallback}
}

// Name returns the pose name for a wire ordinal. Unknown ordinals come back as
// "unknown_<n>" and ok=false so callers can log.
func (r *EntityPoseRegistry) Name(ordinal int32) (string, bool) {
	if r == nil {
		return fmt.Sprintf("unknown_%d", ordinal), false
	}
	if name, ok := r.ordToName[ordinal]; ok {
		return name, true
	}
	return fmt.Sprintf("unknown_%d", ordinal), false
}

// Ordinal returns the wire value for a pose name, or -1 if unknown.
// Lookups are case-insensitive.
func (r *EntityPoseRegistry) Ordinal(name string) (int32, bool) {
	if r == nil {
		return -1, false
	}
	ord, ok := r.nameToOrd[strings.ToLower(name)]
	if !ok {
		return -1, false
	}
	return ord, true
}

// Count returns the number of poses in the registry.
func (r *EntityPoseRegistry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.ordToName)
}

// UsedFallback reports whether the registry was loaded from the built-in
// fallback rather than a poses.json file. Useful for one-time startup warnings.
func (r *EntityPoseRegistry) UsedFallback() bool {
	return r != nil && r.usedFallbk
}

// defaultPoseRegistry is shared by callers that don't have a per-version
// registry handy (e.g. unit tests, the metadata processor before wiring).
var (
	defaultPoseRegistry *EntityPoseRegistry
	defaultPoseOnce     sync.Once
)

// DefaultEntityPoseRegistry returns a process-wide registry seeded from the
// fallback table. Production code should prefer LoadEntityPoseRegistry and
// pass the result explicitly.
func DefaultEntityPoseRegistry() *EntityPoseRegistry {
	defaultPoseOnce.Do(func() {
		defaultPoseRegistry = NewEntityPoseRegistryFromFallback()
	})
	return defaultPoseRegistry
}
