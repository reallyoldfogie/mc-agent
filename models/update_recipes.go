package models

// SlotDisplayType enumerates minecraft:slot_display types.
type SlotDisplayType int

const (
	SlotDisplayTypeEmpty         SlotDisplayType = 0
	SlotDisplayTypeAnyFuel       SlotDisplayType = 1
	SlotDisplayTypeItem          SlotDisplayType = 2
	SlotDisplayTypeItemStack     SlotDisplayType = 3
	SlotDisplayTypeTag           SlotDisplayType = 4
	SlotDisplayTypeSmithingTrim  SlotDisplayType = 5
	SlotDisplayTypeWithRemainder SlotDisplayType = 6
	SlotDisplayTypeComposite     SlotDisplayType = 7
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
	IDs  []int32
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
	ItemID int32
}

type SlotDisplayItemStack struct {
	ItemID int32
	Count  int32
}

type SlotDisplaySmithingTrim struct {
	Base     SlotDisplay
	Material SlotDisplay
	Pattern  int32
}

type SlotDisplayWithRemainder struct {
	Ingredient SlotDisplay
	Remainder  SlotDisplay
}

// PropertySet row in Update Recipes packet.
type PropertySet struct {
	ID    string
	Items []int32
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
