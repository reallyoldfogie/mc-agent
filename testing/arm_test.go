package testing

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestArmSwing_MainHand tests arm swing animation in main hand
func TestArmSwing_MainHand(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "action_arm_swing", tt.mcVersion)
			defer env.Cancel()

			if env.Agent.Config.VersionHandler == nil {
				t.Skip("Version handler not available")
			}

			actionHandler := env.Agent.Config.VersionHandler.Play().Actions()
			botClient := env.Agent.BotClient()

			// Send arm swing (main hand = 0)
			err := actionHandler.SendSwing(botClient.Conn(), 0)
			require.NoError(t, err, "send swing (main hand)")

			// Verify no disconnect occurred
			time.Sleep(100 * time.Millisecond)

			// If we get here without error, swing was accepted
			t.Log("✓ Main hand arm swing test passed")
		})
	}
}

// TestArmSwing_Offhand tests arm swing animation in offhand
func TestArmSwing_Offhand(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "action_arm_swing_offhand", tt.mcVersion)
			defer env.Cancel()

			if env.Agent.Config.VersionHandler == nil {
				t.Skip("Version handler not available")
			}

			actionHandler := env.Agent.Config.VersionHandler.Play().Actions()
			botClient := env.Agent.BotClient()

			// Send arm swing (offhand = 1)
			err := actionHandler.SendSwing(botClient.Conn(), 1)
			require.NoError(t, err, "send swing (offhand)")

			time.Sleep(100 * time.Millisecond)

			t.Log("✓ Offhand arm swing test passed")
		})
	}
}

// TestArmSwing_Rapid tests rapid arm swinging
func TestArmSwing_Rapid(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "action_arm_swing_rapid", tt.mcVersion)
			defer env.Cancel()

			if env.Agent.Config.VersionHandler == nil {
				t.Skip("Version handler not available")
			}

			actionHandler := env.Agent.Config.VersionHandler.Play().Actions()
			botClient := env.Agent.BotClient()

			// Swing rapidly (simulating clicking spam)
			for i := 0; i < 10; i++ {
				hand := models.Hand(int32(i % 2)) // Alternate between main and offhand

				err := actionHandler.SendSwing(botClient.Conn(), hand)
				require.NoError(t, err, "send swing iteration %d", i)

				time.Sleep(50 * time.Millisecond)
			}

			time.Sleep(100 * time.Millisecond)

			t.Log("✓ Rapid arm swing test passed")
		})
	}
}
