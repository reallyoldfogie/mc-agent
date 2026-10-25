package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestVerifyPlayerNearPosition_RequiresRCON(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	a := agentInt.(*agent)
	_, err = a.VerifyPlayerNearPosition(context.Background(), "SomePlayer", models.V3{}, 10)
	if err == nil {
		t.Fatal("want an error when RCON is not configured")
	}
}

func TestVerifyPlayerNearPosition_WithinRadius(t *testing.T) {
	rc := &execOnlyRCON{response: "SomePlayer has the following entity data: [1.0d, 64.0d, 1.0d]"}
	a := newRLSeedTestAgent(t, rc)
	near, err := a.VerifyPlayerNearPosition(context.Background(), "SomePlayer", models.V3{X: 0, Y: 64, Z: 0}, 5)
	if err != nil {
		t.Fatalf("VerifyPlayerNearPosition: %v", err)
	}
	if !near {
		t.Fatalf("near = false, want true (distance ~%.2f is within radius 5)", 1.0*1.414)
	}
}

func TestVerifyPlayerNearPosition_OutsideRadius(t *testing.T) {
	rc := &execOnlyRCON{response: "SomePlayer has the following entity data: [100.0d, 64.0d, 100.0d]"}
	a := newRLSeedTestAgent(t, rc)
	near, err := a.VerifyPlayerNearPosition(context.Background(), "SomePlayer", models.V3{X: 0, Y: 64, Z: 0}, 5)
	if err != nil {
		t.Fatalf("VerifyPlayerNearPosition: %v", err)
	}
	if near {
		t.Fatal("near = true, want false (player is 100+ blocks away)")
	}
}

func TestVerifyPlayerNearPosition_NotLocatableIsNotAnError(t *testing.T) {
	rc := &execOnlyRCON{response: "No entity was found"}
	a := newRLSeedTestAgent(t, rc)
	near, err := a.VerifyPlayerNearPosition(context.Background(), "GhostPlayer", models.V3{}, 5)
	if err != nil {
		t.Fatalf("VerifyPlayerNearPosition: %v, want nil (not-locatable is a normal outcome, not a transport error)", err)
	}
	if near {
		t.Fatal("near = true, want false for an unlocatable player")
	}
}

// posRCON is an execOnlyRCON that answers "data get entity ... Pos"
// queries with a fixed position (near), responding to every other command
// (the heal/give RCON calls SeedMatchReset itself issues) with a plain
// success string — close enough to a real vanilla response to exercise
// SeedMatchReset's own command sequence without a live server.
type posRCON struct {
	execOnlyRCON
	playerName       string
	x, y, z          float64
	positionCommands int
}

func (r *posRCON) Exec(ctx context.Context, cmd string) (string, error) {
	r.cmds = append(r.cmds, cmd)
	if cmd == fmt.Sprintf("data get entity %s Pos", r.playerName) {
		r.positionCommands++
		return fmt.Sprintf("%s has the following entity data: [%.1fd, %.1fd, %.1fd]", r.playerName, r.x, r.y, r.z), nil
	}
	return "", nil
}

func newPosRCONTestAgent(t *testing.T, rc *posRCON) *agent {
	t.Helper()
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565", Name: "TestBot", RCON: rc})
	if err != nil {
		t.Fatal(err)
	}
	return agentInt.(*agent)
}

func TestSeedMatchReset_RefusesWhenPlayerIsFarFromArena(t *testing.T) {
	rc := &posRCON{playerName: "Bystander", x: 500, y: 64, z: 500}
	a := newPosRCONTestAgent(t, rc)
	err := a.SeedMatchReset(context.Background(), "Bystander", "minecraft:iron_sword", models.V3{X: 0, Y: 64, Z: 0}, 10)
	if err == nil {
		t.Fatal("want an error when the named player is far from the arena center")
	}
	for _, cmd := range rc.cmds {
		if strings.Contains(cmd, "effect give") || strings.Contains(cmd, "give Bystander") {
			t.Fatalf("SeedMatchReset must not touch a far-away player's state, but sent: %q", cmd)
		}
	}
}

func TestSeedMatchReset_HealsAndGivesWeaponWhenNear(t *testing.T) {
	rc := &posRCON{playerName: "Combatant", x: 1, y: 64, z: 1}
	a := newPosRCONTestAgent(t, rc)
	if err := a.SeedMatchReset(context.Background(), "Combatant", "minecraft:iron_sword", models.V3{X: 0, Y: 64, Z: 0}, 10); err != nil {
		t.Fatalf("SeedMatchReset: %v", err)
	}
	wantHeal := "effect give Combatant minecraft:instant_health 1 10"
	wantGive := "give Combatant minecraft:iron_sword"
	var sawHeal, sawGive bool
	for _, cmd := range rc.cmds {
		if cmd == wantHeal {
			sawHeal = true
		}
		if cmd == wantGive {
			sawGive = true
		}
	}
	if !sawHeal {
		t.Errorf("commands = %v, want to include %q", rc.cmds, wantHeal)
	}
	if !sawGive {
		t.Errorf("commands = %v, want to include %q", rc.cmds, wantGive)
	}
}
