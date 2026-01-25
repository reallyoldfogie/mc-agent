package agent

import "github.com/reallyoldfogie/mc-agent/models"

// Type aliases for recipe payload structures.
type (
	SlotDisplayType          = models.SlotDisplayType
	IDSetMode                = models.IDSetMode
	IDSet                    = models.IDSet
	SlotDisplay              = models.SlotDisplay
	SlotDisplayItem          = models.SlotDisplayItem
	SlotDisplayItemStack     = models.SlotDisplayItemStack
	SlotDisplaySmithingTrim  = models.SlotDisplaySmithingTrim
	SlotDisplayWithRemainder = models.SlotDisplayWithRemainder
	PropertySet              = models.PropertySet
	StonecutterEntry         = models.StonecutterEntry
	UpdateRecipesPayload     = models.UpdateRecipesPayload
)

const (
	SlotDisplayTypeEmpty         = models.SlotDisplayTypeEmpty
	SlotDisplayTypeAnyFuel       = models.SlotDisplayTypeAnyFuel
	SlotDisplayTypeItem          = models.SlotDisplayTypeItem
	SlotDisplayTypeItemStack     = models.SlotDisplayTypeItemStack
	SlotDisplayTypeTag           = models.SlotDisplayTypeTag
	SlotDisplayTypeSmithingTrim  = models.SlotDisplayTypeSmithingTrim
	SlotDisplayTypeWithRemainder = models.SlotDisplayTypeWithRemainder
	SlotDisplayTypeComposite     = models.SlotDisplayTypeComposite
	IDSetEmpty                   = models.IDSetEmpty
	IDSetSingle                  = models.IDSetSingle
	IDSetList                    = models.IDSetList
)
