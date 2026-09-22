package testing

import (
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// entityAttributeGetter exposes GetEntityAttribute for tests, mirroring the
// models.MountedEntityPositionGetter type-assertion pattern already used
// elsewhere for methods not on models.Agent's main interface.
type entityAttributeGetter interface {
	GetEntityAttribute(entityID int32, attributeName string) (float64, bool)
}

// AttributeModifierFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for entity-attribute-tracking tests: one
// server per version, shared by every test method below, instead of the
// previous per-test-function StartServer/StopServer pattern. WorldGen and
// Difficulty are both left at VersionWorldSuite's own zero-value defaults
// (Flat/Peaceful), matching the pre-conversion TestSpeedEffectModifiesTrackedAttribute's
// own FlatWorldServerConfig() (which is DefaultServerConfig()'s Peaceful
// difficulty plus WorldGenFlat) - unlike EffectsFlatSuite (a different,
// Easy-difficulty suite this test could look similar to at a glance), no
// evidence was found that this test's own original Peaceful choice is safe
// to change, so it gets its own suite rather than being folded into that
// one. Currently one method - not worth converting alone by this session's
// own "single-function files gain nothing" rule, but the README's checklist
// asked for this file by name; a future single-function Peaceful/Flat file
// is a candidate to fold in here later, the same way flying/elytra grew
// from opportunistic multi-file consolidation.
type AttributeModifierFlatSuite struct {
	VersionWorldSuite
}

func TestAttributeModifierFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		return &AttributeModifierFlatSuite{}
	})
}

// TestSpeedEffectModifiesTrackedAttribute verifies PHASE_4_PLAN.md §2.1
// end-to-end against a real server: applying Speed to a tracked entity
// changes what GetEntityAttribute reports for generic.movement_speed. Before
// this fix, ClientboundEntityUpdateAttributes' per-attribute Modifiers array
// was parsed off the wire and discarded, so GetEntityAttribute would have
// kept returning the unmodified base value for as long as the effect was
// active, regardless of amplifier.
//
// Uses a second agent ("Other") as the tracked entity rather than a summoned
// horse, and does not read a pre-effect baseline: observed empirically (both
// for a horse and for a player) that vanilla does not broadcast
// generic.movement_speed via ClientboundEntityUpdateAttributes at all while
// it sits at its unmodified default - matching resolveMountMovementSpeed's
// pre-existing documented fallback in movement/riding_common.go for exactly
// this case. The attribute only appears once something (here, Speed)
// actually attaches a modifier to it. This is a real, separate, pre-existing
// protocol-bandwidth optimization, not a bug in this fix, so the test
// asserts against vanilla's well-known player base movement_speed (0.1,
// PlayerEntity's attribute default) instead of an empirically-read one.
//
// Both agents are spawned via the *NoCam variants: NearestPlayerInfo (used
// below to find Other's entity ID) resolves to whichever tracked
// player-type entity is nearest, with no special-casing to exclude a Cam
// companion - a real risk this session found already flagged for
// perception_nearest_player_test.go, not unique to this file. Querier and
// Other are spawned right next to each other (Other via SpawnAgentNearNoCam
// at Querier's own origin), so without disabling Cam, "nearest" could
// resolve to "OtherCam" instead of "Other".
func (s *AttributeModifierFlatSuite) TestSpeedEffectModifiesTrackedAttribute() {
	t := s.T()

	querier, err := s.SpawnWorkingAreaAgentNoCam("Querier", "attribute_modifier_querier")
	require.NoError(t, err, "spawn querier agent")

	other, err := s.SpawnAgentNearNoCam("Other", "attribute_modifier_other", querier.Origin, 2, 0)
	require.NoError(t, err, "spawn other agent")

	attrGetter, ok := querier.Agent.(entityAttributeGetter)
	require.True(t, ok, "agent should expose GetEntityAttribute")

	var otherEntityID int32
	require.Eventually(t, func() bool {
		info, found := querier.Agent.NearestPlayerInfo(s.Ctx, false)
		if !found {
			return false
		}
		otherEntityID = info.EntityID
		return true
	}, 10*time.Second, 200*time.Millisecond, "Querier should track Other's entity")

	_, err = s.Inst.RCON.Exec(s.Ctx, "effect give "+other.Name+" minecraft:speed 30 1")
	require.NoError(t, err, "apply speed II to Other")

	// Vanilla's player generic.movement_speed default is 0.1
	// (PlayerEntity's attribute registration). Speed II
	// (amplifier=1) is an ADD_MULTIPLIED_TOTAL modifier of
	// 0.2*(amplifier+1) = 0.4 (StatusEffects.java), so the final
	// value should be 0.1 * 1.4.
	const playerBaseMovementSpeed = 0.1
	wantBoosted := playerBaseMovementSpeed * 1.4
	var lastSeen float64
	var lastFound bool
	assert.Eventually(t, func() bool {
		v, found := attrGetter.GetEntityAttribute(otherEntityID, "generic.movement_speed")
		lastSeen, lastFound = v, found
		return found && math.Abs(v-wantBoosted) < 1e-6
	}, 5*time.Second, 100*time.Millisecond,
		"movement_speed should reflect Speed II's modifier once ClientboundEntityUpdateAttributes arrives")
	t.Logf("final movement_speed reading: found=%v value=%.6f (want %.6f)", lastFound, lastSeen, wantBoosted)

	_, _ = s.Inst.RCON.Exec(s.Ctx, "effect clear "+other.Name+" minecraft:speed")
}
