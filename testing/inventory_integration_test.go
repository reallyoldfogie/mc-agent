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
	"github.com/stretchr/testify/suite"
)

// InventoryFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the raw slot-click inventory-move test:
// one server per version, shared by every test method below, instead of
// the previous per-test-function StartServer/StopServer pattern. WorldGen =
// WorldGenFlat, a deliberate deviation from the pre-conversion test's own
// DefaultServerConfig() (WorldGenRandom) - the test only moves an item
// between the bot's own inventory slots (window ID 0, never a real
// container GUI - see 00-plan.md's own note on this file), with no terrain
// dependency of any kind, so nothing is lost preserving Random and nothing
// is gained keeping it; Flat removes any chance of the WorldGenRandom
// working-area risk 19-phase1-projectile-conversion.md found the hard way
// for a genuinely terrain-sensitive cluster. Difficulty is left at
// VersionWorldSuite's own Peaceful default, matching the original.
//
// ExtraEnv carries FORCE_GAMEMODE=true forward from the pre-conversion
// config - also used by container_suite_test.go (the pre-VersionWorldSuite
// prior art this whole pattern generalized from) - via the new
// VersionWorldSuite.ExtraEnv field (see
// docs/plans/integration-test-shared-server/23-phase1-inventory-conversion.md).
type InventoryFlatSuite struct {
	VersionWorldSuite
}

func TestInventoryFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &InventoryFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestInventoryClickIntegration verifies a raw inventory-slot-click move
// (via items.InventoryManager, window 0 - the player's own always-open
// inventory) round-trips correctly against a real server: give an item,
// find its slot, find an empty slot, move it, and verify via RCON that the
// source slot cleared and the target slot received it. Equivalent to the
// pre-Phase-1 TestInventoryClickIntegration.
func (s *InventoryFlatSuite) TestInventoryClickIntegration() {
	t := s.T()

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

	leader, err := s.SpawnWorkingAreaAgent("InventoryBot", "inventory_click")
	require.NoError(t, err, "spawn agent")
	scr := leader.ScreenManager()

	t.Log("giving item to bot")
	_, err = s.Inst.RCON.Exec(s.Ctx, "give "+leader.Name+" minecraft:stone 1")
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

	if itemsBySlot, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name); err == nil {
		t.Logf("inventory after move: %+v", itemsBySlot)
	} else {
		t.Logf("inventory read failed after move: %v", err)
	}

	t.Log("verifying source slot is empty via RCON")
	ok = waitForInventorySlotStateRCON(s.Ctx, s.Inst.RCON, leader.Name, fromNBTSlot, func(item InventoryItem) bool {
		return item.Count == 0
	}, 10*time.Second)
	require.True(t, ok, "source slot did not clear after move")
	t.Log("source slot cleared")

	t.Log("verifying target slot has item via RCON")
	ok = waitForInventorySlotStateRCON(s.Ctx, s.Inst.RCON, leader.Name, toNBTSlot, func(item InventoryItem) bool {
		return item.Count > 0
	}, 10*time.Second)
	require.True(t, ok, "target slot did not receive item")
	t.Log("target slot received item")
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
