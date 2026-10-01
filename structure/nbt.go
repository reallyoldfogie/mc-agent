// Package structure loads build-template files (vanilla Structure Block
// .nbt, and in the future other formats such as Litematica) into a common,
// format-agnostic in-memory representation the rest of the agent can place
// block-by-block. See docs/plans/NBT_STRUCTURE_LOADER_PLAN.md.
package structure

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// TagType is an NBT tag's type ID, as written on the wire (a single byte
// preceding every tag except list elements, which share their list's type).
type TagType byte

// NBT tag type IDs, per the format's binary spec. Short/Float/Double/
// Byte_Array are parsed (so an unexpected tag of one of these types doesn't
// abort the whole read) even though no format this package currently
// supports produces them in a structure that matters here.
const (
	TagEnd TagType = iota
	TagByte
	TagShort
	TagInt
	TagLong
	TagFloat
	TagDouble
	TagByteArray
	TagString
	TagList
	TagCompound
	TagIntArray
	TagLongArray
)

// Tag is a decoded NBT tag's payload. Exactly the field(s) matching Type are
// meaningful; the rest are zero. This is intentionally a plain value type
// (no interface{}/reflection) - the formats this package reads have a small,
// fixed set of tag shapes, and a discriminated struct is simpler to decode
// into and to write format-specific extraction code against than a general
// tree of interface{} would be.
type Tag struct {
	Type TagType

	I64 int64   // Byte, Short, Int, Long - sign-extended to 64 bits
	F64 float64 // Float, Double

	Str string // String

	List     []Tag   // List - every element has type ListType
	ListType TagType // element type of List; TagEnd if the list is empty

	Compound map[string]Tag // Compound

	Bytes []byte  // Byte_Array (raw bytes, signedness left to the caller)
	Ints  []int32 // Int_Array
	Longs []int64 // Long_Array
}

// Int returns t.I64 as an int, for the common case of reading a small
// integer tag (sizes, indices, coordinates) where int is the natural Go
// type to work with.
func (t Tag) Int() int { return int(t.I64) }

// Get returns the named child of a Compound tag, or (Tag{}, false) if t is
// not a Compound or has no such key.
func (t Tag) Get(key string) (Tag, bool) {
	if t.Type != TagCompound {
		return Tag{}, false
	}
	v, ok := t.Compound[key]
	return v, ok
}

// DecodeNBT reads a complete NBT document - gzip-compressed (the normal
// output of a Structure Block's "Save" button and most external tools),
// zlib-compressed, or raw - and returns its root compound tag. The root
// tag's own name (conventionally empty for these files) is read and
// discarded; nothing in this package's schemas uses it.
func DecodeNBT(r io.Reader) (Tag, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Tag{}, fmt.Errorf("read nbt data: %w", err)
	}
	data, err = decompress(data)
	if err != nil {
		return Tag{}, fmt.Errorf("decompress nbt data: %w", err)
	}

	d := &decoder{buf: data}
	typ, err := d.readByte()
	if err != nil {
		return Tag{}, fmt.Errorf("read root tag type: %w", err)
	}
	if TagType(typ) == TagEnd {
		return Tag{Type: TagCompound, Compound: map[string]Tag{}}, nil
	}
	if TagType(typ) != TagCompound {
		return Tag{}, fmt.Errorf("root tag type %d is not a compound", typ)
	}
	if _, err := d.readString(); err != nil {
		return Tag{}, fmt.Errorf("read root tag name: %w", err)
	}
	root, err := d.readCompoundPayload()
	if err != nil {
		return Tag{}, fmt.Errorf("read root compound: %w", err)
	}
	return root, nil
}

// decompress detects gzip or zlib framing by magic bytes and inflates
// accordingly, returning data unchanged if neither magic matches (some
// tools write raw, uncompressed NBT).
func decompress(data []byte) ([]byte, error) {
	switch {
	case len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b: // gzip magic
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	case len(data) >= 2 && data[0] == 0x78 && (data[1] == 0x01 || data[1] == 0x9c || data[1] == 0xda): // zlib magic
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	default:
		return data, nil
	}
}

// decoder is a sequential cursor over an already-decompressed NBT byte
// buffer. All multi-byte numeric fields are big-endian, per the NBT spec.
type decoder struct {
	buf []byte
	pos int
}

func (d *decoder) need(n int) error {
	if d.pos+n > len(d.buf) {
		return fmt.Errorf("unexpected end of NBT data (need %d bytes at offset %d, have %d)", n, d.pos, len(d.buf))
	}
	return nil
}

func (d *decoder) readByte() (byte, error) {
	if err := d.need(1); err != nil {
		return 0, err
	}
	b := d.buf[d.pos]
	d.pos++
	return b, nil
}

func (d *decoder) readInt16() (int16, error) {
	if err := d.need(2); err != nil {
		return 0, err
	}
	v := int16(binary.BigEndian.Uint16(d.buf[d.pos:]))
	d.pos += 2
	return v, nil
}

func (d *decoder) readInt32() (int32, error) {
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := int32(binary.BigEndian.Uint32(d.buf[d.pos:]))
	d.pos += 4
	return v, nil
}

func (d *decoder) readInt64() (int64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := int64(binary.BigEndian.Uint64(d.buf[d.pos:]))
	d.pos += 8
	return v, nil
}

func (d *decoder) readFloat32() (float32, error) {
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := math.Float32frombits(binary.BigEndian.Uint32(d.buf[d.pos:]))
	d.pos += 4
	return v, nil
}

func (d *decoder) readFloat64() (float64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := math.Float64frombits(binary.BigEndian.Uint64(d.buf[d.pos:]))
	d.pos += 8
	return v, nil
}

// readString reads a length-prefixed (uint16 big-endian byte length)
// Modified-UTF-8 string. For the ASCII block/property names these formats
// actually contain, Modified UTF-8 and plain UTF-8 agree byte-for-byte, so
// no special decoding is needed beyond the length-prefixed byte read.
func (d *decoder) readString() (string, error) {
	if err := d.need(2); err != nil {
		return "", err
	}
	n := int(binary.BigEndian.Uint16(d.buf[d.pos:]))
	d.pos += 2
	if err := d.need(n); err != nil {
		return "", err
	}
	s := string(d.buf[d.pos : d.pos+n])
	d.pos += n
	return s, nil
}

// readTagPayload reads the payload of a tag of the given type (the type
// byte and, for named tags, the name must already have been consumed by the
// caller).
func (d *decoder) readTagPayload(typ TagType) (Tag, error) {
	switch typ {
	case TagEnd:
		return Tag{Type: TagEnd}, nil
	case TagByte:
		b, err := d.readByte()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagByte, I64: int64(int8(b))}, nil
	case TagShort:
		v, err := d.readInt16()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagShort, I64: int64(v)}, nil
	case TagInt:
		v, err := d.readInt32()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagInt, I64: int64(v)}, nil
	case TagLong:
		v, err := d.readInt64()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagLong, I64: v}, nil
	case TagFloat:
		v, err := d.readFloat32()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagFloat, F64: float64(v)}, nil
	case TagDouble:
		v, err := d.readFloat64()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagDouble, F64: v}, nil
	case TagByteArray:
		n, err := d.readInt32()
		if err != nil {
			return Tag{}, err
		}
		if n < 0 {
			return Tag{}, fmt.Errorf("negative byte array length %d", n)
		}
		if err := d.need(int(n)); err != nil {
			return Tag{}, err
		}
		b := make([]byte, n)
		copy(b, d.buf[d.pos:d.pos+int(n)])
		d.pos += int(n)
		return Tag{Type: TagByteArray, Bytes: b}, nil
	case TagString:
		s, err := d.readString()
		if err != nil {
			return Tag{}, err
		}
		return Tag{Type: TagString, Str: s}, nil
	case TagList:
		return d.readListPayload()
	case TagCompound:
		return d.readCompoundPayload()
	case TagIntArray:
		n, err := d.readInt32()
		if err != nil {
			return Tag{}, err
		}
		if n < 0 {
			return Tag{}, fmt.Errorf("negative int array length %d", n)
		}
		out := make([]int32, n)
		for i := range out {
			v, err := d.readInt32()
			if err != nil {
				return Tag{}, err
			}
			out[i] = v
		}
		return Tag{Type: TagIntArray, Ints: out}, nil
	case TagLongArray:
		n, err := d.readInt32()
		if err != nil {
			return Tag{}, err
		}
		if n < 0 {
			return Tag{}, fmt.Errorf("negative long array length %d", n)
		}
		out := make([]int64, n)
		for i := range out {
			v, err := d.readInt64()
			if err != nil {
				return Tag{}, err
			}
			out[i] = v
		}
		return Tag{Type: TagLongArray, Longs: out}, nil
	default:
		return Tag{}, fmt.Errorf("unknown NBT tag type %d", typ)
	}
}

func (d *decoder) readListPayload() (Tag, error) {
	elemTypeByte, err := d.readByte()
	if err != nil {
		return Tag{}, fmt.Errorf("read list element type: %w", err)
	}
	elemType := TagType(elemTypeByte)
	n, err := d.readInt32()
	if err != nil {
		return Tag{}, fmt.Errorf("read list length: %w", err)
	}
	if n < 0 {
		return Tag{}, fmt.Errorf("negative list length %d", n)
	}
	list := make([]Tag, 0, n)
	for i := int32(0); i < n; i++ {
		elem, err := d.readTagPayload(elemType)
		if err != nil {
			return Tag{}, fmt.Errorf("read list element %d: %w", i, err)
		}
		list = append(list, elem)
	}
	return Tag{Type: TagList, List: list, ListType: elemType}, nil
}

func (d *decoder) readCompoundPayload() (Tag, error) {
	out := map[string]Tag{}
	for {
		typByte, err := d.readByte()
		if err != nil {
			return Tag{}, fmt.Errorf("read compound entry type: %w", err)
		}
		typ := TagType(typByte)
		if typ == TagEnd {
			return Tag{Type: TagCompound, Compound: out}, nil
		}
		name, err := d.readString()
		if err != nil {
			return Tag{}, fmt.Errorf("read compound entry name: %w", err)
		}
		val, err := d.readTagPayload(typ)
		if err != nil {
			return Tag{}, fmt.Errorf("read compound entry %q: %w", name, err)
		}
		out[name] = val
	}
}
