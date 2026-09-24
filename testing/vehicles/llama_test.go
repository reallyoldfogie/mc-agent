package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// LlamaSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type LlamaSuite struct {
	testingpkg.VersionWorldSuite
}

func TestLlamaSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &LlamaSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// Llamas can be mounted and ridden, but take no saddle and cannot be steered:
// Java's LlamaEntity.getControllingPassenger() never returns the rider, so the
// llama keeps running its own mob AI and the client is never its movement
// authority.
//
// Before llamas were registered as their own vehicle type they fell through the
// riding dispatch to the horse handler, which steered them, applied forward
// thrust, ran the jump-charge state machine and shipped a predicted VehicleMove
// for a server-controlled entity. These tests pin the corrected behaviour.

// TestLlamaMounting verifies the basic mount/dismount cycle on a llama.
func (s *LlamaSuite) TestLlamaMounting() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("LlamaMountBot", "llama_mounting")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	startX, startY, startZ := pos.X, pos.Y, pos.Z

	// Fence it in: a llama wanders under its own AI and can otherwise
	// drift out of interact range before we mount.
	require.NoError(t, helper.BuildHorseEnclosure(ctx, startX, startY, startZ))

	llamaID, err := helper.SummonLlama(ctx, startX, startY, startZ, 0)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, helper.MountEntity(ctx, llamaID))
	require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

	require.NoError(t, helper.DismountEntity())
	require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))

	require.NoError(t, helper.RemoveHorseEnclosure(ctx, startX, startY, startZ))
}

// TestLlamaIsPassivePassenger verifies the passive-passenger contract: rider
// input is applied to nothing, and no local velocity accumulates.
//
// Displacement is deliberately NOT asserted. A llama moves under its own AI, so
// "did it move" cannot distinguish our steering from its wandering. What does
// distinguish them is the drag multiplier: every driving handler sets it to a
// real per-tick value, while the passive-rider handler zeroes it.
func (s *LlamaSuite) TestLlamaIsPassivePassenger() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("LlamaPassBot", "llama_is_passive_passenger")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	startX, startY, startZ := pos.X, pos.Y, pos.Z

	require.NoError(t, helper.BuildHorseEnclosure(ctx, startX, startY, startZ))

	llamaID, err := helper.SummonLlama(ctx, startX, startY, startZ, 0)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, helper.MountEntity(ctx, llamaID))
	require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

	require.NoError(t, helper.EnterManualMode())

	// Apply full forward throttle and steering. Both must be ignored.
	helper.SetManualThrottle(1.0, 1.0)
	time.Sleep(2 * time.Second)

	inspector, ok := helper.GetRidingPhysicsInspector()
	require.True(t, ok, "agent should expose RidingPhysicsInspector")

	dragMultiplier := inspector.GetRidingDragMultiplier()
	velX, velZ := inspector.GetRidingVelocity()
	t.Logf("Llama after 2s of throttle + steering: drag=%.4f vel=(%.4f, %.4f)", dragMultiplier, velX, velZ)

	assert.Zero(t, dragMultiplier,
		"passive-rider handler zeroes the drag multiplier; a non-zero value means a driving handler ran for a llama")
	assert.Zero(t, velX, "a passive rider accumulates no velocity")
	assert.Zero(t, velZ, "a passive rider accumulates no velocity")

	require.NoError(t, helper.ExitManualMode())
	require.NoError(t, helper.DismountEntity())
	require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
	require.NoError(t, helper.RemoveHorseEnclosure(ctx, startX, startY, startZ))
}

// TestLlamaPositionFollowsServer verifies the other half of the passive contract:
// because we do not predict the llama's movement, the agent's position must
// track whatever the server reports for the llama rather than diverging from it.
//
// This is the assertion that would have caught the old behaviour, where the
// horse handler predicted a position and sent it as a VehicleMove, pulling the
// agent's idea of where it was away from the server's.
func (s *LlamaSuite) TestLlamaPositionFollowsServer() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("LlamaSyncBot", "llama_position_follows_server")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	startX, startY, startZ := pos.X, pos.Y, pos.Z

	llamaID, err := helper.SummonLlama(ctx, startX, startY, startZ, 0)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, helper.MountEntity(ctx, llamaID))
	require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))

	require.NoError(t, helper.EnterManualMode())
	// Push forward the whole time. If anything is predicting movement,
	// this is what makes the agent's position run away from the llama's.
	helper.SetManualThrottle(0, 1.0)

	// Sample repeatedly: a single check could pass by luck if the llama
	// happens to be standing still at that instant.
	const samples = 8
	maxDrift := 0.0
	for range samples {
		time.Sleep(400 * time.Millisecond)

		llama := helper.GetTrackedEntity(llamaID)
		if llama == nil {
			continue // Entity not tracked this tick; nothing to compare.
		}
		agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()

		drift := GetDistance(agentPos.X, 0, agentPos.Z, llama.X, 0, llama.Z)
		if drift > maxDrift {
			maxDrift = drift
		}
		t.Logf("agent=(%.2f, %.2f) llama=(%.2f, %.2f) drift=%.2f",
			agentPos.X, agentPos.Z, llama.X, llama.Z, drift)
	}

	// The tolerance covers ordinary interpolation lag between the entity
	// tracker and the rider position, not prediction. Sustained
	// client-side prediction over ~3 seconds at horse speed would put the
	// agent many blocks away.
	assert.Less(t, maxDrift, 3.0,
		"a passive rider must follow the llama's server-reported position; large drift means something is predicting movement")

	require.NoError(t, helper.ExitManualMode())
	require.NoError(t, helper.DismountEntity())
	require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))
}
