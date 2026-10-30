package agent

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/require"
)

// dimensionTypeNBT builds a minimal "minecraft:dimension_type"-shaped NBT
// compound with just the two fields this plan's height-bounds extraction
// actually reads.
func dimensionTypeNBT(minY, height int32) *protocol_models.NBTCompound {
	c := &protocol_models.NBTCompound{}
	c.Set("min_y", &protocol_models.NBTInt{Value: minY})
	c.Set("height", &protocol_models.NBTInt{Value: height})
	return c
}

func TestDimensionHeightFromNBT(t *testing.T) {
	t.Run("nether shape", func(t *testing.T) {
		minY, maxY, ok := dimensionHeightFromNBT(dimensionTypeNBT(0, 256))
		require.True(t, ok)
		require.Equal(t, int32(0), minY)
		require.Equal(t, int32(255), maxY)
	})

	t.Run("overworld shape", func(t *testing.T) {
		minY, maxY, ok := dimensionHeightFromNBT(dimensionTypeNBT(-64, 384))
		require.True(t, ok)
		require.Equal(t, int32(-64), minY)
		require.Equal(t, int32(319), maxY)
	})

	t.Run("nil NBT", func(t *testing.T) {
		_, _, ok := dimensionHeightFromNBT(nil)
		require.False(t, ok)
	})

	t.Run("missing height field", func(t *testing.T) {
		c := &protocol_models.NBTCompound{}
		c.Set("min_y", &protocol_models.NBTInt{Value: 0})
		_, _, ok := dimensionHeightFromNBT(c)
		require.False(t, ok)
	})

	t.Run("wrong type for min_y", func(t *testing.T) {
		c := &protocol_models.NBTCompound{}
		c.Set("min_y", &protocol_models.NBTString{Value: "not an int"})
		c.Set("height", &protocol_models.NBTInt{Value: 256})
		_, _, ok := dimensionHeightFromNBT(c)
		require.False(t, ok)
	})
}

// TestGetDimensionHeightBounds_NotFoundPaths confirms the wrapper fails
// closed (ok=false, no panic) when the registry data a real server would
// have sent hasn't arrived - the common case in a unit test with no live
// connection, and also the real "dimension not in the registry yet" case
// if a Respawn somehow raced configuration-phase registry delivery.
func TestGetDimensionHeightBounds_NotFoundPaths(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	// No live connection was made, so RegistryData() has nothing in it.
	_, _, ok := a.getDimensionHeightBounds("minecraft:the_nether")
	require.False(t, ok)
}
