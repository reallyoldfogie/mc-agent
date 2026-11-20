// Package jsonstore is memorystore's default backend: a single JSON file
// per agent, written atomically on every change. See
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md's "Backend 1" for the
// rationale and its accepted downsides (full-file rewrite per write, full
// scan per query unless indexed in memory on load — this backend builds
// that in-memory index, see store.go).
//
// Deliberately not structure/materials.go's WriteJSON/os.WriteFile
// pattern: that single os.WriteFile call truncates the destination before
// writing, so a crash mid-write can leave a truncated/zero-length file —
// fine for materials.go's operator-triggered, re-runnable export, not
// fine for a store that must survive an unplanned crash (see the plan's
// "Reset/session lifetime"). This backend instead writes a temp file in
// the same directory, fsyncs it, then os.Rename over the real path — a
// POSIX rename is atomic, so a reader (including the next process start)
// only ever sees the old complete file or the new complete file, never a
// torn one.
package jsonstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
)

func init() {
	memorystore.Register("json", New)
}

// fileContents is the whole on-disk shape of one agent's JSON store file.
type fileContents struct {
	Sightings    []memorystore.Sighting    `json:"sightings"`
	Reservations []memorystore.Reservation `json:"reservations"`
}

// Store is jsonstore's memorystore.Store implementation. All state lives
// in memory (contents) and is rewritten to disk in full on every write;
// mu serializes both so a RecordSighting and a RecordReservation from
// different goroutines can't interleave a partial in-memory update into
// the same file write.
type Store struct {
	mu       sync.Mutex
	path     string
	contents fileContents

	// reserved indexes contents.Reservations by Position for O(1)
	// IsReserved lookups instead of a linear scan per check.
	reserved map[memorystore.Position]bool
}

// New opens (or creates) the JSON store file for cfg.AgentName under
// cfg.Dir. Registered under backend name "json".
func New(cfg memorystore.Config) (memorystore.Store, error) {
	if cfg.AgentName == "" {
		return nil, fmt.Errorf("jsonstore: AgentName is required")
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("jsonstore: create dir %s: %w", cfg.Dir, err)
	}
	path := filepath.Join(cfg.Dir, cfg.AgentName+".json")

	s := &Store{path: path, reserved: map[memorystore.Position]bool{}}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.contents); err != nil {
			return nil, fmt.Errorf("jsonstore: decode %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// Fresh store — nothing to load.
	default:
		return nil, fmt.Errorf("jsonstore: read %s: %w", path, err)
	}

	for _, r := range s.contents.Reservations {
		s.reserved[r.Position] = true
	}
	return s, nil
}

// RecordSighting implements memorystore.Store.
func (s *Store) RecordSighting(sighting memorystore.Sighting) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.contents.Sightings {
		if sameSighting(existing, sighting) {
			s.contents.Sightings[i] = sighting
			return s.flushLocked()
		}
	}
	s.contents.Sightings = append(s.contents.Sightings, sighting)
	return s.flushLocked()
}

// sameSighting reports whether a and b refer to the same remembered slot,
// so a fresh observation refreshes the existing record (per Part 1's
// "becomes (or refreshes) a record") instead of piling up duplicates for
// the same spot. A loose (non-container) sighting's identity is its
// item/block name plus position. A container sighting's identity is the
// slot itself (ContainerPos+SlotIndex) only — not which item currently
// occupies it: a different item placed in the same slot must overwrite
// the old record, not sit alongside it, since nothing else could ever
// make the stale one go away (see jsonstore.Store.ForgetContainerSlot for
// the complementary "slot became empty" case, which has no replacement
// item to record under and so can't go through this path at all).
func sameSighting(a, b memorystore.Sighting) bool {
	if (a.Container == nil) != (b.Container == nil) {
		return false
	}
	if a.Container != nil {
		return *a.Container == *b.Container
	}
	return a.ItemOrBlockName == b.ItemOrBlockName && a.Position == b.Position
}

// NearestSighting implements memorystore.Store. A full scan — jsonstore's
// accepted tradeoff for query cost (see the package doc comment and the
// plan's "Backend 1"); the number of records this store holds is expected
// to stay small enough (one agent's own sightings) for this to be fine.
func (s *Store) NearestSighting(itemOrBlockName string, from memorystore.Position, query memorystore.SightingQuery) (memorystore.Sighting, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		best     memorystore.Sighting
		bestDist int64
		found    bool
		now      = time.Now()
	)
	for _, sighting := range s.contents.Sightings {
		if sighting.ItemOrBlockName != itemOrBlockName {
			continue
		}
		if query.ContainerOnly && sighting.Container == nil {
			continue
		}
		if query.ExcludeContainer && sighting.Container != nil {
			continue
		}
		if query.MaxAge > 0 && now.Sub(sighting.ObservedAt) > query.MaxAge {
			continue
		}
		dist := from.Distance2(sighting.Position)
		if !found || dist < bestDist {
			best, bestDist, found = sighting, dist, true
		}
	}
	return best, found, nil
}

// SightingsNear implements memorystore.Store. A linear scan over the
// whole in-memory slice, same accepted tradeoff as NearestSighting above
// - everything's already resident, so there's no index to build or
// bother with.
func (s *Store) SightingsNear(from memorystore.Position, radius int, query memorystore.SightingQuery) ([]memorystore.Sighting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var results []memorystore.Sighting
	for _, sighting := range s.contents.Sightings {
		if query.ContainerOnly && sighting.Container == nil {
			continue
		}
		if query.ExcludeContainer && sighting.Container != nil {
			continue
		}
		if query.MaxAge > 0 && now.Sub(sighting.ObservedAt) > query.MaxAge {
			continue
		}
		if abs(sighting.Position.X-from.X) > radius || abs(sighting.Position.Y-from.Y) > radius || abs(sighting.Position.Z-from.Z) > radius {
			continue
		}
		results = append(results, sighting)
	}
	return results, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// SightingAtPosition implements memorystore.Store. A linear scan, same
// accepted tradeoff as NearestSighting above.
func (s *Store) SightingAtPosition(pos memorystore.Position) (memorystore.Sighting, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sighting := range s.contents.Sightings {
		if sighting.Container == nil && sighting.Position == pos {
			return sighting, true, nil
		}
	}
	return memorystore.Sighting{}, false, nil
}

// RecordReservation implements memorystore.Store.
func (s *Store) RecordReservation(r memorystore.Reservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.reserved[r.Position] {
		return nil // already reserved (e.g. re-placed on a repaired block) — add-only, not an error.
	}
	s.contents.Reservations = append(s.contents.Reservations, r)
	s.reserved[r.Position] = true
	return s.flushLocked()
}

// IsReserved implements memorystore.Store.
func (s *Store) IsReserved(pos memorystore.Position) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reserved[pos], nil
}

// ForgetContainerSlot implements memorystore.Store. Filters in place
// (the standard Go "keep what passes" idiom: reslicing to length 0 of the
// same backing array and appending only kept elements as we scan forward
// is safe here since the kept index never runs ahead of the read index).
func (s *Store) ForgetContainerSlot(containerPos memorystore.Position, slotIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	target := memorystore.ContainerRef{ContainerPos: containerPos, SlotIndex: slotIndex}
	kept := s.contents.Sightings[:0]
	removed := false
	for _, sighting := range s.contents.Sightings {
		if sighting.Container != nil && *sighting.Container == target {
			removed = true
			continue
		}
		kept = append(kept, sighting)
	}
	s.contents.Sightings = kept
	if !removed {
		return nil
	}
	return s.flushLocked()
}

// ForgetSighting implements memorystore.Store. Same in-place filter idiom
// as ForgetContainerSlot above.
func (s *Store) ForgetSighting(itemOrBlockName string, pos memorystore.Position) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.contents.Sightings[:0]
	removed := false
	for _, sighting := range s.contents.Sightings {
		if sighting.Container == nil && sighting.ItemOrBlockName == itemOrBlockName && sighting.Position == pos {
			removed = true
			continue
		}
		kept = append(kept, sighting)
	}
	s.contents.Sightings = kept
	if !removed {
		return nil
	}
	return s.flushLocked()
}

// Close implements memorystore.Store. No open handle to release — every
// write already fsyncs and closes its own temp file — so this is a no-op
// kept only to satisfy the interface.
func (s *Store) Close() error {
	return nil
}

// flushLocked rewrites the whole store file atomically. Caller must hold
// s.mu. See the package doc comment for why this is a temp-file+rename,
// not a direct os.WriteFile.
func (s *Store) flushLocked() error {
	data, err := json.MarshalIndent(s.contents, "", "  ")
	if err != nil {
		return fmt.Errorf("jsonstore: encode %s: %w", s.path, err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("jsonstore: create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if anything below fails before the rename.
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("jsonstore: write temp file %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("jsonstore: fsync temp file %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("jsonstore: close temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("jsonstore: rename %s to %s: %w", tmpPath, s.path, err)
	}
	succeeded = true

	// Fsync the containing directory too — on most POSIX filesystems the
	// rename's directory-entry update is only guaranteed durable after
	// this, not after the rename syscall returns.
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
