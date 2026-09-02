package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// armorSlotIndices are the player-inventory window slot indices for the
// four armor slots (head, chest, legs, feet), matching the numbering used
// throughout this codebase's inventory tests (screen.Manager's own
// convention - shift-clicking an elytra always lands in slot 6).
var armorSlotIndices = [...]int{5, 6, 7, 8}

func itemStackFromScreenSlot(slot mcscreen.Slot) models.ItemStack {
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

// isWornInArmorSlot reports whether an item matching normalizedName is
// currently in one of the four armor slots.
func (a *agent) isWornInArmorSlot(normalizedName string) bool {
	inv := a.GetInventory()
	if inv == nil {
		return false
	}
	slots := inv.GetSlots()
	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()
	if itemMgr == nil {
		return false
	}
	for _, idx := range armorSlotIndices {
		if idx >= len(slots) || slots[idx].Count <= 0 {
			continue
		}
		if normalizeItemName(itemMgr.GetItemNameByID(int32(slots[idx].ID))) == normalizedName {
			return true
		}
	}
	return false
}

// Equip finds an item by name anywhere in the inventory and readies it for
// use. See models.CommandAgent's doc comment for the overall contract.
//
// Already worn (an armor slot) - nothing to do. Otherwise, always
// shift-click it first regardless of where it currently sits (including
// the hotbar): vanilla's ArmorSlot.quickMove() takes priority over the
// default PlayerScreenHandler's hotbar<->main-inventory zone shuffle, so an
// armor/elytra item shift-clicked from ANYWHERE routes to its equipment
// slot (confirmed throughout this codebase's elytra tests) - skipping the
// shift-click whenever an item already happened to be in the hotbar (an
// earlier version of this method did exactly that, to dodge a different
// bug below) would silently never equip armor that a `give` command
// happened to place in the hotbar rather than the main inventory, which is
// common and was a real, confirmed-live regression.
//
// The tradeoff: for a non-armor item that starts in the hotbar, shift-click
// isn't a no-op - it's the "opposite zone" case, and kicks the item OUT to
// the main inventory instead of leaving it in hand (a real bug this method
// used to have in the other direction). SwitchToItem below recovers from
// that by finding it again and swapping it back, so the net effect is
// still correct, just with an extra round trip for that one case.
func (a *agent) Equip(ctx context.Context, itemName string) error {
	normalized := normalizeItemName(itemName)

	slotIndex, found, err := a.FindSlotWith(ctx, normalized, -2)
	if err != nil {
		return fmt.Errorf("search inventory for %s: %w", itemName, err)
	}
	if !found {
		return fmt.Errorf("no %s found in inventory", itemName)
	}

	if isArmorSlotIndex(slotIndex) {
		return nil
	}

	inv := a.GetInventory()
	if inv == nil {
		return fmt.Errorf("inventory not available")
	}
	slots := inv.GetSlots()
	if slotIndex < 0 || slotIndex >= len(slots) {
		return fmt.Errorf("invalid slot index %d", slotIndex)
	}

	if err := a.ShiftClickSlot(int16(slotIndex), itemStackFromScreenSlot(slots[slotIndex])); err != nil {
		return fmt.Errorf("shift-click %s: %w", itemName, err)
	}

	// Wait for the click to be confirmed - shift-click prediction and the
	// real server confirmation both update the same tracked inventory, but
	// not synchronously with this call returning. Real confirmations arrive
	// within tens of milliseconds live; this is generous without being the
	// multi-second stall a fixed 2s wait would add to the (common) case
	// below that needs the SwitchToItem fallback.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if a.isWornInArmorSlot(normalized) {
			return nil
		}
		if hotbarSlot, ok := a.FindHotbarSlotWithItem(ctx, normalized); ok {
			return a.SelectHotbarSlot(ctx, hotbarSlot)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Last resort - the hotbar may have been full, leaving it stuck in the
	// main inventory.
	equipped, err := a.SwitchToItem(ctx, normalized)
	if err != nil {
		return fmt.Errorf("select %s: %w", itemName, err)
	}
	if !equipped {
		return fmt.Errorf("no %s found in inventory", itemName)
	}
	return nil
}

func isArmorSlotIndex(slotIndex int) bool {
	for _, idx := range armorSlotIndices {
		if slotIndex == idx {
			return true
		}
	}
	return false
}

// FlyTo pilots an elytra flight to the given coordinates. See
// models.CommandAgent's doc comment for the overall contract - this mirrors
// the takeoff/cruise/land sequence validated live across the full version
// matrix by testing/elytra_test.go and testing/elytra_navigation_test.go,
// generalized from a fixed test course into an arbitrary destination.
//
// Landing targets a stable altitude near the ground, not the requested Y
// exactly: elytra flight can't hover, so "arrive and hold at height Y"
// isn't physically meaningful the way it is for pathfinding's moveTo - this
// instead treats (x, z) as the real target and lands wherever the ground
// actually is once there, matching how a player would read "fly to a
// location".
func (a *agent) FlyTo(ctx context.Context, x, y, z float64) error {
	_ = y // see doc comment: landing targets the ground under (x, z), not y exactly

	if !a.isWornInArmorSlot("minecraft:elytra") {
		return fmt.Errorf("no elytra equipped - use 'equip elytra' first")
	}

	if err := a.EnterManualMode(); err != nil {
		return fmt.Errorf("enter manual mode: %w", err)
	}
	defer func() { _ = a.ExitManualMode() }()

	const steerInterval = 100 * time.Millisecond
	const climbPitch = -80.0
	const cruisePitch = 0.0
	const landPitch = 15.0
	const arrivalRadius = 5.0
	const climbDuration = 1500 * time.Millisecond
	const cruiseTimeout = 10 * time.Minute
	const landTimeout = 30 * time.Second

	fireRocket := func() {
		if _, err := a.SwitchToItem(ctx, "minecraft:firework_rocket"); err != nil {
			a.logf("[FlyTo] select firework rocket: %v", err)
			return
		}
		if err := a.UseItem(ctx, models.MainHand); err != nil {
			a.logf("[FlyTo] use firework rocket: %v", err)
		}
	}

	if a.IsGliding() {
		_ = a.SendChat(fmt.Sprintf("Continuing flight toward (%.0f, %.0f, %.0f)", x, y, z))
	} else {
		_ = a.SendChat(fmt.Sprintf("Taking off toward (%.0f, %.0f, %.0f)", x, y, z))

		if err := a.SetManualRotation(0, climbPitch); err != nil {
			return err
		}
		// Double-jump: a real release-then-press is required to start
		// gliding (see physics/state.go's applyGlideStateTransition) -
		// holding jump through a normal ground jump's ascent must not
		// also start gliding.
		if err := a.SetManualJump(true); err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond)
		if err := a.SetManualJump(false); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		if err := a.SetManualJump(true); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		if err := a.SetManualJump(false); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)

		if !a.IsGliding() {
			_ = a.SendChat("Failed to start gliding - must be airborne (off the ground) for the double-jump to trigger elytra flight")
			return fmt.Errorf("double-jump did not start gliding")
		}

		fireRocket()

		climbDeadline := time.Now().Add(climbDuration)
		for time.Now().Before(climbDeadline) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := a.SetManualRotation(0, climbPitch); err != nil {
				return err
			}
			if !a.HasActiveFireworkBoost() {
				fireRocket()
			}
			time.Sleep(steerInterval)
		}
		if err := a.SetManualRotation(0, cruisePitch); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Cruise toward (x, z) by yaw, holding a level cruise pitch (gliding
	// ignores WASD throttle entirely - see physics/state.go's
	// applyMovementInputs early-return while isGliding) and re-boosting
	// whenever the current firework's boost lapses. Steering uses yaw only,
	// deliberately ignoring GetYawAndPitch's computed pitch: gliding's own
	// formula is sensitive to pitch in ways that make "look straight at a
	// distant target" an unstable way to fly it, matching the live-tested
	// navigation course.
	cruiseDeadline := time.Now().Add(cruiseTimeout)
	for time.Now().Before(cruiseDeadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		pos, ok := a.GetPositionSimple()
		if !ok {
			return fmt.Errorf("agent position not initialized")
		}
		dx, dz := x-pos.X, z-pos.Z
		if math.Sqrt(dx*dx+dz*dz) <= arrivalRadius {
			break
		}
		yaw, _ := utils.GetYawAndPitch(pos, models.V3{X: x, Y: pos.Y, Z: z})
		if err := a.SetManualRotation(yaw, cruisePitch); err != nil {
			return err
		}
		if !a.HasActiveFireworkBoost() {
			fireRocket()
		}
		time.Sleep(steerInterval)
	}

	// Land: pitch down gently, keep steering toward the target so descent
	// doesn't drift away from it, and wait for altitude to stabilize -
	// same detection the navigation test uses to confirm touchdown.
	landDeadline := time.Now().Add(landTimeout)
	var lastY, stableSince float64
	for time.Now().Before(landDeadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		pos, ok := a.GetPositionSimple()
		if !ok {
			return fmt.Errorf("agent position not initialized")
		}
		yaw, _ := utils.GetYawAndPitch(pos, models.V3{X: x, Y: pos.Y, Z: z})
		if err := a.SetManualRotation(yaw, landPitch); err != nil {
			return err
		}
		if math.Abs(pos.Y-lastY) < 0.02 {
			if stableSince == 0 {
				stableSince = float64(time.Now().UnixMilli())
			} else if float64(time.Now().UnixMilli())-stableSince > 500 {
				_ = a.SendChat(fmt.Sprintf("Landed near (%.0f, %.0f, %.0f)", pos.X, pos.Y, pos.Z))
				return nil
			}
		} else {
			stableSince = 0
		}
		lastY = pos.Y
		time.Sleep(steerInterval)
	}
	_ = a.SendChat("Did not settle to a stable landing within the descent window")
	return fmt.Errorf("landing timed out")
}
