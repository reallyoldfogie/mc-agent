package movement

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
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
