package rlenv

import (
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// MemoryEditAgent is the optional capability Environment.Reset uses, if
// the live agent has it, to invalidate stale remembered sightings within
// a clone-area reshuffle's footprint (Config.MemoryResetClearRadius) and
// to deliberately seed a fresh one (Config.MemorySeedAccurateProb/
// MemorySeedInaccurateProb) — see those fields' own doc comments for why
// a persistent, cross-episode memory store needs both when
// Config.CloneAreaJitter reshuffles what's actually at each position
// every Reset. Deliberately optional/type-asserted, same reasoning as
// WalkabilityAgent: not every LiveAgent implementation has a world-memory
// store wired up, and Reset simply skips both if the agent doesn't
// satisfy this.
type MemoryEditAgent interface {
	// ForgetSightingsNear removes every sighting recorded within radius
	// blocks of pos — see agent.ForgetSightingsNear's own doc comment.
	ForgetSightingsNear(pos models.V3, radius int) error
	// RecordManualSighting writes a sighting of blockName at pos directly
	// into the store, as if just perceived — see
	// agent.RecordManualSighting's own doc comment.
	RecordManualSighting(pos models.V3, blockName string) error
}

// maxMineTargetScanSamples bounds findRealMineTargetPosition's cost: a
// full exhaustive scan of a 129x129 footprint (CloneAreaRadius 64) at
// even a handful of Y layers is already comparable to a single A*
// search's own node count (confirmed live: thousands of cells examined
// well under a second), so this is generous headroom, not a tight
// budget — it exists so a pathological config (a very large explore
// radius/depth) can't turn a once-per-episode scan into something that
// competes with the episode itself for wall time.
const maxMineTargetScanSamples = 5000

// applyMemoryReset invalidates stale sightings and (per
// Config.MemorySeedAccurateProb/MemorySeedInaccurateProb) seeds a fresh
// one for this episode, if mineTargetMemoryEnabled() and the live agent
// implements MemoryEditAgent. anchor is this episode's mine-search
// anchor (e.mineSearchAnchorX/Y/Z, already set by the time Reset calls
// this). Logs and continues on a store error — a failed clear/seed
// shouldn't fail the whole episode, since an unseeded/not-fully-cleared
// memory is exactly today's existing behavior, not a new failure mode.
func (e *Environment) applyMemoryReset(anchor models.V3) {
	if !e.mineTargetMemoryEnabled() || e.cfg.MineTargetBlock == "" {
		return
	}
	memAgent, ok := e.agent.(MemoryEditAgent)
	if !ok {
		return
	}
	if e.cfg.MemoryResetClearRadius > 0 {
		if err := memAgent.ForgetSightingsNear(anchor, e.cfg.MemoryResetClearRadius); err != nil {
			log.Printf("rlenv: MemoryResetClearRadius: failed to clear stale sightings near %s: %v", anchor, err)
		}
	}
	if e.cfg.MemorySeedAccurateProb > 0 || e.cfg.MemorySeedInaccurateProb > 0 {
		e.seedMineTargetMemory(memAgent, anchor)
	}
}

// seedMineTargetMemory implements the three-way split Config.
// MemorySeedAccurateProb/MemorySeedInaccurateProb document: accurate,
// inaccurate-but-near, or no seed at all, drawn fresh from e.rng (this
// Environment's own jitter source, so a fixed JitterSeed still makes a
// run's seeding choices reproducible end to end, same as Jitter/
// CloneAreaJitter already are).
func (e *Environment) seedMineTargetMemory(memAgent MemoryEditAgent, anchor models.V3) {
	accurateProb := e.cfg.MemorySeedAccurateProb
	inaccurateProb := e.cfg.MemorySeedInaccurateProb
	roll := e.rng.Float64()
	if roll >= accurateProb+inaccurateProb {
		return // the remaining probability: no seed, unchanged existing behavior
	}

	found, pos := e.findRealMineTargetPosition(anchor)
	if !found {
		return
	}

	if roll < accurateProb {
		if err := memAgent.RecordManualSighting(pos, e.cfg.MineTargetBlock); err != nil {
			log.Printf("rlenv: MemorySeedAccurateProb: failed to seed accurate sighting at %s: %v", pos, err)
		}
		return
	}

	offset := e.cfg.MemorySeedInaccurateOffset
	if offset <= 0 {
		offset = 8
	}
	sign := 1.0
	if e.rng.Intn(2) == 0 {
		sign = -1.0
	}
	wrong := pos
	switch e.rng.Intn(3) {
	case 0:
		wrong.X += sign * float64(offset)
	case 1:
		wrong.Y += sign * float64(offset)
	default:
		wrong.Z += sign * float64(offset)
	}
	if err := memAgent.RecordManualSighting(wrong, e.cfg.MineTargetBlock); err != nil {
		log.Printf("rlenv: MemorySeedInaccurateProb: failed to seed inaccurate sighting at %s: %v", wrong, err)
	}
}

// findRealMineTargetPosition looks for a genuine, currently-present
// instance of Config.MineTargetBlock within this episode's buried-search
// footprint (buriedExploreRadius horizontally, a band around
// mineSearchAnchorY - buriedExploreDepth vertically), by sampling random
// candidate positions and checking each via e.agent.BlockNameAt — the
// same always-available LiveAgent method environment.go's own Step
// already calls, not a privileged lookup. found=false if nothing turns
// up within maxMineTargetScanSamples tries (a cheap, bounded sample, not
// an exhaustive search — see that constant's own doc comment); the
// caller treats that exactly like the "no seed" case.
func (e *Environment) findRealMineTargetPosition(anchor models.V3) (found bool, pos models.V3) {
	radius := int(e.buriedExploreRadius())
	if radius <= 0 {
		return false, models.V3{}
	}
	depth := int(e.buriedExploreDepth())
	if depth <= 0 {
		depth = 1
	}
	anchorX, anchorY, anchorZ := int(anchor.X), int(anchor.Y), int(anchor.Z)
	minY := anchorY - 2*depth
	ySpan := 2*depth + 1
	xzSpan := 2*radius + 1
	total := xzSpan * xzSpan * ySpan
	if total <= 0 {
		return false, models.V3{}
	}

	samples := total
	if samples > maxMineTargetScanSamples {
		samples = maxMineTargetScanSamples
	}

	// e.rng.Perm(total)[:samples] is a random subset *without*
	// replacement, not independent per-cell draws: the latter leaves
	// ~37% of a same-sized space unvisited on average even when
	// samples == total (the classic "coupon collector" gap), which would
	// make this unreliable right where it matters most — a small,
	// genuinely-exhaustible footprint where a real block should
	// definitely be found if BlockNameAt says it's there.
	for _, idx := range e.rng.Perm(total)[:samples] {
		y := minY + idx%ySpan
		rest := idx / ySpan
		dz := rest % xzSpan
		dx := rest / xzSpan
		x := anchorX - radius + dx
		z := anchorZ - radius + dz
		if e.agent.BlockNameAt(x, y, z) == e.cfg.MineTargetBlock {
			return true, models.V3{X: float64(x), Y: float64(y), Z: float64(z)}
		}
	}
	return false, models.V3{}
}
