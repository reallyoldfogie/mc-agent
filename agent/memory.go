package agent

import (
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	_ "github.com/reallyoldfogie/mc-agent/memorystore/boltstore" // registers backend "bbolt"
	_ "github.com/reallyoldfogie/mc-agent/memorystore/jsonstore" // registers backend "json"
	"github.com/reallyoldfogie/mc-agent/models"
)

// defaultMemoryStoreBackend/defaultMemoryStoreDir are used when
// models.AgentConfig leaves MemoryStoreBackend/MemoryStoreDir unset. json
// is the default backend specifically because it adds no new dependency —
// see docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md's "Data store options."
const (
	defaultMemoryStoreBackend = "json"
	defaultMemoryStoreDir     = "memory"
)

// openWorldMemory opens name's persistent world-memory store per cfg,
// falling back to the package defaults for an unset backend/dir. Called
// once from New and never again for this *agent's lifetime — see the
// plan's "Reset/session lifetime": the store is constructed once per
// long-lived agent instance, not reconstructed or wiped on Init/Start or
// any RL episode reset.
func openWorldMemory(cfg models.AgentConfig, name string) (memorystore.Store, error) {
	backend := cfg.MemoryStoreBackend
	if backend == "" {
		backend = defaultMemoryStoreBackend
	}
	dir := cfg.MemoryStoreDir
	if dir == "" {
		dir = defaultMemoryStoreDir
	}
	return memorystore.Open(backend, memorystore.Config{Dir: dir, AgentName: name})
}

// recordVisibleBlockSightings reports every block
// findAllVisibleSurfaceBlocks actually saw to the world-memory store, per
// Part 1 of docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md ("every
// findAllVisibleSurfaceBlocks result becomes (or refreshes) a record").
// Runs in its own goroutine, tracked by a.wg like every other background
// task New starts, so a slow store write (jsonstore rewrites its whole
// file per call — an accepted tradeoff, see the plan's "Backend 1") never
// adds latency to the caller's actual perception result.
func (a *agent) recordVisibleBlockSightings(blocks []models.VisibleBlockInfo) {
	if a.worldMemory == nil || len(blocks) == 0 {
		return
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		now := time.Now()
		for _, b := range blocks {
			if b.BlockName == "" {
				continue
			}
			if err := a.worldMemory.RecordSighting(memorystore.Sighting{
				ItemOrBlockName: b.BlockName,
				Position:        memorystore.Position{X: b.X, Y: b.Y, Z: b.Z},
				ObservedAt:      now,
			}); err != nil {
				a.logf("[Agent %s] [WorldMemory] failed to record sighting of %s at (%d,%d,%d): %v", a.cfg.Name, b.BlockName, b.X, b.Y, b.Z, err)
			}
		}
	}()
}

// RecordContainerSighting implements models.ContainerSightingRecorder,
// letting items.ContainerHelper report container-slot contents it has
// already read for an unrelated reason — see Part 3 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md. Item-ID-to-name resolution
// happens here, via a.GetItemManager(), rather than inside ContainerHelper,
// so items.ContainerHelper doesn't need its own ItemManager dependency on
// top of its existing optional providers just to report what it already
// read.
func (a *agent) RecordContainerSighting(s models.ContainerSighting) {
	if a.worldMemory == nil || s.ItemID == 0 {
		return
	}
	itemMgr := a.GetItemManager()
	if itemMgr == nil {
		return
	}
	itemName := itemMgr.GetItemNameByID(s.ItemID)
	if itemName == "" {
		return
	}

	pos := memorystore.Position{X: int(s.ContainerPos.X), Y: int(s.ContainerPos.Y), Z: int(s.ContainerPos.Z)}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.worldMemory.RecordSighting(memorystore.Sighting{
			ItemOrBlockName: itemName,
			Position:        pos,
			ObservedAt:      time.Now(),
			Container: &memorystore.ContainerRef{
				ContainerPos: pos,
				SlotIndex:    s.SlotIndex,
			},
		}); err != nil {
			a.logf("[Agent %s] [WorldMemory] failed to record container sighting of %s at %v: %v", a.cfg.Name, itemName, pos, err)
		}
	}()
}

// RecordReservation implements models.ReservationRecorder, letting
// buildStructureWith report every position it successfully places a block
// at — see Part 4 of docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md. Called
// once per placed block (not once per whole structure), specifically so a
// crash partway through a build doesn't lose the reservations for
// whatever was already placed — see the plan's "Reset/session lifetime."
func (a *agent) RecordReservation(pos models.V3, structureID string) {
	if a.worldMemory == nil {
		return
	}
	position := memorystore.Position{
		X: int(math.Round(pos.X)),
		Y: int(math.Round(pos.Y)),
		Z: int(math.Round(pos.Z)),
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.worldMemory.RecordReservation(memorystore.Reservation{
			Position:    position,
			StructureID: structureID,
			PlacedAt:    time.Now(),
		}); err != nil {
			a.logf("[Agent %s] [WorldMemory] failed to record reservation at %v: %v", a.cfg.Name, position, err)
		}
	}()
}

// isReservedPosition reports whether (x,y,z) has already been claimed by
// a build this agent recorded via RecordReservation — see Part 4 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md ("any ordinary
// mining/foraging logic should treat a reserved position as off-limits
// for material-sourcing purposes, even if it technically holds a matching
// block"). Fails open (false) if there's no world-memory store or the
// lookup errors — a transient store problem should never block ordinary
// mining, the same tolerance every other worldMemory path in this file
// gives a failed write.
func (a *agent) isReservedPosition(x, y, z float64) bool {
	if a.worldMemory == nil {
		return false
	}
	pos := memorystore.Position{X: int(math.Round(x)), Y: int(math.Round(y)), Z: int(math.Round(z))}
	reserved, err := a.worldMemory.IsReserved(pos)
	if err != nil {
		a.logf("[Agent %s] [WorldMemory] IsReserved(%v) failed, assuming not reserved: %v", a.cfg.Name, pos, err)
		return false
	}
	return reserved
}
