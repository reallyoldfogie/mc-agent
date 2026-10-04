package structure

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDecodeNBT_RawRoundTrip(t *testing.T) {
	root := compoundTag(map[string]Tag{
		"aByte":   byteTag(-5),
		"aShort":  shortTag(1234),
		"anInt":   intTag(-99999),
		"aLong":   longTag(1 << 40),
		"aFloat":  floatTag(1.5),
		"aDouble": doubleTag(2.25),
		"aString": stringTag("minecraft:oak_stairs"),
		"aList":   listTag(TagInt, intTag(1), intTag(2), intTag(3)),
		"nested": compoundTag(map[string]Tag{
			"inner": stringTag("value"),
		}),
		"ints":  intArrayTag(1, 2, 3),
		"longs": longArrayTag(10, 20, 30),
	})

	data := encodeRootCompound(root)
	got, err := DecodeNBT(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeNBT: %v", err)
	}
	if got.Type != TagCompound {
		t.Fatalf("root type = %d, want TagCompound", got.Type)
	}
	if !reflect.DeepEqual(got, root) {
		t.Fatalf("round trip mismatch:\n got  %+v\n want %+v", got, root)
	}
}

func TestDecodeNBT_GzipWrapped(t *testing.T) {
	root := compoundTag(map[string]Tag{
		"size": listTag(TagInt, intTag(1), intTag(1), intTag(1)),
	})
	data := gzipBytes(encodeRootCompound(root))

	got, err := DecodeNBT(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeNBT (gzip): %v", err)
	}
	if !reflect.DeepEqual(got, root) {
		t.Fatalf("gzip round trip mismatch:\n got  %+v\n want %+v", got, root)
	}
}

func TestDecodeNBT_EmptyList(t *testing.T) {
	root := compoundTag(map[string]Tag{
		"empty": listTag(TagEnd),
	})
	data := encodeRootCompound(root)
	got, err := DecodeNBT(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeNBT: %v", err)
	}
	empty, ok := got.Get("empty")
	if !ok || empty.Type != TagList || len(empty.List) != 0 {
		t.Fatalf("empty list decoded wrong: %+v", empty)
	}
}

func TestDecodeNBT_TruncatedData_ReturnsError(t *testing.T) {
	root := compoundTag(map[string]Tag{
		"aString": stringTag("this string will get cut off"),
	})
	data := encodeRootCompound(root)

	for cut := 0; cut < len(data); cut++ {
		_, err := DecodeNBT(bytes.NewReader(data[:cut]))
		if err == nil {
			t.Fatalf("DecodeNBT on %d/%d bytes: expected an error, got none", cut, len(data))
		}
	}
}

func TestDecodeNBT_NonCompoundRoot_IsError(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(byte(TagInt))
	writeString(&buf, "")
	writeInt32(&buf, 5)

	if _, err := DecodeNBT(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected an error decoding a non-compound root, got none")
	}
}

func TestTagGet_NonCompound_ReturnsFalse(t *testing.T) {
	if _, ok := intTag(5).Get("anything"); ok {
		t.Fatal("Get on a non-compound tag should return ok=false")
	}
}
