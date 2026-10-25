package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestParseInventorySnapshot_EmptyInventory(t *testing.T) {
	snapshot := parseInventorySnapshot("Bob has no item in slot")
	if len(snapshot) != 0 {
		t.Fatalf("snapshot = %+v, want empty", snapshot)
	}
}

func TestParseInventorySnapshot_PlainItemsNoComponents(t *testing.T) {
	resp := `Bob has the following entity data: [{Slot: 0b, id: "minecraft:diamond_sword", count: 1}, {Slot: 9b, id: "minecraft:diamond", count: 5}]`
	snapshot := parseInventorySnapshot(resp)
	if len(snapshot) != 2 {
		t.Fatalf("snapshot = %+v, want 2 entries", snapshot)
	}
	if snapshot[0].Slot != 0 || snapshot[0].ID != "minecraft:diamond_sword" || snapshot[0].Count != 1 {
		t.Errorf("snapshot[0] = %+v, want {0 minecraft:diamond_sword 1 \"\"}", snapshot[0])
	}
	if snapshot[0].Components != "" {
		t.Errorf("snapshot[0].Components = %q, want empty", snapshot[0].Components)
	}
	if snapshot[1].Slot != 9 || snapshot[1].ID != "minecraft:diamond" || snapshot[1].Count != 5 {
		t.Errorf("snapshot[1] = %+v, want {9 minecraft:diamond 5 \"\"}", snapshot[1])
	}
}

func TestParseInventorySnapshot_CapturesComponentsNBT(t *testing.T) {
	resp := `Bob has the following entity data: [{Slot: 103b, id: "minecraft:diamond_helmet", count: 1, components: {"minecraft:enchantments": {levels: {"minecraft:protection": 2}}}}]`
	snapshot := parseInventorySnapshot(resp)
	if len(snapshot) != 1 {
		t.Fatalf("snapshot = %+v, want 1 entry", snapshot)
	}
	want := `{"minecraft:enchantments": {levels: {"minecraft:protection": 2}}}`
	if snapshot[0].Components != want {
		t.Errorf("Components = %q, want %q", snapshot[0].Components, want)
	}
}

func TestConvertComponentsNBTToBracketSyntax(t *testing.T) {
	nbt := `{"minecraft:enchantments": {levels: {"minecraft:sharpness": 1}}, "minecraft:damage": 3}`
	got, err := convertComponentsNBTToBracketSyntax(nbt)
	if err != nil {
		t.Fatalf("convertComponentsNBTToBracketSyntax: %v", err)
	}
	want := `[minecraft:enchantments={levels: {"minecraft:sharpness": 1}},minecraft:damage=3]`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertComponentsNBTToBracketSyntax_RejectsNonCompound(t *testing.T) {
	if _, err := convertComponentsNBTToBracketSyntax("not-a-compound"); err == nil {
		t.Fatal("want an error for a non brace-delimited input")
	}
}

func TestInventorySlotArg(t *testing.T) {
	cases := []struct {
		slot int
		want string
	}{
		{0, "hotbar.0"}, {8, "hotbar.8"},
		{9, "inventory.0"}, {35, "inventory.26"},
		{100, "armor.feet"}, {101, "armor.legs"}, {102, "armor.chest"}, {103, "armor.head"},
		{-106, "weapon.offhand"},
	}
	for _, c := range cases {
		got, err := inventorySlotArg(c.slot)
		if err != nil {
			t.Errorf("inventorySlotArg(%d): %v", c.slot, err)
			continue
		}
		if got != c.want {
			t.Errorf("inventorySlotArg(%d) = %q, want %q", c.slot, got, c.want)
		}
	}
	if _, err := inventorySlotArg(999); err == nil {
		t.Error("want an error for an unsupported slot")
	}
}

// inventoryRCON answers "data get entity <name> Inventory" for a configured
// player with a fixed response, and records every command issued, so
// SaveInventorySnapshot/RestoreInventorySnapshot's own proximity gating and
// command sequence can be exercised without a live server.
type inventoryRCON struct {
	execOnlyRCON
	playerName       string
	x, y, z          float64
	inventoryReponse string
}

func (r *inventoryRCON) Exec(ctx context.Context, cmd string) (string, error) {
	r.cmds = append(r.cmds, cmd)
	if cmd == fmt.Sprintf("data get entity %s Pos", r.playerName) {
		return fmt.Sprintf("%s has the following entity data: [%.1fd, %.1fd, %.1fd]", r.playerName, r.x, r.y, r.z), nil
	}
	if cmd == fmt.Sprintf("data get entity %s Inventory", r.playerName) {
		return r.inventoryReponse, nil
	}
	return "", nil
}

func newInventoryRCONTestAgent(t *testing.T, rc *inventoryRCON) *agent {
	t.Helper()
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565", Name: "TestBot", RCON: rc})
	if err != nil {
		t.Fatal(err)
	}
	return agentInt.(*agent)
}

func TestSaveInventorySnapshot_RefusesWhenPlayerIsFarFromArena(t *testing.T) {
	rc := &inventoryRCON{playerName: "Bystander", x: 500, y: 64, z: 500}
	a := newInventoryRCONTestAgent(t, rc)
	_, err := a.SaveInventorySnapshot(context.Background(), "Bystander", models.V3{X: 0, Y: 64, Z: 0}, 10)
	if err == nil {
		t.Fatal("want an error when the named player is far from the arena center")
	}
	for _, cmd := range rc.cmds {
		if strings.Contains(cmd, "Inventory") {
			t.Fatalf("SaveInventorySnapshot must not read a far-away player's inventory, but sent: %q", cmd)
		}
	}
}

func TestSaveInventorySnapshot_CapturesItemsWhenNear(t *testing.T) {
	rc := &inventoryRCON{
		playerName:       "Combatant",
		x:                1, y: 64, z: 1,
		inventoryReponse: `Combatant has the following entity data: [{Slot: 0b, id: "minecraft:iron_sword", count: 1}]`,
	}
	a := newInventoryRCONTestAgent(t, rc)
	snapshot, err := a.SaveInventorySnapshot(context.Background(), "Combatant", models.V3{X: 0, Y: 64, Z: 0}, 10)
	if err != nil {
		t.Fatalf("SaveInventorySnapshot: %v", err)
	}
	if len(snapshot) != 1 || snapshot[0].ID != "minecraft:iron_sword" {
		t.Fatalf("snapshot = %+v, want one minecraft:iron_sword slot", snapshot)
	}
}

func TestRestoreInventorySnapshot_RefusesWhenPlayerIsFarFromArena(t *testing.T) {
	rc := &inventoryRCON{playerName: "Bystander", x: 500, y: 64, z: 500}
	a := newInventoryRCONTestAgent(t, rc)
	snapshot := InventorySnapshot{{Slot: 0, ID: "minecraft:iron_sword", Count: 1}}
	err := a.RestoreInventorySnapshot(context.Background(), "Bystander", snapshot, models.V3{X: 0, Y: 64, Z: 0}, 10)
	if err == nil {
		t.Fatal("want an error when the named player is far from the arena center")
	}
	for _, cmd := range rc.cmds {
		if cmd == "clear Bystander" {
			t.Fatal("RestoreInventorySnapshot must not touch a far-away player's inventory")
		}
	}
}

func TestRestoreInventorySnapshot_ReplaysSlotsWhenNear(t *testing.T) {
	rc := &inventoryRCON{playerName: "Combatant", x: 1, y: 64, z: 1}
	a := newInventoryRCONTestAgent(t, rc)
	snapshot := InventorySnapshot{
		{Slot: 0, ID: "minecraft:iron_sword", Count: 1},
		{Slot: 103, ID: "minecraft:diamond_helmet", Count: 1, Components: `{"minecraft:enchantments": {levels: {"minecraft:protection": 2}}}`},
	}
	if err := a.RestoreInventorySnapshot(context.Background(), "Combatant", snapshot, models.V3{X: 0, Y: 64, Z: 0}, 10); err != nil {
		t.Fatalf("RestoreInventorySnapshot: %v", err)
	}

	want := []string{
		"clear Combatant",
		"item replace entity Combatant hotbar.0 with minecraft:iron_sword 1",
		`item replace entity Combatant armor.head with minecraft:diamond_helmet[minecraft:enchantments={levels: {"minecraft:protection": 2}}] 1`,
	}
	for _, wantCmd := range want {
		found := false
		for _, cmd := range rc.cmds {
			if cmd == wantCmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("commands = %v, want to include %q", rc.cmds, wantCmd)
		}
	}
}
