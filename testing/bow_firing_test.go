package testing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTargetMechanism places:
// - a target block (to be hit by arrow)
// - a piston adjacent to the target, facing away from it
// - a glowstone block in front of the piston to be pushed
// Returns the original glowstone coordinates.
func setupTargetMechanism(ctx context.Context, t *testing.T, rcon testenv.RCONHelper, targetX, targetY, targetZ int) (glowstoneX, glowstoneY, glowstoneZ int, err error) {
	platformY := targetY - 1 // Platform is below target
	platformX := targetX - 10
	platformZ := targetZ - 10

	// Build the platform first so agent doesn't fall
	BuildPlatform(ctx, rcon, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")

	// Clear area above the platform
	if err := ClearArea(ctx, rcon,
		platformX, platformY+1, platformZ,
		platformX+20, platformY+5, platformZ+20); err != nil {
		t.Logf("warning: failed to clear area: %v", err)
	}

	// Target block at (targetX, targetY, targetZ)
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:target`, targetX, targetY, targetZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("set target: %w", err)
	}

	// Normal piston directly east of target, facing west (toward the target)
	pistonX, pistonY, pistonZ := targetX+1, targetY, targetZ
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:piston[facing=west]`, pistonX, pistonY, pistonZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("set piston: %w", err)
	}

	// Glowstone two blocks east of target (one in front of the piston head)
	glowstoneX, glowstoneY, glowstoneZ = pistonX+1, pistonY, pistonZ
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, glowstoneX, glowstoneY, glowstoneZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("set glowstone: %w", err)
	}

	// Ensure space for piston to push (clear the block one more east)
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:air`, glowstoneX+1, glowstoneY, glowstoneZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("clear space: %w", err)
	}
	return glowstoneX, glowstoneY, glowstoneZ, nil
}

// verifyBlockAtPosition returns true if block is at (gx,gy,gz).
// Uses Java Edition-friendly execute-if/unless.
func verifyBlockAtPosition(ctx context.Context, rcon testenv.RCONHelper, gx, gy, gz int, block string) (bool, error) {
	// If the block at (gx,gy,gz) matches, execute run say to confirm.
	// Presence of the chat message indicates the block is still there.
	validationStr := fmt.Sprintf("BlockFound_%d_%d_%d", gx, gy, gz)
	cmd := fmt.Sprintf(`execute if block %d %d %d %s run say %s`, gx, gy, gz, block, validationStr)
	resp, err := rcon.Exec(ctx, cmd)
	if err != nil {
		return false, err
	}
	// If the say command ran, the block is still there. If no message, block moved/changed.
	return strings.Contains(resp, validationStr), nil
}

// fireAt builds a target mechanism in front of the bot and fires the bow at it.
func fireAt(t *testing.T, inst *TestInstance, agent *ManagedAgent, distance int) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Make sure bow is equipped explicitly (active slot cannot be assumed)
	require.NoError(t, agent.EquipItemByName(ctx, "minecraft:bow"))

	botX, botY, botZ, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	targetX := int(botX) + distance
	targetY := int(botY)
	targetZ := int(botZ)

	gx, gy, gz, err := setupTargetMechanism(ctx, t, inst.RCON, targetX, targetY, targetZ)
	require.NoError(t, err, "build target mechanism")

	time.Sleep(400 * time.Millisecond)

	// Turn bot's body to face the target before firing
	require.NoError(t, agent.TurnTowards(ctx, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5))
	// Wait for server to process the rotation update
	time.Sleep(400 * time.Millisecond)

	// Fire using the new API at the center of the target block
	fireErr := agent.Agent.FireBowAt(float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5)
	if fireErr != nil {
		t.Logf("FireBowAt error: %v", fireErr)
	}
	require.NoError(t, fireErr)
	// Allow time for arrow flight and piston action
	time.Sleep(1800 * time.Millisecond)

	glowstoneStillThere, err := verifyBlockAtPosition(ctx, inst.RCON, gx, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify target impact")
	glowstoneMoved, err := verifyBlockAtPosition(ctx, inst.RCON, gx+1, gy, gz, "minecraft:glowstone")
	assert.False(t, glowstoneStillThere, "glowstone should have moved if target was hit")
	assert.True(t, glowstoneMoved, "glowstone should be at new position if target was hit")
}

// TestBowFiring_FireBowAt_TargetAndPiston verifies FireBowAt hits a target block that triggers a piston
func TestBowFiring_FireBowAt_TargetAndPiston(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.mcVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				srv.Version,
			)
			ag, err := fw.SpawnAgent(ctx, inst, agCfg)
			require.NoError(t, err)
			defer func() {
				if ag != nil && ag.BotClient() != nil {
					_ = ag.BotClient().Close()
				}
			}()

			// Let the agent join fully
			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			fireAt(t, inst, ag, 10)

			time.Sleep(3 * time.Second) // give a little time after the test for inspection etc.
		})
	}
}

// TestBowFiring_MultipleDistances hits targets at multiple ranges
func TestBowFiring_MultipleDistances(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.mcVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				srv.Version,
			)
			ag, err := fw.SpawnAgent(ctx, inst, agCfg)
			require.NoError(t, err)
			defer func() {
				if ag != nil && ag.BotClient() != nil {
					_ = ag.BotClient().Close()
				}
			}()

			// Join + give items
			time.Sleep(2 * time.Second)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 128`, ag.Name))
			require.NoError(t, err)

			for _, d := range []int{5, 10, 15, 30} {
				fireAt(t, inst, ag, d)
			}
		})
	}
}
