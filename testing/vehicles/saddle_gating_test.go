package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Saddle gating: a saddleable mount only obeys its rider while it is actually
// saddled. Java's AbstractHorseEntity.isSaddled() gates
// getControllingPassenger(), so an unsaddled horse ignores rider input and the
// client is not its movement authority.
//
// These two tests are deliberately a matched pair. The negative test proves the
// gate engages; the positive test proves the gate has not broken ordinary
// riding, which is the real regression risk of introducing a gate that can stop
// the agent driving.
//
// The observable used is GetRidingDragMultiplier(). Every driving handler sets
// it to a real per-tick drag value (0.546 on a default block, for instance),
// while the passive-rider handler zeroes it. It is therefore a direct signal of
// which dispatch branch ran, rather than an indirect inference from
// displacement, which can be confounded by the mount's own AI wandering.

// TestUnsaddledHorseIsNotDriven verifies that mounting an unsaddled horse routes
// to the passive-rider handler: no steering, no thrust, no accumulated velocity.
func TestUnsaddledHorseIsNotDriven(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "NoSaddleBot")
			defer cleanup()

			if !helper.SupportsSaddleEquipmentSlot() {
				t.Skipf("saddle state is not observable before %s: the saddle lives in the mount's NBT inventory and is never sent to the client (see docs/horse-nbt-data.md)",
					models.MinSaddleSlotVersion)
			}

			_, err := helper.Instance.RCON.Exec(ctx, "teleport NoSaddleBot 100 1 100")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			// Fence the horse in so its own AI cannot wander it out of interact
			// range during setup.
			require.NoError(t, helper.BuildHorseEnclosure(ctx, startX, startY, startZ))

			horseID, err := helper.SummonUnsaddledHorse(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, horseID))
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

			// Give the server time to deliver the horse's equipment packet. Until
			// it arrives the agent reports saddle state as unknown and keeps
			// driving, so asserting too early would race the packet.
			time.Sleep(1 * time.Second)

			require.NoError(t, helper.EnterManualMode())

			// Drive hard at it. A saddled horse would accelerate; this one must not.
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			inspector, ok := helper.GetRidingPhysicsInspector()
			require.True(t, ok, "agent should expose RidingPhysicsInspector")

			dragMultiplier := inspector.GetRidingDragMultiplier()
			velX, velZ := inspector.GetRidingVelocity()
			t.Logf("Unsaddled horse after 2s of forward throttle: drag=%.4f vel=(%.4f, %.4f)", dragMultiplier, velX, velZ)

			assert.Zero(t, dragMultiplier,
				"passive-rider handler zeroes the drag multiplier; a non-zero value means a driving handler ran for an unsaddled mount")
			assert.Zero(t, velX, "a passive rider accumulates no velocity")
			assert.Zero(t, velZ, "a passive rider accumulates no velocity")

			require.NoError(t, helper.ExitManualMode())
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
			require.NoError(t, helper.RemoveHorseEnclosure(ctx, startX, startY, startZ))
		})
	}
}

// TestSaddledHorseIsStillDriven is the control for TestUnsaddledHorseIsNotDriven.
//
// Saddle gating can only fail in two directions: failing to gate an unsaddled
// mount, or wrongly gating a saddled one. The second is the more damaging
// outcome because it silently disables riding, so it gets its own test rather
// than relying on the other horse tests to notice.
func TestSaddledHorseIsStillDriven(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "SaddledBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport SaddledBot 200 1 200")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			horseID, err := helper.SummonHorse(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, horseID))
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

			// Same settle window as the unsaddled case, so both tests observe the
			// agent in the same state with respect to equipment delivery.
			time.Sleep(1 * time.Second)

			require.NoError(t, helper.EnterManualMode())

			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			inspector, ok := helper.GetRidingPhysicsInspector()
			require.True(t, ok, "agent should expose RidingPhysicsInspector")

			dragMultiplier := inspector.GetRidingDragMultiplier()
			velX, velZ := inspector.GetRidingVelocity()

			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			displacement := GetDistance(finalPos.X, 0, finalPos.Z, startX, 0, startZ)
			t.Logf("Saddled horse after 2s of forward throttle: drag=%.4f vel=(%.4f, %.4f) displacement=%.2f",
				dragMultiplier, velX, velZ, displacement)

			assert.Positive(t, dragMultiplier,
				"a saddled horse must run a driving handler; zero drag means the saddle gate wrongly demoted it to a passenger")
			assert.Greater(t, displacement, 0.5, "a saddled horse under throttle should move")

			require.NoError(t, helper.ExitManualMode())
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
		})
	}
}
