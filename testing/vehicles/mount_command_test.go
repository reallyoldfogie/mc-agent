package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestChatCommand_MountByEntityType verifies that the "mount" chat command
// accepts an entity type name (e.g. "mount horse") in addition to a raw
// numeric entity ID, resolving to the nearest matching entity - closing
// the usability gap where a chat user would otherwise need to already know
// a target's numeric ID before they could mount it. Sent as a real chat
// message via RCON `say`, exercising the actual command pipeline rather
// than calling MountNearest directly.
func TestChatCommand_MountByEntityType(t *testing.T) {
	const botName = "MountCmdBot"
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, botName)
			defer cleanup()

			// Build on the agent's actual (natural) spawn position rather
			// than teleporting to an arbitrary coordinate - confirmed live
			// that "100 65 100" isn't solid ground in this world config,
			// which silently dropped an earlier version of this test into
			// a void, killed it, and respawned it far from the summoned
			// horse.
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
		})
	}
}
