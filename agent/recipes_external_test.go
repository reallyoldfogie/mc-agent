package agent_test

import (
	"testing"

	agentpkg "github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/models"
	_ "github.com/reallyoldfogie/mc-agent/versions"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/stretchr/testify/require"
)

func TestOnUpdateRecipes_ParsesAndStores(t *testing.T) {
	for _, versionTest := range models.StandardVersionTests {
		t.Run(versionTest.Name, func(t *testing.T) {
			version := versionTest.MCVersion
			a, err := agentpkg.New(agentpkg.Config{Version: version, Address: "test"})
			require.NoError(t, err)

			// Build version-specific packet
			pkt, err := buildUpdateRecipesPacket(version)
			require.NoError(t, err)

			// Parse using version handler
			versionHandler, err := common.GetVersionHandler(version)
			require.NoError(t, err)

			payload, err := versionHandler.Play().ParseUpdateRecipes(pkt)
			require.NoError(t, err)
			require.NotNil(t, payload)

			// Store in agent for testing
			a.SetLastUpdateRecipes(payload)

			_, ok := a.LastUpdateRecipes()
			require.True(t, ok)

			// 1.21.1 has a different packet format without PropertySets
			if version != "1.21.1" {
				require.Len(t, payload.PropertySets, 2)
				require.Equal(t, "minecraft:wood", payload.PropertySets[0].ID)
				require.Equal(t, []int32{1, 5}, payload.PropertySets[0].Items)
				require.Equal(t, "minecraft:stone", payload.PropertySets[1].ID)
				require.Empty(t, payload.PropertySets[1].Items)
			}

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

			// 1.21.1 uses plain item results, while 1.21.5+ support with_remainder
			if version == "1.21.1" {
				require.Equal(t, agentpkg.SlotDisplayTypeItem, entry2.Results[0].Type)
				require.Equal(t, int32(301), entry2.Results[0].Item.ItemID)
			} else {
				// result with_remainder
				require.Equal(t, agentpkg.SlotDisplayTypeWithRemainder, entry2.Results[0].Type)
				req2 := entry2.Results[0].WithRemainder
				require.NotNil(t, req2)
				require.Equal(t, agentpkg.SlotDisplayTypeItem, req2.Ingredient.Type)
				require.Equal(t, int32(301), req2.Ingredient.Item.ItemID)
				require.Equal(t, agentpkg.SlotDisplayTypeItem, req2.Remainder.Type)
				require.Equal(t, int32(302), req2.Remainder.Item.ItemID)
			}
		})
	}
}

func TestExportLastUpdateRecipesAsJSON(t *testing.T) {
	for _, versionTest := range models.StandardVersionTests {
		t.Run(versionTest.Name, func(t *testing.T) {
			version := versionTest.MCVersion
			a, err := agentpkg.New(agentpkg.Config{Version: version, Address: "test"})
			require.NoError(t, err)

			// Build version-specific packet
			pkt, err := buildUpdateRecipesPacket(version)
			require.NoError(t, err)

			// Parse using version handler
			versionHandler, err := common.GetVersionHandler(version)
			require.NoError(t, err)

			payload, err := versionHandler.Play().ParseUpdateRecipes(pkt)
			require.NoError(t, err)
			a.SetLastUpdateRecipes(payload)

			js, ok, err := a.ExportLastUpdateRecipesAsJSON(true)
			require.True(t, ok)
			require.NoError(t, err)

			// 1.21.1 doesn't have PropertySets, so it won't contain "minecraft:wood"
			if version != "1.21.1" {
				require.Contains(t, js, "\"minecraft:wood\"")
			}
			// Both versions should have stonecutter results with item IDs
			require.Contains(t, js, "200")
		})
	}
}
