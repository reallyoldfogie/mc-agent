package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaddleSlotSupported pins the version boundary at which saddle state
// becomes observable. Before 1.21.5 the saddle lived in the mount's NBT
// inventory and was never sent to the client, so we must report "unknown"
// rather than "unsaddled" — otherwise every mount on an older server would be
// demoted to a passive ride.
func TestSaddleSlotSupported(t *testing.T) {
	tests := []struct {
		version   string
		supported bool
	}{
		{version: "1.21.1", supported: false},
		{version: "1.21.2", supported: false},
		{version: "1.21.3", supported: false},
		{version: "1.21.4", supported: false},
		{version: "1.21.5", supported: true},
		{version: "1.21.6", supported: true},
		{version: "1.21.7", supported: true},
		{version: "1.21.8", supported: true},
		{version: "1.21.9", supported: true},
		{version: "1.21.10", supported: true},
		{version: "1.21.11", supported: true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.supported, saddleSlotSupported(tt.version))
		})
	}
}

func TestSaddleSlotSupportedRejectsUnusableVersions(t *testing.T) {
	// An unknown or unparseable version must fail closed to "cannot tell",
	// which preserves the previous driving behaviour rather than silently
	// turning the agent into a passenger.
	assert.False(t, saddleSlotSupported(""))
	assert.False(t, saddleSlotSupported("not-a-version"))
}

func TestEquipmentHasSaddle(t *testing.T) {
	t.Run("empty equipment is not saddled", func(t *testing.T) {
		assert.False(t, equipmentHasSaddle(map[models.EquipmentSlotType]models.InventorySlot{}))
	})

	t.Run("occupied saddle slot is saddled", func(t *testing.T) {
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotSaddle: {Present: true, Count: 1},
		}
		assert.True(t, equipmentHasSaddle(equipment))
	})

	t.Run("empty saddle slot is not saddled", func(t *testing.T) {
		// The server clears a slot by sending it with count 0 rather than
		// omitting it, so a present-but-empty entry must read as unsaddled.
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotSaddle: {Present: false, Count: 0},
		}
		assert.False(t, equipmentHasSaddle(equipment))
	})

	t.Run("body armour alone is not a saddle", func(t *testing.T) {
		// Body (6) and saddle (7) are adjacent slots; an armoured but
		// unsaddled horse must not read as saddled.
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotBody: {Present: true, Count: 1},
		}
		assert.False(t, equipmentHasSaddle(equipment))
	})

	t.Run("other equipment does not mask the saddle", func(t *testing.T) {
		equipment := map[models.EquipmentSlotType]models.InventorySlot{
			models.EquipmentSlotMainHand: {Present: true, Count: 1},
			models.EquipmentSlotBody:     {Present: true, Count: 1},
			models.EquipmentSlotSaddle:   {Present: true, Count: 1},
		}
		assert.True(t, equipmentHasSaddle(equipment))
	})
}

// TestIsMountedEntitySaddledWithoutVersionHandler covers the defensive path
// used in tests and during early startup: with no version handler there is no
// way to know the version, so saddle state must be unknown.
func TestIsMountedEntitySaddledWithoutVersionHandler(t *testing.T) {
	testAgent := &agent{
		entities: map[int32]*trackedEntity{
			42: {
				EntityID: 42,
				Equipment: map[models.EquipmentSlotType]models.InventorySlot{
					models.EquipmentSlotSaddle: {Present: true, Count: 1},
				},
			},
		},
	}

	saddled, known := testAgent.IsMountedEntitySaddled(42)
	require.False(t, known, "version is unknown, so saddle state must be too")
	assert.False(t, saddled)
}
