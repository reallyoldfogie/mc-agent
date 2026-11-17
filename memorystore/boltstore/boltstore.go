// Package boltstore is memorystore's optional backend (per
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md's "Backend 2"): a single
// go.etcd.io/bbolt file per agent. Every committed transaction is already
// durable and atomic by construction (fsync-backed commit, copy-on-write
// b-tree), so unlike jsonstore this backend doesn't need a hand-rolled
// atomic-rename and doesn't pay a whole-file rewrite per write.
//
// Selected via config (memorystore.Open("bbolt", cfg)), not a build tag —
// see the plan's "Data store options": bbolt is always compiled in, and
// "optional" only means an operator chooses it instead of the JSON
// default.
package boltstore

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	bolt "go.etcd.io/bbolt"
)

func init() {
	memorystore.Register("bbolt", New)
}

var (
	sightingsBucket    = []byte("sightings")
	reservationsBucket = []byte("reservations")
)

// Store is boltstore's memorystore.Store implementation: a thin wrapper
// around one open *bolt.DB. Every Store method is its own transaction;
// bbolt serializes writers internally, so no extra locking is needed here.
type Store struct {
	db *bolt.DB
}

// New opens (or creates) the bbolt file for cfg.AgentName under cfg.Dir.
// Registered under backend name "bbolt".
func New(cfg memorystore.Config) (memorystore.Store, error) {
	if cfg.AgentName == "" {
		return nil, fmt.Errorf("boltstore: AgentName is required")
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("boltstore: create dir %s: %w", cfg.Dir, err)
	}
	path := filepath.Join(cfg.Dir, cfg.AgentName+".bbolt")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := createEmptyBoltFile(cfg.Dir, path); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("boltstore: stat %s: %w", path, err)
	}

	db, err := bolt.Open(path, 0o644, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("boltstore: open %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// createEmptyBoltFile creates path's bbolt database file, with its
// buckets already present, atomically: the database is built at a temp
// path in dir and only renamed into place once fully initialized and
// closed. Needed because bolt.Open's own first-time initialization of a
// brand-new file (writing its initial meta/freelist pages) is not itself
// atomic — TestCrashSafety confirmed live that killing the process during
// that step can leave a file too small for a later bolt.Open to accept,
// the same class of problem jsonstore's atomic-rename write already
// guards against. Only runs once per agent, the first time its store is
// created; every write after that goes through bbolt's own
// already-durable per-transaction commit against this now-valid file.
func createEmptyBoltFile(dir, path string) error {
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("boltstore: create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("boltstore: close temp file %s: %w", tmpPath, err)
	}

	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpPath)
		}
	}()

	db, err := bolt.Open(tmpPath, 0o644, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return fmt.Errorf("boltstore: init %s: %w", tmpPath, err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(sightingsBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(reservationsBucket); err != nil {
			return err
		}
		return nil
	})
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("boltstore: init buckets in %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("boltstore: rename %s to %s: %w", tmpPath, path, err)
	}
	succeeded = true

	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

// sightingKey is itemOrBlockName + a NUL separator + the sighting's
// encoded position (and, if set, its container ref) — the item-name
// prefix is what lets NearestSighting narrow its scan to exactly one
// item's records via bbolt's ordered b-tree Cursor.Seek, rather than
// walking every sighting this store has ever recorded.
func sightingKey(s memorystore.Sighting) []byte {
	var buf bytes.Buffer
	buf.WriteString(s.ItemOrBlockName)
	buf.WriteByte(0)
	writePosition(&buf, s.Position)
	if s.Container != nil {
		buf.WriteByte(1)
		writePosition(&buf, s.Container.ContainerPos)
		writeInt64(&buf, int64(s.Container.SlotIndex))
	}
	return buf.Bytes()
}

func sightingKeyPrefix(itemOrBlockName string) []byte {
	var buf bytes.Buffer
	buf.WriteString(itemOrBlockName)
	buf.WriteByte(0)
	return buf.Bytes()
}

func reservationKey(pos memorystore.Position) []byte {
	var buf bytes.Buffer
	writePosition(&buf, pos)
	return buf.Bytes()
}

func writePosition(buf *bytes.Buffer, p memorystore.Position) {
	writeInt64(buf, int64(p.X))
	writeInt64(buf, int64(p.Y))
	writeInt64(buf, int64(p.Z))
	buf.WriteString(p.Dimension)
	buf.WriteByte(0)
}

// writeInt64 writes v as a fixed-width, sign-flipped big-endian value so
// byte-wise key ordering matches numeric ordering across negative and
// positive coordinates (plain two's-complement big-endian would sort all
// negatives after all positives).
func writeInt64(buf *bytes.Buffer, v int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v)^(1<<63))
	buf.Write(b[:])
}

// RecordSighting implements memorystore.Store.
func (s *Store) RecordSighting(sighting memorystore.Sighting) error {
	data, err := json.Marshal(sighting)
	if err != nil {
		return fmt.Errorf("boltstore: encode sighting: %w", err)
	}
	key := sightingKey(sighting)
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(sightingsBucket).Put(key, data)
	})
}

// NearestSighting implements memorystore.Store. Scans every key under
// itemOrBlockName's prefix (so cost scales with how many times this one
// item has been seen, not with the whole store) and picks the closest
// match in Go — true spatial range queries are left to a future backend,
// per the plan's "Why not also stand up sqlite now."
func (s *Store) NearestSighting(itemOrBlockName string, from memorystore.Position, query memorystore.SightingQuery) (memorystore.Sighting, bool, error) {
	var (
		best     memorystore.Sighting
		bestDist int64
		found    bool
	)
	now := time.Now()
	prefix := sightingKeyPrefix(itemOrBlockName)

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(sightingsBucket).Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var sighting memorystore.Sighting
			if err := json.Unmarshal(v, &sighting); err != nil {
				return fmt.Errorf("boltstore: decode sighting: %w", err)
			}
			if query.ContainerOnly && sighting.Container == nil {
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
		return nil
	})
	if err != nil {
		return memorystore.Sighting{}, false, err
	}
	return best, found, nil
}

// RecordReservation implements memorystore.Store.
func (s *Store) RecordReservation(r memorystore.Reservation) error {
	key := reservationKey(r.Position)
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(reservationsBucket)
		if b.Get(key) != nil {
			return nil // already reserved — add-only, not an error.
		}
		data, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("boltstore: encode reservation: %w", err)
		}
		return b.Put(key, data)
	})
}

// IsReserved implements memorystore.Store.
func (s *Store) IsReserved(pos memorystore.Position) (bool, error) {
	key := reservationKey(pos)
	var reserved bool
	err := s.db.View(func(tx *bolt.Tx) error {
		reserved = tx.Bucket(reservationsBucket).Get(key) != nil
		return nil
	})
	return reserved, err
}

// Close implements memorystore.Store, releasing bbolt's file lock.
func (s *Store) Close() error {
	return s.db.Close()
}
