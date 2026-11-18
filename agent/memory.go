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

// forgetBlockSighting tells the world-memory store that blockName is
// confirmed gone from (x,y,z) — called from recordBrokenBlock, the one
// point that ever confirms a break server-side (awaitBlockChanged), so
// this is the loose-sighting counterpart to the container "slot became
// empty" fix (see items.ContainerHelper.recordChestSightings' own doc
// comment for the bug that fix closed): without it, a block this agent
// itself mines would stay findable at the old position in its own store
// forever, since nothing else ever re-observes that exact spot to
// naturally refresh/correct it. Runs in its own goroutine like every
// other worldMemory write in this file, so a slow store write never adds
// latency to MineBlockAt's own already-real-time-bound break sequence.
func (a *agent) forgetBlockSighting(x, y, z int, blockName string) {
	if a.worldMemory == nil {
		return
	}
	pos := memorystore.Position{X: x, Y: y, Z: z}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.worldMemory.ForgetSighting(blockName, pos); err != nil {
			a.logf("[Agent %s] [WorldMemory] failed to forget mined %s at %v: %v", a.cfg.Name, blockName, pos, err)
		}
	}()
}

// observeBlockChange keeps a remembered loose sighting accurate against
// *any* block change this agent's own connection observes at that exact
// position — not just ones this agent itself caused (forgetBlockSighting,
// above, already covers this agent's own confirmed mining). Called from
// the ClientboundBlockUpdate/ClientboundSectionBlocksUpdate packet
// handlers (agent/handlers.go), which fire for every block change the
// server tells this client about, regardless of cause.
//
// Operator direction, 2026-11-18: "If the agent saw a block and recorded
// its location, and witnessed its removal, even if the agent wasn't the
// one to remove it, the memory needs to be updated. If the memory store
// isn't accurate, it is almost worthless." Deliberately still scoped to
// this agent's own direct perception, not communication from other
// agents (confirmed boundary, same date, see the plan doc's own "Scope
// note") — a server-sent block-update packet received by this agent's
// own connection is this agent's own perception, same as any other
// packet-driven world state update, not hearsay.
//
// The cheap SightingAtPosition lookup (one indexed point lookup, not a
// full scan — see memorystore.Store.SightingAtPosition's own doc
// comment) is what makes this safe to call on every observed block
// change, including the bulk ones ClientboundSectionBlocksUpdate
// delivers: almost every change is at a position this store has never
// recorded anything for, and the lookup reports that cheaply instead of
// scanning every sighting to find out.
//
// If the position has no sighting at all, this is an immediate no-op —
// nothing to verify or invalidate. If the remembered item still matches
// the new block, the sighting is refreshed (bumping ObservedAt, and so
// its Confidence) rather than left alone, since this is itself a fresh,
// real confirmation it's still there. If it no longer matches (including
// becoming air), the stale sighting is forgotten.
func (a *agent) observeBlockChange(x, y, z int, stateID uint32) {
	if a.worldMemory == nil {
		return
	}
	newName := a.BlockName(stateID)
	pos := memorystore.Position{X: x, Y: y, Z: z}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.reconcileBlockSighting(pos, newName)
	}()
}

// reconcileBlockSighting is observeBlockChange's testable core, split out
// so a test can drive it directly with a plain string block name instead
// of needing a live models.BlockShapeManager just to resolve one.
func (a *agent) reconcileBlockSighting(pos memorystore.Position, newName string) {
	existing, found, err := a.worldMemory.SightingAtPosition(pos)
	if err != nil {
		a.logf("[Agent %s] [WorldMemory] SightingAtPosition(%v) failed: %v", a.cfg.Name, pos, err)
		return
	}
	if !found {
		return
	}
	if newName != "" && newName == existing.ItemOrBlockName {
		if err := a.worldMemory.RecordSighting(memorystore.Sighting{
			ItemOrBlockName: newName,
			Position:        pos,
			ObservedAt:      time.Now(),
		}); err != nil {
			a.logf("[Agent %s] [WorldMemory] failed to refresh %s at %v: %v", a.cfg.Name, newName, pos, err)
		}
		return
	}
	if err := a.worldMemory.ForgetSighting(existing.ItemOrBlockName, pos); err != nil {
		a.logf("[Agent %s] [WorldMemory] failed to forget changed %s at %v: %v", a.cfg.Name, existing.ItemOrBlockName, pos, err)
	}
}

// RecordContainerSighting implements models.ContainerSightingRecorder,
// letting items.ContainerHelper report container-slot contents it has
// already read for an unrelated reason — see Part 3 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md. Item-ID-to-name resolution
// happens here, via a.GetItemManager(), rather than inside ContainerHelper,
// so items.ContainerHelper doesn't need its own ItemManager dependency on
// top of its existing optional providers just to report what it already
// read.
//
// s.ItemID == 0 means the slot is now empty (see that field's own doc
// comment) — forgets whatever was recorded for this exact slot instead of
// trying to record a sighting with no item name, so a slot the bot itself
// emptied doesn't leave a stale "item present" record behind forever.
func (a *agent) RecordContainerSighting(s models.ContainerSighting) {
	if a.worldMemory == nil {
		return
	}
	if s.ItemID == 0 {
		pos := memorystore.Position{X: int(s.ContainerPos.X), Y: int(s.ContainerPos.Y), Z: int(s.ContainerPos.Z)}
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.worldMemory.ForgetContainerSlot(pos, s.SlotIndex); err != nil {
				a.logf("[Agent %s] [WorldMemory] failed to forget container slot %v[%d]: %v", a.cfg.Name, pos, s.SlotIndex, err)
			}
		}()
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
