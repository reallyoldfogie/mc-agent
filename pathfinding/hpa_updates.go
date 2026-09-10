package pathfinding

import (
	"log/slog"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// WorldUpdateHandler handles dynamic world changes for HPA*
type WorldUpdateHandler struct {
	builder *HPABuilder
	logger  *slog.Logger

	// Batching for initial load
	batchMode      bool
	batchMutex     sync.Mutex
	dirtyBatch     map[ClusterID]bool
	lastBatchFlush time.Time

	// Configuration
	batchInterval time.Duration // Flush batch after this interval
	maxBatchSize  int           // Flush batch after this many clusters
}

// NewWorldUpdateHandler creates a new world update handler. Reuses
// builder's own logger so log lines from both stay attributed to the same
// agent without threading a separate logger through this constructor too.
func NewWorldUpdateHandler(builder *HPABuilder) *WorldUpdateHandler {
	logger := utils.SafeLogger(nil)
	if builder != nil {
		logger = utils.SafeLogger(builder.logger)
	}
	return &WorldUpdateHandler{
		builder:        builder,
		logger:         logger,
		batchMode:      false,
		dirtyBatch:     make(map[ClusterID]bool),
		lastBatchFlush: time.Now(),
		batchInterval:  time.Second, // Flush every 1 second
		maxBatchSize:   100,         // Or after 100 cluster updates
	}
}

// EnableBatchMode enables batch mode for handling many updates efficiently
// Use this during initial world load
func (h *WorldUpdateHandler) EnableBatchMode() {
	h.batchMutex.Lock()
	defer h.batchMutex.Unlock()

	h.batchMode = true
	h.dirtyBatch = make(map[ClusterID]bool)
	h.lastBatchFlush = time.Now()
	utils.SafeLogger(h.logger).Debug("[HPA* Updates] batch mode enabled")
}

// DisableBatchMode disables batch mode and flushes any pending updates
func (h *WorldUpdateHandler) DisableBatchMode() {
	h.batchMutex.Lock()
	defer h.batchMutex.Unlock()

	h.batchMode = false
	h.flushBatch()
	utils.SafeLogger(h.logger).Debug("[HPA* Updates] batch mode disabled")
}

// OnBlockChange handles a single block change
func (h *WorldUpdateHandler) OnBlockChange(pos models.V3) {
	h.batchMutex.Lock()
	defer h.batchMutex.Unlock()

	// Get affected clusters (the cluster containing this block and potentially neighbors)
	affectedClusters := h.getAffectedClusters(pos)

	if h.batchMode {
		// Add to batch
		for _, clusterID := range affectedClusters {
			h.dirtyBatch[clusterID] = true
		}

		// Check if we should flush
		if len(h.dirtyBatch) >= h.maxBatchSize || time.Since(h.lastBatchFlush) >= h.batchInterval {
			h.flushBatch()
		}
	} else {
		// Immediate invalidation (but still lazy rebuild)
		for _, clusterID := range affectedClusters {
			h.builder.GetClusterManager().MarkClusterDirty(clusterID)

			// Also remove affected edges from abstract graph
			h.invalidateClusterEdges(clusterID)
		}
	}
}

// OnMultipleBlockChanges handles multiple block changes at once (more efficient)
func (h *WorldUpdateHandler) OnMultipleBlockChanges(positions []models.V3) {
	h.batchMutex.Lock()
	defer h.batchMutex.Unlock()

	affectedSet := make(map[ClusterID]bool)

	// Collect all affected clusters
	for _, pos := range positions {
		clusters := h.getAffectedClusters(pos)
		for _, id := range clusters {
			affectedSet[id] = true
		}
	}

	if h.batchMode {
		// Add to batch
		for clusterID := range affectedSet {
			h.dirtyBatch[clusterID] = true
		}

		// Check if we should flush
		if len(h.dirtyBatch) >= h.maxBatchSize || time.Since(h.lastBatchFlush) >= h.batchInterval {
			h.flushBatch()
		}
	} else {
		// Immediate invalidation
		for clusterID := range affectedSet {
			h.builder.GetClusterManager().MarkClusterDirty(clusterID)
			h.invalidateClusterEdges(clusterID)
		}
	}

	utils.SafeLogger(h.logger).Debug("[HPA* Updates] processed block changes", "changes", len(positions), "clusters", len(affectedSet))
}

// getAffectedClusters returns all clusters that might be affected by a block change
// This includes the cluster containing the block AND neighbors if the block is on a boundary
func (h *WorldUpdateHandler) getAffectedClusters(pos models.V3) []ClusterID {
	cm := h.builder.GetClusterManager()
	mainClusterID := cm.GetClusterID(pos)

	affected := []ClusterID{mainClusterID}

	// Check if block is on a cluster boundary
	// If so, also invalidate adjacent clusters
	cluster := cm.GetCluster(mainClusterID)

	// Check each boundary (with small epsilon for floating point comparison)
	epsilon := 0.01

	// X boundaries
	if pos.X-cluster.Bounds.MinX < epsilon {
		// On west boundary
		affected = append(affected, cm.GetAdjacentClusterID(mainClusterID, West))
	}
	if cluster.Bounds.MaxX-pos.X < epsilon+1.0 {
		// On east boundary (MaxX is exclusive, so within 1 block)
		affected = append(affected, cm.GetAdjacentClusterID(mainClusterID, East))
	}

	// Y boundaries
	if pos.Y-cluster.Bounds.MinY < epsilon {
		affected = append(affected, cm.GetAdjacentClusterID(mainClusterID, Down))
	}
	if cluster.Bounds.MaxY-pos.Y < epsilon+1.0 {
		affected = append(affected, cm.GetAdjacentClusterID(mainClusterID, Up))
	}

	// Z boundaries
	if pos.Z-cluster.Bounds.MinZ < epsilon {
		affected = append(affected, cm.GetAdjacentClusterID(mainClusterID, North))
	}
	if cluster.Bounds.MaxZ-pos.Z < epsilon+1.0 {
		affected = append(affected, cm.GetAdjacentClusterID(mainClusterID, South))
	}

	return affected
}

// flushBatch marks all batched clusters as dirty
// Must be called with batchMutex held
func (h *WorldUpdateHandler) flushBatch() {
	if len(h.dirtyBatch) == 0 {
		return
	}

	count := len(h.dirtyBatch)

	// Mark all batched clusters as dirty
	for clusterID := range h.dirtyBatch {
		h.builder.GetClusterManager().MarkClusterDirty(clusterID)
		h.invalidateClusterEdges(clusterID)
	}

	// Clear batch
	h.dirtyBatch = make(map[ClusterID]bool)
	h.lastBatchFlush = time.Now()

	utils.SafeLogger(h.logger).Debug("[HPA* Updates] flushed batch", "clustersMarkedDirty", count)
}

// FlushBatch manually flushes the batch (thread-safe)
func (h *WorldUpdateHandler) FlushBatch() {
	h.batchMutex.Lock()
	defer h.batchMutex.Unlock()
	h.flushBatch()
}

// invalidateClusterEdges removes edges involving this cluster from the abstract graph
// This is more efficient than rebuilding the entire cluster immediately
func (h *WorldUpdateHandler) invalidateClusterEdges(clusterID ClusterID) {
	cm := h.builder.GetClusterManager()
	cluster := cm.GetCluster(clusterID)

	if cluster == nil {
		return
	}

	// Remove edges from abstract graph for this cluster's entrances
	abstractGraph := h.builder.GetAbstractGraph()

	for _, entrance := range cluster.Entrances {
		// Find node for this entrance
		if node, exists := abstractGraph.Nodes[entrance]; exists {
			// Clear its edges (they'll be recomputed when cluster is rebuilt)
			node.Edges = make([]*AbstractEdge, 0)
		}
	}
}

// GetStats returns statistics about the update handler
func (h *WorldUpdateHandler) GetStats() map[string]any {
	h.batchMutex.Lock()
	defer h.batchMutex.Unlock()

	return map[string]any{
		"batch_mode":         h.batchMode,
		"pending_batch_size": len(h.dirtyBatch),
		"time_since_flush":   time.Since(h.lastBatchFlush).Seconds(),
	}
}

// RebuildDirtyCluster rebuilds a single cluster if it's dirty
// This is called lazily when pathfinding needs the cluster
func (h *WorldUpdateHandler) RebuildDirtyCluster(clusterID ClusterID) {
	cm := h.builder.GetClusterManager()
	cluster := cm.GetCluster(clusterID)

	if cluster.Dirty {
		utils.SafeLogger(h.logger).Debug("[HPA* Updates] lazy rebuilding dirty cluster", "cluster", clusterID.String())
		h.builder.BuildCluster(clusterID)
	}
}

// ClearOldClusters removes clusters that haven't been used recently (LRU eviction)
func (h *WorldUpdateHandler) ClearOldClusters(maxClusters int) {
	// TODO: Implement LRU eviction when memory management is needed
	// For now, clusters remain in memory
	// This could track last access time and evict least recently used clusters
}
