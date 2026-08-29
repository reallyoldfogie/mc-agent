package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestElytraFlareLanding verifies the real "flare" landing technique elytra
// pilots use to avoid fall damage without a long, gradual glide down: come
// in close to the ground at speed, then look sharply up (no rocket boost
// involved) to bleed horizontal speed into a brief climb, and drop the
// remaining short distance to the ground gently.
//
// This works because of GlidingVelocity's "pitching up trades forward
// speed for lift" term (physics/elytra.go): looking up sharply converts a
// large fraction of horizontal speed into upward velocity in a single tick,
// which is exactly what real vanilla's LivingEntity.limitFallDistance()
// needs to avoid fall damage - it caps accumulated fall distance to 1 block
// every tick the entity's vertical velocity is above -0.5, but only while
// gliding (LivingEntity.tickGliding() is what calls it). A steep, sustained
// dive straight into the ground never gets that chance; flaring does.
// Fall damage itself is server-authoritative (this codebase's own
// physics.State doesn't implement limitFallDistance()'s cap at all, since
// client-side fallDistance here is only used for pathfinding drop-cost
// estimation, not damage) - checked via a real RCON health read before and
// after, not inferred from anything client-side.
func TestElytraFlareLanding(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "elytra_flare_landing", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-40, clearY+1, clearZ-10, clearX+40, clearY+60, clearZ+150), "clear flare-approach airspace")
			time.Sleep(300 * time.Millisecond)

			startHealth, err := GetPlayerHealth(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get starting health")
			t.Logf("starting health: %.1f", startHealth)

			// Equip the elytra via a real shift-click (see
			// TestElytraGlideSlowsDescentAndAddsForwardMotion's doc comment
			// for why not RCON).
			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:elytra 1", env.BotName))
			require.NoError(t, err, "give elytra")

			elytraSlot, elytraSlotData, ok := waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "elytra never appeared in bot's main/hotbar inventory")

			require.NoError(t, env.Agent.Agent.ShiftClickSlot(int16(elytraSlot), slotToItemStack(elytraSlotData)), "shift-click elytra to equip it")
			require.True(t, waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool {
				return s.Count > 0
			}, 10*time.Second), "elytra never landed in the chest armor slot")
			t.Log("elytra equipped via shift-click")

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:firework_rocket 5", env.BotName))
			require.NoError(t, err, "give firework rockets")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "firework rockets never appeared in bot's main/hotbar inventory")

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			const climbPitch = -80.0
			const steerInterval = 100 * time.Millisecond
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))

			startPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			t.Logf("start position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			// Double-jump takeoff (see TestElytraDoubleJumpFireworkRocketTakeoff).
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(150 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.UseFireworkRocket(), "use firework rocket to launch")

			// A brief, single-boost climb (no re-boosting) - just enough
			// altitude for a controlled approach, not a sustained ascent.
			// Earlier versions of this test used a longer, re-boosted climb
			// (matching TestElytraDoubleJumpFireworkRocketTakeoff's
			// technique) and found the resulting momentum was still huge
			// once it cascaded through leveling off and diving: flaring
			// converted it into a climb straight back up past 200 blocks
			// instead of a gentle settle - a real, dramatic "balloon" this
			// physics genuinely produces at high energy, just not what a
			// landing-technique test wants to demonstrate.
			const climbDuration = 800 * time.Millisecond
			climbDeadline := time.Now().Add(climbDuration)
			for time.Now().Before(climbDeadline) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))
				time.Sleep(steerInterval)
			}

			apexPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			t.Logf("apex before diving approach: (%.2f, %.2f, %.2f)", apexPos.X, apexPos.Y, apexPos.Z)

			// Coast level for a moment before diving. Momentum doesn't
			// reverse instantly: an earlier version of this test switched
			// straight from climb pitch to dive pitch and found the agent
			// was still ascending (vertical velocity still positive from
			// the climb) for the entire "dive" window that followed - it
			// actually gained altitude instead of losing it, confirmed by
			// the logged apex/flare positions, not assumed. A brief level
			// coast lets gravity bleed off the residual climb momentum
			// first, so the dive that follows is a real dive.
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, 0))
			time.Sleep(600 * time.Millisecond)

			// Now dive - steep enough to actually lose altitude quickly. A
			// shallow dive (pitch 20) was tried too and barely descends at
			// all: gliding's horizontal-ease term keeps building forward
			// speed for as long as the dive continues, and a shallow dive
			// took long enough (100+ blocks of travel to lose just 15
			// blocks of altitude) that accumulated speed was enormous by
			// the time it reached the flare trigger - the same "balloon on
			// flare" problem, just from a different cause (dive duration
			// instead of climb energy). A steep dive converts altitude to
			// speed quickly instead of slowly, bounding total energy
			// buildup.
			const divePitch = 40.0
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, divePitch))

			// Approach: keep diving until close to the ground, then flare -
			// look sharply up, no further rocket use - and ride it down.
			// Also bounded by a maximum dive duration as a second safety
			// net against excess speed buildup, independent of altitude.
			//
			// flareAltitude needs real margin, not just "close to the
			// ground": an earlier version of this test flared at 6 blocks
			// (or wherever maxDiveDuration cut the dive short, sometimes as
			// low as ~3.5) and took real, sometimes lethal fall damage on
			// most versions - confirmed via a live 6-version run, not
			// assumed. Vanilla's LivingEntity.limitFallDistance() only caps
			// accumulated fall distance while vertical velocity is above
			// -0.5, and that condition takes a tick or two of the flare's
			// own upward conversion to actually reach after diving in
			// fast - if the ground arrives first, no cap ever applies and
			// the fall damage already built up during the dive lands in
			// full, real crashes included. Flaring with real altitude to
			// spare gives that conversion time to finish before impact.
			const flareAltitude = 10.0
			const groundY = 0.0
			const maxDiveDuration = 1 * time.Second

			flared := false
			var preFlarePos, prePreFlarePos models.V3
			diveStart := time.Now()
			approachDeadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(approachDeadline) {
				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)

				if !flared && (pos.Y-groundY <= flareAltitude || time.Since(diveStart) >= maxDiveDuration) {
					prePreFlarePos = preFlarePos
					preFlarePos = pos
					flared = true
					t.Logf("flaring at (%.2f, %.2f, %.2f) after %v of diving", pos.X, pos.Y, pos.Z, time.Since(diveStart))
				} else if !flared {
					prePreFlarePos = preFlarePos
					preFlarePos = pos
				}

				require.NoError(t, env.Agent.Agent.SetManualRotation(0, divePitch))
				time.Sleep(steerInterval)

				if flared {
					break
				}
			}
			require.True(t, flared, "should have descended low enough to trigger the flare within the approach window")

			// The flare itself: hold nose-up until velocity confirms the
			// descent has actually been arrested and started to climb again
			// - not for a blind fixed duration. GlidingVelocity's "pitching
			// up trades speed for lift" term scales with current horizontal
			// speed (physics/elytra.go), so a fixed wall-clock pulse
			// produces a wildly different amount of lift depending on how
			// fast the dive happened to be going when it triggered: an
			// earlier version of this test used a fixed 150ms pulse and,
			// watched back in replay, barely looked like a flare at all -
			// just an ordinary glide down. The term also has no velocity
			// gate at all (unlike the dive term, which only fires while
			// already falling), so holding a steep up-look continuously all
			// the way to the ground (a different earlier version's bug)
			// compounds the lift impulse tick after tick and rockets the
			// agent hundreds of blocks back up instead of landing -
			// confirmed live, not assumed. Holding only until vertical
			// velocity crosses a small positive threshold scales the pulse
			// to whatever energy is really there, producing a visible arc
			// every time, and is still bounded: the instant pitch goes
			// non-negative below (settlePitch), the climb term stops firing
			// entirely (it only fires while pitch < 0) - so there's no risk
			// of the earlier balloon bug recurring, since that came from
			// holding across the whole remaining descent, not from crossing
			// this threshold once and then leveling off.
			const pulseFlarePitch = -60.0
			const flarePopVelocityThreshold = 0.8 // blocks/tick, upward
			const settlePitch = 10.0
			const maxFlareDuration = 1500 * time.Millisecond // safety net if velocity is ever unavailable

			preFlareVX, preFlareVY, preFlareVZ, preFlareVelOK := env.Agent.Agent.GetVelocity()
			t.Logf("velocity at flare trigger: (%.3f, %.3f, %.3f) ok=%v", preFlareVX, preFlareVY, preFlareVZ, preFlareVelOK)

			// diveBottomY/flareApexY track the true low and high points of
			// the maneuver, not just the position at the moment the flare
			// was decided: the dive keeps sinking for a beat after that
			// decision while the pop pitch fights the residual downward
			// velocity, confirmed live - the real bottom came in noticeably
			// lower than preFlarePos.Y (13.55 vs 19.92 in one run), so
			// comparing the eventual apex against preFlarePos.Y understated
			// (in one case, entirely erased) the climb that actually
			// happened.
			flareStart := time.Now()
			diveBottomY := preFlarePos.Y
			flareApexY := preFlarePos.Y
			for {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, pulseFlarePitch))
				time.Sleep(steerInterval)

				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)
				if pos.Y < diveBottomY {
					diveBottomY = pos.Y
				}
				if pos.Y > flareApexY {
					flareApexY = pos.Y
				}

				_, vy, _, velOK := env.Agent.Agent.GetVelocity()
				if velOK && vy >= flarePopVelocityThreshold {
					t.Logf("flare pop detected: vy=%.3f after %v", vy, time.Since(flareStart))
					break
				}
				if time.Since(flareStart) >= maxFlareDuration {
					t.Logf("flare safety timeout hit after %v (last vy=%.3f, velOK=%v)", time.Since(flareStart), vy, velOK)
					break
				}
			}
			t.Logf("true dive bottom: %.2f (vs. flare-trigger position %.2f)", diveBottomY, preFlarePos.Y)

			// Coast to the true apex: crossing flarePopVelocityThreshold
			// only confirms upward velocity exists, not that the position
			// has actually risen yet (velocity and position are read at the
			// start/end of the same tick) - switching straight to a steep
			// dive the instant the threshold is crossed killed the climb
			// before it could show up as a position change at all, an
			// earlier version of this test found live (flareApexY came back
			// identical to the flare-trigger position). Leveling off
			// (pitch=0, h≈1, ~25% gravity - physics/elytra.go) instead lets
			// the residual upward velocity actually carry the position up
			// for real over the next several ticks, matching the "coast"
			// phase already used between the climb and dive above for the
			// same momentum-isn't-instant reason.
			const apexCoastPitch = 0.0
			const maxApexCoastDuration = 1 * time.Second

			coastStart := time.Now()
			for time.Now().Before(coastStart.Add(maxApexCoastDuration)) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, apexCoastPitch))
				time.Sleep(steerInterval)

				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)
				if pos.Y > flareApexY {
					flareApexY = pos.Y
				}

				_, vy, _, velOK := env.Agent.Agent.GetVelocity()
				if velOK && vy <= 0 {
					t.Logf("true apex reached: vy=%.3f after %v of coasting", vy, time.Since(coastStart))
					break
				}
			}

			// The drop: GlidingVelocity's gravity and lift terms are both
			// scaled by h = cos²(pitch) (physics/elytra.go) - looking level
			// (h≈1) cuts effective gravity to ~25% and keeps converting sink
			// into lift, which is exactly why an earlier version of this
			// test that leveled off to a shallow pitch (10°) right after the
			// pop just looked like an ordinary glide down - h was still
			// close to 1, so the same floaty cushioning that makes gliding
			// gentle in general kept right on cushioning it. Looking
			// steeply down instead (h≈0, confirmed from decompiled
			// LivingEntity.calcGlidingVelocity: at pitch=±90 the gravity
			// blend reduces to unmodified -gravity, i.e. genuine free fall)
			// removes that cushioning and produces a real, fast, visually
			// obvious drop - still nominally "gliding" (isGliding stays
			// true; vanilla only exits gliding on ground/vehicle/levitation/
			// losing the elytra, confirmed from PlayerEntity.canGlide() -
			// there's no jump-based cancel), which is what keeps
			// limitFallDistance()'s fall-damage cap available for the final
			// settle below. dropPitch is positive (nose down), never
			// negative, specifically so the climb term (pitch<0) can't
			// reactivate and re-trigger the balloon bug.
			//
			// The breakout condition is velocity, not altitude: at h≈0 this
			// phase applies close to full, uncushioned gravity, so vy grows
			// roughly 0.1-0.3 blocks/tick faster each tick and blows past a
			// fixed altitude threshold in a single 100ms poll once it's
			// diving fast - confirmed live (targeting a 6-block breakout
			// altitude actually broke out around 4 blocks, already doing
			// -1.7 blocks/tick, too fast for the settle phase below to
			// recover from before impact: settlePitch's dive-term recovery
			// is only ~10%/tick, so unwinding from -1.7 back above the
			// -0.5 fall-distance-reset threshold needs ~600ms and ~10+
			// blocks of altitude on its own, real damage resulted). Capping
			// the drop phase by vy instead keeps the speed at handoff
			// predictable regardless of how high the apex happened to be,
			// so the settle phase's margin requirement stays the same too.
			const dropPitch = 80.0
			const dropVelocityFloor = -1.0 // blocks/tick; still a fast, visible drop
			const dropSafetyAltitude = 8.0 // stop the steep drop no matter what below this

			dropDeadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(dropDeadline) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, dropPitch))
				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)
				if pos.Y > flareApexY {
					flareApexY = pos.Y
				}
				_, vy, _, velOK := env.Agent.Agent.GetVelocity()
				if pos.Y-groundY <= dropSafetyAltitude {
					break
				}
				if velOK && vy <= dropVelocityFloor {
					break
				}
				time.Sleep(steerInterval)
			}
			t.Logf("flare apex: %.2f (climbed %.2f blocks above the true dive bottom)", flareApexY, flareApexY-diveBottomY)

			// Final settle: level off with real altitude to spare so
			// limitFallDistance()'s vy>-0.5 condition has a few ticks to
			// re-engage (the drop phase above can build vertical speed well
			// past that) before touching down, same margin reasoning as
			// flareAltitude above.
			settleDeadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(settleDeadline) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, settlePitch))
				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)
				if pos.Y-groundY <= 1.5 {
					break
				}
				time.Sleep(steerInterval)
			}

			// Give it a moment to actually settle onto the ground.
			time.Sleep(1 * time.Second)
			finalPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			t.Logf("final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)

			// The descent rate immediately before the flare (the fastest
			// part of the dive) versus immediately after starting it - the
			// flare should visibly slow the descent, not just coincidentally
			// avoid damage.
			preFlareDescentRate := prePreFlarePos.Y - preFlarePos.Y
			t.Logf("descent rate in the tick(s) just before flaring: %.3f blocks/%v", preFlareDescentRate, steerInterval)

			endHealth, err := GetPlayerHealth(ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get ending health")
			t.Logf("ending health: %.1f (started at %.1f)", endHealth, startHealth)

			assert.InDelta(t, startHealth, endHealth, 0.01, "flaring to land should avoid fall damage entirely")
			assert.Less(t, finalPos.Y, 3.0, "should have actually come down to the ground, not gotten stuck hovering")

			// A real flare goes down, then up, then down again - not just a
			// dive that quietly cancels to zero. Require a real, visible
			// climb between the dive's low point and the flare's apex.
			assert.Greater(t, flareApexY-diveBottomY, 1.5, "flare should produce a visible upward arc before dropping, not just arrest the dive")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
		})
	}
}
