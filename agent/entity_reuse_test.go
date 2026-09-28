package agent

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestNewTrackedEntityFromSpawnDoesNotReuseState(t *testing.T) {
	old := &trackedEntity{
		EntityID:  42,
		Health:    0,
		MaxHealth: 20,
		Removed:   true,
		RemovedAt: time.Now(),
		Attributes: map[string]models.AttributeValue{
			"minecraft:generic.movement_speed": {},
		},
		Equipment: map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotMainHand: {Count: 1},
		},
		Effects: map[string]models.ActiveEffect{
			"minecraft:poison": {},
		},
		Inventory: &models.EntityInventory{},
	}

	now := time.Now()
	spawned := newTrackedEntityFromSpawn(42, 7, [16]byte{9}, 1, 2, 3, 4, 5, 6, 7, 8, now)

	if spawned == old {
		t.Fatal("spawn must allocate a new tracker record")
	}
	if spawned.Removed || !spawned.RemovedAt.IsZero() {
		t.Fatalf("spawned entity retained removed state: %+v", spawned)
	}
	if spawned.Health != 0 || spawned.MaxHealth != 0 {
		t.Fatalf("spawned entity retained health state: health=%v max=%v", spawned.Health, spawned.MaxHealth)
	}
	if spawned.Attributes != nil || spawned.Equipment != nil || spawned.Effects != nil || spawned.Inventory != nil {
		t.Fatal("spawned entity retained mutable state from the previous entity")
	}
	if spawned.EntityID != 42 || spawned.EntityType != 7 || spawned.UUID != [16]byte{9} || spawned.X != 1 || spawned.Y != 2 || spawned.Z != 3 {
		t.Fatalf("spawn data was not initialized correctly: %+v", spawned)
	}
}
