package testing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFlyingPhysicsClimbsAndDescends is an end-to-end smoke test for
// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4 step 3: confirms
// creative-mode flying actually produces sustained ascend/descend physics
// through the real movement executor and a live server, not just unit-level
// formula checks (physics/state_flying_test.go already covers those).
func TestFlyingPhysicsClimbsAndDescends(t *testing.T) {
	env := setupStandaloneTestWithModeAndBlockPlacement(t, "flying_physics_climb", GameModeCreative, false, "1.21.1", DifficultyEasy, false)
	defer env.Cancel()

	ctx := context.Background()

	require.Eventually(t, func() bool {
		_, ok := env.Agent.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	require.NoError(t, env.Agent.Agent.SetFlying(ctx, true), "creative mode should allow flying")

	require.NoError(t, env.Agent.Agent.EnterManualMode())
	defer func() { _ = env.Agent.Agent.ExitManualMode() }()

	startPos, ok := env.Agent.Agent.GetPositionSimple()
	require.True(t, ok)

	// Hold jump for 2 seconds: a normal single ground jump only ever gains
	// ~1.25 blocks (JumpVelocity's own arc) regardless of how long jump is
	// held, so sustained climbing well beyond that distance is a real,
	// live-server confirmation that flying's continuous ascend impulse
	// (not a one-shot jump) is actually driving the physics tick.
	require.NoError(t, env.Agent.Agent.SetManualJump(true))
	time.Sleep(2 * time.Second)
	require.NoError(t, env.Agent.Agent.SetManualJump(false))

	climbedPos, ok := env.Agent.Agent.GetPositionSimple()
	require.True(t, ok)
	climbGain := climbedPos.Y - startPos.Y
	t.Logf("climb: start=%.2f end=%.2f gain=%.2f", startPos.Y, climbedPos.Y, climbGain)
	assert.Greater(t, climbGain, 3.0, "holding jump while flying should climb well beyond a single ground jump's ~1.25 block arc")

	// Descend back down with sneak.
	require.NoError(t, env.Agent.Agent.SetManualSneak(true))
	time.Sleep(2 * time.Second)
	require.NoError(t, env.Agent.Agent.SetManualSneak(false))

	descendedPos, ok := env.Agent.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("descend: start=%.2f end=%.2f loss=%.2f", climbedPos.Y, descendedPos.Y, climbedPos.Y-descendedPos.Y)
	assert.Less(t, descendedPos.Y, climbedPos.Y-2.0, "holding sneak while flying should descend")

	require.NoError(t, env.Agent.Agent.SetFlying(ctx, false))
}
