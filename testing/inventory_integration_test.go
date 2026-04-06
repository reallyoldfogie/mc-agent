package testing

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/require"
)

func TestInventoryClickIntegration(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			if err := os.MkdirAll("logs", 0755); err == nil {
				debugPath := filepath.Join("logs", "inventory_click_debug.log")
				t.Logf("enabling click debug log: %s", debugPath)
				_ = os.Setenv("MC_AGENT_CLICK_DEBUG_PATH", debugPath)
				if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
					fmt.Fprintf(f, "\n\n==============================\n")
					fmt.Fprintf(f, "\n[%s] TestInventoryClickIntegration Start\n", time.Now().Format(time.RFC3339Nano))
					_ = f.Close()
				}

				defer func() {
					f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						fmt.Fprintf(f, "[%s] TestInventoryClickIntegration End\n", time.Now().Format(time.RFC3339Nano))
						fmt.Fprintf(f, "==============================\n\n")
						_ = f.Close()
					}
				}()
			}

			cwd, err := os.Getwd()
			require.NoError(t, err, "get current working directory")

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")
			t.Log("framework initialized")

			serverCfg := DefaultServerConfig()
			serverCfg.Memory = "512M"
			serverCfg.Version = tt.MCVersion
			serverCfg.GameMode = "survival"
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}

			serverCfg.PullImage = false

			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestInventoryClickIntegration", tt.MCVersion)
			RequireIntegrationEnv(t, serverCfg)

			// Optionally copy protocol dumper mod for packet debugging
			// Set ENABLE_PROTOCOL_DUMPER=1 to enable
			if os.Getenv("ENABLE_PROTOCOL_DUMPER") == "1" {
				modDir := filepath.Join(serverCfg.CacheDir, "mods")
				os.MkdirAll(modDir, 0755)
				err = copyFile(t, filepath.Join(cwd, "..", "..", "mc-protocol-dumper", "build", "libs", "protocol-dumper-0.0.3-fabric.jar"), filepath.Join(modDir, "protocol-dumper-0.0.3-fabric.jar"))
				if err != nil {
					t.Logf("Warning: failed to copy protocol dumper mod: %v", err)
				} else {
					t.Log("Protocol dumper mod enabled")
				}
			}

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			t.Logf("server started: %s:%d", inst.Server.Host, inst.Server.HostServerPort)
			require.NoError(t, framework.setupAgentLogging(), "setup agent logging for diagnostics")
			defer framework.CloseAgentLog()
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
			botName := "InventoryBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			// Version handler is auto-detected by the framework

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			scr := managedAgent.ScreenManager()
			botClient := managedAgent.BotClient()
			require.NotNil(t, botClient, "bot client should be available")

			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			t.Log("giving item to bot")
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:stone 1", botName))
			require.NoError(t, err, "give item")
			t.Log("give command executed")

			t.Log("waiting for item to appear in bot inventory")
			fromSlot, fromSlotData, ok := waitForInventorySlot(scr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 20*time.Second)
			require.True(t, ok, "no item found in main/hotbar inventory")
			t.Logf("found source slot %d", fromSlot)

			t.Log("Looking for empty slot to put item in")
			toSlot, _, ok := waitForInventorySlot(scr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && index != fromSlot && s.Count <= 0
			}, 20*time.Second)
			require.True(t, ok, "no empty slot available in main/hotbar")
			t.Logf("found target slot %d", toSlot)

			fromNBTSlot, ok := windowSlotToNBT(fromSlot)
			require.True(t, ok, "unsupported source slot for NBT mapping")
			toNBTSlot, ok := windowSlotToNBT(toSlot)
			require.True(t, ok, "unsupported target slot for NBT mapping")

			fromStack := slotToItemStack(fromSlotData)
			toStack := models.ItemStack{}
			t.Logf("from stack: item=%d count=%d components=%d removes=%d", fromStack.ItemID, fromStack.Count, len(fromStack.Components), len(fromStack.RemoveComponents))
			for _, component := range fromStack.Components {
				t.Logf("from stack component: type=%d data=%T", component.Type, component.Data)
			}

			invMgr := items.NewInventoryManager(scr)
			invMgr.SetWindow(0)
			invMgr.SetWaitForUpdates(false) // Test state ID fix without sync wait

			err = invMgr.MoveItem(int16(fromSlot), int16(toSlot), fromStack, toStack)
			require.NoError(t, err, "move item")
			t.Log("move item request sent")

			if itemsBySlot, err := GetInventoryItems(ctx, inst.RCON, botName); err == nil {
				t.Logf("inventory after move: %+v", itemsBySlot)
			} else {
				t.Logf("inventory read failed after move: %v", err)
			}

			t.Log("verifying source slot is empty via RCON")
			ok = waitForInventorySlotStateRCON(ctx, inst.RCON, botName, fromNBTSlot, func(item InventoryItem) bool {
				return item.Count == 0
			}, 10*time.Second)
			require.True(t, ok, "source slot did not clear after move")
			t.Log("source slot cleared")

			t.Log("verifying target slot has item via RCON")
			ok = waitForInventorySlotStateRCON(ctx, inst.RCON, botName, toNBTSlot, func(item InventoryItem) bool {
				return item.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "target slot did not receive item")
			t.Log("target slot received item")

			_ = botClient.Close()
		})
	}
}

func waitForInventorySlot(scr screen.Manager, match func(index int, s screen.Slot) bool, timeout time.Duration) (int, screen.Slot, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for idx, slot := range scr.Inventory().GetSlots() {
			if match(idx, slot) {
				return idx, slot, true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return -1, screen.Slot{}, false
}

func waitForSlotState(scr screen.Manager, slotIndex int, match func(screen.Slot) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if slotIndex >= 0 && slotIndex < len(scr.Inventory().GetSlots()) {
			if match(scr.Inventory().GetSlots()[slotIndex]) {
				return true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func slotToItemStack(slot screen.Slot) models.ItemStack {
	stack := models.ItemStack{
		ItemID: int32(slot.ID),
		Count:  int8(slot.Count),
	}
	for _, component := range slot.Components {
		stack.Components = append(stack.Components, models.ItemComponent{
			Type: int32(component.Type),
			Data: component.Data,
		})
	}
	for _, componentType := range slot.RemoveComponents {
		stack.RemoveComponents = append(stack.RemoveComponents, int32(componentType))
	}
	return stack
}

func waitForInventorySlotStateRCON(ctx context.Context, rcon testenv.RCONHelper, player string, slot int, match func(InventoryItem) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		items, err := GetInventoryItems(ctx, rcon, player)
		if err == nil {
			// Convert slice to map by slot for easy lookup
			for _, item := range items {
				if item.Slot == slot && match(item) {
					return true
				}
			}
			// If slot not found in inventory, treat as empty
			if match(InventoryItem{Slot: slot, ID: "", Count: 0}) {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func windowSlotToNBT(windowSlot int) (int, bool) {
	switch {
	case windowSlot >= 9 && windowSlot <= 35:
		return windowSlot, true
	case windowSlot >= 36 && windowSlot <= 44:
		return windowSlot - 36, true
	default:
		return -1, false
	}
}

// copyFile copies a file from src to dst.
func copyFile(t *testing.T, src, dst string) error {
	t.Logf("copying file from %s to %s", src, dst)
	sourceFile, err := os.Open(src) // Open the source file
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close() // Ensure the source file is closed

	destinationFile, err := os.Create(dst) // Create the destination file
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close() // Ensure the destination file is closed

	// Use io.Copy to efficiently copy data from source to destination
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Ensure all data is written to the destination file's storage
	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

func waitForPlayerOnline(ctx context.Context, rcon testenv.RCONHelper, player string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := rcon.Exec(ctx, "list")
		if err == nil && strings.Contains(resp, player) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}
