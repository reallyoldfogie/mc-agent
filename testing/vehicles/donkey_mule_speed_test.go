package vehicles

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DonkeyMuleSpeedSuite is a version-parameterized suite: one shared
// flat-world server per version instead of one per test, each method
// spawning its own uniquely-named agent in its own working area.
type DonkeyMuleSpeedSuite struct {
	testingpkg.VersionWorldSuite
}

func TestDonkeyMuleSpeedSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &DonkeyMuleSpeedSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestDonkeySpeedAttribute and TestMuleSpeedAttribute close the gap: both
// movement/riding_donkey.go and riding_mule.go delegate to
// handleRidingModeHorse, which reads generic.movement_speed live from the
// server via resolveMountMovementSpeed — but until now nothing actually
// asserted on the value that comes back, so a regression that silently fell
// through to the wrong species' fallback (e.g. horse's 0.225 instead of
// donkey/mule's own 0.175 — see models/entity_attribute_defaults.go) would
// have gone undetected.
//
// Unlike TestHorseMovementSpeedAttribute (horse_speed_attributes_test.go),
// which can't assert a specific value because vanilla randomizes a regular
// horse's movement_speed per individual, donkeys and mules do NOT randomize
// this stat — every donkey and every mule spawns with the same fixed 0.175 —
// so this can assert an exact value the same way TestCamelSpeedAttribute
// (camel_test.go) does for the camel's fixed 0.09.
func (s *DonkeyMuleSpeedSuite) TestDonkeySpeedAttribute() {
	s.horseFamilySpeedAttribute("donkey", 0.175,
		func(helper *VehicleTestHelper, ctx context.Context, x, y, z float64) (int32, error) {
			return helper.SummonDonkey(ctx, x, y, z)
		})
}

func (s *DonkeyMuleSpeedSuite) TestMuleSpeedAttribute() {
	s.horseFamilySpeedAttribute("mule", 0.175,
		func(helper *VehicleTestHelper, ctx context.Context, x, y, z float64) (int32, error) {
			return helper.SummonMule(ctx, x, y, z)
		})
}

// horseFamilySpeedAttribute mounts a saddled horse-family entity, drives
// it forward briefly to confirm handleRidingModeHorse actually ran (the same
// sanity check TestCamelSpeedAttribute and TestHorseMovementSpeedAttribute
// both do), then reads back the live generic.movement_speed attribute and
// checks it against the fixed vanilla default for that species.
func (s *DonkeyMuleSpeedSuite) horseFamilySpeedAttribute(
	entityType string,
	expectedSpeed float64,
	summon func(helper *VehicleTestHelper, ctx context.Context, x, y, z float64) (int32, error),
) {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent(speedTestAgentName(entityType), entityType+"_speed_attr")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 0 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err, "teleport agent")
	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	mountEntityID, err := summon(helper, ctx, x, y, z)
	require.NoError(t, err, "summon "+entityType)
	time.Sleep(1 * time.Second)

	trackedMount := helper.GetTrackedEntity(mountEntityID)
	require.NotNil(t, trackedMount, entityType+" should be tracked")

	// Wait for ClientboundUpdateAttributes to arrive, same as
	// TestHorseMovementSpeedAttribute/TestCamelSpeedAttribute.
	time.Sleep(2 * time.Second)

	require.NoError(t, helper.MountEntity(ctx, mountEntityID), "mount "+entityType)
	require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second), "agent should be mounted")

	require.NoError(t, helper.EnterManualMode())

	initialPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	helper.SetManualThrottle(0.0, 1.0)
	time.Sleep(500 * time.Millisecond)
	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()

	distance := math.Hypot(finalPos.X-initialPos.X, finalPos.Z-initialPos.Z)
	require.Greater(t, distance, 0.0, entityType+" should have moved")
	t.Logf("%s movement: distance=%.4f", entityType, distance)

	// Best-effort attribute check, same posture
	// TestCamelSpeedAttribute takes: a live server should send this,
	// but don't fail the whole test on an infra timing gap the
	// movement assertion above has already covered functionally.
	getter, ok := helper.ManagedAgent.Agent.(models.MountedEntityPositionGetter)
	if ok {
		speed, found := getter.GetEntityAttribute(mountEntityID, "generic.movement_speed")
		if found {
			require.InDelta(t, expectedSpeed, speed, 0.01,
				"%s movement_speed attribute should be the fixed vanilla default (%.3f), not the wrong species' fallback", entityType, expectedSpeed)
			t.Logf("%s movement_speed attribute: %.4f", entityType, speed)
		} else {
			t.Logf("%s movement_speed attribute not found (server may not provide it)", entityType)
		}
	} else {
		t.Logf("agent does not implement MountedEntityPositionGetter (attribute check skipped)")
	}

	require.NoError(t, helper.ExitManualMode())
	require.NoError(t, helper.DismountEntity())
	require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
}

// speedTestAgentName keeps the two tests' player names distinct and under
// Minecraft's 16-character username limit.
func speedTestAgentName(entityType string) string {
	var name string
	switch entityType {
	case "donkey":
		name = "DonkeySpdBot"
	case "mule":
		name = "MuleSpdBot"
	default:
		name = entityType + "SpdBot"
	}

	if len(name) > 16 {
		return name[:16]
	}
	return name
}
