package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise SetNoClip/Tick() end-to-end (PHASE 4.4 step 5:
// spectator noclip) — see decompiled Entity.move()'s `if (this.noClip) {
// setPosition(x+movement.x, ...) } else { ...collision... }`.

func TestState_NoClipPassesThroughWalls(t *testing.T) {
	world, shapes := createFlatWorld()

	// Solid wall directly in front of (Z+1 of) the player's starting
	// position - see TestState_CollisionDetection for this exact setup.
	world.SetBlock(0, 1, 1, BlockStone)

	blocked := NewState(shapes)
	blocked.SetPositionSimple(models.V3{X: 0, Y: 1, Z: 0})
	blocked.SetVelocity(models.V3{})

	noClipping := NewState(shapes)
	noClipping.SetPositionSimple(models.V3{X: 10, Y: 1, Z: 0})
	noClipping.SetVelocity(models.V3{})
	noClipping.SetNoClip(true)

	const ticks = 30
	for range ticks {
		require.NoError(t, blocked.Tick(Inputs{ThrottleZ: 1.0}, world))
		require.NoError(t, noClipping.Tick(Inputs{ThrottleZ: 1.0}, world))
	}

	t.Logf("blocked Z=%.3f, noClip Z=%.3f", blocked.Position().Z, noClipping.Position().Z)
	assert.Less(t, blocked.Position().Z, 1.0, "normal collision should stop the player at the wall")
	assert.Greater(t, noClipping.Position().Z, 1.0, "noclip should pass straight through the wall")
}

func TestState_NoClipForcesOnGroundFalse(t *testing.T) {
	// PlayerEntity.tick(): `if (this.isSpectator() || this.hasVehicle())
	// this.setOnGround(false)` - a spectator standing on solid ground
	// (e.g. immediately after toggling spectator while on a platform)
	// should never report onGround, since collision resolution (its only
	// source in this engine) is bypassed entirely.
	world, shapes := createFlatWorld()

	s := NewState(shapes)
	s.SetPositionSimple(models.V3{X: 0, Y: 1, Z: 0})
	s.SetVelocity(models.V3{})
	s.SetNoClip(true)

	require.NoError(t, s.Tick(Inputs{}, world))

	assert.False(t, s.OnGround(), "noclip should force onGround false even while positioned on solid ground")
}
