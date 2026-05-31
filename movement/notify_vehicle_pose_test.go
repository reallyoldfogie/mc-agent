package movement

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotifyVehiclePose_AppliesSitAndStandToCamel verifies that server-reported
// pose updates flow into the executor's CamelState via the translation rules
// in NotifyVehiclePose. We bypass SetMounted's auto-init because that path
// requires a MountedEntityPositionGetter (covered separately); here we want
// the pose-translation logic isolated from the mount lifecycle.
func TestNotifyVehiclePose_AppliesSitAndStandToCamel(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Manually establish the mount + camel state so we don't depend on the
	// position-getter wiring just to exercise the pose handler.
	exec.mountedEntityMu.Lock()
	exec.mountedEntityID = 99
	exec.camelState = models.NewCamelState(0, false)
	exec.mountedEntityMu.Unlock()

	require.False(t, exec.camelState.IsSitting(), "fresh CamelState should start standing")

	// Server reports sitting → CamelState should transition.
	exec.NotifyVehiclePose(99, "sitting", 10)
	assert.True(t, exec.camelState.IsSitting(), "NotifyVehiclePose(sitting) should drive CamelState into sitting")

	// Server reports standing → CamelState should transition back.
	exec.NotifyVehiclePose(99, "standing", 0)
	assert.False(t, exec.camelState.IsSitting(), "NotifyVehiclePose(standing) should drive CamelState back to standing")

	// Irrelevant pose names are no-ops (camel doesn't use SWIMMING etc.).
	exec.NotifyVehiclePose(99, "swimming", 3)
	assert.False(t, exec.camelState.IsSitting())
}

func TestNotifyVehiclePose_IgnoredForWrongEntity(t *testing.T) {
	exec := createTestPhysicsExecutor()
	exec.mountedEntityMu.Lock()
	exec.mountedEntityID = 99
	exec.camelState = models.NewCamelState(0, false)
	exec.mountedEntityMu.Unlock()

	// Mount is entity 99; pose update for a different entity (a nearby camel)
	// must NOT alter our own mount's state.
	exec.NotifyVehiclePose(123, "sitting", 10)
	assert.False(t, exec.camelState.IsSitting())
}

func TestNotifyVehiclePose_NoCamelStateIsNoOp(t *testing.T) {
	exec := createTestPhysicsExecutor()
	exec.mountedEntityMu.Lock()
	exec.mountedEntityID = 99
	exec.camelState = nil
	exec.mountedEntityMu.Unlock()

	// Should not panic, should not crash; nothing to assert beyond "doesn't blow up".
	exec.NotifyVehiclePose(99, "sitting", 10)
}
