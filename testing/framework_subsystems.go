package testing

import (
	"fmt"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// wireAgentSubsystems provides minimal test-specific wiring.
// Most subsystems (Player, World, Chat, Screen, PathFinder, Movement) are now created
// automatically in agent.Init(), so this function is much simpler.
func wireAgentSubsystems(agnt models.Agent) {
	// All subsystems are created automatically in agent.Init()!
	// We only need to set test-specific adapters here.

	// Set slot resolver (adapts screen manager for following/recipes)
	// Note: The screen manager is created automatically, we just need to provide an adapter
	agnt.SetSlotResolver(slotResolverFromAgent{agnt: agnt})
	agnt.SetItemManager(registryItemMgrAdapter{agnt: agnt})
}

// slotResolverFromAgent adapts the agent's screen operations to the SlotResolver interface.
type slotResolverFromAgent struct {
	agnt models.Agent
}

func (sr slotResolverFromAgent) ResolveSlot(id int, index int16) (itemID int32, count int, ok bool) {
	// Get the screen/inventory from the agent
	var screenContainer interface{}

	if id == -2 {
		// Player inventory
		screenContainer = sr.agnt.GetInventory()
	} else if id >= 0 {
		// Container window
		screenContainer = sr.agnt.GetScreen(id)
	}

	if id == -1 && index == -1 {
		// Cursor slot
		cursor := sr.agnt.GetCursor()
		if cursor.ID >= 0 {
			return int32(cursor.ID), int(cursor.Count), true
		}
		return 0, 0, false
	}

	// Type assert to screen.Inventory to access slots
	if inv, ok := screenContainer.(screen.Inventory); ok {
		if index >= 0 && index < int16(len(inv.GetSlots())) {
			s := inv.GetSlots()[index]
			if s.ID >= 0 {
				return int32(s.ID), int(s.Count), true
			}
		}
		return 0, 0, false
	}

	// Type assert to other container types as needed
	if screenContainer != nil {
		// Generic container access
		if c, ok := screenContainer.(interface{ GetSlots() []screen.Slot }); ok {
			slots := c.GetSlots()
			if index >= 0 && index < int16(len(slots)) {
				s := slots[index]
				if s.ID >= 0 {
					return int32(s.ID), int(s.Count), true
				}
			}
		}
	}

	return 0, 0, false
}

// registryItemMgrAdapter provides item names using the agent's registry data.
type registryItemMgrAdapter struct {
	agnt models.Agent
}

func (m registryItemMgrAdapter) GetItemNameByID(id int32) string {
	if m.agnt == nil {
		return ""
	}
	reg := m.agnt.GetRegistry("minecraft:item")
	if reg == nil || !reg.IsReady() {
		return ""
	}
	if name, ok := reg.GetNameByID(id); ok {
		return name
	}
	return fmt.Sprintf("item_%d", id)
}
