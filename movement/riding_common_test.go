package movement

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
)

// fakeAttributeGetter is a minimal models.MountedEntityPositionGetter double
// for exercising resolveMountMovementSpeed's fallback ordering in isolation,
// without needing a live server or the rest of the riding-handler machinery.
// Only the two attribute methods are meaningfully implemented; every other
// method is a stub since resolveMountMovementSpeed never calls them.
type fakeAttributeGetter struct {
	liveValue    float64
	liveFound    bool
	defaultValue float64
	defaultFound bool
}

func (f *fakeAttributeGetter) GetMountedEntityPosition(int32) (float64, float64, float64, bool) {
	return 0, 0, 0, false
}
func (f *fakeAttributeGetter) GetMountedEntityYaw(int32) (float64, bool) { return 0, false }
func (f *fakeAttributeGetter) GetMountedEntityType(int32) (int32, bool)  { return 0, false }
func (f *fakeAttributeGetter) IsMountedEntityBoat(int32) bool            { return false }
func (f *fakeAttributeGetter) IsMountedEntityMinecart(int32) bool        { return false }
func (f *fakeAttributeGetter) IsMountedEntityCamel(int32) bool           { return false }
func (f *fakeAttributeGetter) IsMountedEntityNautilus(int32) bool        { return false }
func (f *fakeAttributeGetter) IsMountedEntityZombieNautilus(int32) bool  { return false }
func (f *fakeAttributeGetter) IsMountedEntityPig(int32) bool             { return false }
func (f *fakeAttributeGetter) IsMountedEntityStrider(int32) bool         { return false }
func (f *fakeAttributeGetter) IsMountedEntityDonkey(int32) bool          { return false }
func (f *fakeAttributeGetter) IsMountedEntityMule(int32) bool            { return false }
func (f *fakeAttributeGetter) IsMountedEntityLlama(int32) bool           { return false }
func (f *fakeAttributeGetter) IsMountedEntityHappyGhast(int32) bool      { return false }
func (f *fakeAttributeGetter) IsMountedEntityHappyGhastStayingStill(int32) (bool, bool) {
	return false, false
}
func (f *fakeAttributeGetter) IsMountedEntityHarnessed(int32) (bool, bool) { return false, false }
func (f *fakeAttributeGetter) GetMountedPassengerIndex() int               { return -1 }
func (f *fakeAttributeGetter) IsMountedEntitySaddled(int32) (bool, bool)   { return false, false }
func (f *fakeAttributeGetter) GetEntityVelocity(int32) (float64, float64, float64, bool) {
	return 0, 0, 0, false
}
func (f *fakeAttributeGetter) GetRiderHeldItem() (string, bool) { return "", false }

func (f *fakeAttributeGetter) GetOwnActiveEffect(string) (int32, bool) { return 0, false }

func (f *fakeAttributeGetter) GetOwnEquippedChestItem() (string, bool) { return "", false }

func (f *fakeAttributeGetter) GetOwnEquippedFeetItem() (string, bool) { return "", false }

func (f *fakeAttributeGetter) HasActiveFireworkBoost() bool { return false }

func (f *fakeAttributeGetter) GetOwnFlying() (bool, float64) { return false, 0 }

func (f *fakeAttributeGetter) IsSpectator() bool { return false }

func (f *fakeAttributeGetter) GetEntityAttribute(int32, string) (float64, bool) {
	return f.liveValue, f.liveFound
}

func (f *fakeAttributeGetter) GetEntityAttributeDefault(int32, string) (float64, bool) {
	return f.defaultValue, f.defaultFound
}

var _ models.MountedEntityPositionGetter = (*fakeAttributeGetter)(nil)

func TestResolveMountMovementSpeed_LiveWinsOverEverything(t *testing.T) {
	getter := &fakeAttributeGetter{
		liveValue: 0.3, liveFound: true,
		defaultValue: 0.2, defaultFound: true,
	}
	got := resolveMountMovementSpeed(getter, 1, "generic.movement_speed", 0.1)
	assert.Equal(t, 0.3, got)
}

func TestResolveMountMovementSpeed_DefaultWinsOverHardcoded(t *testing.T) {
	getter := &fakeAttributeGetter{
		liveFound:    false,
		defaultValue: 0.175, defaultFound: true,
	}
	got := resolveMountMovementSpeed(getter, 1, "generic.movement_speed", 0.225)
	assert.Equal(t, 0.175, got, "donkey's own data-driven default must win over horse's hardcoded literal")
}

func TestResolveMountMovementSpeed_HardcodedIsLastResort(t *testing.T) {
	getter := &fakeAttributeGetter{liveFound: false, defaultFound: false}
	got := resolveMountMovementSpeed(getter, 1, "generic.movement_speed", 0.225)
	assert.Equal(t, 0.225, got)
}

func TestResolveMountMovementSpeed_NilGetterUsesHardcoded(t *testing.T) {
	got := resolveMountMovementSpeed(nil, 1, "generic.movement_speed", 0.225)
	assert.Equal(t, 0.225, got)
}

// These tests exercise the water/lava/strider-cold physics formulas
// extracted from computeRidingWaterPhysics/computeRidingLavaPhysics/
// computeStriderColdState — the version-gating and surface-classification
// decisions those functions make, isolated from the pe.world/shapeProvider
// I/O that made them previously untestable without a live server.

func TestShouldApplyOldWaterSinkingBehavior(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{name: "pre-1.21.11 sinks", version: "1.21.10", expected: true},
		{name: "well before the change", version: "1.21.1", expected: true},
		{name: "1.21.11 floats", version: "1.21.11", expected: false},
		{name: "26.1 floats", version: "26.1", expected: false},
		{name: "future version floats", version: "1.22.0", expected: false},
		{name: "unparseable version defaults to sinking", version: "not-a-version", expected: true},
		{name: "empty version defaults to sinking", version: "", expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, shouldApplyOldWaterSinkingBehavior(tt.version))
		})
	}
}

func TestRidingWaterDragAndGravity(t *testing.T) {
	sinkDrag, sinkGravity := ridingWaterDragAndGravity(true)
	assert.Equal(t, physics.RideableInWaterDragMultiplier, sinkDrag)
	assert.Equal(t, -physics.RideableInWaterGravity, sinkGravity)

	floatDrag, floatGravity := ridingWaterDragAndGravity(false)
	assert.Equal(t, 0.5, floatDrag)
	assert.Zero(t, floatGravity)
}

func TestClassifyLavaSurface(t *testing.T) {
	tests := []struct {
		name           string
		isLavaBelow    bool
		isLavaAtEntity bool
		expected       ridingLavaPhysicsParams
	}{
		{name: "no lava at all", isLavaBelow: false, isLavaAtEntity: false, expected: ridingLavaPhysicsParams{}},
		{
			name:        "no lava at all, but somehow lava at entity without lava below is still not-lava-below",
			isLavaBelow: false, isLavaAtEntity: true,
			expected: ridingLavaPhysicsParams{},
		},
		{
			name: "floating on surface", isLavaBelow: true, isLavaAtEntity: false,
			expected: ridingLavaPhysicsParams{IsOnLava: true},
		},
		{
			name: "submerged", isLavaBelow: true, isLavaAtEntity: true,
			expected: ridingLavaPhysicsParams{IsSubmergedInLava: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifyLavaSurface(tt.isLavaBelow, tt.isLavaAtEntity))
		})
	}
}

func TestStriderIsCold(t *testing.T) {
	assert.False(t, striderIsCold(true, false), "warm and no cold parent should not be cold")
	assert.True(t, striderIsCold(false, false), "not warm should be cold")
	assert.True(t, striderIsCold(true, true), "warm but cold parent should still be cold")
	assert.True(t, striderIsCold(false, true), "not warm and cold parent should be cold")
}
