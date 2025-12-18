# Recipe System

## Overview

The agent package includes a comprehensive parser for the Minecraft `Update Recipes` packet (added in protocol 1.21.2+). This packet contains server-side recipe data including item property sets and stonecutter recipes.

## Packet Structure

The `Update Recipes` packet contains two main sections:

### 1. Property Sets
Groups of items that share properties (e.g., "all wood types", "all stone types").

**Structure:**
- Registry ID (String): e.g., `"minecraft:wood"`
- Item IDs (Array): List of minecraft:item registry IDs

**Example:**
```json
{
  "id": "minecraft:wood",
  "items": [1, 2, 3, 4]  // oak, spruce, birch, jungle
}
```

### 2. Stonecutter Entries
Input-output mappings for stonecutter recipes.

**Structure:**
- Input (Slot Display): What item(s) can be placed in stonecutter
- Results (Array of Slot Displays): Possible output items

## Slot Display Types

The packet uses a complex "Slot Display" discriminated union with 8 variants:

| Type | ID | Description |
|------|----|----|
| Empty | 0 | No item |
| Any Fuel | 1 | Any burnable item |
| Item | 2 | Single item by ID |
| Item Stack | 3 | Item with count |
| Tag | 4 | Item tag reference |
| Smithing Trim | 5 | Armor trim display |
| With Remainder | 6 | Item that leaves remainder |
| Composite | 7 | Multiple displays combined |

## API Usage

### Get Last Update Recipes Payload

```go
payload, ok := agent.LastUpdateRecipes()
if !ok {
    log.Println("No Update Recipes packet received yet")
    return
}

fmt.Printf("Property Sets: %d\n", len(payload.PropertySets))
fmt.Printf("Stonecutter Entries: %d\n", len(payload.StonecutterEntries))
```

### Export as JSON

```go
// Pretty-printed JSON
json, ok, err := agent.ExportLastUpdateRecipesAsJSON(true)
if err != nil {
    log.Printf("Export failed: %v", err)
    return
}
if !ok {
    log.Println("No recipes available")
    return
}

fmt.Println(json)
```

### Parse Packet Manually

```go
packet := /* received packet */
err := agent.ParseUpdateRecipesPacket(packet)
if err != nil {
    log.Printf("Parse error: %v", err)
}
```

## Data Structures

### UpdateRecipesPayload

```go
type UpdateRecipesPayload struct {
    PropertySets       []PropertySet
    StonecutterEntries []StonecutterEntry
}
```

### PropertySet

```go
type PropertySet struct {
    ID    string   // Registry ID (e.g., "minecraft:wood")
    Items []int32  // minecraft:item registry IDs
}
```

### StonecutterEntry

```go
type StonecutterEntry struct {
    Input   SlotDisplay   // Input item(s)
    Results []SlotDisplay // Possible outputs
}
```

### SlotDisplay

```go
type SlotDisplay struct {
    Type          SlotDisplayType
    Item          *SlotDisplayItem          // Type == 2
    ItemStack     *SlotDisplayItemStack     // Type == 3
    Tag           *string                   // Type == 4
    SmithingTrim  *SlotDisplaySmithingTrim  // Type == 5
    WithRemainder *SlotDisplayWithRemainder // Type == 6
    Composite     []SlotDisplay             // Type == 7
}
```

## Testing

The recipe system includes comprehensive test coverage:

- `agent/recipes_test.go` - Basic parsing tests
- `agent/recipes_external_test.go` - Comparison with mc-protocol-go parser
- `agent/recipes_diagnostic_test.go` - Detailed packet inspection
- `agent/recipes_position_test.go` - Byte position tracking

Run tests:
```bash
go test ./agent -run TestRecipes -v
```

## Implementation Details

### ID Set Encoding

The packet uses a custom "ID Set" encoding for efficient item lists:

**Empty (count = 0):**
- Followed by array of tag names (String array)

**Single ID (count = 1):**
- First ID is `count - 1` (so 0 for count=1)

**Multiple IDs (count > 1):**
- First ID is `count - 1`
- Followed by `count - 1` more IDs

### Slot Display Recursion

Slot Displays can be nested:
- `Composite` contains array of Slot Displays
- `SmithingTrim` has Base and Material (both Slot Displays)
- `WithRemainder` has Ingredient and Remainder (both Slot Displays)

The parser handles arbitrary nesting depth.

## Use Cases

### 1. Item Group Detection
Check if an item belongs to a property set:

```go
func hasProperty(itemID int32, propertyName string, payload UpdateRecipesPayload) bool {
    for _, ps := range payload.PropertySets {
        if ps.ID == propertyName {
            for _, id := range ps.Items {
                if id == itemID {
                    return true
                }
            }
        }
    }
    return false
}
```

### 2. Stonecutter Recipe Discovery
Find all stonecutter outputs for an input item:

```go
func findStonecutterOutputs(inputID int32, payload UpdateRecipesPayload) []int32 {
    var outputs []int32
    for _, entry := range payload.StonecutterEntries {
        if matchesInput(entry.Input, inputID) {
            for _, result := range entry.Results {
                if result.Type == SlotDisplayTypeItem {
                    outputs = append(outputs, result.Item.ItemID)
                }
            }
        }
    }
    return outputs
}
```

### 3. Recipe Export for Analysis
Export recipes for external processing:

```go
json, ok, err := agent.ExportLastUpdateRecipesAsJSON(true)
if err == nil && ok {
    os.WriteFile("recipes.json", []byte(json), 0644)
}
```

## Protocol References

- Minecraft Protocol: https://wiki.vg/Protocol#Update_Recipes
- mc-protocol-go: `data/1.21.5/play/clientbound/types.go`
- Packet ID (1.21.5): `0x7B` (123)

## Limitations

- Components/NBT data in ItemStack is currently omitted (not used by mc-agent)
- Tag names in ID Sets are read but not stored (agent uses numeric IDs only)
- Recipe book categories and other metadata not parsed

## Future Enhancements

- Parse crafting recipes (currently only property sets and stonecutter)
- Store tag name mappings
- Recipe search/query API
- Recipe change detection
