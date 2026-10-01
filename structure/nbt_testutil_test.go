package structure

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"math"
	"sort"
)

// The functions in this file are a minimal NBT *encoder*, used only by
// tests to build byte-level fixtures for the real decoder in nbt.go.
// Production code never writes NBT (this package only ever reads
// structure/template files), so this intentionally doesn't live outside
// _test.go.

// encodeRootCompound serializes root (which must be a TagCompound) as a
// complete NBT document: root tag type, an empty root name, then the
// compound's entries, matching what DecodeNBT expects to read back.
func encodeRootCompound(root Tag) []byte {
	var buf bytes.Buffer
	buf.WriteByte(byte(TagCompound))
	writeString(&buf, "")
	writeCompoundPayload(&buf, root)
	return buf.Bytes()
}

// gzipBytes wraps data in gzip framing, as a real Structure Block "Save"
// produces.
func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(data)
	_ = zw.Close()
	return buf.Bytes()
}

func writeString(buf *bytes.Buffer, s string) {
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(s)))
	buf.Write(lenBuf[:])
	buf.WriteString(s)
}

func writeInt16(buf *bytes.Buffer, v int16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(v))
	buf.Write(b[:])
}

func writeInt32(buf *bytes.Buffer, v int32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	buf.Write(b[:])
}

func writeInt64(buf *bytes.Buffer, v int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	buf.Write(b[:])
}

// writePayload writes t's payload only (no type byte, no name) - the shape
// list elements are written in, and what writeCompoundPayload writes after
// each entry's type+name.
func writePayload(buf *bytes.Buffer, t Tag) {
	switch t.Type {
	case TagEnd:
		// no payload
	case TagByte:
		buf.WriteByte(byte(int8(t.I64)))
	case TagShort:
		writeInt16(buf, int16(t.I64))
	case TagInt:
		writeInt32(buf, int32(t.I64))
	case TagLong:
		writeInt64(buf, t.I64)
	case TagFloat:
		writeInt32(buf, int32(math.Float32bits(float32(t.F64))))
	case TagDouble:
		writeInt64(buf, int64(math.Float64bits(t.F64)))
	case TagByteArray:
		writeInt32(buf, int32(len(t.Bytes)))
		buf.Write(t.Bytes)
	case TagString:
		writeString(buf, t.Str)
	case TagList:
		buf.WriteByte(byte(t.ListType))
		writeInt32(buf, int32(len(t.List)))
		for _, elem := range t.List {
			writePayload(buf, elem)
		}
	case TagCompound:
		writeCompoundPayload(buf, t)
	case TagIntArray:
		writeInt32(buf, int32(len(t.Ints)))
		for _, v := range t.Ints {
			writeInt32(buf, v)
		}
	case TagLongArray:
		writeInt32(buf, int32(len(t.Longs)))
		for _, v := range t.Longs {
			writeInt64(buf, v)
		}
	}
}

// writeCompoundPayload writes a compound's entries (type+name+payload for
// each, sorted by key for deterministic test output) followed by the
// TagEnd terminator.
func writeCompoundPayload(buf *bytes.Buffer, t Tag) {
	keys := make([]string, 0, len(t.Compound))
	for k := range t.Compound {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := t.Compound[k]
		buf.WriteByte(byte(v.Type))
		writeString(buf, k)
		writePayload(buf, v)
	}
	buf.WriteByte(byte(TagEnd))
}

// Convenience constructors for building fixture Tag trees tersely in tests.

func compoundTag(entries map[string]Tag) Tag {
	return Tag{Type: TagCompound, Compound: entries}
}

func listTag(elemType TagType, elems ...Tag) Tag {
	return Tag{Type: TagList, ListType: elemType, List: elems}
}

func intTag(v int32) Tag          { return Tag{Type: TagInt, I64: int64(v)} }
func longTag(v int64) Tag         { return Tag{Type: TagLong, I64: v} }
func shortTag(v int16) Tag        { return Tag{Type: TagShort, I64: int64(v)} }
func byteTag(v int8) Tag          { return Tag{Type: TagByte, I64: int64(v)} }
func stringTag(s string) Tag      { return Tag{Type: TagString, Str: s} }
func floatTag(v float32) Tag      { return Tag{Type: TagFloat, F64: float64(v)} }
func doubleTag(v float64) Tag     { return Tag{Type: TagDouble, F64: v} }
func intArrayTag(v ...int32) Tag  { return Tag{Type: TagIntArray, Ints: v} }
func longArrayTag(v ...int64) Tag { return Tag{Type: TagLongArray, Longs: v} }
