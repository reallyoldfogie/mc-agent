package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayerInventorySlotCount(t *testing.T) {
	// 27 main inventory slots plus a 9-slot hotbar. Every container window
	// appends exactly this many slots, which is what makes the container/player
	// boundary derivable without knowing the window type.
	assert.Equal(t, 36, PlayerInventorySlotCount)
	assert.Equal(t, 27+9, PlayerInventorySlotCount)
}

func TestSplitContainerSlots(t *testing.T) {
	makeSlots := func(count int) []InventorySlot {
		slots := make([]InventorySlot, count)
		for i := range slots {
			// Give each slot a distinct count so ordering is verifiable.
			slots[i] = InventorySlot{Present: true, Count: int32(i + 1)}
		}
		return slots
	}

	t.Run("donkey chest is 15 slots plus player inventory", func(t *testing.T) {
		// A donkey/mule chest contributes 15 slots in vanilla; the exact number
		// does not matter to the split, only that the trailing 36 are ours.
		windowSlots := makeSlots(15 + PlayerInventorySlotCount)

		containerSlots, playerSlots, ok := SplitContainerSlots(windowSlots)
		require.True(t, ok)
		assert.Len(t, containerSlots, 15)
		assert.Len(t, playerSlots, PlayerInventorySlotCount)

		// The container section must be the leading slots, in order.
		assert.EqualValues(t, 1, containerSlots[0].Count)
		assert.EqualValues(t, 15, containerSlots[14].Count)
		// The player section follows immediately after.
		assert.EqualValues(t, 16, playerSlots[0].Count)
	})

	t.Run("window with only a player inventory has no container slots", func(t *testing.T) {
		windowSlots := makeSlots(PlayerInventorySlotCount)

		containerSlots, playerSlots, ok := SplitContainerSlots(windowSlots)
		require.True(t, ok)
		assert.Empty(t, containerSlots)
		assert.Len(t, playerSlots, PlayerInventorySlotCount)
	})

	t.Run("window too small to split is rejected", func(t *testing.T) {
		// Fewer slots than the player section means the layout is not
		// [container][player], so splitting would produce nonsense.
		_, _, ok := SplitContainerSlots(makeSlots(PlayerInventorySlotCount - 1))
		assert.False(t, ok)

		_, _, ok = SplitContainerSlots(nil)
		assert.False(t, ok)
	})
}

func TestEntityInventoryIsEmpty(t *testing.T) {
	t.Run("no slots is empty", func(t *testing.T) {
		assert.True(t, EntityInventory{}.IsEmpty())
	})

	t.Run("all zero-count slots is empty", func(t *testing.T) {
		inventory := EntityInventory{
			Slots: []InventorySlot{{Count: 0}, {Count: 0}},
		}
		assert.True(t, inventory.IsEmpty())
	})

	t.Run("any occupied slot is not empty", func(t *testing.T) {
		inventory := EntityInventory{
			Slots: []InventorySlot{{Count: 0}, {Present: true, Count: 3}},
		}
		assert.False(t, inventory.IsEmpty())
	})
}

func TestEntityInventoryAge(t *testing.T) {
	capturedAt := time.Now()
	inventory := EntityInventory{UpdatedAt: capturedAt}

	assert.Equal(t, time.Duration(0), inventory.Age(capturedAt))
	assert.Equal(t, 5*time.Second, inventory.Age(capturedAt.Add(5*time.Second)))
}
