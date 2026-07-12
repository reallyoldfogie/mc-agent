package movement

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestResolveEntityCollision_NoDeadlockUnderMountedLock is a regression guard for
// the riding-tick self-deadlock.
//
// Every handleRidingMode* handler holds pe.mountedEntityMu (a non-reentrant
// sync.RWMutex) for its whole read-compute-write cycle and calls
// resolveEntityCollision while holding it. resolveEntityCollision used to
// re-acquire that same write lock in its step-up branch, so a mounted entity
// stepping onto a 1-block ledge self-deadlocked the physics tick goroutine —
// which in turn hung agent.Close()'s WaitGroup and timed out the whole
// testing/vehicles package.
//
// This test reproduces the exact locking pattern: a single goroutine takes the
// write lock and then runs resolveEntityCollision through a step-up scenario. If
// a lock is ever reintroduced anywhere on that path, the call blocks forever, so
// the test fails on its deadline instead of hanging the suite. At least one
// scenario must actually take the step-up branch (the entity climbs onto the
// ledge) so the exact former lock site is exercised; that is asserted at the end.
func TestResolveEntityCollision_NoDeadlockUnderMountedLock(t *testing.T) {
	exec := createTestPhysicsExecutor()

	world, ok := exec.world.(*MockWorld)
	require.True(t, ok, "expected the test executor to use *MockWorld")

	// The floor top is Y=64 (createTestPhysicsExecutor fills Y=0..63). Build a
	// 1-block-tall ledge directly ahead (+Z) whose top surface is Y=65, mirroring
	// the "step up" terrain the horse cliff integration test creates.
	for x := -4; x <= 4; x++ {
		for z := 2; z <= 8; z++ {
			world.SetBlock(x, 64, z, 1) // stone
		}
	}

	// Horse hitbox (1.4 wide x 1.6 tall). Probe several forward approaches into
	// the ledge; each runs the collision resolver (including the step-up path)
	// while the mounted-state write lock is held.
	type scenario struct {
		name string
		pos  models.V3
		vel  models.V3
	}
	scenarios := []scenario{
		{"approach_far", models.V3{X: 0.5, Y: 64, Z: 0.5}, models.V3{X: 0, Y: -0.08, Z: 0.6}},
		{"approach_near", models.V3{X: 0.5, Y: 64, Z: 1.0}, models.V3{X: 0, Y: -0.08, Z: 0.6}},
		{"approach_slow", models.V3{X: 0.5, Y: 64, Z: 1.0}, models.V3{X: 0, Y: -0.08, Z: 0.2}},
		{"approach_fast", models.V3{X: 0.5, Y: 64, Z: 0.8}, models.V3{X: 0, Y: -0.08, Z: 1.0}},
		{"approach_diag", models.V3{X: 0.3, Y: 64, Z: 1.0}, models.V3{X: 0.2, Y: -0.08, Z: 0.6}},
	}

	anyClimbed := false
	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			type collisionResult struct {
				newPos   models.V3
				onGround bool
			}
			done := make(chan collisionResult, 1)

			go func() {
				// Simulate a riding handler: hold the mounted-state write lock for
				// the entire compute cycle, exactly as handleRidingMode* does, then
				// run the collision resolution that used to re-lock.
				exec.mountedEntityMu.Lock()
				defer exec.mountedEntityMu.Unlock()

				newPos, _, onGround, _, _ := resolveEntityCollision(exec, sc.pos, sc.vel, 1.4, 1.6)
				done <- collisionResult{newPos: newPos, onGround: onGround}
			}()

			select {
			case res := <-done:
				climbed := res.newPos.Y > sc.pos.Y+0.5
				if climbed {
					anyClimbed = true
				}
				t.Logf("newPos=(%.3f, %.3f, %.3f) onGround=%t climbed=%t",
					res.newPos.X, res.newPos.Y, res.newPos.Z, res.onGround, climbed)
				require.GreaterOrEqual(t, res.newPos.Y, sc.pos.Y,
					"entity should rest on the floor/ledge, not sink")
			case <-time.After(5 * time.Second):
				t.Fatal("resolveEntityCollision deadlocked while mountedEntityMu was held: " +
					"a reentrant lock on the riding collision path has been reintroduced")
			}
		})
	}

	// Guard the guard: at least one scenario must take the step-up branch (climb
	// onto the ledge), which is where the reentrant lock used to live. If none
	// climb, this test would no longer cover the former deadlock site and the
	// scenarios above need to be adjusted.
	require.True(t, anyClimbed,
		"expected at least one scenario to exercise the step-up branch (entity climbing the ledge)")
}
