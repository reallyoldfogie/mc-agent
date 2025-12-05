package agent

import (
	"bytes"
	"log"

	pk "github.com/Tnze/go-mc/net/packet"

	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

type RegistryID string

type CustomRegistry interface {
	GetID() string
	GetNameByID(id int32) (string, bool)
	GetIDByName(name string) (int32, bool)
	IsReady() bool
}

// customRegistry stores a single registry's data (ID -> name mappings)
type customRegistry struct {
	id     RegistryID
	byID   map[int32]string
	byName map[string]int32
	ready  bool
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

// registryHandlers returns config-phase handlers to capture registry data.
func (a *Agent) registryHandlers() []PacketHandler {
	if a.packetMgr == nil {
		return nil
	}
	return []PacketHandler{
		{
			ID:       int32(a.packetMgr.GetClientboundConfigPacketID("ClientboundConfigRegistryData")),
			Priority: 100,
			F:        a.onRegistryData,
		},
	}
}

// onRegistryData processes ClientboundConfigRegistryData packets to build registries.
func (a *Agent) onRegistryData(p pk.Packet) error {
	reader := bytes.NewReader(p.Data)

	var registryID pk.String
	if _, err := registryID.ReadFrom(reader); err != nil {
		log.Printf("Failed to read registry ID: %v", err)
		return nil
	}

	var numEntries pk.VarInt
	if _, err := numEntries.ReadFrom(reader); err != nil {
		log.Printf("Failed to read number of entries for %s: %v", registryID, err)
		return nil
	}

	reg := &customRegistry{
		id:     RegistryID(registryID),
		byID:   make(map[int32]string),
		byName: make(map[string]int32),
		ready:  false,
	}

	for i := 0; i < int(numEntries); i++ {
		var entryKey pk.String
		var hasData pk.Boolean
		if _, err := entryKey.ReadFrom(reader); err != nil {
			log.Printf("Failed to read entry %d key: %v", i, err)
			break
		}
		if _, err := hasData.ReadFrom(reader); err != nil {
			log.Printf("Failed to read entry %d hasData: %v", i, err)
			break
		}
		reg.byID[int32(i)] = string(entryKey)
		reg.byName[string(entryKey)] = int32(i)
		if hasData {
			var nbtData protocol_models.NBTField
			if _, err := nbtData.ReadFrom(reader); err != nil {
				// skip errors; continue processing
				log.Printf("[ERROR][onRegistryData] Failed to read entry %d NBT data: %v", i, err)
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
func (a *Agent) GetRegistry(id RegistryID) CustomRegistry {
	a.regMu.RLock()
	defer a.regMu.RUnlock()
	return a.registries[id]
}

// onRegistryDataCallback handles registry data received during configuration phase.
// This is invoked by the bot client as each registry is processed.
func (a *Agent) onRegistryDataCallback(registryID string, entries map[string]int32) {
	// Handle entity type registry to set player entity type for movement mirror
	if registryID == "minecraft:entity_type" && a.moveMirror != nil {
		if playerTypeID, ok := entries["minecraft:player"]; ok {
			log.Printf("[Registry] Setting player entity type to %d for movement mirror", playerTypeID)
			a.moveMirror.SetEntityType(playerTypeID)
		} else {
			log.Printf("[Registry] WARNING: minecraft:player not found in entity_type registry")
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
	}

	for name, id := range entries {
		reg.byID[id] = name
		reg.byName[name] = id
	}

	a.regMu.Lock()
	if a.registries == nil {
		a.registries = make(map[RegistryID]CustomRegistry)
	}
	a.registries[reg.id] = reg
	a.regMu.Unlock()

	log.Printf("[Registry] Stored %s registry with %d entries", registryID, len(entries))
}
