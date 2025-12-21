package agent

import (
	"io"

	pk "github.com/Tnze/go-mc/net/packet"
)

// SlotDisplayType enumerates minecraft:slot_display types.
type SlotDisplayType int

const (
	SlotDisplayTypeEmpty         SlotDisplayType = 0 // minecraft:empty
	SlotDisplayTypeAnyFuel       SlotDisplayType = 1 // minecraft:any_fuel
	SlotDisplayTypeItem          SlotDisplayType = 2 // minecraft:item
	SlotDisplayTypeItemStack     SlotDisplayType = 3 // minecraft:item_stack
	SlotDisplayTypeTag           SlotDisplayType = 4 // minecraft:tag
	SlotDisplayTypeSmithingTrim  SlotDisplayType = 5 // minecraft:smithing_trim
	SlotDisplayTypeWithRemainder SlotDisplayType = 6 // minecraft:with_remainder
	SlotDisplayTypeComposite     SlotDisplayType = 7 // minecraft:composite
)

// IDSetMode mirrors the ID Set discriminant.
type IDSetMode int

const (
	IDSetEmpty  IDSetMode = 0
	IDSetSingle IDSetMode = 1
	IDSetList   IDSetMode = 2
)

// IDSet is the decoded ID Set structure from the protocol.
type IDSet struct {
	Mode IDSetMode
	IDs  []int32 // empty when Mode==IDSetEmpty
}

// SlotDisplay holds one of the Slot Display variants.
type SlotDisplay struct {
	Type          SlotDisplayType
	Item          *SlotDisplayItem
	ItemStack     *SlotDisplayItemStack
	Tag           *string
	SmithingTrim  *SlotDisplaySmithingTrim
	WithRemainder *SlotDisplayWithRemainder
	Composite     []SlotDisplay
}

type SlotDisplayItem struct {
	ItemID int32 // minecraft:item registry id
}

type SlotDisplayItemStack struct {
	ItemID int32
	Count  int32
	// NOTE: components/NBT omitted for now; mc-agent does not consume them
}

type SlotDisplaySmithingTrim struct {
	Base     SlotDisplay
	Material SlotDisplay
	Pattern  int32 // trim_pattern registry id
}

type SlotDisplayWithRemainder struct {
	Ingredient SlotDisplay
	Remainder  SlotDisplay
}

// PropertySet row in Update Recipes packet.
type PropertySet struct {
	ID    string
	Items []int32 // minecraft:item registry IDs
}

// StonecutterEntry groups an input with its possible stonecutter results.
 type StonecutterEntry struct {
	Input   SlotDisplay
	Results []SlotDisplay
 }
 
 // UpdateRecipesPayload keeps the full decoded packet for later inspection.
 type UpdateRecipesPayload struct {
	PropertySets       []PropertySet
	StonecutterEntries []StonecutterEntry
 }

// helper: decode an ID Set from the packet into our struct.
// IDSet encoding: First varint is the "count":
//  - If count == 0: Tag list follows (varint count, then that many strings)
//  - If count != 0: First ID = count-1, then read count-1 more IDs
func scanIDSet(r io.Reader) (IDSet, error) {
	var count pk.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		return IDSet{}, err
	}

	if count == 0 {
		// Tag list representation - read array of tag names
		var numTags pk.VarInt
		if _, err := numTags.ReadFrom(r); err != nil {
			return IDSet{}, err
		}
		// For now, we just skip the tag names since the agent doesn't use them
		for i := 0; i < int(numTags); i++ {
			var tagName pk.String
			if _, err := tagName.ReadFrom(r); err != nil {
				return IDSet{}, err
			}
		}
		return IDSet{Mode: IDSetEmpty}, nil
	}

	// IDs representation: first ID is count-1
	ids := make([]int32, int(count))
	ids[0] = int32(count - 1)
	// Read remaining count-1 IDs
	for i := 1; i < int(count); i++ {
		var id pk.VarInt
		if _, err := id.ReadFrom(r); err != nil {
			return IDSet{}, err
		}
		ids[i] = int32(id)
	}

	// Determine mode based on count
	if count == 1 {
		return IDSet{Mode: IDSetSingle, IDs: ids}, nil
	}
	return IDSet{Mode: IDSetList, IDs: ids}, nil
}
