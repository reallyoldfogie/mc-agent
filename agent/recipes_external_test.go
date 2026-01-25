package agent_test

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	agentpkg "github.com/reallyoldfogie/mc-agent/agent"
	"github.com/stretchr/testify/require"
)

// helper: write Slot Display into builder recursively
func writeSlotDisplay(pb *pk.Builder, t int32, f func()) {
	pb.WriteField(pk.VarInt(t))
	if f != nil {
		f()
	}
}

// helper: write IDSet (ingredient format)
// scanIDSet decoding logic:
// - Read count
// - if count==0: tag list follows (varint count + tag names)
// - if count!=0:
//   - ids[0] = count-1
//   - Read count-1 more IDs into ids[1..count-1]
//
// So to encode [A, B, C] (3 IDs):
// - We want: ids[0]=A, then read 2 more (B, C)
// - count-1 must equal A
// - count = A+1
// - But then we read count-1 = A more IDs, which is wrong!
//
// Actually: to get N IDs, write count=N, then N-1 IDs (since ids[0]=count-1)
// - Write count = N
// - ids[0] will be set to N-1
// - Read N-1 more IDs
// - Total: N IDs, first one is N-1
//
// So this encoding only works when the first ID = len-1!
// For [A, B, C], we need A = 2 (len-1), so we can only encode [2, B, C]
func writeIDSet(pb *pk.Builder, ids ...int32) {
	if len(ids) == 0 {
		// Empty - write count 0 then empty tag list
		pb.WriteField(pk.VarInt(0), pk.VarInt(0))
		return
	}
	// Write count = len(ids)
	pb.WriteField(pk.VarInt(len(ids)))
	// Write remaining IDs (all but the first, since first = count-1)
	for i := 1; i < len(ids); i++ {
		pb.WriteField(pk.VarInt(ids[i]))
	}
}

func buildUpdateRecipesPacket() pk.Packet {
	var pb pk.Builder
	// Property Sets
	pb.WriteField(pk.VarInt(2)) // two property sets
	// Property set 1
	pb.WriteField(pk.String("minecraft:wood"))
	pb.WriteField(pk.VarInt(2), pk.VarInt(1), pk.VarInt(5))
	// Property set 2
	pb.WriteField(pk.String("minecraft:stone"))
	pb.WriteField(pk.VarInt(0))

	// Stonecutter entries (IDSet + SlotDisplay result)
	pb.WriteField(pk.VarInt(2)) // two entries
	// Entry 1: input IDSet [1, 100], result SlotDisplay: item(200)
	// With scanIDSet: count=2, ids[0]=1, read 1 more (100) → [1, 100]
	writeIDSet(&pb, 1, 100)           // first must be len-1 = 1
	writeSlotDisplay(&pb, 2, func() { // result: item
		pb.WriteField(pk.VarInt(200))
	})
	// Entry 2: input IDSet [1, 400], result SlotDisplay: with_remainder(item(301), item(302))
	writeIDSet(&pb, 1, 400)           // first must be len-1 = 1
	writeSlotDisplay(&pb, 6, func() { // result: with_remainder
		// ingredient
		writeSlotDisplay(&pb, 2, func() { pb.WriteField(pk.VarInt(301)) })
		// remainder
		writeSlotDisplay(&pb, 2, func() { pb.WriteField(pk.VarInt(302)) })
	})
	return pb.Packet(0)
}

func TestOnUpdateRecipes_ParsesAndStores(t *testing.T) {
	a, err := agentpkg.New(agentpkg.Config{Address: "test"})
	require.NoError(t, err)
	pkt := buildUpdateRecipesPacket()
	require.NoError(t, a.ParseUpdateRecipesPacket(pkt))

	payload, ok := a.LastUpdateRecipes()
	require.True(t, ok)
	require.Len(t, payload.PropertySets, 2)
	require.Equal(t, "minecraft:wood", payload.PropertySets[0].ID)
	require.Equal(t, []int32{1, 5}, payload.PropertySets[0].Items)
	require.Equal(t, "minecraft:stone", payload.PropertySets[1].ID)
	require.Empty(t, payload.PropertySets[1].Items)

	require.Len(t, payload.StonecutterEntries, 2)

	// Entry 1 assertions
	entry1 := payload.StonecutterEntries[0]
	// IDSet [1, 100] becomes a Composite with two item options
	require.Equal(t, agentpkg.SlotDisplayTypeComposite, entry1.Input.Type)
	require.Len(t, entry1.Input.Composite, 2)
	require.Equal(t, int32(1), entry1.Input.Composite[0].Item.ItemID)
	require.Equal(t, int32(100), entry1.Input.Composite[1].Item.ItemID)
	require.Len(t, entry1.Results, 1)
	require.Equal(t, agentpkg.SlotDisplayTypeItem, entry1.Results[0].Type)
	require.Equal(t, int32(200), entry1.Results[0].Item.ItemID)

	// Entry 2 assertions
	entry2 := payload.StonecutterEntries[1]
	// IDSet [1, 400] becomes a Composite with two item options
	require.Equal(t, agentpkg.SlotDisplayTypeComposite, entry2.Input.Type)
	require.Len(t, entry2.Input.Composite, 2)
	require.Equal(t, int32(1), entry2.Input.Composite[0].Item.ItemID)
	require.Equal(t, int32(400), entry2.Input.Composite[1].Item.ItemID)
	require.Len(t, entry2.Results, 1)
	// result
	require.Equal(t, agentpkg.SlotDisplayTypeWithRemainder, entry2.Results[0].Type)
	req2 := entry2.Results[0].WithRemainder
	require.NotNil(t, req2)
	require.Equal(t, agentpkg.SlotDisplayTypeItem, req2.Ingredient.Type)
	require.Equal(t, int32(301), req2.Ingredient.Item.ItemID)
	require.Equal(t, agentpkg.SlotDisplayTypeItem, req2.Remainder.Type)
	require.Equal(t, int32(302), req2.Remainder.Item.ItemID)
}

func TestExportLastUpdateRecipesAsJSON(t *testing.T) {
	a, err := agentpkg.New(agentpkg.Config{Address: "test"})
	require.NoError(t, err)
	pkt := buildUpdateRecipesPacket()
	require.NoError(t, a.ParseUpdateRecipesPacket(pkt))

	js, ok, err := a.ExportLastUpdateRecipesAsJSON(true)
	require.True(t, ok)
	require.NoError(t, err)
	require.Contains(t, js, "\"minecraft:wood\"")
	require.Contains(t, js, "200")
}
