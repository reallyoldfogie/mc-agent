package agent

import (
	"context"
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
		{"on the floor", 10.5, -60, 3.2, 12, -60, 3},
		{"mid-fall over negative ground", 10.5, -59.5, 3.2, 12, -60, 3},
		{"negative x and z", -0.5, -60, -3.2, 1, -60, -4},
		{"positive coordinates", 4.9, 64.1, 7.5, 6, 64, 7},
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
