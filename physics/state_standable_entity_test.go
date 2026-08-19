package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// These tests cover the walking player's ability to stand on a "standable"
// entity — a happy ghast while staying still, in practice — added for
// PHASE_6_PLAN.md §5/§6.4. See computeCollisionYXZWithStandableEntities and
// getStandableEntityBoxes in state.go.

func newTestPlayerState() *state {
	sp := newMockShapeProvider()
	return &state{
		width:         PlayerWidth,
		height:        PlayerHeight,
		shapeProvider: sp,
	}
}

func TestComputeCollisionYXZWithStandableEntities_SupportsFromAbove(t *testing.T) {
	w := newMockWorld()
	// A 4x4x4 "happy ghast" box resting with its top surface at y=10,
	// centered under the player.
	w.AddEntity(models.EntityBounds{
		EntityID:  1,
		AABB:      models.NewAABB(-2, 6, -2, 2, 10, 2),
		Standable: true,
	})

	s := newTestPlayerState()
	// 2 blocks of clear air above the ghast's top surface (Y=10).
	s.Pos = models.V3{X: 0, Y: 12, Z: 0}
	playerBB := s.getAABBUnsafe()

	// Falling 3 blocks' worth of velocity — enough to pass clean through the
	// ghast's top surface if nothing clamps it, since only 2 blocks of gap
	// actually exist (playerBB.Y.Min=12 down to the ghast's top at Y=10).
	vel := models.V3{X: 0, Y: -3, Z: 0}
	_, outVel := s.computeCollisionYXZWithStandableEntities(playerBB, vel, w)

	const wantClamped = -2.0 // exactly the gap: 12 - 10
	if outVel.Y != wantClamped {
		t.Errorf("expected downward velocity to be clamped to %v (landing exactly on the standable entity), got %v", wantClamped, outVel.Y)
	}
}

func TestComputeCollisionYXZWithStandableEntities_NonStandableDoesNotSupport(t *testing.T) {
	w := newMockWorld()
	// Identical box and position to the test above, but Standable=false —
	// the negative control proving the flag actually gates the behavior,
	// not just "any nearby entity."
	w.AddEntity(models.EntityBounds{
		EntityID:  1,
		AABB:      models.NewAABB(-2, 6, -2, 2, 10, 2),
		Standable: false,
	})

	s := newTestPlayerState()
	s.Pos = models.V3{X: 0, Y: 12, Z: 0}
	playerBB := s.getAABBUnsafe()

	vel := models.V3{X: 0, Y: -3, Z: 0}
	_, outVel := s.computeCollisionYXZWithStandableEntities(playerBB, vel, w)

	if outVel.Y != -3 {
		t.Errorf("expected a non-standable entity to not affect vertical velocity, got %v (want -3, unclamped)", outVel.Y)
	}
}

func TestComputeCollisionYXZWithStandableEntities_NoEntityFallsFreely(t *testing.T) {
	w := newMockWorld() // no entities at all
	s := newTestPlayerState()
	s.Pos = models.V3{X: 0, Y: 12, Z: 0}
	playerBB := s.getAABBUnsafe()

	vel := models.V3{X: 0, Y: -3, Z: 0}
	_, outVel := s.computeCollisionYXZWithStandableEntities(playerBB, vel, w)

	if outVel.Y != -3 {
		t.Errorf("expected free fall with no entities in range, got %v (want -3, unclamped)", outVel.Y)
	}
}

func TestGetStandableEntityBoxes_FiltersOutNonStandable(t *testing.T) {
	w := newMockWorld()
	w.AddEntity(models.EntityBounds{EntityID: 1, AABB: models.NewAABB(0, 0, 0, 1, 1, 1), Standable: true})
	w.AddEntity(models.EntityBounds{EntityID: 2, AABB: models.NewAABB(5, 5, 5, 6, 6, 6), Standable: false})

	s := newTestPlayerState()
	boxes := s.getStandableEntityBoxes(models.NewAABB(-10, -10, -10, 10, 10, 10), w)

	if len(boxes) != 1 {
		t.Fatalf("expected exactly 1 standable box, got %d", len(boxes))
	}
	if boxes[0].X.Max != 1 {
		t.Errorf("expected the standable box (entity 1), got a box with X.Max=%v", boxes[0].X.Max)
	}
}

func TestHandleEntityCollisions_SkipsStandableEntities(t *testing.T) {
	w := newMockWorld()
	// Positioned to produce a real horizontal push if handleEntityCollisions
	// processed it — chosen close enough to be within the separation
	// formula's effective range (chebyshev < 1) but not exactly coincident
	// (chebyshev >= 0.01, or the "same position" special case would also
	// legitimately produce zero push and the test wouldn't distinguish the
	// two code paths).
	w.AddEntity(models.EntityBounds{
		EntityID:  1,
		AABB:      models.NewAABB(0.3, 0, 0, 1.3, 2, 1),
		Standable: true,
	})

	s := newTestPlayerState()
	s.Pos = models.V3{X: 0, Y: 0, Z: 0}
	playerBB := s.getAABBUnsafe()

	vel := models.V3{X: 0, Y: 0, Z: 0}
	s.handleEntityCollisions(playerBB, &vel, w)

	if vel.X != 0 || vel.Z != 0 {
		t.Errorf("expected a Standable entity to be skipped by the soft push-away path, got velocity (%v, %v)", vel.X, vel.Z)
	}
}

func TestHandleEntityCollisions_StillPushesNonStandableEntities(t *testing.T) {
	// Control for the test above: the same overlapping geometry, but
	// Standable=false, must still produce the ordinary push — proving the
	// skip is specific to Standable, not a general regression.
	w := newMockWorld()
	w.AddEntity(models.EntityBounds{
		EntityID:  1,
		AABB:      models.NewAABB(0.3, 0, 0, 1.3, 2, 1),
		Standable: false,
	})

	s := newTestPlayerState()
	s.Pos = models.V3{X: 0, Y: 0, Z: 0}
	playerBB := s.getAABBUnsafe()

	vel := models.V3{X: 0, Y: 0, Z: 0}
	s.handleEntityCollisions(playerBB, &vel, w)

	if vel.X == 0 && vel.Z == 0 {
		t.Errorf("expected a non-Standable overlapping entity to still produce a separation push, got zero velocity")
	}
}
