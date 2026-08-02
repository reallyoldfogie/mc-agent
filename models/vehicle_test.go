package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetVehicleTypeCoversLlamas guards the mapping that routes a mounted llama
// to its own handler. Before llamas were registered, GetVehicleType returned
// VehicleTypeNone and the riding dispatch fell through to the horse handler,
// which applied steering, forward thrust and jump charge to an entity that
// ignores all three.
func TestGetVehicleTypeCoversLlamas(t *testing.T) {
	assert.Equal(t, VehicleTypeLlama, GetVehicleType(EntityTypeLlama))
	assert.Equal(t, VehicleTypeTraderLlama, GetVehicleType(EntityTypeTraderLlama))
	assert.Equal(t, "llama", VehicleTypeLlama.String())
	assert.Equal(t, "trader_llama", VehicleTypeTraderLlama.String())
}

func TestEntityTypeRideableVersusSteerable(t *testing.T) {
	tests := []struct {
		name       string
		entityType EntityType
		rideable   bool
		steerable  bool
	}{
		{name: "horse", entityType: EntityTypeHorse, rideable: true, steerable: true},
		{name: "donkey", entityType: EntityTypeDonkey, rideable: true, steerable: true},
		{name: "mule", entityType: EntityTypeMule, rideable: true, steerable: true},
		{name: "pig", entityType: EntityTypePig, rideable: true, steerable: true},
		{name: "strider", entityType: EntityTypeStrider, rideable: true, steerable: true},
		{name: "camel", entityType: EntityTypeCamel, rideable: true, steerable: true},
		{name: "boat", entityType: EntityTypeBoat, rideable: true, steerable: true},
		// Llamas accept a passenger but take no saddle and ignore rider input.
		{name: "llama", entityType: EntityTypeLlama, rideable: true, steerable: false},
		{name: "trader llama", entityType: EntityTypeTraderLlama, rideable: true, steerable: false},
		// Not rideable at all.
		{name: "zombie", entityType: EntityTypeZombie, rideable: false, steerable: false},
		{name: "villager", entityType: EntityTypeVillager, rideable: false, steerable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.rideable, tt.entityType.IsRideable(), "IsRideable")
			assert.Equal(t, tt.steerable, tt.entityType.IsSteerable(), "IsSteerable")
		})
	}
}

// TestSteerableImpliesRideable pins the invariant that nothing can be steered
// without also being rideable.
func TestSteerableImpliesRideable(t *testing.T) {
	allTypes := []EntityType{
		EntityTypeHorse, EntityTypeSkeletonHorse, EntityTypeZombieHorse,
		EntityTypeDonkey, EntityTypeMule, EntityTypeLlama, EntityTypeTraderLlama,
		EntityTypeCamel, EntityTypeCamelHusk, EntityTypeBoat, EntityTypeChestBoat,
		EntityTypePig, EntityTypeStrider, EntityTypeNautilus, EntityTypeZombieNautilus,
		EntityTypeZombie, EntityTypeVillager, EntityTypeUnknown,
	}

	for _, entityType := range allTypes {
		if entityType.IsSteerable() {
			require.True(t, entityType.IsRideable(),
				"%s is steerable but not rideable", entityType)
		}
	}
}

func TestVehicleTypeRideableVersusSteerable(t *testing.T) {
	t.Run("none is neither", func(t *testing.T) {
		assert.False(t, VehicleTypeNone.IsRideable())
		assert.False(t, VehicleTypeNone.IsSteerable())
	})

	t.Run("llamas are rideable but not steerable", func(t *testing.T) {
		assert.True(t, VehicleTypeLlama.IsRideable())
		assert.False(t, VehicleTypeLlama.IsSteerable())
		assert.True(t, VehicleTypeTraderLlama.IsRideable())
		assert.False(t, VehicleTypeTraderLlama.IsSteerable())
	})

	t.Run("conventional mounts are both", func(t *testing.T) {
		for _, vehicleType := range []VehicleType{
			VehicleTypeHorse, VehicleTypeBoat, VehicleTypeMinecart,
			VehicleTypeCamel, VehicleTypePig, VehicleTypeStrider,
			VehicleTypeDonkey, VehicleTypeMule, VehicleTypeNautilus,
		} {
			assert.True(t, vehicleType.IsRideable(), "%s should be rideable", vehicleType)
			assert.True(t, vehicleType.IsSteerable(), "%s should be steerable", vehicleType)
		}
	})
}

// TestLlamaIsLivingEntity ensures llamas participate in living-entity handling
// (metadata, attributes) the same way other mounts do.
func TestLlamaIsLivingEntity(t *testing.T) {
	assert.True(t, EntityTypeLlama.IsLivingEntity())
	assert.True(t, EntityTypeTraderLlama.IsLivingEntity())
}
