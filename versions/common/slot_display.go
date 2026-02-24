// Package common provides shared interfaces and utilities for version-specific
// network traffic handlers.
package common

import (
	"fmt"
	"io"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// ScanIDSet decodes an ID Set from the packet reader.
// IDSet encoding: First varint is the "count":
//   - If count == 0: Tag list follows (varint count, then that many strings)
//   - If count != 0: First ID = count-1, then read count-1 more IDs
func ScanIDSet(reader io.Reader) (models.IDSet, error) {
	var count pk.VarInt
	if _, err := count.ReadFrom(reader); err != nil {
		return models.IDSet{}, err
	}

	if count == 0 {
		// Tag list representation - read array of tag names
		var numTags pk.VarInt
		if _, err := numTags.ReadFrom(reader); err != nil {
			return models.IDSet{}, err
		}
		// For now, we just skip the tag names since the agent doesn't use them
		for range int(numTags) {
			var tagName pk.String
			if _, err := tagName.ReadFrom(reader); err != nil {
				return models.IDSet{}, err
			}
		}
		return models.IDSet{Mode: models.IDSetEmpty}, nil
	}

	// IDs representation: first ID is count-1
	ids := make([]int32, int(count))
	ids[0] = int32(count - 1)
	// Read remaining count-1 IDs
	for idx := 1; idx < int(count); idx++ {
		var id pk.VarInt
		if _, err := id.ReadFrom(reader); err != nil {
			return models.IDSet{}, err
		}
		ids[idx] = int32(id)
	}

	// Determine mode based on count
	if count == 1 {
		return models.IDSet{Mode: models.IDSetSingle, IDs: ids}, nil
	}
	return models.IDSet{Mode: models.IDSetList, IDs: ids}, nil
}

// ParseSlotDisplay reads and parses a Slot Display structure recursively.
func ParseSlotDisplay(reader io.Reader) (models.SlotDisplay, error) {
	var slotDisplayType pk.VarInt
	if _, err := slotDisplayType.ReadFrom(reader); err != nil {
		return models.SlotDisplay{}, err
	}

	switch int(slotDisplayType) {
	case 0:
		// empty
		return models.SlotDisplay{Type: models.SlotDisplayTypeEmpty}, nil
	case 1:
		// any_fuel
		return models.SlotDisplay{Type: models.SlotDisplayTypeAnyFuel}, nil
	case 2:
		// minecraft:item -> item registry VarInt
		var itemID pk.VarInt
		if _, err := itemID.ReadFrom(reader); err != nil {
			return models.SlotDisplay{}, err
		}
		return models.SlotDisplay{Type: models.SlotDisplayTypeItem, Item: &models.SlotDisplayItem{ItemID: int32(itemID)}}, nil
	case 3:
		// minecraft:item_stack -> Slot
		var slot mcscreen.Slot
		if _, err := slot.ReadFrom(reader); err != nil {
			return models.SlotDisplay{}, err
		}
		if slot.Count <= 0 {
			return models.SlotDisplay{Type: models.SlotDisplayTypeItemStack, ItemStack: &models.SlotDisplayItemStack{ItemID: 0, Count: 0}}, nil
		}
		return models.SlotDisplay{Type: models.SlotDisplayTypeItemStack, ItemStack: &models.SlotDisplayItemStack{ItemID: int32(slot.ID), Count: int32(slot.Count)}}, nil
	case 4:
		// minecraft:tag -> Identifier
		var tag pk.Identifier
		if _, err := tag.ReadFrom(reader); err != nil {
			return models.SlotDisplay{}, err
		}
		str := fmt.Sprintf("%s", tag)
		return models.SlotDisplay{Type: models.SlotDisplayTypeTag, Tag: &str}, nil
	case 5:
		// minecraft:smithing_trim -> Base SlotDisplay, Material SlotDisplay, Pattern VarInt
		base, err := ParseSlotDisplay(reader)
		if err != nil {
			return models.SlotDisplay{}, err
		}
		material, err := ParseSlotDisplay(reader)
		if err != nil {
			return models.SlotDisplay{}, err
		}
		var pattern pk.VarInt
		if _, err := pattern.ReadFrom(reader); err != nil {
			return models.SlotDisplay{}, err
		}
		return models.SlotDisplay{Type: models.SlotDisplayTypeSmithingTrim, SmithingTrim: &models.SlotDisplaySmithingTrim{Base: base, Material: material, Pattern: int32(pattern)}}, nil
	case 6:
		// minecraft:with_remainder -> Ingredient SlotDisplay, Remainder SlotDisplay
		ing, err := ParseSlotDisplay(reader)
		if err != nil {
			return models.SlotDisplay{}, err
		}
		rem, err := ParseSlotDisplay(reader)
		if err != nil {
			return models.SlotDisplay{}, err
		}
		return models.SlotDisplay{Type: models.SlotDisplayTypeWithRemainder, WithRemainder: &models.SlotDisplayWithRemainder{Ingredient: ing, Remainder: rem}}, nil
	case 7:
		// minecraft:composite -> VarInt count + that many SlotDisplays
		var count pk.VarInt
		if _, err := count.ReadFrom(reader); err != nil {
			return models.SlotDisplay{}, err
		}
		options := make([]models.SlotDisplay, int(count))
		for idx := range int(count) {
			opt, err := ParseSlotDisplay(reader)
			if err != nil {
				return models.SlotDisplay{}, err
			}
			options[idx] = opt
		}
		return models.SlotDisplay{Type: models.SlotDisplayTypeComposite, Composite: options}, nil
	default:
		return models.SlotDisplay{Type: models.SlotDisplayType(slotDisplayType)}, nil
	}
}
