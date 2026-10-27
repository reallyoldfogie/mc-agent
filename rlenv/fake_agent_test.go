package rlenv_test

import (
	"context"
	"errors"
	"sync"
	"time"

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
	timeOfDayKnown   bool

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
	mineBlockHidden bool
	collectDropsCalls                  int

	// Chain simulation (chain_test's env tests): when chainInv is non-nil the
	// fake models a tiny world of blocks and an inventory with the real
	// recipes, instead of the single mine block / craft target above.
	chainInv         map[string]int
	chainBlocks      map[[3]int]string
	// chainBlocksHidden mirrors mineBlockHidden's own role (see its doc
	// comment), but for the chainBlocks-backed FindVisibleBlock path: a
	// block can be present in chainBlocks (so BlockNameAt/MineBlockAt
	// still resolve it) while this makes FindVisibleBlock report it as
	// not found, simulating a buried target that hasn't been glimpsed yet.
	chainBlocksHidden bool
	chainDrops        int
	clearedInventory int
	clearedDropAreas [][4]int
	failNextPlace    bool
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
func (f *fakeAgent) IsGliding() bool                         { return false }

func (f *fakeAgent) MoveForward(context.Context, float64) error    { return nil }
func (f *fakeAgent) MoveUp(context.Context, float64) error         { return nil }
func (f *fakeAgent) MoveUpAndSneak(context.Context, float64) error { return nil }
func (f *fakeAgent) MoveToAndSneak(context.Context, float64, float64, float64) error {
	return nil
}
func (f *fakeAgent) LineToAndSneak(context.Context, float64, float64, float64) error {
	return nil
}
func (f *fakeAgent) StartSneaking() error { return nil }
func (f *fakeAgent) StopSneaking() error  { return nil }
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
func (f *fakeAgent) Follow(context.Context, string) error                    { return nil }
func (f *fakeAgent) StopFollow(context.Context) error                        { return nil }
func (f *fakeAgent) FollowStatus(context.Context) string                     { return "" }

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
func (f *fakeAgent) HasFollowManager() bool { return false }
func (f *fakeAgent) IsFollowing() bool      { return false }
func (f *fakeAgent) IsSprinting() bool      { return false }
func (f *fakeAgent) PlanStatus(context.Context) models.PlanStatus {
	return models.PlanStatus{}
}
func (f *fakeAgent) StopPlan(context.Context) error { return nil }
func (f *fakeAgent) FireBow(context.Context) error  { return nil }
func (f *fakeAgent) FireBowAt(context.Context, float64, float64, float64, ...models.ProjectileHitCallback) ([]models.TrajectoryPoint, error) {
	return nil, nil
}
func (f *fakeAgent) MountEntity(context.Context, int32) error   { return nil }
func (f *fakeAgent) MountNearest(context.Context, string) error { return nil }
func (f *fakeAgent) DismountEntity() error                      { return nil }
func (f *fakeAgent) JumpVehicle(context.Context, int32) error   { return nil }
func (f *fakeAgent) Equip(context.Context, string) error        { return nil }
func (f *fakeAgent) UseItem(context.Context, models.Hand) error { return nil }
func (f *fakeAgent) FlyTo(context.Context, float64, float64, float64) error {
	return nil
}
func (f *fakeAgent) NearestPlayerInfo(context.Context, bool) (models.NearestPlayerInfo, bool) {
	return models.NearestPlayerInfo{}, false
}
func (f *fakeAgent) FindPlayerByName(context.Context, string) (x, y, z float64, found bool, err error) {
	return 0, 0, 0, false, nil
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

func (f *fakeAgent) FindNearestVisibleItem(context.Context, float64) (entityID int32, x, y, z float64, found bool, err error) {
	return 0, 0, 0, 0, false, nil
}

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
	if f.chainInv == nil || f.chainInv[itemName] == 0 {
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
