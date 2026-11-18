package items

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

type fakeContainerSightingRecorder struct {
	sightings []ContainerSighting
}

func (f *fakeContainerSightingRecorder) RecordContainerSighting(s ContainerSighting) {
	f.sightings = append(f.sightings, s)
}

// newTestChestHelper builds a ContainerHelper whose screenMgr reports a
// single-row (9-slot) chest at windowID, with chest.Slots sized like a
// real wire window: the chest's own 9 slots followed by the player's 36
// inventory slots mirrored in (see mc-bot-go's ContainerTypeInfo.TotalSlots) -
// the shape that caused recordChestSightings' own pre-existing bug, and
// that ReportSlotObserved/ReportWindowObserved must also respect.
func newTestChestHelper(t *testing.T, windowID byte, pos models.V3) (*ContainerHelper, *fakeContainerSightingRecorder, *mcscreen.Chest) {
	t.Helper()
	chest := &mcscreen.Chest{Rows: 1, Slots: make([]mcscreen.Slot, 9+36)}
	screenMgr := &fakeScreenManager{screens: map[int]mcscreen.Container{int(windowID): chest}}
	ch := NewContainerHelper(nil, nil, screenMgr, nil, nil)
	recorder := &fakeContainerSightingRecorder{}
	ch.SetSightingRecorder(recorder)
	ch.openPositions = map[byte]models.V3{windowID: pos}
	return ch, recorder, chest
}

func TestReportSlotObserved_ReportsAChestSlot(t *testing.T) {
	pos := models.V3{X: 1, Y: 2, Z: 3}
	ch, recorder, _ := newTestChestHelper(t, 1, pos)

	ch.ReportSlotObserved(1, 4, models.InventorySlot{Present: true, ItemID: 99, Count: 1})

	if len(recorder.sightings) != 1 {
		t.Fatalf("got %d sightings, want 1", len(recorder.sightings))
	}
	got := recorder.sightings[0]
	if got.ContainerPos != pos || got.SlotIndex != 4 || got.ItemID != 99 {
		t.Errorf("got %+v, want ContainerPos=%v SlotIndex=4 ItemID=99", got, pos)
	}
}

func TestReportSlotObserved_EmptySlotReportsForget(t *testing.T) {
	ch, recorder, _ := newTestChestHelper(t, 1, models.V3{})

	ch.ReportSlotObserved(1, 2, models.InventorySlot{Present: false})

	if len(recorder.sightings) != 1 || recorder.sightings[0].ItemID != 0 {
		t.Fatalf("got %+v, want one sighting with ItemID 0 (forget)", recorder.sightings)
	}
}

// A copper golem (or another player) changing a slot that's really part
// of the mirrored player-inventory section, not the chest's own section,
// must not be recorded as "seen in the chest" - that's the whole class
// of bug recordChestSightings itself used to have.
func TestReportSlotObserved_IgnoresPlayerInventorySection(t *testing.T) {
	ch, recorder, _ := newTestChestHelper(t, 1, models.V3{})

	ch.ReportSlotObserved(1, 9, models.InventorySlot{Present: true, ItemID: 5, Count: 1}) // slot 9 = first player-inventory slot for a 1-row chest

	if len(recorder.sightings) != 0 {
		t.Fatalf("got %d sightings, want 0 (slot 9 is outside the 9-slot chest section)", len(recorder.sightings))
	}
}

func TestReportSlotObserved_NoOpWithoutRecordedPosition(t *testing.T) {
	ch, recorder, _ := newTestChestHelper(t, 1, models.V3{})
	ch.openPositions = map[byte]models.V3{} // no entry for windowID 1

	ch.ReportSlotObserved(1, 0, models.InventorySlot{Present: true, ItemID: 1, Count: 1})

	if len(recorder.sightings) != 0 {
		t.Fatalf("got %d sightings, want 0 (no recorded position for this window)", len(recorder.sightings))
	}
}

func TestReportWindowObserved_ReportsOnlyTheChestSection(t *testing.T) {
	pos := models.V3{X: 10, Y: 20, Z: 30}
	ch, recorder, _ := newTestChestHelper(t, 1, pos)

	slots := make([]models.InventorySlot, 9+36)
	slots[3] = models.InventorySlot{Present: true, ItemID: 42, Count: 1} // chest section
	slots[20] = models.InventorySlot{Present: true, ItemID: 7, Count: 1} // player-inventory section

	ch.ReportWindowObserved(1, slots)

	if len(recorder.sightings) != 9 {
		t.Fatalf("got %d sightings, want exactly 9 (the chest's own section)", len(recorder.sightings))
	}
	for _, s := range recorder.sightings {
		if s.ItemID == 7 {
			t.Fatalf("got a sighting for ItemID 7, which was only in the player-inventory section (slot 20)")
		}
	}
	if recorder.sightings[3].ItemID != 42 {
		t.Errorf("sightings[3].ItemID = %d, want 42", recorder.sightings[3].ItemID)
	}
}

func TestReportSlotObserved_NoOpWhenWindowIsNotAChest(t *testing.T) {
	ch := NewContainerHelper(nil, nil, &fakeScreenManager{screens: map[int]mcscreen.Container{}}, nil, nil)
	recorder := &fakeContainerSightingRecorder{}
	ch.SetSightingRecorder(recorder)
	ch.openPositions = map[byte]models.V3{1: {}}

	ch.ReportSlotObserved(1, 0, models.InventorySlot{Present: true, ItemID: 1, Count: 1})

	if len(recorder.sightings) != 0 {
		t.Fatalf("got %d sightings, want 0 (window 1 has no chest screen)", len(recorder.sightings))
	}
}

// Regression test for the GetContainerSlotCount counterpart of the same
// bug: it used to return len(chest.Slots) (chest section + the
// mirrored-in player inventory) instead of just the chest's own section.
func TestGetContainerSlotCount_ExcludesPlayerInventorySection(t *testing.T) {
	ch, _, _ := newTestChestHelper(t, 1, models.V3{})

	if got := ch.GetContainerSlotCount(1); got != 9 {
		t.Fatalf("GetContainerSlotCount = %d, want 9 (the 1-row chest's own section, not 9+36)", got)
	}
}

// Regression test for the bug found while building ReportSlotObserved/
// ReportWindowObserved: recordChestSightings used to iterate chest.Slots
// directly (the full wire window, chest section + the player's own
// mirrored-in inventory), reporting the player's current inventory
// contents as if they were sitting inside the chest.
func TestRecordChestSightings_ExcludesPlayerInventorySection(t *testing.T) {
	pos := models.V3{X: 1, Y: 1, Z: 1}
	ch, recorder, chest := newTestChestHelper(t, 1, pos)
	chest.Slots[20] = mcscreen.Slot{ID: 7, Count: 1} // player-inventory section

	ch.recordChestSightings(1, chest)

	if len(recorder.sightings) != 9 {
		t.Fatalf("got %d sightings, want exactly 9 (the chest's own section)", len(recorder.sightings))
	}
	for _, s := range recorder.sightings {
		if s.ItemID == 7 {
			t.Fatalf("got a sighting for ItemID 7, which was only in the player-inventory section (slot 20)")
		}
	}
}
