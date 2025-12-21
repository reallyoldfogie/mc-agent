package agent

import (
	"log"

	"github.com/davecgh/go-spew/spew"
	worldpkg "github.com/reallyoldfogie/mc-bot-go/bot/world"
)

// onChunkLoad logs chunk load events. In future this can update internal world state.
func (a *agent) onChunkLoad(pos worldpkg.ChunkPos) error {
	log.Println("[onChunkLoad] Loaded chunk:", pos)
	// If we later inject and expose a concrete world, we can dump details.
	// Placeholder: no internal world stored yet.
	spew.Dump(pos)
	return nil
}

// onChunkUnload logs chunk unload events.
func (a *agent) onChunkUnload(pos worldpkg.ChunkPos) error {
	log.Println("[onChunkUnload] Unload chunk:", pos)
	return nil
}

// Exported wrappers for external wiring
func (a *agent) HandleChunkLoad(pos worldpkg.ChunkPos) error   { return a.onChunkLoad(pos) }
func (a *agent) HandleChunkUnload(pos worldpkg.ChunkPos) error { return a.onChunkUnload(pos) }
