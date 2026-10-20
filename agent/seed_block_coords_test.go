package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

func TestSeedBlockCoords_FloorsNegativeCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name       string
		x, y, z    float64
		wx, wy, wz int64
	}{
		{"on the floor", 10.5, -60, 3.2, 12, -61, 3},
		{"mid-fall over negative ground", 10.5, -59.5, 3.2, 12, -61, 3},
		{"negative x and z", -0.5, -60, -3.2, 1, -61, -4},
		{"positive coordinates", 4.9, 64.1, 7.5, 6, 63, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y, z := seedBlockCoords(tc.x, tc.y, tc.z)
			if x != tc.wx || y != tc.wy || z != tc.wz {
				t.Fatalf("seedBlockCoords(%v,%v,%v) = (%d,%d,%d), want (%d,%d,%d)", tc.x, tc.y, tc.z, x, y, z, tc.wx, tc.wy, tc.wz)
			}
		})
	}
}

// Reset finds these by runtime type assertion, so a signature drift would
// silently disable them; pin that the real agent satisfies each.
func TestAgentSatisfiesRLResetCapabilities(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := agentInt.(rlenv.BlockRestorer); !ok {
		t.Error("agent must satisfy rlenv.BlockRestorer")
	}
	if _, ok := agentInt.(rlenv.FarSeedAgent); !ok {
		t.Error("agent must satisfy rlenv.FarSeedAgent")
	}
	if _, ok := agentInt.(rlenv.AreaClearer); !ok {
		t.Error("agent must satisfy rlenv.AreaClearer")
	}
	if _, ok := agentInt.(rlenv.ResetAgent); !ok {
		t.Error("agent must satisfy rlenv.ResetAgent")
	}
}

// execOnlyRCON satisfies testenv.RCONHelper for the one method ClearAir
// uses (the embedded nil interface panics if anything else is called).
type execOnlyRCON struct {
	testenv.RCONHelper
	response string
	cmds     []string
}

func (r *execOnlyRCON) Exec(_ context.Context, cmd string) (string, error) {
	r.cmds = append(r.cmds, cmd)
	return r.response, nil
}

func TestClearAir_WaitsForBlockUpdatesOnlyWhenBlocksWereRemoved(t *testing.T) {
	newAgent := func(rc *execOnlyRCON) *agent {
		agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565", RCON: rc})
		if err != nil {
			t.Fatal(err)
		}
		return agentInt.(*agent)
	}

	removed := &execOnlyRCON{response: "Successfully filled 1 block(s)"}
	start := time.Now()
	if err := newAgent(removed).ClearAir(context.Background(), -3, -60, -3, 3, -56, 3); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < clearAirSyncDelay-50*time.Millisecond {
		t.Errorf("returned in %v: after removing blocks it must wait for the client to see the updates", time.Since(start))
	}
	if want := "fill -3 -60 -3 3 -56 3 minecraft:air"; len(removed.cmds) != 1 || removed.cmds[0] != want {
		t.Errorf("commands = %v, want [%s]", removed.cmds, want)
	}

	unchanged := &execOnlyRCON{response: "No blocks were filled"}
	start = time.Now()
	if err := newAgent(unchanged).ClearAir(context.Background(), -3, -60, -3, 3, -56, 3); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > clearAirSyncDelay/2 {
		t.Errorf("took %v: a fill that changed nothing has nothing to wait for", time.Since(start))
	}
}

// TestSeedGiveItem_GivesViaRCONNotPlayerCommand covers the 2026-10-14 fix:
// SeedGiveItem used to try a player/chat "give @s ..." command first,
// falling back to RCON only if that returned a transport error. Found live
// against a real dev/test server: a non-op bot's chat "give" is silently
// rejected server-side with no error at all, so the RCON fallback never
// fired and the item never arrived — "op the bot" was the workaround, not
// the fix. Training has no reason to require a bot be opped, so this now
// goes straight to RCON, which still targets this bot by name and still
// reaches its own connection with the resulting inventory update. There is
// no live connection in this test to ever satisfy InventoryCount, so the
// call is expected to time out via ctx — the point is confirming the one
// and only command it issued was the RCON give, never a player command.
func TestSeedGiveItem_GivesViaRCONNotPlayerCommand(t *testing.T) {
	rc := &execOnlyRCON{response: "Gave 1 [Wooden Pickaxe] to BuriedSmoke"}
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565", Name: "BuriedSmoke", RCON: rc})
	if err != nil {
		t.Fatal(err)
	}
	a := agentInt.(*agent)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = a.SeedGiveItem(ctx, "minecraft:wooden_pickaxe", 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SeedGiveItem error = %v, want context.DeadlineExceeded (no live connection in this test to sync inventory)", err)
	}
	if want := "give BuriedSmoke minecraft:wooden_pickaxe 1"; len(rc.cmds) != 1 || rc.cmds[0] != want {
		t.Errorf("RCON commands = %v, want exactly [%s] — must give via RCON directly, never a player command first", rc.cmds, want)
	}
}

// TestSeedCraftIngredients_GivesViaRCONNotPlayerCommand is
// TestSeedGiveItem_GivesViaRCONNotPlayerCommand's counterpart for the craft
// path (giveCraftIngredients) — same bug, same fix, same reasoning: see
// that test's doc comment. minecraft:stick is used because its recipe fits
// the player's own 2x2 grid, so no crafting table placement (and the
// SeedNearbyBlock complexity that would add) is involved.
func TestSeedCraftIngredients_GivesViaRCONNotPlayerCommand(t *testing.T) {
	baseCacheDir, err := findAnyCachedVersionDataDir(t)
	if err != nil {
		t.Skipf("no cached data_generator output found on disk: %v", err)
	}

	rc := &execOnlyRCON{response: "ok"}
	a := &agent{cfg: models.AgentConfig{Version: baseCacheDir.version, Name: "BuriedSmoke", RCON: rc}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = a.SeedCraftIngredients(ctx, "minecraft:stick")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SeedCraftIngredients error = %v, want context.DeadlineExceeded (no live connection in this test to sync inventory)", err)
	}
	if len(rc.cmds) == 0 || rc.cmds[0] != "clear BuriedSmoke" {
		t.Fatalf("RCON commands = %v, want [0] = \"clear BuriedSmoke\"", rc.cmds)
	}
	foundGive := false
	for _, cmd := range rc.cmds[1:] {
		if strings.HasPrefix(cmd, "give BuriedSmoke minecraft:") {
			foundGive = true
		}
		if strings.Contains(cmd, "@s") {
			t.Errorf("RCON command %q looks like a player/chat command, not RCON's targeted form", cmd)
		}
	}
	if !foundGive {
		t.Errorf("RCON commands = %v, want at least one \"give BuriedSmoke minecraft:...\" — must give via RCON directly, never a player command first", rc.cmds)
	}
}
