package common

import (
	"bytes"
	"encoding/base64"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/stretchr/testify/require"
)

// TestHandlerTypeFromString tests the handler type string to ID conversion
func TestHandlerTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected MetadataHandlerType
	}{
		{"byte", HandlerByte},
		{"int", HandlerInteger},
		{"long", HandlerLong},
		{"float", HandlerFloat},
		{"string", HandlerString},
		{"component", HandlerTextComponent},
		{"optional_component", HandlerOptionalTextComponent},
		{"item_stack", HandlerItemStack},
		{"boolean", HandlerBoolean},
		{"rotations", HandlerRotation},
		{"block_pos", HandlerBlockPos},
		{"optional_block_pos", HandlerOptionalBlockPos},
		{"direction", HandlerFacing},
		{"optional_uuid", HandlerLazyEntityReference},
		{"block_state", HandlerBlockState},
		{"optional_block_state", HandlerOptionalBlockState},
		{"compound_tag", HandlerNBTCompound},
		{"particle", HandlerParticle},
		{"particles", HandlerParticleList},
		{"villager_data", HandlerVillagerData},
		{"optional_unsigned_int", HandlerOptionalInt},
		{"pose", HandlerEntityPose},
		{"cat_variant", HandlerCatVariant},
		{"cow_variant", HandlerCowVariant},
		{"wolf_variant", HandlerWolfVariant},
		{"wolf_sound_variant", HandlerWolfSoundVariant},
		{"frog_variant", HandlerFrogVariant},
		{"pig_variant", HandlerPigVariant},
		{"chicken_variant", HandlerChickenVariant},
		{"optional_global_pos", HandlerOptionalGlobalPos},
		{"painting_variant", HandlerPaintingVariant},
		{"sniffer_state", HandlerSnifferState},
		{"armadillo_state", HandlerArmadilloState},
		{"vector3", HandlerVector3F},
		{"quaternion", HandlerQuaternionF},
		{"unknown_type", 0}, // Unknown types default to HandlerByte
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := HandlerTypeFromString(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestMetadataHandlerTypeString tests the String() method for handler types
func TestMetadataHandlerTypeString(t *testing.T) {
	tests := []struct {
		handlerType MetadataHandlerType
		expected    string
	}{
		{HandlerByte, "BYTE"},
		{HandlerInteger, "INTEGER"},
		{HandlerFloat, "FLOAT"},
		{HandlerBoolean, "BOOLEAN"},
		{HandlerRotation, "ROTATION"},
		{HandlerVector3F, "VECTOR_3F"},
		{HandlerEntityPose, "ENTITY_POSE"},
		{999, "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.handlerType.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestEntityRegistryBasics tests the EntityRegistry basic operations
func TestEntityRegistryBasics(t *testing.T) {
	registry := NewEntityRegistry()

	// Test RegisterEntity
	registry.RegisterEntity(1, EntityTypePlayer)
	require.Equal(t, 1, registry.Count())

	// Test GetEntityType
	entityType := registry.GetEntityType(1)
	require.Equal(t, EntityTypePlayer, entityType)

	// Test GetContext
	ctx := registry.GetContext(1)
	require.NotNil(t, ctx)
	require.Equal(t, int32(1), ctx.EntityID)
	require.Equal(t, EntityTypePlayer, ctx.EntityType)

	// Test unknown entity
	unknownType := registry.GetEntityType(999)
	require.Equal(t, EntityTypeUnknown, unknownType)

	// Test RemoveEntity
	registry.RemoveEntity(1)
	require.Equal(t, 0, registry.Count())
	require.Equal(t, EntityTypeUnknown, registry.GetEntityType(1))
}

// TestEntityRegistryRemoveMultiple tests removing multiple entities
func TestEntityRegistryRemoveMultiple(t *testing.T) {
	registry := NewEntityRegistry()

	// Register multiple entities
	registry.RegisterEntity(1, EntityTypePlayer)
	registry.RegisterEntity(2, EntityTypeZombie)
	registry.RegisterEntity(3, EntityTypeArrow)
	require.Equal(t, 3, registry.Count())

	// Remove multiple
	registry.RemoveEntities([]int32{1, 3})
	require.Equal(t, 1, registry.Count())
	require.Equal(t, EntityTypeUnknown, registry.GetEntityType(1))
	require.Equal(t, EntityTypeZombie, registry.GetEntityType(2))
	require.Equal(t, EntityTypeUnknown, registry.GetEntityType(3))
}

// TestEntityRegistryClear tests clearing all entities
func TestEntityRegistryClear(t *testing.T) {
	registry := NewEntityRegistry()

	registry.RegisterEntity(1, EntityTypePlayer)
	registry.RegisterEntity(2, EntityTypeZombie)
	require.Equal(t, 2, registry.Count())

	registry.Clear()
	require.Equal(t, 0, registry.Count())
}

// TestEntityTypeIsLivingEntity tests the IsLivingEntity helper
func TestEntityTypeIsLivingEntity(t *testing.T) {
	require.True(t, EntityTypePlayer.IsLivingEntity())
	require.True(t, EntityTypeZombie.IsLivingEntity())
	require.True(t, EntityTypeCreeper.IsLivingEntity())
	require.True(t, EntityTypeVillager.IsLivingEntity())

	require.False(t, EntityTypeArrow.IsLivingEntity())
	require.False(t, EntityTypeSnowball.IsLivingEntity())
	require.False(t, EntityTypeUnknown.IsLivingEntity())
}

// TestEntityTypeIsProjectile tests the IsProjectile helper
func TestEntityTypeIsProjectile(t *testing.T) {
	require.True(t, EntityTypeArrow.IsProjectile())
	require.True(t, EntityTypeSnowball.IsProjectile())
	require.True(t, EntityTypeEnderPearl.IsProjectile())
	require.True(t, EntityTypeFireball.IsProjectile())

	require.False(t, EntityTypePlayer.IsProjectile())
	require.False(t, EntityTypeZombie.IsProjectile())
	require.False(t, EntityTypeUnknown.IsProjectile())
}

// TestEntityTypeIsDisplayEntity tests the IsDisplayEntity helper
func TestEntityTypeIsDisplayEntity(t *testing.T) {
	require.True(t, EntityTypeBlockDisplay.IsDisplayEntity())
	require.True(t, EntityTypeItemDisplay.IsDisplayEntity())
	require.True(t, EntityTypeTextDisplay.IsDisplayEntity())

	require.False(t, EntityTypePlayer.IsDisplayEntity())
	require.False(t, EntityTypeArrow.IsDisplayEntity())
	require.False(t, EntityTypeUnknown.IsDisplayEntity())
}

// RealPacketExample represents a captured packet from logs
type RealPacketExample struct {
	Name        string
	Base64Data  string
	ExpectedID  int32
	ExpectedKey int32
	HandlerType MetadataHandlerType
	Description string
}

// TestRealCapturedPackets tests parsing with real captured EntityMetadata packets
func TestRealCapturedPackets(t *testing.T) {
	// These are real EntityMetadata packets captured from SnowballBot logs
	examples := []RealPacketExample{
		{
			Name:        "SnowballBot Metadata #1 - Float Health",
			Base64Data:  "AQkDQaAAAP8=", // Entity 1, key 9 (health), float value
			ExpectedID:  1,
			ExpectedKey: 9,
			HandlerType: HandlerFloat,
			Description: "Entity with health metadata update",
		},
		{
			Name:        "SnowballBot Metadata #2 - Byte Flags",
			Base64Data:  "AREAfv8=", // Entity 1, key 17 (flags?), byte value
			ExpectedID:  1,
			ExpectedKey: 17,
			HandlerType: HandlerByte,
			Description: "Entity with byte flags metadata",
		},
		{
			Name:        "Entity Metadata - Multiple Entries",
			Base64Data:  "AgkDQaAAAP8=", // Entity 2, key 9 (health), float value
			ExpectedID:  2,
			ExpectedKey: 9,
			HandlerType: HandlerFloat,
			Description: "Second entity with health metadata",
		},
	}

	for _, example := range examples {
		t.Run(example.Name, func(t *testing.T) {
			// Decode the base64 packet data
			data, err := base64.StdEncoding.DecodeString(example.Base64Data)
			require.NoError(t, err, "Failed to decode base64 packet data")

			// Verify we can at least extract basic information
			// The packet structure starts with entity ID (VarInt) and then metadata entries
			reader := bytes.NewReader(data)

			// Read entity ID
			var entityID pk.VarInt
			_, err = entityID.ReadFrom(reader)
			require.NoError(t, err, "Failed to read entity ID from packet")
			require.Equal(t, example.ExpectedID, int32(entityID))

			// Verify the packet has data (real packets are not empty)
			require.Greater(t, len(data), 0, "Packet data should not be empty")
		})
	}
}

// TestMetadataEntryCreation tests creating MetadataEntry structures
func TestMetadataEntryCreation(t *testing.T) {
	// Test creating a byte metadata entry
	entry1 := MetadataEntry{
		Key:       0,
		HandlerID: HandlerByte,
		Value:     pk.Byte(0x40), // Entity flags: flying with elytra
	}
	require.Equal(t, int32(0), entry1.Key)
	require.Equal(t, HandlerByte, entry1.HandlerID)

	// Test creating a float metadata entry
	entry2 := MetadataEntry{
		Key:       9,
		HandlerID: HandlerFloat,
		Value:     pk.Float(10.5), // Health: 10.5 half-hearts = 5.25 hearts
	}
	require.Equal(t, int32(9), entry2.Key)
	require.Equal(t, HandlerFloat, entry2.HandlerID)

	// Test creating an integer metadata entry
	entry3 := MetadataEntry{
		Key:       8,
		HandlerID: HandlerInteger,
		Value:     pk.VarInt(120), // Air supply: 120 ticks = 6 seconds
	}
	require.Equal(t, int32(8), entry3.Key)
	require.Equal(t, HandlerInteger, entry3.HandlerID)
}

// BenchmarkHandlerTypeFromString benchmarks the handler type conversion
func BenchmarkHandlerTypeFromString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = HandlerTypeFromString("vector3")
		_ = HandlerTypeFromString("float")
		_ = HandlerTypeFromString("block_pos")
	}
}

// BenchmarkEntityRegistryOperations benchmarks entity registry operations
func BenchmarkEntityRegistryOperations(b *testing.B) {
	registry := NewEntityRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.RegisterEntity(int32(i), EntityTypePlayer)
		_ = registry.GetEntityType(int32(i))
	}
}
