package agent

import (
	"io"
	"math"

	pk "github.com/Tnze/go-mc/net/packet"
)

// LpVec3 is a compressed 3D vector format used in Minecraft 1.21.5+ for velocities.
// It encodes three doubles in typically just 1-7 bytes using bit packing and scale factors.
// Reference: https://minecraft.wiki/w/Java_Edition_protocol/Data_types#LpVec3
type LpVec3 struct {
	X, Y, Z float64
}

// ReadFrom reads and decodes an LpVec3 from the packet.
func (v *LpVec3) ReadFrom(r io.Reader) (n int64, err error) {
	// Read first 2 bytes in little-endian
	var b1, b2 pk.Byte
	var nn int64
	if nn, err = b1.ReadFrom(r); err != nil {
		return 0, err
	}
	n += nn
	if nn, err = b2.ReadFrom(r); err != nil {
		return n, err
	}
	n += nn

	// Check if all values are near zero (special case)
	if b1 == 0 && b2 == 0 {
		v.X, v.Y, v.Z = 0, 0, 0
		return n, nil
	}

	// Extract 15-bit packed values for X, Y, Z
	// Bits layout in first 2 bytes + next 4 bytes:
	// Byte 1 (LE): bits 0-7 of packed X
	// Byte 2 (LE): bits 8-14 of packed X (7 bits), continuation flag (1 bit)
	ub1, ub2 := uint8(b1), uint8(b2)
	packed1 := uint16(ub1) | (uint16(ub2&0x7F) << 8) // 15 bits for first value

	continuationFlag := (ub2 & 0x80) != 0

	// Read remaining 4 bytes in big-endian order (bytes 6, 5, 4, 3)
	var b3, b4, b5, b6 pk.Byte
	if nn, err = b6.ReadFrom(r); err != nil {
		return n, err
	}
	n += nn
	if nn, err = b5.ReadFrom(r); err != nil {
		return n, err
	}
	n += nn
	if nn, err = b4.ReadFrom(r); err != nil {
		return n, err
	}
	n += nn
	if nn, err = b3.ReadFrom(r); err != nil {
		return n, err
	}
	n += nn

	// Extract packed Y and Z (15 bits each) and scale factor (2 bits)
	ub3, ub4, ub5, ub6 := uint8(b3), uint8(b4), uint8(b5), uint8(b6)
	packed2 := (uint16(ub6) << 7) | (uint16(ub5) >> 1)
	packed3 := ((uint16(ub5) & 0x01) << 14) | (uint16(ub4) << 6) | (uint16(ub3) >> 2)
	scaleFactor := ub3 & 0x03

	// If continuation flag is set, read additional VarInt scale bits
	if continuationFlag {
		var extraScale pk.VarInt
		if nn, err = extraScale.ReadFrom(r); err != nil {
			return n, err
		}
		n += nn
		scaleFactor |= uint8(extraScale << 2)
	}

	// Convert packed 15-bit values to signed (-32766 to +32766)
	signedX := int16(packed1) - 16383
	signedY := int16(packed2) - 16383
	signedZ := int16(packed3) - 16383

	// Calculate scale divisor
	scale := math.Pow(2, float64(scaleFactor))

	// Denormalize to get actual double values
	v.X = float64(signedX) / 32766.0 * scale
	v.Y = float64(signedY) / 32766.0 * scale
	v.Z = float64(signedZ) / 32766.0 * scale

	return n, nil
}

// varIntSize returns the number of bytes a VarInt will occupy
func varIntSize(value int32) int {
	for i := 1; i <= 5; i++ {
		if (value & ^0x7F) == 0 {
			return i
		}
		value >>= 7
	}
	return 5
}
