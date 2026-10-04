package structure

import (
	"os"
	"testing"
)

// fixtureFormat is a minimal stand-in for a future real format (litematica,
// sponge-schem): it recognizes a root by a single marker key rather than
// vanilla's shape, so these tests can exercise multi-format dispatch
// without a second real format existing yet.
type fixtureFormat struct {
	marker string
	name   string
}

func (f fixtureFormat) Name() string { return f.name }

func (f fixtureFormat) Sniff(root Tag) bool {
	_, ok := root.Get(f.marker)
	return ok
}

func (f fixtureFormat) Decode(root Tag) (*Structure, error) {
	return &Structure{}, nil
}

func init() {
	RegisterFormat(fixtureFormat{marker: "OtherFormatMarker", name: "fixture-other"})
}

func TestRegisterFormat_DuplicateNamePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RegisterFormat with a duplicate name should panic")
		}
	}()
	RegisterFormat(fixtureFormat{marker: "x", name: "vanilla"})
}

func TestLoadFile_DispatchesToMatchingFormat(t *testing.T) {
	// Content that only fixtureFormat recognizes (has "OtherFormatMarker",
	// not vanilla's size/palette/blocks) should decode via fixtureFormat,
	// not fail just because vanilla's Sniff declines it.
	root := compoundTag(map[string]Tag{
		"OtherFormatMarker": intTag(1),
	})
	path := t.TempDir() + "/other.dat"
	if err := os.WriteFile(path, encodeRootCompound(root), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if s == nil {
		t.Fatal("LoadFile returned a nil Structure with no error")
	}
}

func TestLoadFile_NoFormatMatches_ReturnsDescriptiveError(t *testing.T) {
	root := compoundTag(map[string]Tag{
		"totallyUnrecognized": intTag(1),
	})
	path := t.TempDir() + "/unknown.dat"
	if err := os.WriteFile(path, encodeRootCompound(root), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile: expected an error for unrecognized content, got none")
	}
}

func TestLoadFile_MissingFile_ReturnsError(t *testing.T) {
	if _, err := LoadFile("/nonexistent/path/does-not-exist.nbt"); err == nil {
		t.Fatal("LoadFile: expected an error for a missing file, got none")
	}
}

func TestRegisteredFormats_ExtensionHintOrdersPreferredFirst(t *testing.T) {
	ordered := registeredFormats("template.nbt")
	if len(ordered) == 0 {
		t.Fatal("registeredFormats returned nothing")
	}
	if ordered[0].Name() != "vanilla" {
		t.Errorf("registeredFormats(\"template.nbt\")[0].Name() = %q, want \"vanilla\"", ordered[0].Name())
	}
}
