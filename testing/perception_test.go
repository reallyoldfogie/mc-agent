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

// PerceptionFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for tests where one agent queries another
// agent's tracked entity state (attributes, nearest-player resolution) - one
// server per version, shared by every test method below, instead of the
// previous per-test-function StartServer/StopServer pattern. Named after the
// existing "perception" terminology this codebase already uses for this
// category (see testing/vehicles/perception_test.go, and
// perception_nearest_player_test.go's own doc comment referencing it) -
// started as attribute_modifier_test.go's own single-method
// AttributeModifierFlatSuite, renamed once perception_nearest_player_test.go
// turned out to need the exact same config and two-agent NoCam pattern (see
// docs/plans/integration-test-shared-server/20-phase1-attribute-modifier-conversion.md
// and 21-phase1-perception-conversion.md).
//
// WorldGen and Difficulty are both left at VersionWorldSuite's own
// zero-value defaults (Flat/Peaceful), matching both pre-conversion
// functions' own FlatWorldServerConfig() (DefaultServerConfig()'s Peaceful
// difficulty plus WorldGenFlat) - unlike EffectsFlatSuite (a different,
// Easy-difficulty suite these tests could look similar to at a glance), no
// evidence was found that switching to Easy is safe, so this suite keeps
// its own config rather than folding into that one.
type PerceptionFlatSuite struct {
	VersionWorldSuite
}

func TestPerceptionFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		return &PerceptionFlatSuite{}
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
// companion (confirmed: by default a Cam companion is a normal survival
// player, not spectator - it only switches via EnableCamFollow, which
// defaults false - so it's fully visible/trackable and a real collision
// risk). Querier and Other are spawned right next to each other (Other via
// SpawnAgentNearNoCam at Querier's own origin), so without disabling Cam,
// "nearest" could resolve to "OtherCam" instead of "Other".
func (s *PerceptionFlatSuite) TestSpeedEffectModifiesTrackedAttribute() {
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

// TestNearestPlayerInfoHonorsBlindness verifies PHASE_4_PLAN.md §4.13
// end-to-end against a real server for the NearestPlayerInfo call site
// (backing the real `follow` with no name and `fireBowAt nearest` commands,
// and the startTracking tick loop) - not just FindRideableEntitiesNear,
// which testing/vehicles/perception_test.go already covers. Calls
// NearestPlayerInfo directly rather than going through the chat-command
// layer: the no-args `follow`/`fireBowAt nearest` commands only report their
// outcome via outgoing chat messages, and no chat-capture test utility
// exists in this package, so asserting on the exposed method (the exact
// thing every real caller invokes) gives equivalent coverage without
// needing to build one. Equivalent to the pre-Phase-1
// TestNearestPlayerInfoHonorsBlindness (perception_nearest_player_test.go).
//
// Both agents spawned NoCam, same rationale as
// TestSpeedEffectModifiesTrackedAttribute above. Other spawns right at
// Querier's own working-area origin (offset 0,0) rather than the
// pre-conversion test's "both happen to spawn at the same natural world
// spawn point" - the shared-server equivalent of the same starting
// condition, since a working area's origin IS this suite's agents' own
// natural spawn point (see SpawnWorkingAreaAgent's doc comment).
//
// docs/plans/integration-test-shared-server/00-plan.md's own risk note for
// this file: a test assuming "the nearest player" needs its working-area
// separation to exceed the server's reduced view distance, or a
// neighboring working area's agent could become visible and corrupt the
// assertion. The 20-block distance used here (chosen to clear Blindness's
// 5-block and Darkness's 15-block vision caps) is comfortably inside
// SharedServerConfig's 6-chunk (96-block) VIEW_DISTANCE, but nowhere near
// NextWorkingAreaOffset's 256-block separation - no neighboring working
// area's agent should ever become visible to Querier.
func (s *PerceptionFlatSuite) TestNearestPlayerInfoHonorsBlindness() {
	t := s.T()

	querier, err := s.SpawnWorkingAreaAgentNoCam("BlindQuerier", "perception_blind_querier")
	require.NoError(t, err, "spawn querier agent")

	other, err := s.SpawnAgentNearNoCam("BlindOther", "perception_blind_other", querier.Origin, 0, 0)
	require.NoError(t, err, "spawn other agent")

	// 20 blocks: outside Blindness's 5-block and Darkness's 15-block vision
	// caps alike, well within an unrestricted search.
	targetX := querier.Origin.X + 20
	_, err = s.Inst.RCON.Teleport(s.Ctx, other.Name, targetX, querier.Origin.Y, querier.Origin.Z).Exec(s.Ctx)
	require.NoError(t, err, "teleport Other 20 blocks from Querier")

	// Poll Querier's own *client-side* tracked distance to Other
	// (honorPerceptionEffects=false, so it reflects raw tracking with no cap
	// applied) rather than trusting the RCON teleport alone. The RCON
	// command lands on the server immediately, but Querier's own
	// entity-position cache for Other only updates once its connection
	// separately receives and processes the resulting move/teleport packet
	// - a second, independent propagation delay that can lag well behind
	// the server-side position. Confirmed empirically (pre-conversion):
	// even after verifying the teleport server-side via RCON, Querier's own
	// NearestPlayerInfo was still observed reporting Other at distances as
	// low as ~3-5 blocks (its stale pre-teleport tracked position), which
	// is *why* a subsequent Blindness check could fail non-deterministically
	// - not because the 5-block vision cap or effect handling were wrong,
	// but because the test was reading a distance that hadn't caught up
	// yet, some genuinely under 5 blocks by coincidence.
	require.Eventually(t, func() bool {
		info, ok := querier.Agent.NearestPlayerInfo(s.Ctx, false)
		return ok && info.Distance > 15
	}, 10*time.Second, 200*time.Millisecond, "Querier's own tracked distance to Other should reflect the teleport")

	_, found := querier.Agent.NearestPlayerInfo(s.Ctx, true)
	assert.True(t, found, "should find the other player when no perception-restricting effect is active")

	ignoringEffects, found := querier.Agent.NearestPlayerInfo(s.Ctx, false)
	assert.True(t, found, "honorPerceptionEffects=false should always find the other player regardless of effect state")
	_ = ignoringEffects

	_, err = s.Inst.RCON.Exec(s.Ctx, "effect give "+querier.Name+" minecraft:blindness 30 0")
	require.NoError(t, err, "apply blindness effect")

	// Poll rather than sleep-then-check-once: ClientboundEntityEffect
	// arrival/processing time is not bounded by a fixed constant - it is
	// visibly sensitive to concurrent Docker load, so any fixed sleep is a
	// flaky race by construction regardless of how generous it is.
	assert.Eventually(t, func() bool {
		_, found := querier.Agent.NearestPlayerInfo(s.Ctx, true)
		return !found
	}, 5*time.Second, 200*time.Millisecond,
		"should NOT find the other player 20 blocks away while blind (5-block vision cap)")

	stillIgnoringEffects, found := querier.Agent.NearestPlayerInfo(s.Ctx, false)
	assert.True(t, found, "honorPerceptionEffects=false should still find the other player even while blind")
	_ = stillIgnoringEffects

	_, _ = s.Inst.RCON.Exec(s.Ctx, "effect clear "+querier.Name+" minecraft:blindness")
}
