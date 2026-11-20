package agent

import (
	"context"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
)

// memoryScanRadiusDefault/memoryScanIntervalDefault are used when
// AgentConfig leaves MemoryScanRadius/MemoryScanIntervalSeconds at their
// zero value. Deliberately smaller/slower than cmd/rsi-train's own
// worldstream spectator defaults (radius 24, 1s) - that loop only ever
// ray-traces one bot at a time, for as long as a human is actually
// watching; this one runs for every agent with a store, continuously,
// for the agent's whole lifetime, so the per-tick cost is multiplied
// across however many agents are running rather than capped at one.
const (
	memoryScanRadiusDefault   = 16
	memoryScanIntervalDefault = 2 * time.Second
)

// startMemoryScanLoop starts the two goroutines that keep worldMemory
// populated from this agent's own ordinary operation, not only from
// whatever else happens to call findAllVisibleSurfaceBlocks for its own
// reason (e.g. cmd/rsi-train's spectator worldstream). No-op if worldMemory
// is nil (memoryScanSightings is nil too in that case - see New) - ties
// this loop's whole cost to a caller actually opting into persistent
// memory, same as openWorldMemory's own "none" default.
//
// Called from Start, not New: it needs baseCtx (set up in Init) and a
// live connection to report a meaningful position from, consistent with
// every other background loop Start kicks off (startEntityCleanup,
// startRenderLoop, ...).
func (a *agent) startMemoryScanLoop(ctx context.Context) {
	if a.worldMemory == nil {
		return
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.consumeMemoryScanSightings(ctx)
	}()

	radius := a.cfg.MemoryScanRadius
	if radius <= 0 {
		radius = memoryScanRadiusDefault
	}
	interval := time.Duration(a.cfg.MemoryScanIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = memoryScanIntervalDefault
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.scanVisibleBlocksOnce(ctx, radius)
			}
		}
	}()
}

// scanVisibleBlocksOnce ray-traces this agent's own currently-visible
// blocks and feeds them to recordVisibleBlockSightings, exactly like any
// other caller of FindAllVisibleBlocksInSphere - the ambient loop has no
// special path into worldMemory, it just calls the same public method
// cmd/rsi-train's spectator worldstream does. Bounded by its own timeout,
// independent of the tick interval, so one slow scan (e.g. a momentarily
// overloaded host) delays rather than stalls the loop - same reasoning as
// worldstream.go's own scanCtx.
func (a *agent) scanVisibleBlocksOnce(ctx context.Context, radius int) {
	scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := a.FindAllVisibleBlocksInSphere(scanCtx, radius); err != nil {
		// Expected and not worth escalating during the brief window before
		// the first position update arrives (FindAllVisibleBlocksInSphere's
		// own "position not initialized" case) - the next tick retries.
		a.logf("[Agent %s] [WorldMemory] ambient scan: %v", a.cfg.Name, err)
	}
}

// consumeMemoryScanSightings is the one writer worldMemory gets: it
// drains memoryScanSightings (fed by recordVisibleBlockSightings,
// regardless of which scan - ambient or otherwise - produced a given
// block) and keeps an in-memory cache of the (position, name) pairs
// already written, so a position whose occupant hasn't changed since the
// last scan costs one map lookup instead of a RecordSighting call. Owned
// entirely by this one goroutine - no lock needed, nothing else ever
// touches the cache.
//
// Only ever written-through on something actually new or changed, so a
// block this agent keeps staring at without its contents ever changing
// never gets its ObservedAt/Confidence refreshed. Accepted: the point of
// the cache is to not pay a write for "nothing changed," and a *wrong*
// memory (one that no longer matches the live block) still gets
// corrected promptly either by this same scan (a changed name at that
// position overwrites the cache entry and writes through) or by
// observeBlockChange's own packet-driven reconciliation.
func (a *agent) consumeMemoryScanSightings(ctx context.Context) {
	cache := make(map[memorystore.Position]string)
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-a.memoryScanSightings:
			if !ok {
				return
			}
			if isAirBlockName(b.BlockName) {
				continue
			}
			pos := memorystore.Position{X: b.X, Y: b.Y, Z: b.Z}
			if cache[pos] == b.BlockName {
				continue
			}
			if err := a.worldMemory.RecordSighting(memorystore.Sighting{
				ItemOrBlockName: b.BlockName,
				Position:        pos,
				ObservedAt:      time.Now(),
			}); err != nil {
				a.logf("[Agent %s] [WorldMemory] failed to record sighting of %s at (%d,%d,%d): %v", a.cfg.Name, b.BlockName, b.X, b.Y, b.Z, err)
				continue
			}
			cache[pos] = b.BlockName
		}
	}
}
