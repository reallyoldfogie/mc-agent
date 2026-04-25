package agent

import (
	"log"
	"reflect"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// screenManagerSlotResolver adapts the screen manager to implement SlotResolver.
// It reads slot data from the screen manager's inventory state, supporting both
// player inventory and open container windows.
type screenManagerSlotResolver struct {
	agent *agent
}

// ResolveSlot resolves an inventory slot to its item ID and count.
// windowID -2 represents the player's main inventory (9x4 slots + hotbar).
// windowID 0 also represents the player's inventory.
// windowID > 0 represents an open container window.
// Returns (itemID, count, ok).
func (r *screenManagerSlotResolver) ResolveSlot(windowID int, slotIndex int16) (itemID int32, count int, ok bool) {
	if r.agent == nil {
		return 0, 0, false
	}

	screenMgr := r.agent.screenMgr
	if screenMgr == nil {
		return 0, 0, false
	}

	// Map windowID -2 to 0 (player inventory)
	if windowID == -2 {
		windowID = 0
	}

	// Handle player inventory (window 0)
	if windowID == 0 {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[SlotResolver] Recovered from panic in GetPlayerInventory: %v", rec)
			}
		}()

		inventory := screenMgr.GetPlayerInventory()
		if inventory == nil || slotIndex < 0 || slotIndex >= int16(len(inventory.GetSlots())) {
			return 0, 0, false
		}

		slot := inventory.GetSlots()[slotIndex]
		return int32(slot.ID), int(slot.Count), true
	}

	// Handle container windows (window > 0)
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[SlotResolver] Recovered from panic in GetScreenByID: %v", rec)
		}
	}()

	container := screenMgr.GetScreenByID(windowID)
	if container == nil {
		return 0, 0, false
	}

	// Get slots from the container - use type assertion as various container types exist
	return getSlotFromContainer(container, slotIndex)
}

// getSlotFromContainer extracts a slot from any container type.
// Containers have a Slots field that's accessible via type assertion.
func getSlotFromContainer(container mcscreen.Container, slotIndex int16) (itemID int32, count int, ok bool) {
	// Check bounds
	if slotIndex < 0 {
		return 0, 0, false
	}

	// Handle known container types via type assertion
	// This is safer than reflection and covers most use cases
	switch c := container.(type) {
	case *mcscreen.Chest:
		if slotIndex >= int16(len(c.Slots)) {
			return 0, 0, false
		}
		slot := c.Slots[slotIndex]
		return int32(slot.ID), int(slot.Count), true

	case mcscreen.Inventory:
		slots := c.GetSlots()
		if slotIndex >= int16(len(slots)) {
			return 0, 0, false
		}
		slot := slots[slotIndex]
		return int32(slot.ID), int(slot.Count), true

	default:
		// For unknown container types, we can't resolve slots safely
		// Log a warning for debugging purposes
		log.Printf("[SlotResolver] Unknown container type: %T (slotIndex=%d)", container, slotIndex)
		return 0, 0, false
	}
}

// getSlotViaReflection attempts to extract a slot using reflection for generic containers.
// This is a fallback for container types not explicitly handled.
func getSlotViaReflection(container interface{}, slotIndex int) (itemID int, count int, ok bool) {
	if container == nil {
		return 0, 0, false
	}

	// Safely dereference the container pointer
	rv := reflect.ValueOf(container)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return 0, 0, false
	}

	v := rv.Elem()
	if !v.IsValid() {
		return 0, 0, false
	}

	// Try to access Slots field
	slotsField := v.FieldByName("Slots")
	if !slotsField.IsValid() || slotsField.Kind() != reflect.Slice {
		return 0, 0, false
	}

	if slotIndex < 0 || slotIndex >= slotsField.Len() {
		return 0, 0, false
	}

	// Get the slot at the index
	slot := slotsField.Index(slotIndex)
	if !slot.IsValid() {
		return 0, 0, false
	}

	// Extract ID and Count fields from slot
	idField := slot.FieldByName("ID")
	countField := slot.FieldByName("Count")

	if !idField.IsValid() || !countField.IsValid() {
		return 0, 0, false
	}

	// Convert to int - handle various int types
	var itemID32 int64
	var count32 int64

	switch idField.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		itemID32 = idField.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		itemID32 = int64(idField.Uint())
	default:
		return 0, 0, false
	}

	switch countField.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		count32 = countField.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		count32 = int64(countField.Uint())
	default:
		return 0, 0, false
	}

	return int(itemID32), int(count32), true
}

// newScreenManagerSlotResolver creates a new screen manager slot resolver.
func newScreenManagerSlotResolver(a *agent) models.SlotResolver {
	return &screenManagerSlotResolver{agent: a}
}

// initializeSlotResolver sets up the slot resolver after the screen manager is ready.
func (a *agent) initializeSlotResolver() {
	if a.screenMgr == nil {
		log.Printf("[Agent %s] Cannot initialize slot resolver: screen manager not set", a.cfg.Name)
		return
	}

	resolver := newScreenManagerSlotResolver(a)
	a.SetSlotResolver(resolver)
	log.Printf("[Agent %s] Slot resolver initialized from screen manager", a.cfg.Name)
}
