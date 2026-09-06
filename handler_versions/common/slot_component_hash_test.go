package common

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
)

// TestComponentHash_PointerToScalar is a regression test for a bug found
// while porting mc-bot-go's bot/screen/hashops.go: sliceHashValues called
// reflect.Value.FieldByName on a dereferenced pointer without first
// checking its Kind is Struct, panicking for any component whose Data is a
// pointer to a plain scalar (pk.VarInt, pk.Boolean, ...) -- exactly the
// shape every simple component (e.g. "damage", "unbreakable") takes once
// decoded via a version's own generated basetypes.SlotComponent.ReadFrom
// (every case in its switch does `t.Data = &val`), so this would have hit
// in practice for nearly any real item, not an edge case.
func TestComponentHash_PointerToScalar(t *testing.T) {
	varInt := pk.VarInt(5)
	boolean := pk.Boolean(true)
	cases := []struct {
		name  string
		value any
	}{
		{"pointer to VarInt", &varInt},
		{"pointer to Boolean", &boolean},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ComponentHash(tc.value); err != nil {
				t.Fatalf("ComponentHash(%#v): %v", tc.value, err)
			}
		})
	}
}
