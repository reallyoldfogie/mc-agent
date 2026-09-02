package agent

import ()

// OnScreenSlotChange logs slot changes. If SlotResolver and ItemManager are set,
// it also prints decoded item info.
func (a *agent) OnScreenSlotChange(id int, index int16) error {
	a.itemMgrMu.RLock()
	defer a.itemMgrMu.RUnlock()
	if a.slots == nil || a.itemMgr == nil {
		a.logf("Screen slot change: screenID=%d index=%d", id, index)
		return nil
	}
	itemID, count, ok := a.slots.ResolveSlot(id, index)
	if !ok {
		a.logf("Screen slot change: screenID=%d index=%d (empty)", id, index)
		return nil
	}
	name := a.itemMgr.GetItemNameByID(itemID)
	if name == "" {
		name = "unknown"
	}
	a.logf("Screen slot change: screenID=%d index=%d -> [%s] x%d (id=%d)", id, index, name, count, itemID)
	return nil
}
