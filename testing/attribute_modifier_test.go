package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entityAttributeGetter exposes GetEntityAttribute for tests, mirroring the
// models.MountedEntityPositionGetter type-assertion pattern already used
// elsewhere for methods not on models.Agent's main interface.
type entityAttributeGetter interface {
	GetEntityAttribute(entityID int32, attributeName string) (float64, bool)
}

// TestSpeedEffectModifiesTrackedAttribute verifies PHASE_4_PLAN.md §2.1
// end-to-end against a real server: applying Speed to a tracked entity
// changes what GetEntityAttribute reports for generic.movement_speed. Before
// this fix, ClientboundEntityUpdateAttributes' per-attribute Modifiers array
// was parsed off the wire and discarded, so GetEntityAttribute would have
// kept returning the unmodified base value for as long as the effect was
// active, regardless of amplifier.
//
// Uses a second player ("Other") as the tracked entity rather than a
// summoned horse, and does not read a pre-effect baseline: observed
// empirically (both for a horse and for a player) that vanilla does not
// broadcast generic.movement_speed via ClientboundEntityUpdateAttributes at
// all while it sits at its unmodified default — matching
// resolveMountMovementSpeed's pre-existing documented fallback in
// movement/riding_common.go for exactly this case. The attribute only
// appears once something (here, Speed) actually attaches a modifier to it.
// This is a real, separate, pre-existing protocol-bandwidth optimization,
// not a bug in this fix, so the test asserts against vanilla's well-known
// player base movement_speed (0.1, PlayerEntity's attribute default)
// instead of an empirically-read one.
func TestSpeedEffectModifiesTrackedAttribute(t *testing.T) {
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

			querierCfg := DefaultAgentConfig("Querier", fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort), serverCfg.Version)
			querierCfg.EnableCamAgent = false
			querier, err := framework.SpawnAgent(ctx, inst, querierCfg)
			require.NoError(t, err, "spawn querier agent")

			otherCfg := DefaultAgentConfig("Other", fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort), serverCfg.Version)
			otherCfg.EnableCamAgent = false
			other, err := framework.SpawnAgent(ctx, inst, otherCfg)
			require.NoError(t, err, "spawn other agent")

			attrGetter, ok := querier.Agent.(entityAttributeGetter)
			require.True(t, ok, "agent should expose GetEntityAttribute")

			var otherEntityID int32
			require.Eventually(t, func() bool {
				info, found := querier.Agent.NearestPlayerInfo(ctx, false)
				if !found {
					return false
				}
				otherEntityID = info.EntityID
				return true
			}, 10*time.Second, 200*time.Millisecond, "Querier should track Other's entity")

			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:speed 30 1", other.Name))
			require.NoError(t, err, "apply speed II to Other")

			// Vanilla's player generic.movement_speed default is 0.1
			// (PlayerEntity's attribute registration). Speed II
			// (amplifier=1) is an ADD_MULTIPLIED_TOTAL modifier of
			// 0.2*(amplifier+1) = 0.4 (StatusEffects.java), so the final
			// value should be 0.1 * 1.4.
			const playerBaseMovementSpeed = 0.1
			wantBoosted := playerBaseMovementSpeed * 1.4
			var lastSeen float64
			var lastFound bool
			assert.Eventually(t, func() bool {
				v, found := attrGetter.GetEntityAttribute(otherEntityID, "generic.movement_speed")
				lastSeen, lastFound = v, found
				return found && math.Abs(v-wantBoosted) < 1e-6
			}, 5*time.Second, 100*time.Millisecond,
				"movement_speed should reflect Speed II's modifier once ClientboundEntityUpdateAttributes arrives")
			t.Logf("final movement_speed reading: found=%v value=%.6f (want %.6f)", lastFound, lastSeen, wantBoosted)

			_, _ = inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:speed", other.Name))
		})
	}
}
