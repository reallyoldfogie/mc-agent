package agent

import "github.com/reallyoldfogie/mc-agent/models"

// ScreenOperations implementation

// GetScreenManager returns the concrete screen manager for tests that need direct access.
// This is primarily for testing. Production code should use the ScreenOperations interface methods.
func (a *agent) GetScreenManager() models.ScreenSubsystem {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.screenMgr
}

// GetScreen returns the screen/container for a given window ID.
// Returns nil if the window doesn't exist.
// Implements ScreenOperations interface.
func (a *agent) GetScreen(windowID int) Screen {
	a.mu.Lock()
	sm := a.screenMgr
	a.mu.Unlock()

	if sm == nil {
		return nil
	}

	// Call interface method (sm is ScreenSubsystem interface)
	return sm.GetScreenByID(windowID)
}

// GetInventory returns the player's main inventory.
// Implements ScreenOperations interface.
func (a *agent) GetInventory() Screen {
	a.mu.Lock()
	sm := a.screenMgr
	a.mu.Unlock()

	if sm == nil {
		return nil
	}

	// Call interface method (sm is ScreenSubsystem interface)
	return sm.GetPlayerInventory()
}

// GetCursor returns the cursor slot.
// Implements ScreenOperations interface.
func (a *agent) GetCursor() Slot {
	a.mu.Lock()
	sm := a.screenMgr
	a.mu.Unlock()

	if sm == nil {
		return Slot{ID: -1, Count: 0}
	}

	// Call interface method to get cursor slot (returns screen.Slot directly)
	cursorSlot := sm.GetCursorSlot()

	// Convert from screen.Slot to agent.Slot
	return Slot{
		ID:    int32(cursorSlot.ID),
		Count: int8(cursorSlot.Count),
	}
}

// WorldOperations implementation

// GetWorld returns the world manager for block queries and pathfinding.
// Returns nil if the world is not initialized.
// Implements WorldOperations interface.
func (a *agent) GetWorld() models.World {
	a.mu.Lock()
	wm := a.worldMgr
	a.mu.Unlock()

	if wm == nil {
		return nil
	}

	// world.World implements the World interface (has GetBlockAt method)
	return wm
}
