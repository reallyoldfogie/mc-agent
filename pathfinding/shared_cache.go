package pathfinding

import (
	"log/slog"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// NewSharedBlockShapeManager and NewSharedStatePropertyLoader cache
// NewBlockShapeManager/NewStatePropertyLoader's results for the lifetime
// of the process, keyed by the inputs that determine their output.
//
// This is safe because every input is either identical across all agents
// in one process or produces byte-identical data: blockMgr is already a
// per-version package-level singleton (mc_versions.GetBlockMgrForVersion),
// dataBasePath/blocksJSONPath point at the same on-disk mc-data-gen
// snapshot for every agent running the same Minecraft version, and
// neither constructor's result is ever mutated afterward (blockShapeManager's
// own blockInfoCache is pure memoization of a function that's already
// deterministic in these inputs, so sharing it across agents only warms it
// faster, it doesn't change what it returns).
//
// Before this, every parallel agent independently walked the blocks
// directory and re-parsed blocks.json from scratch. Found via heap
// profile on a 10-parallel-env RL training run (2026-11-18): this
// duplicated, read-only data was ~60% of the process's in-use heap.
var (
	sharedBlockShapeManagersMu sync.Mutex
	sharedBlockShapeManagers   = map[string]*sharedBlockShapeManagerEntry{}

	sharedStatePropertyLoadersMu sync.Mutex
	sharedStatePropertyLoaders   = map[string]*sharedStatePropertyLoaderEntry{}
)

type sharedBlockShapeManagerEntry struct {
	once sync.Once
	mgr  models.BlockShapeManager
	err  error
}

type sharedStatePropertyLoaderEntry struct {
	once sync.Once
	spl  *StatePropertyLoader
	err  error
}

// NewSharedBlockShapeManager is NewBlockShapeManager, cached per
// (version, dataBasePath) for the process's lifetime. Safe to call
// concurrently from multiple agents' Init() goroutines: the first caller
// for a given key performs the real load; every other caller for that
// key (including ones racing against it) blocks until that load
// finishes, then gets the same instance.
func NewSharedBlockShapeManager(
	version string,
	dataBasePath string,
	blockMgr mc_versions.BlockMgr,
	stateProps *StatePropertyLoader,
	logger *slog.Logger,
) (models.BlockShapeManager, error) {
	key := version + "\x00" + dataBasePath

	sharedBlockShapeManagersMu.Lock()
	entry, ok := sharedBlockShapeManagers[key]
	if !ok {
		entry = &sharedBlockShapeManagerEntry{}
		sharedBlockShapeManagers[key] = entry
	}
	sharedBlockShapeManagersMu.Unlock()

	entry.once.Do(func() {
		entry.mgr, entry.err = NewBlockShapeManager(version, dataBasePath, blockMgr, stateProps, logger)
	})
	return entry.mgr, entry.err
}

// NewSharedStatePropertyLoader is NewStatePropertyLoader, cached per
// blocksJSONPath for the process's lifetime. See
// NewSharedBlockShapeManager's doc comment for why sharing is safe and
// NewSharedBlockShapeManager itself for the concurrency contract.
func NewSharedStatePropertyLoader(blocksJSONPath string) (*StatePropertyLoader, error) {
	sharedStatePropertyLoadersMu.Lock()
	entry, ok := sharedStatePropertyLoaders[blocksJSONPath]
	if !ok {
		entry = &sharedStatePropertyLoaderEntry{}
		sharedStatePropertyLoaders[blocksJSONPath] = entry
	}
	sharedStatePropertyLoadersMu.Unlock()

	entry.once.Do(func() {
		entry.spl, entry.err = NewStatePropertyLoader(blocksJSONPath)
	})
	return entry.spl, entry.err
}
