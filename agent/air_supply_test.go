package agent

import "testing"

// TestGetOwnAirSupply covers WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 6: the bot's own AIR
// metadata value must be exposed correctly, and distinguished from "never received an update"
// (unlike HorseFlags/IsBaby, a full air value of 300 is a real, frequently-resent value - it must
// not be conflated with "unknown").
func TestGetOwnAirSupply(t *testing.T) {
	t.Run("no update received yet reads as not found", func(t *testing.T) {
		a := &agent{}
		ticks, found := a.GetOwnAirSupply()
		if found {
			t.Errorf("expected found=false before any AIR metadata update, got ticks=%d found=%v", ticks, found)
		}
	})

	t.Run("reports the last stored value once set", func(t *testing.T) {
		a := &agent{}
		a.ownAirSupply = 300
		a.hasOwnAirSupply = true

		ticks, found := a.GetOwnAirSupply()
		if !found {
			t.Fatal("expected found=true after an AIR metadata update")
		}
		if ticks != 300 {
			t.Errorf("expected ticks=300, got %d", ticks)
		}
	})

	t.Run("low air value is not mistaken for unset", func(t *testing.T) {
		a := &agent{}
		a.ownAirSupply = 0
		a.hasOwnAirSupply = true

		ticks, found := a.GetOwnAirSupply()
		if !found {
			t.Fatal("expected found=true even when air has run out (ticks=0), not conflated with never-updated")
		}
		if ticks != 0 {
			t.Errorf("expected ticks=0, got %d", ticks)
		}
	})
}
