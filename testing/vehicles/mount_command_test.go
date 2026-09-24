package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MountCommandSuite is a version-parameterized suite: one shared flat-world
// server per version instead of one per test.
type MountCommandSuite struct {
	testingpkg.VersionWorldSuite
}

func TestMountCommandSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &MountCommandSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestChatCommand_MountByEntityType verifies that the "mount" chat command
// accepts an entity type name (e.g. "mount horse") in addition to a raw
// numeric entity ID, resolving to the nearest matching entity - closing
// the usability gap where a chat user would otherwise need to already know
// a target's numeric ID before they could mount it. Sent as a real chat
// message via RCON `say`, exercising the actual command pipeline rather
// than calling MountNearest directly.
func (s *MountCommandSuite) TestChatCommand_MountByEntityType() {
	const botName = "MountCmdBot"
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent(botName, "mount_by_entity_type")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Build on the agent's actual position (its own working-area origin)
	// rather than teleporting to an arbitrary coordinate - confirmed live
	// that "100 65 100" isn't solid ground in this world config, which
	// silently dropped an earlier version of this test into a void, killed
	// it, and respawned it far from the summoned horse.
	pos, ok := helper.ManagedAgent.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	x, y, z := pos.X, pos.Y, pos.Z

	// NoAI so the horse can't wander out of interact range before
	// the chat command resolves and mounts it.
	_, err := helper.SummonHorse(ctx, x, y, z+3, 90, WithNoAI())
	require.NoError(t, err, "summon horse")
	time.Sleep(500 * time.Millisecond)

	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("say >>>%s<<< mount horse", botName))
	require.NoError(t, err, "send mount chat command")

	require.NoError(t, helper.WaitForMounted(ctx, 10*time.Second), "agent should be mounted via chat command")

	require.NoError(t, helper.EnableEntityAI(ctx, "minecraft:horse"), "re-enable horse AI")
}
