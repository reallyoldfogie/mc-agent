package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rideableEntityFinder exposes FindRideableEntitiesNear for tests, mirroring
// the models.MountedEntityPositionGetter type-assertion pattern already used
// elsewhere in this package for methods not on models.Agent's main interface.
type rideableEntityFinder interface {
	FindRideableEntitiesNear(center models.V3, radius float64, honorPerceptionEffects bool) []pathfinding.RideableEntity
}

// TestFindRideableEntitiesNearHonorsBlindness verifies PHASE_4_PLAN.md §4.13
// end-to-end against a real server: a rideable entity outside Blindness's
// 5-block vision cap but inside the search radius is found normally with
// honorPerceptionEffects=false (or no active effect), and excluded once
// Blindness is active and honorPerceptionEffects=true — while
// honorPerceptionEffects=false continues to find it regardless, confirming
// the parameter actually gates the behavior rather than always applying it.
func TestFindRideableEntitiesNearHonorsBlindness(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "PerceptionBot")
			defer cleanup()

			finder, ok := helper.ManagedAgent.Agent.(rideableEntityFinder)
			require.True(t, ok, "agent should expose FindRideableEntitiesNear")

			pos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// 20 blocks: inside a typical 32-block pathfinding search radius,
			// outside Blindness's 5-block and Darkness's 15-block caps alike.
			horseX, horseY, horseZ := pos.X+20, pos.Y, pos.Z
			_, err := helper.SummonHorse(ctx, horseX, horseY, horseZ, 0)
			require.NoError(t, err, "summon horse")

			const searchRadius = 32.0

			// Poll for the agent's own entity tracking to pick up the newly
			// summoned horse (via its AddEntity spawn packet) rather than a
			// fixed sleep: this lagged unpredictably under concurrent Docker
			// load in the sibling NearestPlayerInfo test (see its comment for
			// the confirmed failure mode there), so the same margin-of-error
			// risk applies here even though it wasn't observed directly for
			// this test.
			require.Eventually(t, func() bool {
				return len(finder.FindRideableEntitiesNear(pos, searchRadius, false)) == 1
			}, 10*time.Second, 200*time.Millisecond, "agent should pick up the summoned horse")

			withoutEffect := finder.FindRideableEntitiesNear(pos, searchRadius, true)
			assert.Len(t, withoutEffect, 1, "should find the horse when no perception-restricting effect is active")

			ignoringEffects := finder.FindRideableEntitiesNear(pos, searchRadius, false)
			assert.Len(t, ignoringEffects, 1, "honorPerceptionEffects=false should always find the horse regardless of effect state")

			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:blindness 30 0", helper.AgentName))
			require.NoError(t, err, "apply blindness effect")

			// Poll rather than sleep-then-check-once: ClientboundEntityEffect
			// arrival/processing time is not bounded by a fixed constant — it
			// is visibly sensitive to concurrent Docker load (observed to
			// occasionally exceed 1.5s when several version subtests run
			// back-to-back), so any fixed sleep is a flaky race by
			// construction regardless of how generous it is.
			assert.Eventually(t, func() bool {
				return len(finder.FindRideableEntitiesNear(pos, searchRadius, true)) == 0
			}, 5*time.Second, 200*time.Millisecond,
				"should NOT find the horse 20 blocks away while blind (5-block vision cap)")

			stillIgnoringEffects := finder.FindRideableEntitiesNear(pos, searchRadius, false)
			assert.Len(t, stillIgnoringEffects, 1, "honorPerceptionEffects=false should still find the horse even while blind")

			_, _ = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:blindness", helper.AgentName))
		})
	}
}
