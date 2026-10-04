package structure

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Format decodes one on-disk template format (vanilla Structure Block .nbt
// today; Litematica .litematic, Sponge .schem, etc. in the future - see
// "Format support" in docs/STRUCTURE_LOADER.md) out of an already-parsed
// NBT tag tree and into the format-agnostic Structure schema. Implementations
// register themselves via RegisterFormat, typically from their own
// package-level init() (see vanilla.go) - adding a new format is then
// "write one file implementing this interface," with no changes needed
// anywhere else in this package or its callers.
type Format interface {
	// Name identifies the format for error messages and diagnostics (e.g.
	// "vanilla", "litematica").
	Name() string

	// Sniff reports whether root's shape matches this format - checking for
	// the specific top-level keys/structure each format's schema requires,
	// not just "is this a compound." LoadFile uses Sniff (not just file
	// extension) as the real dispatch, so a misnamed or extensionless file
	// still decodes correctly as long as some registered Format recognizes
	// its shape.
	Sniff(root Tag) bool

	// Decode fully decodes root, which Sniff has already confirmed this
	// format recognizes, into a Structure.
	Decode(root Tag) (*Structure, error)
}

var (
	formatsMu sync.Mutex
	formats   []Format
)

// RegisterFormat adds f to the set LoadFile tries. Intended to be called
// from a Format implementation's package-level init(); panics on a
// duplicate Name so two formats accidentally sharing a name fail loudly at
// program startup rather than silently shadowing one another.
func RegisterFormat(f Format) {
	formatsMu.Lock()
	defer formatsMu.Unlock()
	for _, existing := range formats {
		if existing.Name() == f.Name() {
			panic(fmt.Sprintf("structure: format %q already registered", f.Name()))
		}
	}
	formats = append(formats, f)
}

// registeredFormats returns a snapshot of the current registry, extension
// hint first (if any registered format's typical extension matches path),
// then the rest in registration order - so the common case (a correctly-
// named file) matches on the first try, while a misnamed file still falls
// back to shape-sniffing every other registered format.
func registeredFormats(path string) []Format {
	formatsMu.Lock()
	defer formatsMu.Unlock()
	ext := strings.ToLower(filepath.Ext(path))

	ordered := make([]Format, 0, len(formats))
	var preferred Format
	for _, f := range formats {
		if preferred == nil && extensionHints[ext] == f.Name() {
			preferred = f
			continue
		}
		ordered = append(ordered, f)
	}
	if preferred != nil {
		ordered = append([]Format{preferred}, ordered...)
	}
	return ordered
}

// extensionHints maps a file extension to the Format.Name() it suggests
// trying first. Purely an ordering hint for registeredFormats - Sniff is
// still what actually decides, so an entry here for a format that hasn't
// registered yet (or at all) is harmless.
var extensionHints = map[string]string{
	".nbt":       "vanilla",
	".litematic": "litematica",
	".schem":     "sponge-schem",
	".schematic": "sponge-schem",
}

// LoadFile reads path, decodes it as NBT, and decodes the result with
// whichever registered Format recognizes its shape (see Format.Sniff).
// Returns an error naming every format that was tried if none match.
func LoadFile(path string) (*Structure, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	root, err := DecodeNBT(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	candidates := registeredFormats(path)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("decode %s: no structure formats registered", path)
	}
	tried := make([]string, 0, len(candidates))
	for _, f := range candidates {
		if !f.Sniff(root) {
			tried = append(tried, f.Name())
			continue
		}
		s, err := f.Decode(root)
		if err != nil {
			return nil, fmt.Errorf("decode %s as %s format: %w", path, f.Name(), err)
		}
		return s, nil
	}
	return nil, fmt.Errorf("decode %s: content did not match any known structure format (tried: %s)", path, strings.Join(tried, ", "))
}
