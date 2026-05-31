package models

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicMetadataProcessor_PoseFlowsToResult(t *testing.T) {
	reg := NewEntityRegistry()
	reg.RegisterEntity(42, EntityTypeCamel)
	proc := NewBasicMetadataProcessor(reg)

	// Ordinal 10 maps to "sitting" in the fallback table that the processor
	// is seeded with by NewBasicMetadataProcessor.
	poseVal := pk.VarInt(10)
	entry := MetadataEntry{
		Key:       5,
		HandlerID: HandlerEntityPose,
		Value:     &poseVal,
	}

	result, err := proc.HandleMetadata(42, entry)
	require.NoError(t, err)
	assert.True(t, result.HasPose, "HasPose should be set when the entry is a pose")
	assert.Equal(t, int32(10), result.Pose)
	assert.Equal(t, "sitting", result.PoseName)
}

func TestBasicMetadataProcessor_UnknownPoseOrdinal(t *testing.T) {
	reg := NewEntityRegistry()
	reg.RegisterEntity(7, EntityTypePlayer)
	proc := NewBasicMetadataProcessor(reg)

	poseVal := pk.VarInt(999)
	entry := MetadataEntry{
		Key:       5,
		HandlerID: HandlerEntityPose,
		Value:     &poseVal,
	}

	result, err := proc.HandleMetadata(7, entry)
	require.NoError(t, err)
	assert.True(t, result.HasPose)
	assert.Equal(t, int32(999), result.Pose)
	assert.Equal(t, "unknown_999", result.PoseName)
}

func TestBasicMetadataProcessor_InjectedRegistryOverridesFallback(t *testing.T) {
	reg := NewEntityRegistry()
	reg.RegisterEntity(1, EntityTypeCamel)
	proc := NewBasicMetadataProcessor(reg)

	// Pretend a future version reorders the enum: sitting at ordinal 2.
	custom := newRegistryFromOrdinalMap(map[int32]string{
		0: "standing",
		1: "fall_flying",
		2: "sitting",
	}, false)
	proc.SetPoseRegistry(custom)

	poseVal := pk.VarInt(2)
	entry := MetadataEntry{
		Key:       5,
		HandlerID: HandlerEntityPose,
		Value:     &poseVal,
	}
	result, err := proc.HandleMetadata(1, entry)
	require.NoError(t, err)
	assert.Equal(t, "sitting", result.PoseName, "injected registry should drive the name")

	// And ordinal 10 (sitting in fallback) is now out of range, so unknown.
	poseVal2 := pk.VarInt(10)
	entry.Value = &poseVal2
	result2, err := proc.HandleMetadata(1, entry)
	require.NoError(t, err)
	assert.Equal(t, "unknown_10", result2.PoseName)
}

func TestBasicMetadataProcessor_NonPoseEntryDoesNotSetHasPose(t *testing.T) {
	reg := NewEntityRegistry()
	reg.RegisterEntity(1, EntityTypePlayer)
	proc := NewBasicMetadataProcessor(reg)

	val := pk.Float(20.0)
	entry := MetadataEntry{
		Key:       9,
		HandlerID: HandlerFloat,
		Value:     &val,
	}
	result, err := proc.HandleMetadata(1, entry)
	require.NoError(t, err)
	assert.False(t, result.HasPose)
	assert.Empty(t, result.PoseName)
}
