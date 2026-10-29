package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// setEntityTypeRegistry injects a minimal, ready "minecraft:entity_type"
// registry directly (bypassing the real config-phase packet decode in
// onRegistryData, same package so the unexported fields are reachable) —
// GetEntityTypeID/FindNearestEntityByType depend on this being populated,
// which a bare New(...) agent never does without a live connection.
func setEntityTypeRegistry(a *agent, names map[string]int32) {
	byID := make(map[int32]string, len(names))
	for name, id := range names {
		byID[id] = name
	}
	a.regMu.Lock()
	if a.registries == nil {
		a.registries = make(map[RegistryID]CustomRegistry)
	}
	a.registries["minecraft:entity_type"] = &customRegistry{id: "minecraft:entity_type", byID: byID, byName: names, ready: true}
	a.regMu.Unlock()
}

// setTrackedEntity injects a live entity directly into the tracker, the
// same way FindNearestEntityByType reads it.
func setTrackedEntity(a *agent, id, entityType int32, x, y, z float64) {
	a.entitiesMu.Lock()
	if a.entities == nil {
		a.entities = make(map[int32]*trackedEntity)
	}
	a.entities[id] = &trackedEntity{EntityID: id, EntityType: entityType, X: x, Y: y, Z: z}
	a.entitiesMu.Unlock()
}

func newRLSeedTestAgent(t *testing.T, rc *execOnlyRCON) *agent {
	t.Helper()
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565", Name: "TestBot", RCON: rc})
	if err != nil {
		t.Fatal(err)
	}
	return agentInt.(*agent)
}

func TestSeedEntityCoords(t *testing.T) {
	for _, tc := range []struct {
		name       string
		x, y, z    float64
		wx, wy, wz int64
	}{
		{"on the floor", 10.5, -60, 3.2, 12, -60, 3},
		{"mid-fall over negative ground", 10.5, -59.5, 3.2, 12, -60, 3},
		{"negative x and z", -0.5, -60, -3.2, 1, -60, -4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y, z := seedEntityCoords(tc.x, tc.y, tc.z)
			if x != tc.wx || y != tc.wy || z != tc.wz {
				t.Fatalf("seedEntityCoords(%v,%v,%v) = (%d,%d,%d), want (%d,%d,%d)", tc.x, tc.y, tc.z, x, y, z, tc.wx, tc.wy, tc.wz)
			}
		})
	}
}

func TestSeedEntity_RequiresRCON(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	a := agentInt.(*agent)
	if _, err := a.SeedEntity(context.Background(), "minecraft:zombie", 8, ""); err == nil {
		t.Fatal("want error, got nil (RCON not configured)")
	}
}

// TestSeedEntity_UnknownEntityTypeNeverCallsRCON confirms a mob type this
// bot's registry doesn't know about fails loudly before ever reaching
// RCON, rather than silently summoning something.
func TestSeedEntity_UnknownEntityTypeNeverCallsRCON(t *testing.T) {
	rc := &execOnlyRCON{}
	a := newRLSeedTestAgent(t, rc)
	a.setPosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	if _, err := a.SeedEntity(context.Background(), "minecraft:zombie", 8, ""); err == nil {
		t.Fatal("want error, got nil (no entity_type registry loaded)")
	}
	if len(rc.cmds) != 0 {
		t.Fatalf("RCON commands = %v, want none (must not summon with an unresolved entity type)", rc.cmds)
	}
}

func TestSeedEntity_NoopWhenAlreadyWithinRadius(t *testing.T) {
	rc := &execOnlyRCON{}
	a := newRLSeedTestAgent(t, rc)
	a.setPosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)
	setEntityTypeRegistry(a, map[string]int32{"minecraft:zombie": 54})
	setTrackedEntity(a, 999, 54, 3, 64, 0)

	entityID, err := a.SeedEntity(context.Background(), "minecraft:zombie", 8, "")
	if err != nil {
		t.Fatalf("SeedEntity: %v", err)
	}
	if entityID != 999 {
		t.Fatalf("entityID = %d, want 999 (the already-nearby instance)", entityID)
	}
	if len(rc.cmds) != 0 {
		t.Fatalf("RCON commands = %v, want none (an instance already exists within radius)", rc.cmds)
	}
}

// TestSeedEntity_SummonsAndSendsExpectedCommand confirms the RCON command
// content when nothing matching is nearby — ctx's short deadline makes the
// post-summon sync-poll loop time out deterministically (this test agent
// never actually tracks the "summoned" entity), mirroring
// TestSeedGiveItem_GivesViaRCONNotPlayerCommand's established pattern for
// verifying exactly one command was sent without a live connection.
func TestSeedEntity_SummonsAndSendsExpectedCommand(t *testing.T) {
	rc := &execOnlyRCON{}
	a := newRLSeedTestAgent(t, rc)
	a.setPosition(models.V3{X: 10, Y: 64, Z: 3}, 0, 0)
	setEntityTypeRegistry(a, map[string]int32{"minecraft:zombie": 54})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := a.SeedEntity(ctx, "minecraft:zombie", 8, "{NoAI:1b,PersistenceRequired:1b}")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SeedEntity error = %v, want context.DeadlineExceeded (no live tracking in this test to confirm the summon)", err)
	}
	want := "summon minecraft:zombie 12 64 3 {NoAI:1b,PersistenceRequired:1b}"
	if len(rc.cmds) != 1 || rc.cmds[0] != want {
		t.Fatalf("RCON commands = %v, want exactly [%s]", rc.cmds, want)
	}
}

func TestSeedVehicle_AppliesTameNBTOnlyWhenRequested(t *testing.T) {
	for _, tc := range []struct {
		name string
		tame bool
		want string
	}{
		{"tame", true, "summon minecraft:horse 12 64 3 {Tame:1b}"},
		{"untamed", false, "summon minecraft:horse 12 64 3 "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc := &execOnlyRCON{}
			a := newRLSeedTestAgent(t, rc)
			a.setPosition(models.V3{X: 10, Y: 64, Z: 3}, 0, 0)
			setEntityTypeRegistry(a, map[string]int32{"minecraft:horse": 27})

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if _, err := a.SeedVehicle(ctx, "minecraft:horse", 8, tc.tame); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("SeedVehicle error = %v, want context.DeadlineExceeded", err)
			}
			if len(rc.cmds) != 1 || rc.cmds[0] != tc.want {
				t.Fatalf("RCON commands = %v, want exactly [%s]", rc.cmds, tc.want)
			}
		})
	}
}

// TestSeedDroppedItem_SendsExpectedCommandWithoutAnEntityRegistry confirms
// SeedDroppedItem still works (reaches RCON) even though
// FindNearestVisibleItem itself errors on a bare agent with no
// entity_type registry loaded — that error is treated as "nothing found
// yet", not fatal, mirroring every other seed method's "already there?"
// check degrading gracefully rather than blocking the seed attempt.
func TestSeedDroppedItem_SendsExpectedCommandWithoutAnEntityRegistry(t *testing.T) {
	rc := &execOnlyRCON{}
	a := newRLSeedTestAgent(t, rc)
	a.setPosition(models.V3{X: 10, Y: 64, Z: 3}, 0, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := a.SeedDroppedItem(ctx, "minecraft:stone", 8)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SeedDroppedItem error = %v, want context.DeadlineExceeded", err)
	}
	want := `summon minecraft:item 12 64 3 {Item:{id:"minecraft:stone",Count:1}}`
	if len(rc.cmds) != 1 || rc.cmds[0] != want {
		t.Fatalf("RCON commands = %v, want exactly [%s]", rc.cmds, want)
	}
}

func TestSeedDroppedItem_RequiresRCON(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	a := agentInt.(*agent)
	if err := a.SeedDroppedItem(context.Background(), "minecraft:stone", 8); err == nil {
		t.Fatal("want error, got nil (RCON not configured)")
	}
}

func TestClearEntitiesOfType_BuildsExpectedKillCommand(t *testing.T) {
	rc := &execOnlyRCON{}
	a := newRLSeedTestAgent(t, rc)

	if err := a.ClearEntitiesOfType(context.Background(), 10, 64, 3, 12, "minecraft:zombie"); err != nil {
		t.Fatalf("ClearEntitiesOfType: %v", err)
	}
	want := "execute positioned 10 64 3 run kill @e[type=minecraft:zombie,distance=..12]"
	if len(rc.cmds) != 1 || rc.cmds[0] != want {
		t.Fatalf("RCON commands = %v, want exactly [%s]", rc.cmds, want)
	}
}

func TestClearEntitiesOfType_RequiresRCON(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	a := agentInt.(*agent)
	if err := a.ClearEntitiesOfType(context.Background(), 0, 64, 0, 8, "minecraft:zombie"); err == nil {
		t.Fatal("want error, got nil (RCON not configured)")
	}
}
