package agent

import (
	"log"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

const (
	clutchMinTicks = 4
	clutchMaxTicks = 18
	clutchMinFall  = 5.0
)

func (a *agent) handleClutchPlan(usage *items.ItemUsage, plan physics.ClutchPlan) {
	if plan.FallDistance < clutchMinFall {
		return
	}
	if plan.TicksToImpact < clutchMinTicks || plan.TicksToImpact > clutchMaxTicks {
		return
	}

	slot, itemName := a.findClutchItemSlot()
	if slot < 0 {
		log.Printf("[Clutch] No clutch item found in hotbar")
		return
	}

	if err := usage.SwitchToSlot(slot); err != nil {
		log.Printf("[Clutch] Failed to switch to slot %d: %v", slot, err)
		return
	}

	err := usage.UseItemOnBlock(
		models.V3{X: plan.PlacePos.X, Y: plan.PlacePos.Y, Z: plan.PlacePos.Z},
		models.FaceUp,
		models.MainHand,
	)
	if err != nil {
		log.Printf("[Clutch] UseItemOnBlock failed for %s: %v", itemName, err)
		return
	}
	a.noteClutchAction(time.Now(), plan, itemName)
}

func (a *agent) findClutchItemSlot() (int16, string) {
	slot, name := a.findHotbarSlotByName("minecraft:water_bucket")
	if slot >= 0 {
		return slot, name
	}
	slot, name = a.findHotbarSlotByName("minecraft:powder_snow_bucket")
	return slot, name
}

func (a *agent) findHotbarSlotByName(itemName string) (int16, string) {
	a.slotsMu.RLock()
	defer a.slotsMu.RUnlock()
	a.itemMgrMu.RLock()
	defer a.itemMgrMu.RUnlock()

	if a.slots == nil || a.itemMgr == nil {
		return -1, ""
	}

	for i := mcscreen.HotbarSlotStart; i <= mcscreen.HotbarSlotEnd; i++ {
		itemID, _, ok := a.slots.ResolveSlot(-2, i)
		if !ok {
			continue
		}
		name := a.itemMgr.GetItemNameByID(itemID)
		if name == itemName {
			return i - mcscreen.HotbarSlotStart, name
		}
	}
	return -1, ""
}

func (a *agent) ensureClutchItemManager() {
	if a.itemMgr == nil {
		a.itemMgr = registryItemManager{registryGetter: a.GetRegistry}
	}
}

func (a *agent) ensureClutchSafety(now time.Time) bool {
	if a.lastClutchAction.IsZero() {
		return true
	}
	return now.Sub(a.lastClutchAction) > 500*time.Millisecond
}

func (a *agent) noteClutchAction(now time.Time, plan physics.ClutchPlan, itemName string) {
	a.lastClutchAction = now
	log.Printf("[Clutch] Actioned %s using %s at (%.0f,%.0f,%.0f) in %d ticks",
		plan.Type, itemName, plan.PlacePos.X, plan.PlacePos.Y, plan.PlacePos.Z, plan.TicksToImpact)
}
