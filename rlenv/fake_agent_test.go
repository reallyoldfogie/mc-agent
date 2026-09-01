package rlenv_test

import (
	"context"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// fakeAgent is a minimal, in-memory models.CommandAgent + models.HealthProvider
// for testing rlenv.Environment without a live server, per
// RL_POLICY_INTEGRATION_PLAN.md's verification requirement ("unit-testable
// without a live server"). MoveToWithChat is the only method Environment's
// tests actually exercise the effect of; every other CommandAgent method
// exists solely so fakeAgent satisfies the interface actions.NewRegistry()'s
// real actions (in particular actions.MoveTo) require, matching how the real
// dispatch path is exercised end to end in environment_test.go rather than
// mocking the registry away.
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
func (f *fakeAgent) FindPath(context.Context, float64, float64, float64) (*models.Path, error) {
	return nil, nil
}
func (f *fakeAgent) ExecutePath(context.Context, *models.Path) error         { return nil }
func (f *fakeAgent) LookAt(context.Context, float64, float64, float64) error { return nil }
func (f *fakeAgent) Follow(context.Context, string) error                    { return nil }
func (f *fakeAgent) StopFollow(context.Context) error                        { return nil }
func (f *fakeAgent) FollowStatus(context.Context) string                     { return "" }

// --- models.CommandAgent (beyond MovementAgent/ChatOperations) ---

// MoveToWithChat simulates instant arrival: production movement is real
// pathfinding over many ticks, but this fake only needs to prove
// rlenv.Environment reacts correctly to *some* position change reaching the
// target, not to simulate pathfinding itself.
func (f *fakeAgent) MoveToWithChat(_ context.Context, x, y, z float64) error {
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
