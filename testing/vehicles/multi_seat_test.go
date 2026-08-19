package vehicles

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiSeatBoatDriverPromotion is the first real coverage of the seat-index
// mechanism (agent.mountedPassengerIndex / GetMountedPassengerIndex, fixed
// 2026-08-02) with two actual client connections sharing one vehicle, rather
// than a single agent asserting its own index in isolation.
//
// A boat seats two. Only the passenger at index 0 is the controlling
// passenger (Java's isLogicalSideForUpdatingMovement) and therefore predicts
// movement and sends VehicleMove; every other seat must ride passively
// (movement/riding_passenger.go's handleRidingModePassenger zeroes riding
// velocity/drag, same as an unsaddled mount). When the driver dismounts,
// vanilla re-orders the passenger list and the remaining rider is promoted to
// index 0, which must flip it from passive to driving on the very next tick.
//
// The observable is the same one saddle_gating_test.go established:
// GetRidingDragMultiplier()/GetRidingVelocity() from models.RidingPhysicsInspector,
// which is a direct signal of which dispatch branch ran rather than an
// indirect inference from displacement.
func TestMultiSeatBoatDriverPromotion(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "SeatDriver")
			defer cleanup()

			// Same water setup TestBoatSteering uses: boats need to be afloat
			// to respond meaningfully to thrust, and the promoted rider's
			// driving check below needs real displacement to be believable.
			cmd := "fill 0 -25 0 25 -1 25 water"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("%s => %s", cmd, resp)

			cmd = "fill -1 -25 1 -1 -1 1 minecraft:stone"
			resp, err = helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill shore block")
			t.Logf("%s => %s", cmd, resp)

			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s -1 0 1", helper.ManagedAgent.Name))
			require.NoError(t, err, "teleport driver")
			time.Sleep(500 * time.Millisecond)

			boatEntityID, err := helper.SummonBoat(ctx, 1, 1, 1, "oak")
			require.NoError(t, err, "summon boat")
			time.Sleep(500 * time.Millisecond)

			// Spawn the second player against the SAME server instance so it
			// shares the boat's entity ID space with the driver.
			rider := spawnVehicleTestAgent(t, ctx, helper, "SeatRider")

			// Stand the rider next to the boat too, close enough to interact,
			// but not on top of the driver.
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s -1 0 2", rider.Name))
			require.NoError(t, err, "teleport rider")
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, boatEntityID), "mount driver")
			require.NoError(t, helper.WaitForMounted(ctx, 15*time.Second), "driver should mount")

			require.NoError(t, rider.Agent.MountEntity(ctx, boatEntityID), "mount rider")
			require.NoError(t, waitForAgentMounted(ctx, rider.Agent, 15*time.Second), "rider should mount")

			driverIndex, ok := mountedPassengerIndex(t, helper.ManagedAgent.Agent)
			require.True(t, ok, "driver agent should expose MountedEntityPositionGetter")
			riderIndex, ok := mountedPassengerIndex(t, rider.Agent)
			require.True(t, ok, "rider agent should expose MountedEntityPositionGetter")

			t.Logf("Seat assignment after mounting: driver=%d rider=%d", driverIndex, riderIndex)
			assert.Equal(t, 0, driverIndex, "first to mount should be the controlling passenger (seat 0)")
			assert.Equal(t, 1, riderIndex, "second to mount should be a passive passenger (seat 1)")

			require.NoError(t, helper.EnterManualMode())

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			driverInspector, ok := helper.GetRidingPhysicsInspector()
			require.True(t, ok, "driver should expose RidingPhysicsInspector")
			riderInspector, ok := rider.Agent.(models.RidingPhysicsInspector)
			require.True(t, ok, "rider should expose RidingPhysicsInspector")

			driverDrag := driverInspector.GetRidingDragMultiplier()
			driverVelX, driverVelZ := driverInspector.GetRidingVelocity()
			riderDrag := riderInspector.GetRidingDragMultiplier()
			riderVelX, riderVelZ := riderInspector.GetRidingVelocity()
			t.Logf("Under driver throttle: driver drag=%.4f vel=(%.4f,%.4f); rider drag=%.4f vel=(%.4f,%.4f)",
				driverDrag, driverVelX, driverVelZ, riderDrag, riderVelX, riderVelZ)

			assert.Positive(t, driverDrag, "seat 0 must run the driving handler while thrusting")
			assert.False(t, driverVelX == 0 && driverVelZ == 0, "seat 0 should accumulate riding velocity under throttle")
			assert.Zero(t, riderDrag, "seat 1 must stay on the passive-passenger path even while seat 0 thrusts")
			assert.Zero(t, riderVelX, "a passive passenger accumulates no velocity")
			assert.Zero(t, riderVelZ, "a passive passenger accumulates no velocity")

			helper.SetManualThrottle(0, 0)
			require.NoError(t, helper.WaitForRidingVelocityZero(ctx, coastTimeout))
			require.NoError(t, helper.ExitManualMode())

			// Driver dismounts; the rider should be promoted to seat 0.
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))

			require.Eventually(t, func() bool {
				idx, ok := mountedPassengerIndex(t, rider.Agent)
				return ok && idx == 0
			}, 5*time.Second, 100*time.Millisecond, "rider should be promoted to seat 0 after driver dismounts")

			agentImpl, ok := rider.Agent.(interface{ IsMounted() bool })
			require.True(t, ok)
			assert.True(t, agentImpl.IsMounted(), "rider should remain mounted, not be kicked out, when promoted")

			// Promoted rider should now be the one driving.
			require.NoError(t, rider.Agent.EnterManualMode())
			startPos, initialized := rider.Agent.GetPositionSimple()
			require.True(t, initialized)

			require.NoError(t, rider.Agent.SetManualThrottle(0, 1.0))
			time.Sleep(2 * time.Second)

			promotedDrag := riderInspector.GetRidingDragMultiplier()
			endPos, _ := rider.Agent.GetPositionSimple()
			displacement := GetDistance(startPos.X, 0, startPos.Z, endPos.X, 0, endPos.Z)
			t.Logf("Promoted rider after 2s of forward throttle: drag=%.4f displacement=%.2f", promotedDrag, displacement)

			assert.Positive(t, promotedDrag, "promoted rider must run the driving handler once in seat 0")
			assert.Greater(t, displacement, 0.5, "promoted rider under throttle should move the boat")

			require.NoError(t, rider.Agent.ExitManualMode())
			require.NoError(t, rider.Agent.DismountEntity())
			require.NoError(t, waitForAgentDismounted(ctx, rider.Agent, 5*time.Second))
		})
	}
}

// TestMultiSeatHappyGhastPromotion extends TestMultiSeatBoatDriverPromotion's
// coverage to a happy ghast's full four seats. See PHASE_6_PLAN.md §4.11 for
// why the boat's 2-seat coverage isn't enough on its own: the server's
// passenger-list reindexing on dismount has only ever been exercised at
// capacity 2, where "the other seat" and "the promoted seat" are the same
// seat, and the happyGhastStayingStill gate
// (movement/physics_executor.go's case happyGhastStayingStill) has no boat
// analogue to exercise it against at all.
func TestMultiSeatHappyGhastPromotion(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if !isVersionGreaterOrEqual(tt.MCVersion, happyGhastMinVersion) {
				t.Skipf("Happy ghast not available in version %s (requires %s+)", tt.MCVersion, happyGhastMinVersion)
			}

			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "GhastSeat0")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport GhastSeat0 100 5 100")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			ghastID, err := helper.SummonHappyGhast(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			riderNames := []string{"GhastSeat1", "GhastSeat2", "GhastSeat3"}
			riders := make([]*testingpkg.ManagedAgent, 0, len(riderNames))
			for i, name := range riderNames {
				rider := spawnVehicleTestAgent(t, ctx, helper, name)
				riders = append(riders, rider)

				// Stand each rider near the ghast's 4x4 hitbox, close enough
				// to interact-mount but not stacked on each other.
				_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %f %f %f", name, startX, startY, startZ+float64(i+1)))
				require.NoError(t, err, "teleport %s", name)
			}
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, ghastID), "mount seat 0")
			require.NoError(t, helper.WaitForMounted(ctx, 15*time.Second), "seat 0 should mount")

			for i, rider := range riders {
				require.NoError(t, rider.Agent.MountEntity(ctx, ghastID), "mount seat %d", i+1)
				require.NoError(t, waitForAgentMounted(ctx, rider.Agent, 15*time.Second), "seat %d should mount", i+1)
			}

			// Restore AI only after all four have mounted, matching
			// happy_ghast_test.go's existing sequencing.
			require.NoError(t, helper.EnableEntityAI(ctx, "minecraft:happy_ghast"))

			indices := make([]int, 4)
			idx, ok := mountedPassengerIndex(t, helper.ManagedAgent.Agent)
			require.True(t, ok, "seat 0 agent should expose MountedEntityPositionGetter")
			indices[0] = idx
			for i, rider := range riders {
				idx, ok := mountedPassengerIndex(t, rider.Agent)
				require.True(t, ok, "seat %d agent should expose MountedEntityPositionGetter", i+1)
				indices[i+1] = idx
			}
			t.Logf("Seat assignment after mounting all four: %v", indices)
			assert.Equal(t, []int{0, 1, 2, 3}, indices, "each mount should claim the next passenger index in order")

			// Climb to a settled hover altitude, the same sequence
			// TestHappyGhastHoversWithZeroGravity uses, so the post-dismount
			// staying-still check below isn't confounded by the initial
			// mount-settle window.
			require.NoError(t, helper.EnterManualMode())
			helper.SetManualJump(true)
			time.Sleep(2 * time.Second)
			helper.SetManualJump(false)
			helper.SetManualThrottle(0, 0)
			time.Sleep(3 * time.Second)

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			driverInspector, ok := helper.GetRidingPhysicsInspector()
			require.True(t, ok, "seat 0 should expose RidingPhysicsInspector")
			driverDrag := driverInspector.GetRidingDragMultiplier()
			driverVelX, driverVelZ := driverInspector.GetRidingVelocity()
			t.Logf("Seat 0 under forward throttle: drag=%.4f vel=(%.4f,%.4f)", driverDrag, driverVelX, driverVelZ)
			assert.Positive(t, driverDrag, "seat 0 must run the driving handler while thrusting")
			assert.False(t, driverVelX == 0 && driverVelZ == 0, "seat 0 should accumulate riding velocity under throttle")

			for i, rider := range riders {
				inspector, ok := rider.Agent.(models.RidingPhysicsInspector)
				require.True(t, ok, "seat %d should expose RidingPhysicsInspector", i+1)
				drag := inspector.GetRidingDragMultiplier()
				velX, velZ := inspector.GetRidingVelocity()
				t.Logf("Seat %d under seat-0 throttle: drag=%.4f vel=(%.4f,%.4f)", i+1, drag, velX, velZ)
				assert.Zero(t, drag, "seat %d must stay on the passive-passenger path while seat 0 thrusts", i+1)
				assert.Zero(t, velX, "seat %d should accumulate no velocity", i+1)
				assert.Zero(t, velZ, "seat %d should accumulate no velocity", i+1)
			}

			helper.SetManualThrottle(0, 0)
			require.NoError(t, helper.WaitForRidingVelocityZero(ctx, coastTimeout))
			require.NoError(t, helper.ExitManualMode())

			// Seat 0 dismounts; seat 1 should be promoted to seat 0.
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))

			promoted := riders[0]
			remaining := riders[1:]

			require.Eventually(t, func() bool {
				idx, ok := mountedPassengerIndex(t, promoted.Agent)
				return ok && idx == 0
			}, 5*time.Second, 100*time.Millisecond, "seat 1 should be promoted to seat 0 after seat 0 dismounts")

			// Document, don't assert on, whether the promoted rider is
			// briefly staying-still-gated right after promotion — see
			// PHASE_6_PLAN.md §4.11 point 2. The exact duration is a vanilla
			// implementation detail not worth pinning a test to.
			if entityGetter, ok := promoted.Agent.(models.MountedEntityPositionGetter); ok {
				stayingStill, known := entityGetter.IsMountedEntityHappyGhastStayingStill(ghastID)
				t.Logf("Promoted rider staying-still state immediately after promotion: stayingStill=%v known=%v", stayingStill, known)
			}

			for i, rider := range remaining {
				idx, ok := mountedPassengerIndex(t, rider.Agent)
				require.True(t, ok)
				t.Logf("Remaining rider (was seat %d) now reports seat %d", i+2, idx)
				assert.Equal(t, i+1, idx, "remaining riders should shift down by one seat, not keep their old number")
			}

			agentImpl, ok := promoted.Agent.(interface{ IsMounted() bool })
			require.True(t, ok)
			assert.True(t, agentImpl.IsMounted(), "promoted rider should remain mounted, not be kicked out")

			// Give any staying-still window time to clear before asserting
			// the promoted rider actually drives — §4.11 point 2 notes it
			// can gate even a freshly-promoted seat 0 briefly.
			time.Sleep(1 * time.Second)

			require.NoError(t, promoted.Agent.EnterManualMode())
			startPos, initialized := promoted.Agent.GetPositionSimple()
			require.True(t, initialized)

			require.NoError(t, promoted.Agent.SetManualThrottle(0, 1.0))
			time.Sleep(2 * time.Second)

			promotedInspector, ok := promoted.Agent.(models.RidingPhysicsInspector)
			require.True(t, ok)
			promotedDrag := promotedInspector.GetRidingDragMultiplier()
			endPos, _ := promoted.Agent.GetPositionSimple()
			displacement := GetDistance(startPos.X, 0, startPos.Z, endPos.X, 0, endPos.Z)
			t.Logf("Promoted rider after 2s of forward throttle: drag=%.4f displacement=%.2f", promotedDrag, displacement)

			assert.Positive(t, promotedDrag, "promoted rider must run the driving handler once in seat 0")
			assert.Greater(t, displacement, 0.5, "promoted rider under throttle should move the happy ghast")

			require.NoError(t, promoted.Agent.SetManualThrottle(0, 0))
			require.NoError(t, promoted.Agent.ExitManualMode())
			require.NoError(t, promoted.Agent.DismountEntity())
			require.NoError(t, waitForAgentDismounted(ctx, promoted.Agent, 5*time.Second))

			for _, rider := range remaining {
				require.NoError(t, rider.Agent.DismountEntity())
				require.NoError(t, waitForAgentDismounted(ctx, rider.Agent, 5*time.Second))
			}
		})
	}
}

// spawnVehicleTestAgent spawns an additional player against the same server
// instance as helper, for tests needing more than one client on one vehicle.
// Registers its own cleanup via t.Cleanup, matching the shape
// VehicleTestHelper.Cleanup already uses for helper.ManagedAgent.
func spawnVehicleTestAgent(t *testing.T, ctx context.Context, helper *VehicleTestHelper, name string) *testingpkg.ManagedAgent {
	t.Helper()

	addr := fmt.Sprintf("%s:%d", helper.Instance.Server.Host, helper.Instance.Server.HostServerPort)
	cfg := testingpkg.AgentConfig{
		Name:           name,
		ServerAddress:  addr,
		Version:        helper.Instance.Server.Version,
		MCDataGenPath:  "",
		EnableCamAgent: true,
	}
	agent, err := helper.Framework.SpawnAgent(ctx, helper.Instance, cfg)
	require.NoError(t, err, "spawn agent %s", name)

	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := agent.Stop(stopCtx); err != nil {
			t.Logf("WARNING: agent %s stop returned error: %v", name, err)
		}
	})

	require.True(t, testingpkg.WaitForPlayerOnline(ctx, helper.Instance.RCON, name, 30*time.Second),
		"agent %s never appeared in server player list", name)

	return agent
}

// mountedPassengerIndex type-asserts agent to models.MountedEntityPositionGetter
// and returns its current seat index, mirroring VehicleTestHelper.GetRidingPhysicsInspector's
// pattern for the analogous interface.
func mountedPassengerIndex(t *testing.T, agent models.Agent) (int, bool) {
	t.Helper()
	positionGetter, ok := agent.(models.MountedEntityPositionGetter)
	if !ok {
		return 0, false
	}
	return positionGetter.GetMountedPassengerIndex(), true
}

// waitForAgentMounted polls an arbitrary agent's mount state, for the second
// player in a multi-agent test where VehicleTestHelper.WaitForMounted (which
// only ever checks its own single ManagedAgent) doesn't apply.
func waitForAgentMounted(ctx context.Context, agent models.Agent, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if agent.IsMounted() {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("agent did not mount after %v", timeout)
			}
		}
	}
}

// waitForAgentDismounted is the dismount-side counterpart to waitForAgentMounted.
func waitForAgentDismounted(ctx context.Context, agent models.Agent, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if !agent.IsMounted() {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("agent did not dismount after %v", timeout)
			}
		}
	}
}
