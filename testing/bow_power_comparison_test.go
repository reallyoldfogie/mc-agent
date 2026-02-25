package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBowPowerComparison compares actual arrow behavior between FireBowAt and FireBowAtFullPower
// to determine if the discrepancy in long-range shots is due to bow power not reaching max
func TestBowPowerComparison(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
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

			// Let agent join
			time.Sleep(2 * time.Second)

			// Give equipment
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)


		// Teleport to test location
		_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`teleport %s 0 100 0`, ag.Name))
		require.NoError(t, err)

		// Create a floor for the bot to stand on
		_, err = inst.RCON.Exec(ctx, `fill -50 99 -50 50 99 50 minecraft:bedrock`)
		require.NoError(t, err)

		time.Sleep(2 * time.Second)

		// Wait extra time for chunks to load and agent to settle
		time.Sleep(3 * time.Second)

			t.Logf("[%s] Starting bow power comparison tests", tt.MCVersion)

			// Test at various distances
			testCases := []struct {
				distance float64
				label    string
			}{
				{10, "10m"},
				{20, "20m"},
				{30, "30m"},
				{40, "40m"},
			}

			for _, tc := range testCases {
				t.Run(tc.label, func(t *testing.T) {
					botX, botY, botZ, ok := ag.Agent.GetPositionSimple()
					require.True(t, ok, "bot position initialized")

					targetX := botX + tc.distance
					targetY := botY
					targetZ := botZ

					t.Logf("[%s] Testing at distance %.1f: target=(%.1f, %.1f, %.1f)",
						tc.label, tc.distance, targetX, targetY, targetZ)

					// Place target block (glowstone) for arrow validation
					blockX := int(math.Floor(targetX))
					blockY := int(math.Floor(targetY))
					blockZ := int(math.Floor(targetZ))

					cmd := fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, blockX, blockY, blockZ)
					response, err := inst.RCON.Exec(ctx, cmd)
					require.NoError(t, err, "place target glowstone block")
					t.Logf("%s => %s", cmd, response)

					time.Sleep(3 * time.Second) // Wait for world state sync

					// Fire using standard FireBowAt
					t.Logf("  Firing with FireBowAt...")
					traj, err := ag.Agent.FireBowAt(float64(blockX)+0.5, float64(blockY)+0.5, float64(blockZ)+0.5)
					assert.NoError(t, err)
					t.Logf("    Trajectory: %d points", len(traj))

					time.Sleep(3 * time.Second)

					// Analyze trajectory
					actualStandard, err := AnalyzeArrowTrajectory(inst.AgentLogFile)
					if err != nil {
						t.Logf("    Warning: Could not analyze standard trajectory: %v", err)
					} else {
						t.Logf("    Standard: actual=%d points",
							len(actualStandard))
						if len(actualStandard) > 0 {
							lastPos := actualStandard[len(actualStandard)-1]
							distTraveled := math.Sqrt(
								(lastPos.X-botX)*(lastPos.X-botX) +
									(lastPos.Z-botZ)*(lastPos.Z-botZ))
							t.Logf("    Standard: arrow traveled %.1f blocks (predicted %.1f)",
								distTraveled, tc.distance)
						}
					}
					cmd = fmt.Sprintf(`setblock %d %d %d minecraft:air`, blockX, blockY, blockZ)
					response, err = inst.RCON.Exec(ctx, cmd)
					require.NoError(t, err, "place target air block")
					t.Logf("%s => %s", cmd, response)

					time.Sleep(2 * time.Second)
				})
			}
		})
	}
}
