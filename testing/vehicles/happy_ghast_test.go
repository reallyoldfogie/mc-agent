package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Happy ghast was added in Minecraft 1.21.6 ("Chase the Skies"). Unlike every
// other mount in this codebase, mounting requires no taming at all — only a
// harness (the body equipment slot) — and flight is genuinely zero-gravity,
// not reduced gravity, confirmed by reading HappyGhastEntity.java directly
// (mc-data-gen/extractedSrc/<version>/), not inferred. See
// docs/plans/physics_and_movement_engine_enhancement/PHASE_6_PLAN.md §1.9/§4.9.

const happyGhastMinVersion = "1.21.6"

// TestHappyGhastMounting verifies the basic mount/dismount cycle on a
// harnessed happy ghast.
func TestHappyGhastMounting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if !isVersionGreaterOrEqual(tt.MCVersion, happyGhastMinVersion) {
				t.Skipf("Happy ghast not available in version %s (requires %s+)", tt.MCVersion, happyGhastMinVersion)
			}

			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "GhastMountBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport GhastMountBot 100 5 100")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			ghastID, err := helper.SummonHappyGhast(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, ghastID))
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
		})
	}
}

// TestUnharnessedHappyGhastIsNotDriven verifies that mounting an unharnessed
// happy ghast (e.g. via /ride, since a normal interact would be refused
// server-side before this even matters — see PHASE_6_PLAN.md §4.5) routes to
// the passive-rider handler: no steering, no thrust, no accumulated
// velocity. Paired with TestHarnessedHappyGhastIsStillDriven below, the same
// way TestUnsaddledHorseIsNotDriven is paired with
// TestSaddledHorseIsStillDriven in saddle_gating_test.go.
//
// The observable is GetRidingDragMultiplier(): every driving handler sets it
// to a real per-tick value, while the passive-rider handler zeroes it.
func TestUnharnessedHappyGhastIsNotDriven(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if !isVersionGreaterOrEqual(tt.MCVersion, happyGhastMinVersion) {
				t.Skipf("Happy ghast not available in version %s (requires %s+)", tt.MCVersion, happyGhastMinVersion)
			}

			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "NoHarnessBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport NoHarnessBot 100 5 100")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			// The entity ID is unused here: the mount below goes through
			// /ride against an entity selector, not MountEntity(entityID).
			_, err = helper.SummonUnharnessedHappyGhast(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Vanilla refuses a normal right-click interaction on an
			// unharnessed happy ghast entirely (HappyGhastEntity.interactMob
			// only calls addPassenger when isWearingBodyArmor()), so
			// MountEntity's interact packet is expected to be a no-op here —
			// there is no SetPassengers response to wait for. This test
			// exercises the client-side defense-in-depth gate
			// (needsHarness/unharnessed in physics_executor.go), which
			// exists for edge cases like /ride, not the normal interact
			// path. Force the mount server-side via /ride so the gate has
			// something to actually gate.
			_, err = helper.Instance.RCON.Exec(ctx, "ride NoHarnessBot mount @e[type=minecraft:happy_ghast,limit=1]")
			require.NoError(t, err)
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

			require.NoError(t, helper.EnterManualMode())
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			inspector, ok := helper.GetRidingPhysicsInspector()
			require.True(t, ok, "agent should expose RidingPhysicsInspector")

			dragMultiplier := inspector.GetRidingDragMultiplier()
			velX, velZ := inspector.GetRidingVelocity()
			t.Logf("Unharnessed happy ghast after 2s of forward throttle: drag=%.4f vel=(%.4f, %.4f)", dragMultiplier, velX, velZ)

			assert.Zero(t, dragMultiplier,
				"passive-rider handler zeroes the drag multiplier; a non-zero value means a driving handler ran for an unharnessed happy ghast")
			assert.Zero(t, velX, "a passive rider accumulates no velocity")
			assert.Zero(t, velZ, "a passive rider accumulates no velocity")

			require.NoError(t, helper.ExitManualMode())
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
		})
	}
}

// TestHarnessedHappyGhastIsStillDriven is the control for
// TestUnharnessedHappyGhastIsNotDriven. Harness gating can only fail in two
// directions: failing to gate an unharnessed mount, or wrongly gating a
// harnessed one — the second silently disables flight entirely, so it gets
// its own test rather than relying on the flight tests below to notice.
func TestHarnessedHappyGhastIsStillDriven(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if !isVersionGreaterOrEqual(tt.MCVersion, happyGhastMinVersion) {
				t.Skipf("Happy ghast not available in version %s (requires %s+)", tt.MCVersion, happyGhastMinVersion)
			}

			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HarnessBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport HarnessBot 200 5 200")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			ghastID, err := helper.SummonHappyGhast(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, ghastID))
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))
			require.NoError(t, helper.EnableEntityAI(ctx, "minecraft:happy_ghast"))

			require.NoError(t, helper.EnterManualMode())
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			inspector, ok := helper.GetRidingPhysicsInspector()
			require.True(t, ok, "agent should expose RidingPhysicsInspector")

			dragMultiplier := inspector.GetRidingDragMultiplier()
			velX, velZ := inspector.GetRidingVelocity()

			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			displacement := GetDistance(finalPos.X, 0, finalPos.Z, startX, 0, startZ)
			t.Logf("Harnessed happy ghast after 2s of forward throttle: drag=%.4f vel=(%.4f, %.4f) displacement=%.2f",
				dragMultiplier, velX, velZ, displacement)

			assert.Positive(t, dragMultiplier,
				"a harnessed happy ghast must run the driving handler; zero drag means the harness gate wrongly demoted it to a passenger")
			assert.Greater(t, displacement, 0.5, "a harnessed happy ghast under forward throttle should move")

			require.NoError(t, helper.ExitManualMode())
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
		})
	}
}

// TestHappyGhastHoversWithZeroGravity is the single most direct end-to-end
// check of the decompiled-source finding that travelFlying's non-water/lava
// branch has no gravity term at all (PHASE_6_PLAN.md §1.9/§3, question 1):
// an idle, harnessed happy ghast under zero throttle should neither climb
// nor fall. Every other mount in this codebase either sits on the ground
// (no gravity question to ask) or actively fights gravity to stay airborne
// (nautilus in water); this is the only one that should just hang there.
func TestHappyGhastHoversWithZeroGravity(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if !isVersionGreaterOrEqual(tt.MCVersion, happyGhastMinVersion) {
				t.Skipf("Happy ghast not available in version %s (requires %s+)", tt.MCVersion, happyGhastMinVersion)
			}

			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "GhastHoverBot")
			defer cleanup()

			// Mount at ground level, the same as every other happy ghast
			// test — the agent is not flying until it's mounted, so
			// teleporting it into open air first would just make it
			// free-fall under ordinary walking gravity while summon/mount
			// setup is still in progress, and hit the ground long before
			// mounting completes.
			_, err := helper.Instance.RCON.Exec(ctx, "teleport GhastHoverBot 100 5 100")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			ghastID, err := helper.SummonHappyGhast(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, ghastID))
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))
			require.NoError(t, helper.EnableEntityAI(ctx, "minecraft:happy_ghast"))

			require.NoError(t, helper.EnterManualMode())

			// Climb well clear of the ground first, so the hover check below
			// (idle throttle) is unambiguous — a small residual sink close
			// to the ground could otherwise be masked by landing.
			helper.SetManualJump(true)
			time.Sleep(2 * time.Second)
			helper.SetManualJump(false)
			helper.SetManualThrottle(0, 0) // deliberately idle from here on

			// Releasing jump does not stop the climb immediately: with zero
			// gravity, the upward velocity accumulated while jump was held
			// only decays via the 0.91/tick drag, so the ghast keeps rising
			// for a bit before settling — itself further evidence there's no
			// gravity term pulling it back down. WaitForRidingVelocityZero
			// doesn't help here (GetRidingVelocity only reports horizontal
			// X/Z, and this climb is purely vertical), so wait out the decay
			// with a fixed sleep instead: at 0.91/tick, the residual velocity
			// is under 1% of its post-jump value within ~49 ticks (~2.5s).
			time.Sleep(3 * time.Second)

			hoverStart, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			t.Logf("Settled at y=%.2f before hover check", hoverStart.Y)
			require.Greater(t, hoverStart.Y, startY+2.0, "should have climbed clear of the ground before the hover check")

			const samples = 6
			minY, maxY := hoverStart.Y, hoverStart.Y
			for range samples {
				time.Sleep(500 * time.Millisecond)
				p, _ := helper.ManagedAgent.Agent.GetPositionSimple()
				if p.Y < minY {
					minY = p.Y
				}
				if p.Y > maxY {
					maxY = p.Y
				}
				t.Logf("y=%.3f", p.Y)
			}

			// Tolerance covers drag settling and ordinary interpolation, not
			// a real fall — a gravity-affected entity of this scale would
			// drop many blocks over 3 seconds, not fractions of one.
			assert.Less(t, maxY-minY, 1.0,
				"an idle happy ghast should hover, not fall — a large Y spread means gravity is being applied where the decompiled source has none")

			require.NoError(t, helper.ExitManualMode())
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
		})
	}
}

// TestHappyGhastFlightMovement verifies pitch-driven 3D flight end-to-end:
// forward throttle moves the agent horizontally, and jump moves it upward —
// the two input paths getControlledMovementInput combines
// (PHASE_6_PLAN.md §1.9/§4.7).
func TestHappyGhastFlightMovement(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			if !isVersionGreaterOrEqual(tt.MCVersion, happyGhastMinVersion) {
				t.Skipf("Happy ghast not available in version %s (requires %s+)", tt.MCVersion, happyGhastMinVersion)
			}

			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "GhastFlyBot")
			defer cleanup()

			// Mount at ground level — see TestHappyGhastHoversWithZeroGravity
			// for why teleporting into open air before mounting doesn't work.
			_, err := helper.Instance.RCON.Exec(ctx, "teleport GhastFlyBot 100 5 100")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			startX, startY, startZ := pos.X, pos.Y, pos.Z

			ghastID, err := helper.SummonHappyGhast(ctx, startX, startY, startZ, 0)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, helper.MountEntity(ctx, ghastID))
			require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))
			require.NoError(t, helper.EnableEntityAI(ctx, "minecraft:happy_ghast"))

			require.NoError(t, helper.EnterManualMode())

			// Forward flight: pitch level (0), full forward throttle.
			require.NoError(t, helper.ManagedAgent.Agent.LookAt(ctx, startX, startY, startZ+100))
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(3 * time.Second)

			afterForward, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			horizontalDisplacement := GetDistance(afterForward.X, 0, afterForward.Z, startX, 0, startZ)
			t.Logf("After 3s forward: pos=(%.2f,%.2f,%.2f) horizontalDisplacement=%.2f", afterForward.X, afterForward.Y, afterForward.Z, horizontalDisplacement)
			assert.Greater(t, horizontalDisplacement, 1.0, "forward throttle should move the ghast horizontally")

			// Ascend: stop forward throttle, hold jump.
			helper.SetManualThrottle(0, 0)
			helper.SetManualJump(true)
			time.Sleep(2 * time.Second)
			helper.SetManualJump(false)

			afterJump, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			t.Logf("After 2s jump: pos=(%.2f,%.2f,%.2f)", afterJump.X, afterJump.Y, afterJump.Z)
			assert.Greater(t, afterJump.Y, afterForward.Y+0.5, "holding jump should climb")

			require.NoError(t, helper.ExitManualMode())
			require.NoError(t, helper.DismountEntity())
			require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
		})
	}
}
