package agent

import (
	"strings"
	"testing"
)

func TestValidateCreeperWeaponRequiresSelectedWeapon(t *testing.T) {
	tests := []struct {
		name     string
		itemName string
		wantErr  bool
	}{
		{name: "empty selection", itemName: "", wantErr: true},
		{name: "sword", itemName: "minecraft:netherite_sword"},
		{name: "axe", itemName: "minecraft:diamond_axe"},
		{name: "spear", itemName: "minecraft:spear"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreeperWeapon(tt.itemName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCreeperWeapon(%q) error = %v, wantErr %v", tt.itemName, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreeperWeaponErrorExplainsCallerSetup(t *testing.T) {
	err := validateCreeperWeapon("")
	if err == nil || !strings.Contains(err.Error(), "must already be selected") {
		t.Fatalf("validateCreeperWeapon() error = %v, want selected-weapon guidance", err)
	}
}
