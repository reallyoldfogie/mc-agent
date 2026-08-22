package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNearestPlayerInfoHonorsBlindness verifies PHASE_4_PLAN.md §4.13
// end-to-end against a real server for the NearestPlayerInfo call site
// (backing the real `follow` with no name and `fireBowAt nearest` commands,
// and the startTracking tick loop) — not just FindRideableEntitiesNear,
// which testing/vehicles/perception_test.go already covers. Calls
// NearestPlayerInfo directly rather than going through the chat-command
// layer: the no-args `follow`/`fireBowAt nearest` commands only report their
// outcome via outgoing chat messages, and no chat-capture test utility
// exists in this package, so asserting on the exposed method (the exact
// thing every real caller invokes) gives equivalent coverage without
// needing to build one.
func TestNearestPlayerInfoHonorsBlindness(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.MCVersion
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				if err := framework.StopServer(stopCtx, inst, true); err != nil {
					t.Logf("warning: failed to stop server: %v", err)
				}
			}()

			// EnableCamAgent=false: SpawnAgent otherwise always spawns a
			// companion "<name>Cam" bot alongside the main agent, which is
			// itself a real tracked player entity positioned near its owner.
			// NearestPlayerInfo has no way to distinguish it from a genuine
			// nearby player, so with cam agents enabled it was confirmed
			// (via diagnostic logging) to always win as "nearest" over the
			// deliberately-distant Other, no matter how far Other moved —
			// masking this test's actual scenario entirely.
			querierCfg := DefaultAgentConfig("Querier", fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort), serverCfg.Version)
			querierCfg.EnableCamAgent = false
			querier, err := framework.SpawnAgent(ctx, inst, querierCfg)
			require.NoError(t, err, "spawn querier agent")

			otherCfg := DefaultAgentConfig("Other", fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort), serverCfg.Version)
			otherCfg.EnableCamAgent = false
			other, err := framework.SpawnAgent(ctx, inst, otherCfg)
			require.NoError(t, err, "spawn other agent")

			time.Sleep(5 * time.Second)

			querierX, querierY, querierZ, err := inst.RCON.GetEntityPos(ctx, querier.Name)
			require.NoError(t, err, "get querier position")

			// 20 blocks: outside Blindness's 5-block and Darkness's 15-block
			// vision caps alike, well within an unrestricted search.
			targetX := querierX + 20
			_, err = inst.RCON.Teleport(ctx, other.Name, targetX, querierY, querierZ).Exec(ctx)
			require.NoError(t, err, "teleport Other 20 blocks from Querier")

			// Poll Querier's own *client-side* tracked distance to Other
			// (honorPerceptionEffects=false, so it reflects raw tracking with
			// no cap applied) rather than trusting the RCON teleport alone.
			// The RCON command lands on the server immediately, but Querier's
			// own entity-position cache for Other only updates once its
			// connection separately receives and processes the resulting
			// move/teleport packet — a second, independent propagation delay
			// that can lag well behind the server-side position. Confirmed
			// empirically: even after verifying the teleport server-side via
			// RCON, Querier's own NearestPlayerInfo was still observed
			// reporting Other at distances as low as ~3-5 blocks (its stale
			// pre-teleport tracked position), which is *why* a subsequent
			// Blindness check could fail non-deterministically — not because
			// the 5-block vision cap or effect handling were wrong, but
			// because the test was reading a distance that hadn't caught up
			// yet, some genuinely under 5 blocks by coincidence.
			require.Eventually(t, func() bool {
				info, ok := querier.Agent.NearestPlayerInfo(ctx, false)
				return ok && info.Distance > 15
			}, 10*time.Second, 200*time.Millisecond, "Querier's own tracked distance to Other should reflect the teleport")

			_, found := querier.Agent.NearestPlayerInfo(ctx, true)
			assert.True(t, found, "should find the other player when no perception-restricting effect is active")

			ignoringEffects, found := querier.Agent.NearestPlayerInfo(ctx, false)
			assert.True(t, found, "honorPerceptionEffects=false should always find the other player regardless of effect state")
			_ = ignoringEffects

			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:blindness 30 0", querier.Name))
			require.NoError(t, err, "apply blindness effect")

			// Poll rather than sleep-then-check-once: ClientboundEntityEffect
			// arrival/processing time is not bounded by a fixed constant — it
			// is visibly sensitive to concurrent Docker load, so any fixed
			// sleep is a flaky race by construction regardless of how
			// generous it is.
			assert.Eventually(t, func() bool {
				_, found := querier.Agent.NearestPlayerInfo(ctx, true)
				return !found
			}, 5*time.Second, 200*time.Millisecond,
				"should NOT find the other player 20 blocks away while blind (5-block vision cap)")

			stillIgnoringEffects, found := querier.Agent.NearestPlayerInfo(ctx, false)
			assert.True(t, found, "honorPerceptionEffects=false should still find the other player even while blind")
			_ = stillIgnoringEffects

			_, _ = inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:blindness", querier.Name))
		})
	}
}
