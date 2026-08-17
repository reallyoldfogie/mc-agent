package v1_21_10

// Covers the 2026-08-16 fix: convertSlotFromProtocol (and, via the same shared
// helper, ParseEntityEquipment) previously left InventorySlot.ItemID at 0 with
// a TODO. slotItemID is the single extraction point both parsers now share.

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/basetypes"
)

func TestSlotItemID(t *testing.T) {
	t.Run("empty slot returns 0", func(t *testing.T) {
		slot := basetypes.Slot{ItemCount: 0}
		if got := slotItemID(slot); got != 0 {
			t.Errorf("expected 0 for empty slot, got %d", got)
		}
	})

	t.Run("non-empty slot returns the item ID", func(t *testing.T) {
		slot := basetypes.Slot{
			ItemCount: 1,
			UnnamedType0001: &basetypes.SlotUnnamedType0001Default{
				ItemId: pk.VarInt(42),
			},
		}
		if got := slotItemID(slot); got != 42 {
			t.Errorf("expected 42, got %d", got)
		}
	})
}

func TestConvertSlotFromProtocol_PopulatesItemID(t *testing.T) {
	slot := basetypes.Slot{
		ItemCount: 3,
		UnnamedType0001: &basetypes.SlotUnnamedType0001Default{
			ItemId: pk.VarInt(777),
		},
	}

	result := convertSlotFromProtocol(slot)

	if !result.Present {
		t.Fatal("expected Present=true for a non-empty slot")
	}
	if result.ItemID != 777 {
		t.Errorf("expected ItemID 777, got %d", result.ItemID)
	}
	if result.Count != 3 {
		t.Errorf("expected Count 3, got %d", result.Count)
	}
}

func TestConvertSlotFromProtocol_EmptySlot(t *testing.T) {
	slot := basetypes.Slot{ItemCount: 0}
	result := convertSlotFromProtocol(slot)
	if result.Present {
		t.Error("expected Present=false for an empty slot")
	}
	if result.ItemID != 0 {
		t.Errorf("expected ItemID 0 for empty slot, got %d", result.ItemID)
	}
}
