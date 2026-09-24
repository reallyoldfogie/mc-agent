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
	moveToWithChatCalls int

	// Mine simulation: a single block instance at (mineBlockX,Y,Z) whose
	// name is mineBlockName ("" means nothing is there). FindVisibleBlock
	// only reports it found when the queried name matches; MineBlockAt
	// "breaks" it by setting mineBlockName to "minecraft:air", so
	// Environment's before/after BlockNameAt diff (see environment.go's
	// Step) sees a real change, the same way MoveToWithChat's fake actually
	// moves the tracked position rather than just recording the call.
	mineBlockName                      string
	mineBlockX, mineBlockY, mineBlockZ float64
	mineBlockAtErr                     error
	mineBlockAtCalls                   int
	findVisibleBlockCalls              int

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
func (f *fakeAgent) MoveTo(_ context.Context, x, y, z float64, _ bool) error {
	f.mu.Lock()
	f.moveToWithChatCalls++
	err := f.moveToWithChatErr
	delay := f.moveToWithChatDelay
	f.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
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

// FindVisibleBlock reports the simulated mine block's position only when
// its current name matches blockName — mirrors the real implementation's
// contract (finds the nearest visible instance of that specific block name)
// closely enough for rlenv.Environment's dispatch/observation logic to
// exercise for real, without needing a live server.
func (f *fakeAgent) FindVisibleBlock(_ context.Context, blockName string, _ int) (x, y, z float64, found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findVisibleBlockCalls++
	if f.mineBlockName == "" || f.mineBlockName != blockName {
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
