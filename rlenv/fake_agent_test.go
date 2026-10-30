package rlenv_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// fakeAgent is a minimal, in-memory models.CommandAgent + models.HealthProvider
// for testing rlenv.Environment without a live server, per
// RL_POLICY_INTEGRATION_PLAN.md's verification requirement ("unit-testable
// without a live server"). MoveTo is the only method Environment's tests
// actually exercise the effect of (via ActionGoToTarget's "movetoquiet"
// dispatch — see rlenv/action.go); every other CommandAgent
// method exists solely so fakeAgent satisfies the interface
// actions.NewRegistry()'s real actions (in particular actions.MoveToQuiet)
// require, matching how the real dispatch path is exercised end to end in
// environment_test.go rather than mocking the registry away.
type fakeAgent struct {
	mu                  sync.Mutex
	x, y, z             float64
	yaw, pitch          float64
	posKnown            bool
	health              float32
	food                int32
	saturation          float32
	healthKnown         bool
	moveToWithChatErr   error
	moveToWithChatDelay time.Duration // if set, MoveToWithChat sleeps this long before returning
	moveToCanceled      bool          // set when a delayed MoveTo observed its context being canceled
	moveToWithChatCalls int

	// Sleep/time-of-day simulation: bedX/Y/Z/bedFound is what
	// FindNearestBed reports; timeOfDayTicks/timeOfDayKnown is what
	// GetTimeOfDay reports (see TimeOfDayAgent). sleepErr, if set, is
	// what SleepInBed returns instead of succeeding - lets a test drive
	// the "position never snapped into the bed" rejection path without
	// a real server. sleepCalls/wakeUpCalls just count invocations.
	bedX, bedY, bedZ float64
	bedFound         bool
	sleepErr         error
	sleepCalls       int
	wakeUpCalls      int
	timeOfDayTicks   int64

	// Portal simulation: portalX/Y/Z/portalFound is what FindNearestPortal
	// reports; usePortalAtErr, if set, is what UsePortalAt returns instead
	// of succeeding (simulating the dimension never changing) -
	// usePortalAtCalls just counts invocations. Mirrors bed/sleep's own
	// shape exactly.
	portalX, portalY, portalZ float64
	portalFound               bool
	usePortalAtErr            error
	usePortalAtCalls          int
	timeOfDayKnown            bool

	// Mine simulation: a single block instance at (mineBlockX,Y,Z) whose
	// name is mineBlockName ("" means nothing is there). FindVisibleBlock
	// only reports it found when the queried name matches; MineBlockAt
	// "breaks" it by setting mineBlockName to "minecraft:air", so
	// Environment's before/after BlockNameAt diff (see environment.go's
	// Step) sees a real change, the same way MoveToWithChat's fake actually
	// moves the tracked position rather than just recording the call.
	mineBlockName                      string
	restoredBlocks                     []restoredBlock
	clearedBoxes                       []clearedBox
	clonedAreas                        []clonedArea
	seedBlockAtCalls                   []farSeedCall
	seedCraftAtCalls                   []farSeedCall
	mineBlockX, mineBlockY, mineBlockZ float64
	mineBlockAtErr                     error
	mineBlockAtCalls                   int
	// mineBlockHidden, when true, makes FindVisibleBlock report
	// mineBlockName as not found even though BlockNameAt still reports it
	// at its real coordinates - simulates losing line-of-sight to a block
	// that's still physically there (as opposed to setMineBlock("", ...),
	// which simulates it no longer existing at all), for exercising the
	// mine-target "memory" (docs/plans/14-buried-block-mining-and-memory-map.md,
	// mc-rsi-trainer).
	mineBlockHidden   bool
	collectDropsCalls int

	// Chain simulation (chain_test's env tests): when chainInv is non-nil the
	// fake models a tiny world of blocks and an inventory with the real
	// recipes, instead of the single mine block / craft target above.
	chainInv    map[string]int
	chainBlocks map[[3]int]string
	// chainBlocksHidden mirrors mineBlockHidden's own role (see its doc
	// comment), but for the chainBlocks-backed FindVisibleBlock path: a
	// block can be present in chainBlocks (so BlockNameAt/MineBlockAt
	// still resolve it) while this makes FindVisibleBlock report it as
	// not found, simulating a buried target that hasn't been glimpsed yet.
	chainBlocksHidden bool
	chainDrops        int
	clearedInventory  int
	clearedDropAreas  [][4]int
	failNextPlace     bool
	// placeThenFail places the block but reports failure once: the world
	// changed, the caller was not told (the bot's view lagged).
	placeThenFail         bool
	findVisibleBlockCalls int

	// Crafting table simulation: FindVisibleBlock reports a table (at the
	// origin) only while tableVisible is set, and records the radius asked.
	tableVisible     bool
	tableSearchRadii []int

	// Craft simulation: a single target item, craftTargetName, whose
	// currently-held count is craftHeldCount. Craftable reports true only
	// once craftIngredientsReady is set (simulating "ingredients are in
	// inventory"), and CraftItem increments craftHeldCount by one when
	// called with a matching item name while ready — mirrors
	// mineBlockName's single-target simplicity (see its own doc comment)
	// and MineBlockAt's "actually mutates state" precedent, so
	// Environment's real craft dispatch/reward logic is exercised for real
	// rather than stubbed out (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1).
	craftTargetName       string
	craftIngredientsReady bool
	craftHeldCount        int
	craftItemErr          error
	craftItemCalls        int

	// Seeding simulation (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4):
	// records calls so tests can assert an EpisodeSeeder was actually
	// invoked with the expected arguments, and optionally returns
	// seedErr to exercise Reset's error path.
	seedNearbyBlockCalls      []seedNearbyBlockCall
	seedCraftIngredientsCalls []string
	seedGiveItemCalls         []seedGiveItemCall
	seedErr                   error

	// Teleport simulation (rlenv.Config.ResetOrigin): records calls so
	// tests can assert Reset actually teleported, and optionally returns
	// teleportErr to exercise Reset's error path — same shape as
	// seedErr above. Unlike seedErr, a successful TeleportTo also mutates
	// x/y/z, mirroring MoveTo's "actually move the tracked position"
	// precedent (see this file's own top doc comment) rather than just
	// recording the call.
	teleportCalls []teleportCall
	teleportErr   error

	// teleportFailFirstN, if > 0, makes exactly that many leading
	// TeleportTo calls fail with a transient error before returning to
	// normal (successful) behavior - unlike teleportErr's permanent
	// failure, this simulates a one-off flaky teleport to exercise
	// Environment.Reset's own outer retry-on-attempt-failure loop (see
	// TestResetOuterRetryRecoversFromATransientTeleportFailure).
	teleportFailFirstN int

	// FindPath simulation (rlenv's reachability gate, walkability.go):
	// findPathUnreachable, if set, is called for every FindPath candidate
	// position — returning true simulates a real pathfinder failure ("no
	// path found") for exactly that position, independent of whether
	// findWalkableTarget's own standability check already accepted it.
	// nil (the default) means every position is reachable, matching
	// FindPath's behavior before this simulation existed. findPathCalls
	// records every (x,y,z) queried, so a test can assert exactly which
	// candidates the reachability gate tried, not just how many.
	findPathUnreachable func(x, y, z float64) bool
	findPathCalls       []models.V3

	// Equip/UseItem simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 1): heldItem is whatever Equip was last called with — mirrors
	// mineBlockName's "actually mutates state" precedent, so Environment's
	// real equip dispatch/reward logic is exercised for real rather than
	// stubbed out. useItemCalls/useItemHands record every UseItem call so
	// tests can assert dispatch args reached it correctly.
	heldItem     string
	useItemCalls int
	useItemHands []models.Hand

	// Item Pickup simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 2): a single target item, pickupTargetName, dropped at
	// (pickupItemX,Y,Z), visible while pickupItemPresent. MoveTo/
	// MoveToWithChat reaching within pickupRange of it "collects" it
	// (clears pickupItemPresent, increments pickupHeldCount) — mirrors
	// vanilla's own walk-near-it auto-collect behavior the real
	// PickUpItem/pickUpNearbyItem commands rely on, and mineBlockName's
	// "actually mutates state" precedent rather than a no-op stub.
	pickupTargetName                      string
	pickupItemX, pickupItemY, pickupItemZ float64
	pickupItemPresent                     bool
	pickupHeldCount                       int
	findItemOfTypeCalls                   int

	// gliding backs IsGliding — see FlyTo's own doc comment
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3).
	gliding bool

	// Mob Riding simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 4): a single target vehicle type, rideTargetType, visible at
	// (rideX,Y,Z) with entity ID rideEntityID while rideVisible.
	// MountEntity "mounts" it (mirrors mineBlockName's "actually mutates
	// state" precedent) only when given that exact entity ID — matching
	// ActionMount's own dispatch-by-resolved-ID contract, not by name.
	rideTargetType        string
	rideX, rideY, rideZ   float64
	rideEntityID          int32
	rideVisible           bool
	mounted               bool
	vehicleJumpCalls      int
	findEntityOfTypeCalls int

	// Combat simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 5): a single target mob type, combatTargetType, visible at
	// (combatX,Y,Z) with entity ID combatEntityID and health combatHealth
	// while combatVisible. RunCombat "kills" it by setting combatHealth
	// to 0 (mirrors mineBlockName's "actually mutates state" precedent)
	// — see killCombatTarget.
	combatTargetType          string
	combatX, combatY, combatZ float64
	combatEntityID            int32
	combatHealth              float32
	combatVisible             bool
	runCombatCalls            []runCombatCall

	// Standalone Place simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
	// item 2): a single target item, placeTargetName, with held count
	// placeHeldCount — PlaceHeldBlock decrements it by one on a successful
	// placement when chainInv is nil (standalone mode; a chain episode
	// keeps using chainInv, unaffected by this).
	placeTargetName string
	placeHeldCount  int

	// Sneak-on-arrival simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
	// item 3): sneaking backs StopSneaking's own no-op-when-not-sneaking
	// contract closely enough for tests; moveToAndSneakCalls/
	// stopSneakingCalls record call counts for dispatch assertions.
	sneaking            bool
	moveToAndSneakCalls int
	stopSneakingCalls   int

	// Follow simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item
	// 4): followablePlayerName is the one name FindPlayerByName resolves
	// (empty = nobody resolvable, the common "no second player" case this
	// task's own precondition describes); following backs IsFollowing.
	followablePlayerName string
	following            bool

	// Creeper-gunpowder simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
	// item 5): a single creeper, visible at (creeperX,Y,Z) with entity ID
	// creeperEntityID while creeperVisible. KillCreeperForGunpowder
	// "fights" it by clearing creeperVisible (mirrors killCombatTarget's
	// own "actually mutates state" precedent) and, if
	// creeperDropsGunpowder, increments gunpowderHeldCount — the real
	// method's own documented "no-drop encounter" is a valid outcome, so
	// this defaults to true rather than always dropping.
	creeperX, creeperY, creeperZ float64
	creeperEntityID              int32
	creeperVisible               bool
	creeperDropsGunpowder        bool
	gunpowderHeldCount           int

	// Spear-charge simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
	// item 6): a single target mob type, spearTargetType, visible at
	// (spearX,Y,Z) with entity ID spearEntityID and health spearHealth
	// while spearVisible. SpearChargeAtWithApproach "charges" it by
	// setting spearHealth to 0 (mirrors killCombatTarget's own "actually
	// mutates state" precedent) — see killSpearTarget.
	spearTargetType        string
	spearX, spearY, spearZ float64
	spearEntityID          int32
	spearHealth            float32
	spearVisible           bool
	spearChargeCalls       []spearChargeCall

	// Mace-smash simulation (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
	// item 7): a single target mob type, maceTargetType, visible at
	// (maceX,Y,Z) with entity ID maceEntityID and health maceHealth while
	// maceVisible. MaceSmashAtWithApproach "smashes" it by setting
	// maceHealth to 0 — mirrors spearTargetType's own "actually mutates
	// state" precedent.
	maceTargetType      string
	maceX, maceY, maceZ float64
	maceEntityID        int32
	maceHealth          float32
	maceVisible         bool
	maceSmashCalls      []maceSmashCall
}

// maceSmashCall records one MaceSmashAtWithApproach call's args, so tests
// can assert rlenv's real dispatch reached it with the right values.
type maceSmashCall struct {
	targetID int32
	itemName string
}

// spearChargeCall records one SpearChargeAtWithApproach call's args, so
// tests can assert rlenv's real dispatch reached it with the right
// values.
type spearChargeCall struct {
	targetID               int32
	itemName               string
	hold, engaged, tired   time.Duration
	minSpeed, minAlignment float64
}

// setFollowablePlayer configures the simulated resolvable player — see
// followablePlayerName's own doc comment.
func (f *fakeAgent) setFollowablePlayer(name string) {
	f.mu.Lock()
	f.followablePlayerName = name
	f.mu.Unlock()
}

// setCreeperTarget configures the simulated creeper — see creeperX's own
// doc comment. dropsGunpowder controls whether a subsequent
// KillCreeperForGunpowder call yields a drop.
func (f *fakeAgent) setCreeperTarget(entityID int32, x, y, z float64, dropsGunpowder bool) {
	f.mu.Lock()
	f.creeperEntityID = entityID
	f.creeperX, f.creeperY, f.creeperZ = x, y, z
	f.creeperVisible = true
	f.creeperDropsGunpowder = dropsGunpowder
	f.mu.Unlock()
}

// setSpearTarget configures the simulated spear-charge target — see
// spearTargetType's own doc comment.
func (f *fakeAgent) setSpearTarget(entityType string, entityID int32, x, y, z float64, health float32) {
	f.mu.Lock()
	f.spearTargetType = entityType
	f.spearEntityID = entityID
	f.spearX, f.spearY, f.spearZ = x, y, z
	f.spearHealth = health
	f.spearVisible = true
	f.mu.Unlock()
}

// killSpearTarget mirrors killCombatTarget's own doc comment, for the
// spear-charge target.
func (f *fakeAgent) killSpearTarget() {
	f.mu.Lock()
	f.spearHealth = 0
	f.mu.Unlock()
}

// setMaceTarget configures the simulated mace-smash target — see
// maceTargetType's own doc comment.
func (f *fakeAgent) setMaceTarget(entityType string, entityID int32, x, y, z float64, health float32) {
	f.mu.Lock()
	f.maceTargetType = entityType
	f.maceEntityID = entityID
	f.maceX, f.maceY, f.maceZ = x, y, z
	f.maceHealth = health
	f.maceVisible = true
	f.mu.Unlock()
}

// killMaceTarget mirrors killCombatTarget's own doc comment, for the
// mace-smash target.
func (f *fakeAgent) killMaceTarget() {
	f.mu.Lock()
	f.maceHealth = 0
	f.mu.Unlock()
}

// setPlaceTarget configures the simulated standalone-placement target —
// see placeTargetName's own doc comment.
func (f *fakeAgent) setPlaceTarget(itemName string, count int) {
	f.mu.Lock()
	f.placeTargetName = itemName
	f.placeHeldCount = count
	f.mu.Unlock()
}

type runCombatCall struct {
	radius         float64
	includePlayers bool
	includeNeutral bool
}

// setRideVehicle configures the simulated Mob Riding target — see
// rideTargetType's own doc comment.
func (f *fakeAgent) setRideVehicle(entityType string, entityID int32, x, y, z float64) {
	f.mu.Lock()
	f.rideTargetType = entityType
	f.rideEntityID = entityID
	f.rideX, f.rideY, f.rideZ = x, y, z
	f.rideVisible = true
	f.mu.Unlock()
}

// setCombatTarget configures the simulated Combat target — see
// combatTargetType's own doc comment.
func (f *fakeAgent) setCombatTarget(entityType string, entityID int32, x, y, z float64, health float32) {
	f.mu.Lock()
	f.combatTargetType = entityType
	f.combatEntityID = entityID
	f.combatX, f.combatY, f.combatZ = x, y, z
	f.combatHealth = health
	f.combatVisible = true
	f.mu.Unlock()
}

// killCombatTarget simulates RunCombat landing a killing blow: zeroes the
// target's health, matching GetEntityHealth's "health <= 0 counts as
// dead" contract (it stays "found", mirroring a kill the server hasn't
// yet confirmed via removal — see that method's own doc comment on why
// either signal counts).
func (f *fakeAgent) killCombatTarget() {
	f.mu.Lock()
	f.combatHealth = 0
	f.mu.Unlock()
}

// pickupRange is how close MoveTo must land to the simulated pickup item
// to "collect" it — mirrors vanilla's actual pickup-range generosity
// (collects without landing exactly on the drop's cell).
const pickupRange = 1.5

// setPickupItem configures the simulated Item Pickup target — see
// pickupTargetName's own doc comment.
func (f *fakeAgent) setPickupItem(name string, x, y, z float64) {
	f.mu.Lock()
	f.pickupTargetName = name
	f.pickupItemX, f.pickupItemY, f.pickupItemZ = x, y, z
	f.pickupItemPresent = true
	f.mu.Unlock()
}

type seedNearbyBlockCall struct {
	blockName string
	radius    int
}

type seedGiveItemCall struct {
	itemName string
	count    int
}

type teleportCall struct {
	x, y, z float64
}

func newFakeAgent(x, y, z float64) *fakeAgent {
	return &fakeAgent{x: x, y: y, z: z, posKnown: true}
}

func (f *fakeAgent) setPosition(x, y, z float64) {
	f.mu.Lock()
	f.x, f.y, f.z = x, y, z
	f.mu.Unlock()
}

func (f *fakeAgent) setHealth(health float32, food int32, saturation float32) {
	f.mu.Lock()
	f.health, f.food, f.saturation, f.healthKnown = health, food, saturation, true
	f.mu.Unlock()
}

// setMineBlock places a simulated block instance of the given name at
// (x,y,z) — see mineBlockName's doc comment. name == "" means nothing is
// there (FindVisibleBlock never finds it, matching the zero-value default).
func (f *fakeAgent) setMineBlock(name string, x, y, z float64) {
	f.mu.Lock()
	f.mineBlockName = name
	f.mineBlockX, f.mineBlockY, f.mineBlockZ = x, y, z
	f.mu.Unlock()
}

// setMineBlockHidden toggles mineBlockHidden — see its own doc comment.
func (f *fakeAgent) setMineBlockHidden(hidden bool) {
	f.mu.Lock()
	f.mineBlockHidden = hidden
	f.mu.Unlock()
}

// setCraftTarget configures the simulated craft target — see
// craftTargetName's doc comment. ingredientsReady controls Craftable's
// result; initialHeldCount seeds craftHeldCount (0 for "the bot holds none
// of the target item yet").
func (f *fakeAgent) setCraftTarget(name string, ingredientsReady bool, initialHeldCount int) {
	f.mu.Lock()
	f.craftTargetName = name
	f.craftIngredientsReady = ingredientsReady
	f.craftHeldCount = initialHeldCount
	f.mu.Unlock()
}

// --- models.HealthProvider ---

func (f *fakeAgent) Health() (health float32, food int32, saturation float32, known bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.health, f.food, f.saturation, f.healthKnown
}

// --- models.Position (embedded in models.MovementAgent) ---

func (f *fakeAgent) UpdatePosition(pos models.V3, yaw, pitch float64) {
	f.mu.Lock()
	f.x, f.y, f.z = pos.X, pos.Y, pos.Z
	f.yaw, f.pitch = yaw, pitch
	f.posKnown = true
	f.mu.Unlock()
}

func (f *fakeAgent) GetPosition() (pos models.V3, yaw, pitch float64, initialized bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return models.V3{X: f.x, Y: f.y, Z: f.z}, f.yaw, f.pitch, f.posKnown
}

func (f *fakeAgent) GetPositionSimple() (pos models.V3, initialized bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return models.V3{X: f.x, Y: f.y, Z: f.z}, f.posKnown
}

// --- models.ChatOperations ---

func (f *fakeAgent) SendChat(string) error { return nil }

// --- models.MovementAgent (beyond Position) ---

func (f *fakeAgent) GetVelocity() (x, y, z float64, ok bool) { return 0, 0, 0, false }
func (f *fakeAgent) IsGliding() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gliding
}

func (f *fakeAgent) MoveForward(context.Context, float64) error    { return nil }
func (f *fakeAgent) MoveUp(context.Context, float64) error         { return nil }
func (f *fakeAgent) MoveUpAndSneak(context.Context, float64) error { return nil }

// MoveToAndSneak moves the tracked position to (x, y, z) and marks the bot
// sneaking — mirrors MoveTo's own "actually mutates state" precedent, so
// rlenv's real ActionGoToTarget dispatch (Config.ArriveSneaking,
// docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 3) is exercised for
// real rather than stubbed out.
func (f *fakeAgent) MoveToAndSneak(_ context.Context, x, y, z float64) error {
	f.mu.Lock()
	f.x, f.y, f.z = x, y, z
	f.sneaking = true
	f.moveToAndSneakCalls++
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) LineToAndSneak(context.Context, float64, float64, float64) error {
	return nil
}
func (f *fakeAgent) StartSneaking() error {
	f.mu.Lock()
	f.sneaking = true
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) StopSneaking() error {
	f.mu.Lock()
	f.sneaking = false
	f.stopSneakingCalls++
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) FindPath(_ context.Context, x, y, z float64) (*models.Path, error) {
	f.mu.Lock()
	f.findPathCalls = append(f.findPathCalls, models.V3{X: x, Y: y, Z: z})
	unreachable := f.findPathUnreachable
	f.mu.Unlock()
	if unreachable != nil && unreachable(x, y, z) {
		return nil, errors.New("fakeAgent: no path found")
	}
	return &models.Path{Found: true}, nil
}
func (f *fakeAgent) ExecutePath(context.Context, *models.Path) error         { return nil }
func (f *fakeAgent) LookAt(context.Context, float64, float64, float64) error { return nil }

// Follow "starts" following — mirrors mineBlockName's own "actually
// mutates state" precedent, so rlenv's real ActionFollow dispatch/reward
// logic is exercised for real (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
// item 4).
func (f *fakeAgent) Follow(_ context.Context, _ string) error {
	f.mu.Lock()
	f.following = true
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) StopFollow(_ context.Context) error {
	f.mu.Lock()
	f.following = false
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) FollowStatus(context.Context) string { return "" }

// --- models.CommandAgent (beyond MovementAgent/ChatOperations) ---

// MoveTo simulates instant arrival: production movement is real
// pathfinding over many ticks, but this fake only needs to prove
// rlenv.Environment reacts correctly to *some* position change reaching the
// target, not to simulate pathfinding itself. notifyChat is ignored (this
// fake never sends real chat) — it exists only so fakeAgent satisfies
// models.CommandAgent's full MoveTo/MoveToWithChat pair. The
// moveToWithChat* fields/counter are shared by both entry points, mirroring
// how the real *agent's MoveToWithChat is just MoveTo(ctx,x,y,z,true) —
// see rlenv/action.go's ActionGoToTarget, which dispatches through the
// quiet path (actions.MoveToQuiet) exactly like production rlenv training
// does.
func (f *fakeAgent) MoveTo(ctx context.Context, x, y, z float64, _ bool) error {
	f.mu.Lock()
	f.moveToWithChatCalls++
	err := f.moveToWithChatErr
	delay := f.moveToWithChatDelay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			f.mu.Lock()
			f.moveToCanceled = true
			f.mu.Unlock()
			return ctx.Err()
		}
	}

	if err == nil {
		f.mu.Lock()
		f.x, f.y, f.z = x, y, z
		if f.pickupItemPresent {
			dx, dy, dz := x-f.pickupItemX, y-f.pickupItemY, z-f.pickupItemZ
			if dx*dx+dy*dy+dz*dz <= pickupRange*pickupRange {
				f.pickupItemPresent = false
				f.pickupHeldCount++
			}
		}
		f.mu.Unlock()
	}
	return err
}

func (f *fakeAgent) MoveToWithChat(ctx context.Context, x, y, z float64) error {
	return f.MoveTo(ctx, x, y, z, true)
}

func (f *fakeAgent) LineTo(context.Context, float64, float64, float64, bool) error {
	return nil
}
func (f *fakeAgent) TestMove()              {}
func (f *fakeAgent) TestPath()              {}
func (f *fakeAgent) StartTracking()         {}
func (f *fakeAgent) StopTracking()          {}
func (f *fakeAgent) HasFollowManager() bool { return true }
func (f *fakeAgent) IsFollowing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.following
}
func (f *fakeAgent) IsSprinting() bool { return false }
func (f *fakeAgent) PlanStatus(context.Context) models.PlanStatus {
	return models.PlanStatus{}
}
func (f *fakeAgent) StopPlan(context.Context) error { return nil }
func (f *fakeAgent) FireBow(context.Context) error  { return nil }
func (f *fakeAgent) FireBowAt(context.Context, float64, float64, float64, ...models.ProjectileHitCallback) ([]models.TrajectoryPoint, error) {
	return nil, nil
}

// MountEntity "mounts" entityID if it matches the simulated ride target —
// mirrors mineBlockName's "actually mutates state" precedent (see
// rideTargetType's own doc comment).
func (f *fakeAgent) MountEntity(_ context.Context, entityID int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rideTargetType == "" || !f.rideVisible || entityID != f.rideEntityID {
		return fmt.Errorf("entity %d not found", entityID)
	}
	f.mounted = true
	return nil
}
func (f *fakeAgent) MountNearest(context.Context, string) error { return nil }
func (f *fakeAgent) DismountEntity() error {
	f.mu.Lock()
	f.mounted = false
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) JumpVehicle(context.Context, int32) error {
	f.mu.Lock()
	f.vehicleJumpCalls++
	f.mu.Unlock()
	return nil
}

// Equip "equips" itemName by recording it as heldItem — see heldItem's own
// doc comment for why this mutates state rather than being a no-op stub.
func (f *fakeAgent) Equip(_ context.Context, itemName string) error {
	f.mu.Lock()
	f.heldItem = itemName
	f.mu.Unlock()
	return nil
}

// UseItem records the call and which hand it was sent for, so tests can
// assert ActionUseItem's dispatch reached it with the right args.
func (f *fakeAgent) UseItem(_ context.Context, hand models.Hand) error {
	f.mu.Lock()
	f.useItemCalls++
	f.useItemHands = append(f.useItemHands, hand)
	f.mu.Unlock()
	return nil
}

// FindNearestVisibleEntityOfType reports the simulated ride vehicle's
// position/ID only when its type matches and it's still present — mirrors
// FindNearestVisibleItemOfType's own contract (docs/plans/
// RL_ACTION_SPACE_EXPANSION_II.md Phase 4).
func (f *fakeAgent) FindNearestVisibleEntityOfType(_ context.Context, entityTypeName string, _ float64) (entityID int32, x, y, z float64, found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findEntityOfTypeCalls++
	if f.rideVisible && f.rideTargetType != "" && entityTypeName == f.rideTargetType {
		return f.rideEntityID, f.rideX, f.rideY, f.rideZ, true, nil
	}
	if f.combatVisible && f.combatTargetType != "" && entityTypeName == f.combatTargetType {
		return f.combatEntityID, f.combatX, f.combatY, f.combatZ, true, nil
	}
	if f.creeperVisible && entityTypeName == "minecraft:creeper" {
		return f.creeperEntityID, f.creeperX, f.creeperY, f.creeperZ, true, nil
	}
	if f.spearVisible && f.spearTargetType != "" && entityTypeName == f.spearTargetType {
		return f.spearEntityID, f.spearX, f.spearY, f.spearZ, true, nil
	}
	if f.maceVisible && f.maceTargetType != "" && entityTypeName == f.maceTargetType {
		return f.maceEntityID, f.maceX, f.maceY, f.maceZ, true, nil
	}
	return 0, 0, 0, 0, false, nil
}

// GetEntityHealth reports the simulated combat target's health when
// entityID matches — found=false otherwise (not tracked), matching
// agent.GetEntityHealth's own contract.
func (f *fakeAgent) GetEntityHealth(entityID int32) (health float32, found bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.combatVisible && entityID == f.combatEntityID {
		return f.combatHealth, true
	}
	if f.spearVisible && entityID == f.spearEntityID {
		return f.spearHealth, true
	}
	if f.maceVisible && entityID == f.maceEntityID {
		return f.maceHealth, true
	}
	// The simulated pickup item has no meaningful health — only "found"
	// matters for Config.PickupAnyItem's own reward check (see
	// pickupItemEntityID's own doc comment).
	if f.pickupItemPresent && entityID == pickupItemEntityID {
		return 0, true
	}
	return 0, false
}

// RunCombat "fights" the simulated combat target by killing it (see
// killCombatTarget) — mirrors MineBlockAt's "actually mutates state"
// precedent so rlenv's real ActionCombat dispatch/reward logic is
// exercised for real (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5).
// A no-op (recording the call only) if nothing is currently configured as
// the combat target, matching a real empty-radius runCombat call that
// simply finds nothing to fight.
func (f *fakeAgent) RunCombat(_ context.Context, radius float64, includeNeutral bool) error {
	f.mu.Lock()
	f.runCombatCalls = append(f.runCombatCalls, runCombatCall{radius: radius, includeNeutral: includeNeutral})
	if f.combatVisible {
		f.combatHealth = 0
	}
	f.mu.Unlock()
	return nil
}

// RunCombatWithPolicy mirrors RunCombat's own "actually mutates state"
// precedent — rlenv's real ActionCombat dispatch now exclusively targets
// "runcombatwithpolicy" (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md's
// "Deferred" section, closed out once the user explicitly decided
// Config.CombatIncludePlayers should be opt-in/default-false), so this is
// what ActionCombat's own tests actually exercise now; RunCombat above
// stays in place unused by any current rlenv test, mirroring how
// agent.RunCombat itself is just a convenience wrapper around
// agent.RunCombatWithPolicy in production.
func (f *fakeAgent) RunCombatWithPolicy(_ context.Context, radius float64, policy combat.TargetPolicy) error {
	f.mu.Lock()
	f.runCombatCalls = append(f.runCombatCalls, runCombatCall{radius: radius, includePlayers: policy.IncludePlayers, includeNeutral: policy.IncludeNeutral})
	if f.combatVisible {
		f.combatHealth = 0
	}
	f.mu.Unlock()
	return nil
}

// KillCreeperForGunpowder "fights" the simulated creeper by clearing
// creeperVisible and, if creeperDropsGunpowder, incrementing
// gunpowderHeldCount — mirrors RunCombat's own "actually mutates state"
// precedent so rlenv's real ActionKillCreeperForGunpowder dispatch/reward
// logic is exercised for real (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
// item 5). Errors if nothing is currently configured as the creeper
// target, matching the real method's own "not found" contract rather than
// a silent no-op (unlike RunCombat, this method always resolves one
// specific creeper, so there is no "empty radius" equivalent).
func (f *fakeAgent) KillCreeperForGunpowder(_ context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.creeperVisible {
		return false, errors.New("kill creeper: creeper not found")
	}
	f.creeperVisible = false
	if f.creeperDropsGunpowder {
		f.gunpowderHeldCount++
		return true, nil
	}
	return false, nil
}

// SpearChargeAtWithApproach "charges" the simulated spear target by
// zeroing its health (mirrors killCombatTarget's own "actually mutates
// state" precedent) — see spearTargetType's own doc comment. A no-op
// (recording the call only) if nothing is currently configured as the
// spear target, matching a real call against an untracked/vanished
// target.
func (f *fakeAgent) SpearChargeAtWithApproach(_ context.Context, targetID int32, itemName string, hold, engaged, tired time.Duration, minSpeed, minAlignment float64) error {
	f.mu.Lock()
	f.spearChargeCalls = append(f.spearChargeCalls, spearChargeCall{targetID, itemName, hold, engaged, tired, minSpeed, minAlignment})
	if f.spearVisible && targetID == f.spearEntityID {
		f.spearHealth = 0
	}
	f.mu.Unlock()
	return nil
}

// MaceSmashAtWithApproach "smashes" the simulated mace target by zeroing
// its health — mirrors SpearChargeAtWithApproach's own "actually mutates
// state" precedent. A no-op (recording the call only) if nothing is
// currently configured as the mace target, matching a real call against
// an untracked/vanished target.
func (f *fakeAgent) MaceSmashAtWithApproach(_ context.Context, targetID int32, itemName string) error {
	f.mu.Lock()
	f.maceSmashCalls = append(f.maceSmashCalls, maceSmashCall{targetID: targetID, itemName: itemName})
	if f.maceVisible && targetID == f.maceEntityID {
		f.maceHealth = 0
	}
	f.mu.Unlock()
	return nil
}

// IsMounted reports the simulated mount state — see MountEntity/
// DismountEntity's own doc comments.
func (f *fakeAgent) IsMounted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mounted
}

// IsEquipped reports whether itemName matches heldItem — mirrors the real
// implementation's "worn or held" contract closely enough for this
// package's equip-task tests, which only ever equip one item at a time.
func (f *fakeAgent) IsEquipped(itemName string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.heldItem != "" && f.heldItem == itemName
}

// FlyTo "flies" by moving the tracked position directly to (x, y, z) and
// marking the bot as gliding — mirrors MoveTo's own "actually mutates
// state" precedent (see this file's top doc comment) rather than a no-op
// stub, so rlenv's real ActionFlyTo dispatch/arrival/reward logic is
// exercised for real (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3).
func (f *fakeAgent) FlyTo(_ context.Context, x, y, z float64) error {
	f.mu.Lock()
	f.x, f.y, f.z = x, y, z
	f.gliding = true
	f.mu.Unlock()
	return nil
}
func (f *fakeAgent) NearestPlayerInfo(context.Context, bool) (models.NearestPlayerInfo, bool) {
	return models.NearestPlayerInfo{}, false
}

// FindPlayerByName reports the simulated resolvable player only when the
// queried name matches followablePlayerName — mirrors the real contract
// deliberately: returns an error (not found=false) when the name doesn't
// resolve, matching agent.FindPlayerByName's own "not found" behavior,
// since Environment.refreshFollowTarget is specifically built to treat
// that as routine rather than fatal (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 4).
func (f *fakeAgent) FindPlayerByName(_ context.Context, name string) (x, y, z float64, found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.followablePlayerName == "" || name != f.followablePlayerName {
		return 0, 0, 0, false, errors.New("player not found")
	}
	return 0, 0, 0, true, nil
}
func (f *fakeAgent) GetPlayerAbilities() (models.PlayerAbilities, bool) {
	return models.PlayerAbilities{}, false
}
func (f *fakeAgent) GetGameMode() (models.GameMode, bool) {
	return models.GameModeSurvival, false
}
func (f *fakeAgent) SetFlying(context.Context, bool) error { return nil }

func (f *fakeAgent) StartCamFollow(context.Context, string, float64) error { return nil }
func (f *fakeAgent) StopCamFollow() error                                  { return nil }

// FindNearestBed reports the simulated bed's position, same simple
// contract as the mine-block simulation fields above - one instance,
// visible exactly when the test configured it to be.
func (f *fakeAgent) FindNearestBed(context.Context, int) (models.V3, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.bedFound {
		return models.V3{}, false, nil
	}
	return models.V3{X: f.bedX, Y: f.bedY, Z: f.bedZ}, true, nil
}

// SleepInBed records the call and returns sleepErr if the test configured
// one (simulating the server rejecting/never confirming the attempt),
// nil otherwise (simulating a successful sleep with no further state
// change - this fake has no notion of "currently sleeping" to track).
func (f *fakeAgent) SleepInBed(context.Context, models.V3) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sleepCalls++
	return f.sleepErr
}

// WakeUp just records the call - nothing downstream of it needs
// WakeUp's own effect simulated yet (there is no ActionWakeUp; see
// action.go's own doc comment on why).
func (f *fakeAgent) WakeUp(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wakeUpCalls++
	return nil
}

// GetTimeOfDay implements rlenv.TimeOfDayAgent for tests that need direct
// control over simulated time-of-day (the sleep legality gate, the
// night-to-morning reward) without a real server's own clock.
func (f *fakeAgent) GetTimeOfDay() (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.timeOfDayTicks, f.timeOfDayKnown
}

// FindVisibleBlock reports the simulated mine block's position only when
// its current name matches blockName — mirrors the real implementation's
// contract (finds the nearest visible instance of that specific block name)
// closely enough for rlenv.Environment's dispatch/observation logic to
// exercise for real, without needing a live server.
func (f *fakeAgent) FindVisibleBlock(_ context.Context, blockName string, radius int) (x, y, z float64, found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findVisibleBlockCalls++
	if f.chainBlocks != nil {
		if !f.chainBlocksHidden {
			for pos, name := range f.chainBlocks {
				if name == blockName {
					return float64(pos[0]), float64(pos[1]), float64(pos[2]), true, nil
				}
			}
		}
		return 0, 0, 0, false, nil
	}
	if blockName == "minecraft:crafting_table" {
		f.tableSearchRadii = append(f.tableSearchRadii, radius)
		return 0, 0, 0, f.tableVisible, nil
	}
	if f.mineBlockHidden || f.mineBlockName == "" || f.mineBlockName != blockName {
		return 0, 0, 0, false, nil
	}
	return f.mineBlockX, f.mineBlockY, f.mineBlockZ, true, nil
}

// MineBlockAt "breaks" the simulated mine block if pos matches its current
// position, setting its name to "minecraft:air" — see mineBlockName's doc
// comment for why this actually mutates state rather than just recording
// the call.
func (f *fakeAgent) MineBlockAt(_ context.Context, pos models.V3, _ models.BlockFace) error {
	f.mu.Lock()
	f.mineBlockAtCalls++
	if f.chainBlocks != nil {
		key := [3]int{int(pos.X), int(pos.Y), int(pos.Z)}
		// Removing whatever's tracked there, not just oak_log, so a
		// ChainMine-stage episode's buried block (docs/plans/14-buried-
		// block-mining-and-memory-map.md, mc-rsi-trainer) can be "mined"
		// in this simulation too - chainDrops stays oak_log-specific,
		// feeding only the existing gather-stage log-collection
		// simulation (CollectNearbyItems below), which nothing else
		// needs generalizing for.
		if name, ok := f.chainBlocks[key]; ok {
			delete(f.chainBlocks, key)
			if name == "minecraft:oak_log" {
				f.chainDrops++
			}
		}
		f.mu.Unlock()
		return nil
	}
	err := f.mineBlockAtErr
	if err == nil && int(pos.X) == int(f.mineBlockX) && int(pos.Y) == int(f.mineBlockY) && int(pos.Z) == int(f.mineBlockZ) {
		f.mineBlockName = "minecraft:air"
	}
	f.mu.Unlock()
	return err
}

// BlockNameAt returns the simulated mine block's name if (ix,iy,iz) matches
// its position, "minecraft:air" otherwise — matches the real
// implementation's contract of never erroring, just reporting air for
// anywhere nothing of interest is tracked.
func (f *fakeAgent) BlockNameAt(ix, iy, iz int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chainBlocks != nil {
		if name, ok := f.chainBlocks[[3]int{ix, iy, iz}]; ok {
			return name
		}
		return "minecraft:air"
	}
	if f.mineBlockName != "" && ix == int(f.mineBlockX) && iy == int(f.mineBlockY) && iz == int(f.mineBlockZ) {
		return f.mineBlockName
	}
	return "minecraft:air"
}

// CraftItem "crafts" the simulated target item by incrementing
// craftHeldCount by one, if itemName matches craftTargetName and
// craftIngredientsReady is set — mirrors MineBlockAt's "actually mutates
// state" precedent (see mineBlockName's doc comment) rather than being a
// no-op stub, since Environment's craft dispatch is now exercised for real
// by rlenv's own tests (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1).
func (f *fakeAgent) CraftItem(_ context.Context, itemName string) error {
	f.mu.Lock()
	f.craftItemCalls++
	if f.chainInv != nil {
		defer f.mu.Unlock()
		return f.craftChainLocked(itemName)
	}
	err := f.craftItemErr
	if err == nil && f.craftTargetName != "" && itemName == f.craftTargetName && f.craftIngredientsReady {
		f.craftHeldCount++
	}
	f.mu.Unlock()
	return err
}

// InventoryCount returns the simulated held count of the craft target item
// if itemName matches craftTargetName, 0 otherwise — mirrors the real
// implementation's "0 for anything not tracked" contract.
func (f *fakeAgent) InventoryCount(itemName string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chainInv != nil {
		return f.chainInv[itemName]
	}
	if f.craftTargetName != "" && itemName == f.craftTargetName {
		return f.craftHeldCount
	}
	if f.pickupTargetName != "" && itemName == f.pickupTargetName {
		return f.pickupHeldCount
	}
	if f.placeTargetName != "" && itemName == f.placeTargetName {
		return f.placeHeldCount
	}
	if itemName == "minecraft:gunpowder" {
		return f.gunpowderHeldCount
	}
	return 0
}

// Craftable reports craftIngredientsReady if itemName matches
// craftTargetName, false otherwise — mirrors the real implementation's
// "false for anything not tracked/no known recipe" contract.
func (f *fakeAgent) Craftable(itemName string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.craftTargetName != "" && itemName == f.craftTargetName && f.craftIngredientsReady
}

func (f *fakeAgent) FindAllVisibleEntitiesInSphere(context.Context, float64) ([]models.VisibleEntityInfo, error) {
	return nil, nil
}

// FindNearestVisibleItem reports the simulated pickup item regardless of
// its name — the unfiltered counterpart to FindNearestVisibleItemOfType,
// used by Config.PickupAnyItem's own tests (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 1).
func (f *fakeAgent) FindNearestVisibleItem(_ context.Context, _ float64) (entityID int32, x, y, z float64, found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pickupItemPresent {
		return 0, 0, 0, 0, false, nil
	}
	return pickupItemEntityID, f.pickupItemX, f.pickupItemY, f.pickupItemZ, true, nil
}

// FindNearestVisibleItemOfType reports the simulated pickup item's position
// only when its name matches and it's still present — mirrors
// FindVisibleBlock's "finds the nearest visible instance of that specific
// name" contract closely enough to exercise rlenv's real dispatch/
// observation logic without a live server.
func (f *fakeAgent) FindNearestVisibleItemOfType(_ context.Context, itemName string, _ float64) (entityID int32, x, y, z float64, found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findItemOfTypeCalls++
	if !f.pickupItemPresent || f.pickupTargetName == "" || itemName != f.pickupTargetName {
		return 0, 0, 0, 0, false, nil
	}
	return pickupItemEntityID, f.pickupItemX, f.pickupItemY, f.pickupItemZ, true, nil
}

// pickupItemEntityID is the fixed simulated entity ID FindNearestVisibleItem/
// FindNearestVisibleItemOfType report for the simulated pickup item — also
// recognized by GetEntityHealth, so Config.PickupAnyItem's reward check
// (which asks "is this entity ID still tracked") works against the fake the
// same way it does against the real agent.
const pickupItemEntityID = 1

// --- rlenv.SeedAgent (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4) ---

func (f *fakeAgent) SeedNearbyBlock(_ context.Context, blockName string, radius int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedNearbyBlockCalls = append(f.seedNearbyBlockCalls, seedNearbyBlockCall{blockName: blockName, radius: radius})
	return f.seedErr
}

func (f *fakeAgent) SeedCraftIngredients(_ context.Context, itemName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedCraftIngredientsCalls = append(f.seedCraftIngredientsCalls, itemName)
	return f.seedErr
}

// --- rlenv.ToolSeedAgent (docs/plans/14-buried-block-mining-and-memory-map.md, mc-rsi-trainer) ---

func (f *fakeAgent) SeedGiveItem(_ context.Context, itemName string, count int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedGiveItemCalls = append(f.seedGiveItemCalls, seedGiveItemCall{itemName: itemName, count: count})
	return f.seedErr
}

// fakeEnchantedToolAgent wraps fakeAgent to additionally satisfy
// rlenv.EnchantedToolSeedAgent — a separate type, not a method directly on
// fakeAgent, so existing tests using plain fakeAgent keep exercising
// DefaultEpisodeSeeder's fallback path (an agent that only implements
// ToolSeedAgent) rather than silently gaining the enchanted capability too.
type fakeEnchantedToolAgent struct {
	*fakeAgent
	seedGiveEnchantedItemCalls []seedGiveEnchantedItemCall
}

type seedGiveEnchantedItemCall struct {
	itemName     string
	enchantments map[string]int
	count        int
}

// fakeBuriedTargetAgent wraps fakeAgent to additionally satisfy
// rlenv.BuriedTargetAgent — a separate type for the same reason
// fakeEnchantedToolAgent is: existing tests using plain fakeAgent keep
// exercising refreshMineBuriedExistence's own "agent doesn't support this
// capability" no-op path. exists is mutable directly by the test (not
// computed from any simulated world state) so a test can flip it mid-
// episode to simulate the buried target being destroyed.
type fakeBuriedTargetAgent struct {
	*fakeAgent
	exists                   bool
	blockExistsInRadiusCalls []blockExistsInRadiusCall
}

type blockExistsInRadiusCall struct {
	x, y, z   float64
	blockName string
	radius    int
}

func (f *fakeBuriedTargetAgent) BlockExistsInRadius(_ context.Context, x, y, z float64, blockName string, radius int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blockExistsInRadiusCalls = append(f.blockExistsInRadiusCalls, blockExistsInRadiusCall{x: x, y: y, z: z, blockName: blockName, radius: radius})
	return f.exists, nil
}

// fakeBlocksMinedAgent wraps fakeAgent to additionally satisfy
// rlenv.BlocksMinedCounter — same reason as fakeBuriedTargetAgent: existing
// tests using plain fakeAgent keep exercising the digProgressBonus "agent
// doesn't support this capability" no-op path. count is mutable directly
// by the test (not derived from any simulated mining action) so a test can
// bump it to simulate confirmed block breaks between Step calls.
type fakeBlocksMinedAgent struct {
	*fakeAgent
	count int64
}

func (f *fakeBlocksMinedAgent) BlocksMinedCount() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

func (f *fakeEnchantedToolAgent) SeedGiveEnchantedItem(_ context.Context, itemName string, enchantments map[string]int, count int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedGiveEnchantedItemCalls = append(f.seedGiveEnchantedItemCalls, seedGiveEnchantedItemCall{itemName: itemName, enchantments: enchantments, count: count})
	return f.seedErr
}

// --- rlenv.ResetAgent (Config.ResetOrigin) ---

func (f *fakeAgent) TeleportTo(_ context.Context, x, y, z float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.teleportCalls = append(f.teleportCalls, teleportCall{x: x, y: y, z: z})
	if f.teleportFailFirstN > 0 {
		f.teleportFailFirstN--
		return errors.New("fakeAgent: forced transient TeleportTo failure")
	}
	if f.teleportErr != nil {
		return f.teleportErr
	}
	f.x, f.y, f.z = x, y, z
	f.posKnown = true
	return nil
}

// SendCommand satisfies models.CommandAgent (which rlenv's ResetAgent/
// seeding paths now require); the fake never needs to observe commands.
func (f *fakeAgent) SendCommand(string) error { return nil }

// restoredBlock records one RestoreBlock call.
type restoredBlock struct {
	x, y, z int
	name    string
}

// RestoreBlock satisfies rlenv.BlockRestorer, recording each call.
func (f *fakeAgent) RestoreBlock(_ context.Context, x, y, z int, blockName string) error {
	f.mu.Lock()
	f.restoredBlocks = append(f.restoredBlocks, restoredBlock{x, y, z, blockName})
	if f.chainBlocks != nil {
		if blockName == "minecraft:air" {
			delete(f.chainBlocks, [3]int{x, y, z})
		} else {
			f.chainBlocks[[3]int{x, y, z}] = blockName
		}
	}
	f.mu.Unlock()
	return nil
}

// clearedBox records one ClearAir call.
type clearedBox struct{ x1, y1, z1, x2, y2, z2 int }

// ClearAir satisfies rlenv.AreaClearer, recording each call.
func (f *fakeAgent) ClearAir(_ context.Context, x1, y1, z1, x2, y2, z2 int) error {
	f.mu.Lock()
	f.clearedBoxes = append(f.clearedBoxes, clearedBox{x1, y1, z1, x2, y2, z2})
	f.mu.Unlock()
	return nil
}

// clonedArea records one CloneArea call.
type clonedArea struct{ srcX1, srcY1, srcZ1, srcX2, srcY2, srcZ2, dstX1, dstY1, dstZ1 int }

// CloneArea satisfies rlenv.AreaCloner, recording each call.
func (f *fakeAgent) CloneArea(_ context.Context, srcX1, srcY1, srcZ1, srcX2, srcY2, srcZ2, dstX1, dstY1, dstZ1 int) error {
	f.mu.Lock()
	f.clonedAreas = append(f.clonedAreas, clonedArea{srcX1, srcY1, srcZ1, srcX2, srcY2, srcZ2, dstX1, dstY1, dstZ1})
	f.mu.Unlock()
	return nil
}

// farSeedCall records one SeedBlockAt / SeedCraftIngredientsAt call.
type farSeedCall struct {
	name    string // block or item
	x, y, z int
}

// SeedBlockAt and SeedCraftIngredientsAt satisfy rlenv.FarSeedAgent.
func (f *fakeAgent) SeedBlockAt(_ context.Context, blockName string, x, y, z int) error {
	f.mu.Lock()
	f.seedBlockAtCalls = append(f.seedBlockAtCalls, farSeedCall{blockName, x, y, z})
	if f.chainBlocks != nil {
		f.chainBlocks[[3]int{x, y, z}] = blockName
	}
	f.mu.Unlock()
	return nil
}

func (f *fakeAgent) SeedCraftIngredientsAt(_ context.Context, itemName string, x, y, z int) error {
	f.mu.Lock()
	f.seedCraftAtCalls = append(f.seedCraftAtCalls, farSeedCall{itemName, x, y, z})
	f.mu.Unlock()
	return nil
}

// CollectNearbyItems makes fakeAgent a models.ItemCollector; it only records
// that a collect was asked for.
func (f *fakeAgent) CollectNearbyItems(context.Context, float64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.collectDropsCalls++
	if f.chainInv != nil {
		n := f.chainDrops
		f.chainInv["minecraft:oak_log"] += n
		f.chainDrops = 0
		return n, nil
	}
	return 1, nil
}

// craftChainLocked applies the real recipes the chain uses; the caller holds mu.
func (f *fakeAgent) craftChainLocked(item string) error {
	take := func(name string, n int) bool {
		if f.chainInv[name] < n {
			return false
		}
		f.chainInv[name] -= n
		return true
	}
	switch item {
	case "minecraft:oak_planks":
		if !take("minecraft:oak_log", 1) {
			return errors.New("missing ingredient: log")
		}
		f.chainInv[item] += 4
	case "minecraft:crafting_table":
		if !take("minecraft:oak_planks", 4) {
			return errors.New("missing ingredient: planks")
		}
		f.chainInv[item]++
	case "minecraft:stick":
		if !take("minecraft:oak_planks", 2) {
			return errors.New("missing ingredient: planks")
		}
		f.chainInv[item] += 4
	case "minecraft:bowl", "minecraft:chest", "minecraft:wooden_pickaxe":
		if !f.chainTablePlacedLocked() {
			return errors.New("no crafting table found within 32 blocks")
		}
		cost := map[string]int{"minecraft:bowl": 3, "minecraft:chest": 8, "minecraft:wooden_pickaxe": 3}[item]
		if item == "minecraft:wooden_pickaxe" && f.chainInv["minecraft:stick"] < 2 {
			return errors.New("missing ingredient: sticks")
		}
		if !take("minecraft:oak_planks", cost) {
			return errors.New("missing ingredient: planks")
		}
		if item == "minecraft:wooden_pickaxe" {
			f.chainInv["minecraft:stick"] -= 2
		}
		f.chainInv[item]++
	default:
		return errors.New("unknown recipe " + item)
	}
	return nil
}

func (f *fakeAgent) chainTablePlacedLocked() bool {
	for _, name := range f.chainBlocks {
		if name == "minecraft:crafting_table" {
			return true
		}
	}
	return false
}

// PlaceHeldBlock makes fakeAgent a models.BlockPlacer: it puts the table
// one block +Z of the bot, unless failNextPlace asks for a failure.
func (f *fakeAgent) PlaceHeldBlock(_ context.Context, itemName string) (models.V3, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNextPlace {
		f.failNextPlace = false
		return models.V3{}, errors.New("the block never appeared")
	}
	if f.chainInv == nil {
		// Standalone mode (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
		// item 2) — chainBlocks bookkeeping doesn't apply outside a chain
		// episode, so this only needs to move the held count, which is
		// all the standalone reward check (InventoryCount decrease) reads.
		if f.placeTargetName == "" || itemName != f.placeTargetName || f.placeHeldCount == 0 {
			return models.V3{}, errors.New("none in the inventory")
		}
		f.placeHeldCount--
		return models.V3{X: f.x, Y: f.y, Z: f.z + 1}, nil
	}
	if f.chainInv[itemName] == 0 {
		return models.V3{}, errors.New("none in the inventory")
	}
	f.chainInv[itemName]--
	cell := [3]int{int(f.x), int(f.y), int(f.z) + 1}
	f.chainBlocks[cell] = itemName
	if f.placeThenFail {
		f.placeThenFail = false
		return models.V3{}, errors.New("the block never appeared")
	}
	return models.V3{X: float64(cell[0]), Y: float64(cell[1]), Z: float64(cell[2])}, nil
}

// PlaceBlockAt makes fakeAgent satisfy models.CommandAgent's PlaceBlockAt:
// records itemName at pos in the same chainBlocks bookkeeping PlaceHeldBlock
// uses, unless failNextPlace asks for a failure. No test currently exercises
// arbitrary-position placement (fakeAgent's existing callers only use
// PlaceHeldBlock's bot-relative placement), so this is a minimal, honest
// stub to satisfy the interface rather than a fully-modeled implementation.
func (f *fakeAgent) PlaceBlockAt(_ context.Context, pos models.V3, itemName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNextPlace {
		f.failNextPlace = false
		return errors.New("the block never appeared")
	}
	if f.chainInv == nil || f.chainInv[itemName] == 0 {
		return errors.New("none in the inventory")
	}
	f.chainInv[itemName]--
	cell := [3]int{int(pos.X), int(pos.Y), int(pos.Z)}
	f.chainBlocks[cell] = itemName
	return nil
}

// BuildStructure makes fakeAgent satisfy models.CommandAgent's
// BuildStructure. No test currently exercises it (nothing in rlenv drives
// structure loading), so this is a minimal stub - not a fake file loader -
// that simply reports no file support, satisfying the interface without
// pretending to implement behavior nothing here exercises or verifies.
func (f *fakeAgent) BuildStructure(_ context.Context, _ string, _ models.V3) (models.BuildStructureResult, error) {
	return models.BuildStructureResult{}, errors.New("fakeAgent does not support BuildStructure")
}

// UsePortal mirrors the real agent's own shape: resolve via
// FindNearestPortal, then delegate to UsePortalAt. Simulated, not stubbed
// out, now that ActionUsePortal actually dispatches through this path
// (see refreshPortalTarget/resolveDispatch).
func (f *fakeAgent) UsePortal(ctx context.Context, maxDistance int) error {
	pos, found, err := f.FindNearestPortal(ctx, maxDistance)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no portal block found within %d blocks", maxDistance)
	}
	return f.UsePortalAt(ctx, pos)
}

// FindNearestPortal reports portalX/Y/Z/portalFound, mirroring
// FindNearestBed's own shape exactly.
func (f *fakeAgent) FindNearestPortal(context.Context, int) (models.V3, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.portalFound {
		return models.V3{}, false, nil
	}
	return models.V3{X: f.portalX, Y: f.portalY, Z: f.portalZ}, true, nil
}

// UsePortalAt records the call and returns usePortalAtErr if the test
// configured one (simulating the dimension never changing), nil otherwise
// (simulating a successful transfer with no further state change - this
// fake has no notion of "currently in a different dimension" to track).
// Mirrors SleepInBed's own shape exactly.
func (f *fakeAgent) UsePortalAt(context.Context, models.V3) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usePortalAtCalls++
	return f.usePortalAtErr
}

// GetDimension reports the Overworld, uninitialized, same as a real agent
// before its first Login/Respawn. No test currently needs this fake to
// simulate an actual dimension change (ActionUsePortal has no reward
// component of its own yet - see action.go's own doc comment), so this
// stays a minimal, honest stub rather than tracked state.
func (f *fakeAgent) GetDimension() (dimensionTypeID int32, dimensionName string, initialized bool) {
	return 0, "", false
}

// ClearInventory and ClearDroppedItems make fakeAgent a rlenv.ChainSeedAgent.
func (f *fakeAgent) ClearInventory(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clearedInventory++
	if f.chainInv != nil {
		for k := range f.chainInv {
			delete(f.chainInv, k)
		}
	}
	return nil
}

func (f *fakeAgent) ClearDroppedItems(_ context.Context, x, y, z, radius int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clearedDropAreas = append(f.clearedDropAreas, [4]int{x, y, z, radius})
	f.chainDrops = 0
	return nil
}
