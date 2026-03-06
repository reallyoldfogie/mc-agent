package agent

import (
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// onChunkLoad logs chunk load events. In future this can update internal world state.
func (a *agent) onChunkLoad(pos models.ChunkPos) error {
	log.Printf("[onChunkLoad %s] Loaded chunk: %#v", a.cfg.Name, pos)
	// If we later inject and expose a concrete world, we can dump details.
	// Placeholder: no internal world stored yet.
	//spew.Dump(pos)

	// Note: HPA* update handler uses automatic batch flushing
	// Batch mode is always enabled - it flushes automatically after:
	// - 1 second of inactivity, OR
	// - 100 cluster updates accumulated
	// This handles both initial load and dynamic chunk loading during movement

	return nil
}

// onChunkUnload logs chunk unload events.
func (a *agent) onChunkUnload(pos models.ChunkPos) error {
	log.Printf("[onChunkUnload %s] Unload chunk: %#v", a.cfg.Name, pos)
	return nil
}

// Exported wrappers for external wiring
func (a *agent) HandleChunkLoad(pos models.ChunkPos) error   { return a.onChunkLoad(pos) }
func (a *agent) HandleChunkUnload(pos models.ChunkPos) error { return a.onChunkUnload(pos) }
