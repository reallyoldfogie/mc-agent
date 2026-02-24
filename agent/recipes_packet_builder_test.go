package agent_test

import (
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
	protocol_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// isVersion121 checks if the version is exactly 1.21.1
func isVersion121(version string) bool {
	return version == "1.21.1"
}

// buildUpdateRecipesPacket builds a DeclareRecipes packet appropriate for the given version.
// It handles version-specific differences in packet structure.
func buildUpdateRecipesPacket(version string) (pk.Packet, error) {
	if isVersion121(version) {
		return buildUpdateRecipesPacket121(version)
	}
	// 1.21.2 through 1.21.8 use the same format as 1.21.5
	return buildUpdateRecipesPacket125Plus(version)
}

// buildUpdateRecipesPacket121 builds the legacy 1.21.1 DeclareRecipes packet format.
// This version uses a single recipe array with type discriminators and ingredient/Slot structures.
func buildUpdateRecipesPacket121(version string) (pk.Packet, error) {
	pktMgr := protocol_versions.GetPacketMgrForVersion(version)
	if pktMgr == nil {
		return pk.Packet{}, fmt.Errorf("no packet manager for version %s", version)
	}
	pktID := pktMgr.GetClientboundPacketID("ClientboundUpdateRecipes")

	// Create 1.21.1 DeclareRecipes packet with stonecutting recipes
	pb := pk.Builder{}

	// 1.21.1 format: recipes array with type discriminators
	// We'll build 2 stonecutting recipes to match the 1.21.5+ test expectations
	pb.WriteField(pk.VarInt(2)) // 2 recipes

	// Recipe 1: minecraft:stonecutting
	pb.WriteField(pk.String("minecraft:stonecutting_1")) // recipe name
	pb.WriteField(pk.VarInt(19))                         // type = minecraft:stonecutting (value 19)
	// stonecutting recipe data: group, ingredient, result
	pb.WriteField(pk.String("")) // group (empty)
	// ingredient: Array[VarInt, Slot] with 2 slots (items 1 and 100)
	pb.WriteField(pk.VarInt(2)) // ingredient count
	// Slot 1: itemCount=1, itemId=1, no components
	pb.WriteField(pk.VarInt(1)) // itemCount
	pb.WriteField(pk.VarInt(1)) // itemId
	pb.WriteField(pk.VarInt(0)) // addedComponentCount
	pb.WriteField(pk.VarInt(0)) // removedComponentCount
	// Slot 2: itemCount=1, itemId=100, no components
	pb.WriteField(pk.VarInt(1))   // itemCount
	pb.WriteField(pk.VarInt(100)) // itemId
	pb.WriteField(pk.VarInt(0))   // addedComponentCount
	pb.WriteField(pk.VarInt(0))   // removedComponentCount
	// result: Slot
	pb.WriteField(pk.VarInt(1))   // itemCount
	pb.WriteField(pk.VarInt(200)) // itemId
	pb.WriteField(pk.VarInt(0))   // addedComponentCount
	pb.WriteField(pk.VarInt(0))   // removedComponentCount

	// Recipe 2: minecraft:stonecutting
	pb.WriteField(pk.String("minecraft:stonecutting_2")) // recipe name
	pb.WriteField(pk.VarInt(19))                         // type = minecraft:stonecutting
	// stonecutting recipe data: group, ingredient, result
	pb.WriteField(pk.String("")) // group (empty)
	// ingredient: Array[VarInt, Slot] with 2 slots (items 1 and 400)
	pb.WriteField(pk.VarInt(2)) // ingredient count
	// Slot 1: itemCount=1, itemId=1, no components
	pb.WriteField(pk.VarInt(1)) // itemCount
	pb.WriteField(pk.VarInt(1)) // itemId
	pb.WriteField(pk.VarInt(0)) // addedComponentCount
	pb.WriteField(pk.VarInt(0)) // removedComponentCount
	// Slot 2: itemCount=1, itemId=400, no components
	pb.WriteField(pk.VarInt(1))   // itemCount
	pb.WriteField(pk.VarInt(400)) // itemId
	pb.WriteField(pk.VarInt(0))   // addedComponentCount
	pb.WriteField(pk.VarInt(0))   // removedComponentCount
	// result: Slot
	pb.WriteField(pk.VarInt(1))   // itemCount
	pb.WriteField(pk.VarInt(301)) // itemId
	pb.WriteField(pk.VarInt(0))   // addedComponentCount
	pb.WriteField(pk.VarInt(0))   // removedComponentCount

	pkt := pb.Packet(int32(pktID))
	return pkt, nil
}

// buildUpdateRecipesPacket125Plus builds the 1.21.5+ DeclareRecipes packet format.
// This version has separate arrays for recipes (PropertySets) and stonecutter recipes.
func buildUpdateRecipesPacket125Plus(version string) (pk.Packet, error) {
	pktMgr := protocol_versions.GetPacketMgrForVersion(version)
	if pktMgr == nil {
		return pk.Packet{}, fmt.Errorf("no packet manager for version %s", version)
	}
	pktID := pktMgr.GetClientboundPacketID("ClientboundUpdateRecipes")

	// Use raw packet building to ensure compatibility across 1.21.5+
	pb := pk.Builder{}

	// Recipes array (2 property sets)
	pb.WriteField(pk.VarInt(2))
	// Recipe 1: minecraft:wood with items [1, 5]
	pb.WriteField(pk.String("minecraft:wood"))
	pb.WriteField(pk.VarInt(2), pk.VarInt(1), pk.VarInt(5))
	// Recipe 2: minecraft:stone with no items
	pb.WriteField(pk.String("minecraft:stone"))
	pb.WriteField(pk.VarInt(0))

	// StoneCutterRecipes array (2 entries)
	pb.WriteField(pk.VarInt(2))

	// Entry 1: IDSet [1, 100] -> SlotDisplay item(200)
	// IDSet: count=2, first_id=1 (count-1), remaining_ids=[100]
	pb.WriteField(pk.VarInt(2), pk.VarInt(100))
	// SlotDisplay: type=2 (item), data=200
	pb.WriteField(pk.VarInt(2), pk.VarInt(200))

	// Entry 2: IDSet [1, 400] -> SlotDisplay with_remainder(item(301), item(302))
	// IDSet: count=2, first_id=1, remaining_ids=[400]
	pb.WriteField(pk.VarInt(2), pk.VarInt(400))
	// SlotDisplay: type=6 (with_remainder)
	pb.WriteField(pk.VarInt(6))
	// Ingredient: type=2 (item), data=301
	pb.WriteField(pk.VarInt(2), pk.VarInt(301))
	// Remainder: type=2 (item), data=302
	pb.WriteField(pk.VarInt(2), pk.VarInt(302))

	pkt := pb.Packet(int32(pktID))
	return pkt, nil
}
