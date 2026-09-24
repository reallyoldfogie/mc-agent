package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// PigSuite is a version-parameterized suite for the saddled-pig tests: one
// shared flat-world server per version instead of one server per test
// function. Each method spawns its own uniquely-named agent in its own
// working area, so the pigs each test summons never share space.
type PigSuite struct {
	testingpkg.VersionWorldSuite
}

func TestPigSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &PigSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestPigMounting verifies that the agent can mount and dismount a saddled pig.
func (s *PigSuite) TestPigMounting() {
	t := s.T()
	leader, err := s.SpawnWorkingAreaAgent("PigMountBot", "pig_mounting")
	require.NoError(t, err, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon without AI so the pig cannot wander out of interact range
	// before we mount (pigs wander significantly within ~1-2s of
	// spawning); AI is restored after mounting to match
	// SummonHorse/SummonCamel.
	pigID, err := helper.SummonPig(ctx, x+2, y, z, WithNoAI())
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, pigID)
	require.NoError(t, err)

	// Wait for mounted
	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	err = helper.EnableEntityAI(ctx, "minecraft:pig")
	require.NoError(t, err)

	// Dismount
	err = helper.DismountEntity()
	require.NoError(t, err)

	// Wait for dismounted
	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// TestPigMovement verifies that a mounted pig responds to throttle inputs with proper velocity and steering.
func (s *PigSuite) TestPigMovement() {
	t := s.T()
	leader, err := s.SpawnWorkingAreaAgent("PigMoveBot", "pig_movement")
	require.NoError(t, err, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon and mount pig. NoAI keeps it from wandering out of interact
	// range before we mount; AI is restored after mounting since a NoAI pig
	// also can't respond to rider steering.
	pigID, err := helper.SummonPig(ctx, x, y, z, WithNoAI())
	require.NoError(t, err)

	// A saddled pig only responds to steering when the rider holds a
	// carrot on a stick (https://minecraft.wiki/w/Pig) — without it the
	// pig cannot be controlled at all, regardless of throttle input.
	_, _ = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("give %s carrot_on_a_stick", helper.AgentName))
	time.Sleep(500 * time.Millisecond)

	err = helper.MountEntity(ctx, pigID)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	err = helper.EnableEntityAI(ctx, "minecraft:pig")
	require.NoError(t, err)

	// Enter manual mode for controlled movement
	err = helper.EnterManualMode()
	require.NoError(t, err)

	// Move forward
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	// Stop
	helper.SetManualThrottle(0, 0)
	time.Sleep(1 * time.Second)

	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

	// Pig should move (base speed ~0.25)
	require.Greater(t, displacement, 1.0)

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// TestPigSpeedAttribute verifies pig speed handling and attribute reading.
func (s *PigSuite) TestPigSpeedAttribute() {
	t := s.T()
	leader, err := s.SpawnWorkingAreaAgent("PigSpeedBot", "pig_speed_attribute")
	require.NoError(t, err, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon and mount pig. NoAI keeps it from wandering out of interact
	// range before we mount; AI is restored after mounting since a NoAI pig
	// also can't respond to rider steering.
	pigID, err := helper.SummonPig(ctx, x, y, z, WithNoAI())
	require.NoError(t, err)

	// A saddled pig only responds to steering when the rider holds a
	// carrot on a stick (https://minecraft.wiki/w/Pig) — without it the
	// pig cannot be controlled at all, regardless of throttle input.
	_, _ = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("give %s carrot_on_a_stick", helper.AgentName))
	time.Sleep(500 * time.Millisecond)

	err = helper.MountEntity(ctx, pigID)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	err = helper.EnableEntityAI(ctx, "minecraft:pig")
	require.NoError(t, err)

	// Try to get pig's movement speed attribute
	pigAgent := helper.ManagedAgent.Agent
	getter, ok := pigAgent.(models.MountedEntityPositionGetter)
	if !ok {
		t.Skip("Agent does not implement MountedEntityPositionGetter")
	}

	speedAttr, found := getter.GetEntityAttribute(pigID, "generic.movement_speed")

	if found {
		t.Logf("Pig movement_speed attribute: %.4f", speedAttr)
		require.Greater(t, speedAttr, 0.1)
	} else {
		t.Logf("Pig does not have generic.movement_speed attribute")
	}

	// Verify movement
	err = helper.EnterManualMode()
	require.NoError(t, err)

	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	finalPos, _ := pigAgent.GetPositionSimple()
	movement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

	require.Greater(t, movement, 2.0)
	require.Less(t, movement, 15.0)

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// controllabilityBaselineEpsilon is the threshold below which a "without the
// controlling item" run is treated as AI wander/noise rather than genuine
// player-controlled movement, for both TestPigCarrotBoost and
// TestStriderControllability. A saddled pig/strider ignores rider input
// entirely without carrot_on_a_stick/warped_fungus_on_a_stick (vanilla
// getControllingPassenger() gates on it), so the "without" run should be
// near-zero, not a meaningful slower baseline for a speed ratio. Recorded
// verification data showed 0.15-2.23 blocks of AI-wander noise without the
// item across all 6 versions for both mobs, and 2.85+ blocks of real
// controlled movement with it - this sits comfortably between the two.
const controllabilityBaselineEpsilon = 3.0

// TestPigCarrotBoost verifies that a saddled pig is only controllable when the
// rider holds carrot_on_a_stick. In vanilla Java, getControllingPassenger()
// returns the player only when the player is holding the carrot AND the pig
// is saddled — without it, the pig ignores player movement input entirely.
// This is not a speed-boost comparison: the "without carrot" run is expected
// to be near-zero (residual AI wander, not slower steering), so this branches
// on controllabilityBaselineEpsilon rather than computing a ratio against a
// near-zero denominator.
func (s *PigSuite) TestPigCarrotBoost() {
	t := s.T()
	leader, err := s.SpawnWorkingAreaAgent("PigCarrotBot", "pig_carrot_boost")
	require.NoError(t, err, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x1, y1, z1 := pos.X, pos.Y, pos.Z

	// Test movement WITHOUT carrot. NoAI keeps the pig from wandering out of
	// interact range before we mount; AI is restored after mounting since a
	// NoAI pig also can't respond to rider steering.
	pigID1, err := helper.SummonPig(ctx, x1, y1, z1, WithNoAI())
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	err = helper.MountEntity(ctx, pigID1)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	err = helper.EnableEntityAI(ctx, "minecraft:pig")
	require.NoError(t, err)

	err = helper.EnterManualMode()
	require.NoError(t, err)

	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	pos1, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	movementWithoutCarrot := GetDistance(pos1.X, pos1.Y, pos1.Z, x1, y1, z1)

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Teleport to a fresh spot 100 blocks along Z, still inside this
	// method's own working area (working areas are 256 blocks apart).
	x2, y2, z2 := leader.Origin.X, leader.Origin.Y, leader.Origin.Z+100
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f %.1f %.1f", helper.AgentName, x2, y2, z2))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Test movement WITH carrot (same NoAI-then-restore sequence).
	pigID2, err := helper.SummonPig(ctx, x2, y2, z2, WithNoAI())
	require.NoError(t, err)

	// Give carrot and ensure it's in hand
	_, _ = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("give %s carrot_on_a_stick", helper.AgentName))
	time.Sleep(500 * time.Millisecond)

	time.Sleep(500 * time.Millisecond)

	err = helper.MountEntity(ctx, pigID2)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	err = helper.EnableEntityAI(ctx, "minecraft:pig")
	require.NoError(t, err)

	err = helper.EnterManualMode()
	require.NoError(t, err)

	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	pos2, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	movementWithCarrot := GetDistance(pos2.X, pos2.Y, pos2.Z, x2, y2, z2)

	t.Logf("Movement without carrot: %.2f blocks", movementWithoutCarrot)
	t.Logf("Movement with carrot: %.2f blocks", movementWithCarrot)

	if movementWithoutCarrot < controllabilityBaselineEpsilon {
		// Expected: without the carrot, the pig isn't controllable at
		// all, so "movement" here is just AI wander noise, not a
		// meaningful slower baseline.
		require.Greater(t, movementWithCarrot, 0.5, "pig should move when carrot is held")
	} else {
		// Unexpectedly high baseline (e.g. a version where the gate
		// isn't taking effect) — fall back to the ratio check so this
		// still catches a real regression instead of masking it
		// behind the epsilon branch.
		actualRatio := movementWithCarrot / movementWithoutCarrot
		t.Logf("Speed ratio (with/without): %.2f", actualRatio)
		require.Greater(t, actualRatio, 1.0, "carrot should increase movement speed")
	}

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}
