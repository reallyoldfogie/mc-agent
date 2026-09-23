package agent

import ()

// OnScreenSlotChange logs slot changes. If SlotResolver and ItemManager are set,
// it also prints decoded item info.
//
// Logged at Debug, not Info (via a.logf's own always-Info a.log().Info(...)
// path): a full inventory/container sync (ClientboundContainerSetContent)
// fires this once per slot - up to 46 calls for one packet - and every
// seed/give/clear during episode reset does exactly that. Structured
// fields (not a pre-formatted a.logf string) so the Sprintf-equivalent
// cost is also skipped when Debug is disabled, not just the write - see
// cmd/rsi-train's own -parallel-envs x tick-rate scaling investigation
// (2026-09-23) that found this one call site alone responsible for
// roughly a third of one run's entire log volume (322k of 948k lines),
// a real contributor to client-side CPU cost under load, not just noise.
func (a *agent) OnScreenSlotChange(id int, index int16) error {
	a.itemMgrMu.RLock()
	defer a.itemMgrMu.RUnlock()
	if a.slots == nil || a.itemMgr == nil {
		a.log().Debug("Screen slot change", "screenID", id, "index", index)
		return nil
	}
	itemID, count, ok := a.slots.ResolveSlot(id, index)
	if !ok {
		a.log().Debug("Screen slot change", "screenID", id, "index", index, "empty", true)
		return nil
	}
	name := a.itemMgr.GetItemNameByID(itemID)
	if name == "" {
		name = "unknown"
	}
	a.log().Debug("Screen slot change", "screenID", id, "index", index, "item", name, "count", count, "itemID", itemID)
	return nil
}
