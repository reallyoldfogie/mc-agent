// Package memorystore is the per-agent persistent world-memory layer
// described by docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md: a durable
// record of where this specific agent instance has personally seen
// items/blocks (including inside containers it has opened) and which
// positions it has already claimed by building something there.
//
// This is deliberately private, per-agent memory, not a shared/community
// store — see the plan's "Scope note." Every agent instance gets its own
// Store, opened once per long-lived agent/environment instance (never
// reset per episode — see the plan's "Reset/session lifetime").
//
// Storage is behind the Store interface so the two built-in backends
// (jsonstore, boltstore) and any later third backend are interchangeable:
// Parts 1-4 of the plan only ever talk to a Store, never to a specific
// backend's on-disk format.
package memorystore

import (
	"fmt"
	"sync"
	"time"
)

// Position is a world coordinate. Dimension distinguishes e.g. the
// overworld from the nether/end if this project ever tracks more than one
// concurrently — empty is fine for a single-dimension deployment.
type Position struct {
	X, Y, Z   int
	Dimension string
}

// ContainerRef identifies a single slot inside a specific container, for
// a Sighting that came from actually opening that container rather than
// seeing a loose block/item in the world.
type ContainerRef struct {
	ContainerPos Position
	SlotIndex    int
}

// Sighting is one remembered observation of an item or block at a
// position, per Part 1 of the plan. Every Sighting must trace back to a
// real perception event (findAllVisibleSurfaceBlocks, GetTrackedEntities,
// or an actually-opened container) — see the plan's "Not X-ray" risk.
type Sighting struct {
	ItemOrBlockName string
	Position        Position
	ObservedAt      time.Time
	// Container is non-nil when this sighting came from reading a
	// container's slot contents rather than a loose block/entity.
	Container *ContainerRef
}

// Reservation is one position an agent has claimed as part of something
// it built, per Part 4 of the plan. Reservations are add-only for v1 —
// see the plan's Open Questions on whether a deliberate release should
// ever exist.
type Reservation struct {
	Position    Position
	StructureID string
	PlacedAt    time.Time
}

// SightingQuery narrows NearestSighting's search.
type SightingQuery struct {
	// ContainerOnly, if true, only considers sightings with a non-nil
	// Container (i.e. remembered container-slot contents).
	ContainerOnly bool
	// ExcludeContainer, if true, only considers loose (non-container)
	// sightings — the opposite restriction from ContainerOnly, for a
	// caller that means to go mine a block and so can't use a result
	// sitting inside a container. Mutually exclusive with ContainerOnly
	// in practice; setting both just returns no results.
	ExcludeContainer bool
	// MaxAge, if non-zero, excludes sightings older than this relative to
	// time.Now() — the simplest staleness policy the plan recommends for
	// v1 (age-based expiry, not active invalidation).
	MaxAge time.Duration
}

// Store is the storage-agnostic interface every backend implements. See
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md's "Data store options" for
// why this exists: callers (Parts 1-4) code against this interface only,
// never against a specific backend.
type Store interface {
	// RecordSighting records (or refreshes, if an equivalent sighting of
	// the same item/block at the same position — and, if set, the same
	// Container+SlotIndex — already exists) a sighting. Must be durable
	// before this returns; see the plan's "Reset/session lifetime."
	RecordSighting(s Sighting) error

	// NearestSighting returns the closest known sighting of
	// itemOrBlockName to from, honoring query, or found=false if none
	// match.
	NearestSighting(itemOrBlockName string, from Position, query SightingQuery) (sighting Sighting, found bool, err error)

	// RecordReservation records a reserved position. Must be durable
	// before this returns, for the same reason as RecordSighting — a
	// build must not lose already-placed blocks' reservations to a
	// mid-build crash.
	RecordReservation(r Reservation) error

	// IsReserved reports whether pos has already been claimed by a prior
	// (or in-progress) build.
	IsReserved(pos Position) (bool, error)

	// ForgetContainerSlot removes whatever sighting is currently recorded
	// for the exact slot (containerPos, slotIndex), regardless of which
	// item it names. A container slot's identity is the slot itself, not
	// whichever item currently occupies it — recording a *different* item
	// at the same slot already overwrites cleanly (RecordSighting), but
	// the slot becoming *empty* has no item name to record a replacement
	// under, so this is the only way to clear a now-stale "item present"
	// record. Must be durable before this returns, same as every other
	// write. A no-op, not an error, if nothing was recorded at that slot.
	ForgetContainerSlot(containerPos Position, slotIndex int) error

	// ForgetSighting removes the loose (non-container) sighting of
	// itemOrBlockName at pos, if one is recorded — for a caller that has
	// just confirmed, via a real perception event, that it's no longer
	// there (e.g. the bot mined it: see agent.recordBrokenBlock). Unlike
	// a container slot, a loose sighting's identity already includes the
	// item name (two different items can't occupy the same world
	// position), so this takes both rather than position alone. A
	// no-op, not an error, if nothing was recorded there. Must be
	// durable before this returns, same as every other write.
	ForgetSighting(itemOrBlockName string, pos Position) error

	// Close releases any resources (open file handles, bbolt's file
	// lock, ...) the backend holds. Safe to call once a Store is no
	// longer needed; not required before process exit, since every write
	// is already durable on return.
	Close() error
}

// Config is what a Factory needs to open or create a per-agent Store.
type Config struct {
	// Dir is the directory holding per-agent store files. Created if it
	// doesn't exist.
	Dir string
	// AgentName keys the store file — see the plan's "Store granularity"
	// for why this is models.AgentConfig.Name, not a Mojang UUID.
	AgentName string
}

// Factory constructs a Store from Config. Registered under a backend name
// via Register.
type Factory func(cfg Config) (Store, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register makes a backend available under name for Open to construct.
// Backend packages (jsonstore, boltstore, or a third-party/operator one)
// call this from an init() func — the same registration-point pattern as
// models.ActionRegistrar, per the plan's "Extensibility" section. Panics
// on a duplicate name, the same way e.g. database/sql/driver.Register
// does, since that can only happen from a programming error (two
// backends registering the same name), never from runtime/config input.
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("memorystore: backend %q already registered", name))
	}
	registry[name] = factory
}

// Open constructs the named backend's Store from cfg. The backend must
// already be registered — callers pick up registration by importing the
// backend package (e.g. `_ "github.com/reallyoldfogie/mc-agent/memorystore/jsonstore"`)
// for its side-effecting init().
func Open(backend string, cfg Config) (Store, error) {
	registryMu.RLock()
	factory, ok := registry[backend]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("memorystore: unknown backend %q (is it imported for registration?)", backend)
	}
	return factory(cfg)
}

// Distance2 returns the squared Euclidean distance between a and b —
// squared so callers comparing distances don't need a sqrt, and so this
// has no floating-point precision concerns despite Position being
// integer-valued.
func (a Position) Distance2(b Position) int64 {
	dx := int64(a.X - b.X)
	dy := int64(a.Y - b.Y)
	dz := int64(a.Z - b.Z)
	return dx*dx + dy*dy + dz*dz
}
