package actions

import (
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestFormatDependencyNode_Inventory(t *testing.T) {
	node := models.DependencyNode{Item: "minecraft:stick", Needed: 4, Source: models.SourceInventory}
	remaining := craftPlanChatLimit
	got := formatDependencyNode(node, &remaining)
	want := "minecraft:stick x4: have"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatDependencyNode_Memory(t *testing.T) {
	node := models.DependencyNode{
		Item: "minecraft:oak_log", Needed: 2, Source: models.SourceMemory,
		LocationPos: models.V3{X: 10, Y: 64, Z: -5}, InContainer: false, Confidence: 0.82,
	}
	remaining := craftPlanChatLimit
	got := formatDependencyNode(node, &remaining)
	if !strings.Contains(got, "minecraft:oak_log x2") {
		t.Errorf("got %q, want it to name the item and needed count", got)
	}
	if !strings.Contains(got, "(10,64,-5)") {
		t.Errorf("got %q, want the remembered position", got)
	}
	if !strings.Contains(got, "82%") {
		t.Errorf("got %q, want the confidence rendered as a percentage", got)
	}
	if strings.Contains(got, "in a container") {
		t.Errorf("got %q, want \"loose\" phrasing for a non-container sighting", got)
	}
}

func TestFormatDependencyNode_MemoryInContainer(t *testing.T) {
	node := models.DependencyNode{
		Item: "minecraft:iron_ingot", Needed: 1, Source: models.SourceMemory,
		LocationPos: models.V3{X: 1, Y: 2, Z: 3}, InContainer: true, Confidence: 0.5,
	}
	remaining := craftPlanChatLimit
	got := formatDependencyNode(node, &remaining)
	if !strings.Contains(got, "in a container") {
		t.Errorf("got %q, want it to note the sighting is inside a container", got)
	}
}

func TestFormatDependencyNode_Unknown(t *testing.T) {
	node := models.DependencyNode{Item: "minecraft:diamond", Needed: 1, Source: models.SourceUnknown}
	remaining := craftPlanChatLimit
	got := formatDependencyNode(node, &remaining)
	if !strings.Contains(got, "minecraft:diamond x1") || !strings.Contains(got, "unknown") {
		t.Errorf("got %q, want it to name the item and flag it unknown-source", got)
	}
}

func TestFormatDependencyNode_CraftRendersChildren(t *testing.T) {
	node := models.DependencyNode{
		Item: "minecraft:stick", Needed: 1, Source: models.SourceCraft,
		Children: []models.DependencyNode{
			{Item: "minecraft:oak_planks", Needed: 2, Source: models.SourceInventory},
		},
	}
	remaining := craftPlanChatLimit
	got := formatDependencyNode(node, &remaining)
	if !strings.Contains(got, "minecraft:stick x1: craft(") {
		t.Errorf("got %q, want a craft(...) wrapper naming the item", got)
	}
	if !strings.Contains(got, "minecraft:oak_planks x2: have") {
		t.Errorf("got %q, want the child rendered inside the craft(...) wrapper", got)
	}
}

// A pathologically deep/wide tree must still produce one bounded line,
// the same spam risk lookAroundChatLimit/materialListSummaryMaxItems
// guard against elsewhere - not one unbounded chat message.
func TestFormatDependencyNode_TruncatesDeepTree(t *testing.T) {
	leaf := models.DependencyNode{Item: "minecraft:stick", Needed: 1, Source: models.SourceInventory}
	children := make([]models.DependencyNode, 0, craftPlanChatLimit+5)
	for i := 0; i < craftPlanChatLimit+5; i++ {
		children = append(children, leaf)
	}
	node := models.DependencyNode{Item: "minecraft:chest", Needed: 1, Source: models.SourceCraft, Children: children}

	remaining := craftPlanChatLimit
	got := formatDependencyNode(node, &remaining)
	if !strings.Contains(got, "...") {
		t.Errorf("got %q, want truncation marker once the shared budget runs out", got)
	}
	if remaining > 0 {
		t.Errorf("remaining = %d after rendering a tree larger than the limit, want 0", remaining)
	}
}
