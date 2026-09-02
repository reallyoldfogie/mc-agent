package agent

import (
	"bytes"
	"fmt"
	"log/slog"

	pk "github.com/Tnze/go-mc/net/packet"

	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

type RegistryID string

// customRegistry stores a single registry's data (ID -> name mappings)
type customRegistry struct {
	id     RegistryID
	byID   map[int32]string
	byName map[string]int32
	ready  bool
	logger *slog.Logger
}

func (r *customRegistry) GetID() string {
	return string(r.id)
}

func (r *customRegistry) GetNameByID(id int32) (string, bool) {
	name, ok := r.byID[id]
	return name, ok
}

func (r *customRegistry) GetIDByName(name string) (int32, bool) {
	id, ok := r.byName[name]
	return id, ok
}

func (r *customRegistry) IsReady() bool {
	return r.ready
}

func (r *customRegistry) Dump() {
	logger := safeLogger(r.logger)
	logger.Info(fmt.Sprintf("\n%s", r.id))
	for k, v := range r.byName {
		logger.Info(fmt.Sprintf("\t%s: %d", k, v))
	}
}

// registryHandlers returns config-phase handlers to capture registry data.
func (a *agent) registryHandlers() []bot.PacketHandler {
	if a.packetMgr == nil {
		return nil
	}
	return []bot.PacketHandler{
		{
			ID:       a.packetMgr.GetClientboundConfigPacketID("ClientboundConfigRegistryData"),
			Priority: 100,
			F:        a.onRegistryData,
		},
	}
}

// onRegistryData processes ClientboundConfigRegistryData packets to build registries.
func (a *agent) onRegistryData(p pk.Packet) error {
	reader := bytes.NewReader(p.Data)

	var registryID pk.String
	if _, err := registryID.ReadFrom(reader); err != nil {
		a.logf("Failed to read registry ID: %v", err)
		return nil
	}

	var numEntries pk.VarInt
	if _, err := numEntries.ReadFrom(reader); err != nil {
		a.logf("Failed to read number of entries for %s: %v", registryID, err)
		return nil
	}

	reg := &customRegistry{
		id:     RegistryID(registryID),
		byID:   make(map[int32]string),
		byName: make(map[string]int32),
		ready:  false,
		logger: a.logger,
	}

	for i := 0; i < int(numEntries); i++ {
		var entryKey pk.String
		var hasData pk.Boolean
		if _, err := entryKey.ReadFrom(reader); err != nil {
			a.logf("Failed to read entry %d key: %v", i, err)
			break
		}
		if _, err := hasData.ReadFrom(reader); err != nil {
			a.logf("Failed to read entry %d hasData: %v", i, err)
			break
		}
		reg.byID[int32(i)] = string(entryKey)
		reg.byName[string(entryKey)] = int32(i)
		if hasData {
			var nbtData protocol_models.NBTField
			if _, err := nbtData.ReadFrom(reader); err != nil {
				// skip errors; continue processing
				a.logf("[ERROR][onRegistryData] Failed to read entry %d NBT data: %v", i, err)
			}
		}
	}

	reg.ready = true
	a.regMu.Lock()
	if a.registries == nil {
		a.registries = make(map[RegistryID]CustomRegistry)
	}
	a.registries[reg.id] = reg
	// Cache player entity type for movement mirror
	if reg.id == "minecraft:entity_type" && a.moveMirror != nil {
		if id, ok := reg.byName["minecraft:player"]; ok {
			a.moveMirror.SetEntityType(id)
		}
	}
	a.regMu.Unlock()
	return nil
}

// GetRegistry retrieves a custom registry by ID.
func (a *agent) GetRegistry(id string) CustomRegistry {
	a.regMu.RLock()
	defer a.regMu.RUnlock()
	return a.registries[RegistryID(id)]
}

// GetEntityTypeID looks up an entity type ID by name in the entity_type registry.
// Returns (id, true) if found, (0, false) if not found or registry not ready.
// Example: id, ok := agent.GetEntityTypeID("minecraft:horse")
func (a *agent) GetEntityTypeID(entityName string) (int32, bool) {
	reg := a.GetRegistry("minecraft:entity_type")
	if reg == nil || !reg.IsReady() {
		return 0, false
	}
	return reg.GetIDByName(entityName)
}

func (a *agent) DumpRegistry(regName string) {
	reg := a.GetRegistry(regName)
	if reg == nil || !reg.IsReady() {
		a.logf("[DUMP %s %s] registry %s not loaded", a.cfg.Name, a.cfg.Version, regName)
		return
	}

	a.logf("[DUMP %s %s] %s registry", a.cfg.Name, a.cfg.Version, regName)
	reg.Dump()
}

// onRegistryDataCallback handles registry data received during configuration phase.
// This is invoked by the bot client as each registry is processed.
func (a *agent) onRegistryDataCallback(registryID string, entries map[string]int32) {
	// Handle entity type registry to set player entity type for movement mirror
	if registryID == "minecraft:entity_type" && a.moveMirror != nil {
		if playerTypeID, ok := entries["minecraft:player"]; ok {
			a.logf("[Registry %s] Setting player entity type to %d for movement mirror", a.cfg.Name, playerTypeID)
			a.moveMirror.SetEntityType(playerTypeID)
		} else {
			a.logf("[Registry %s][WARN] minecraft:player not found in entity_type registry", a.cfg.Name)
		}
	}

	// Store registry data in agent's custom registry system
	// This allows the existing registry.go handlers to work if needed
	if len(entries) == 0 {
		return
	}

	reg := &customRegistry{
		id:     RegistryID(registryID),
		byID:   make(map[int32]string),
		byName: make(map[string]int32),
		ready:  true,
		logger: a.logger,
	}

	for name, id := range entries {
		reg.byID[id] = name
		reg.byName[name] = id
	}

	a.regMu.Lock()
	if a.registries == nil {
		a.registries = make(map[RegistryID]CustomRegistry)
	}

	// Check if we're overwriting an existing registry
	// In practice, file-loaded registries (entity_type, menu, block, item, etc.) and
	// packet-sent registries (worldgen/biome, chat_type, trim_pattern, etc.) are
	// complementary - they don't overlap. But we check anyway for robustness.
	_, existed := a.registries[reg.id]
	a.registries[reg.id] = reg
	a.regMu.Unlock()

	if existed {
		a.logf("[Registry %s] ⚠ Overwriting %s registry with %d entries", a.cfg.Name, registryID, len(entries))
	} else {

		a.logf("[Registry %s] Stored %s registry with %d entries", a.cfg.Name, registryID, len(entries))
	}
}
