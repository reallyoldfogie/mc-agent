package agent

import (
	"io"

	pk "github.com/Tnze/go-mc/net/packet"
)

// helper: decode an ID Set from the packet into our struct.
// IDSet encoding: First varint is the "count":
//   - If count == 0: Tag list follows (varint count, then that many strings)
//   - If count != 0: First ID = count-1, then read count-1 more IDs
func scanIDSet(r io.Reader) (IDSet, error) {
	var count pk.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		return IDSet{}, err
	}

	if count == 0 {
		// Tag list representation - read array of tag names
		var numTags pk.VarInt
		if _, err := numTags.ReadFrom(r); err != nil {
			return IDSet{}, err
		}
		// For now, we just skip the tag names since the agent doesn't use them
		for i := 0; i < int(numTags); i++ {
			var tagName pk.String
			if _, err := tagName.ReadFrom(r); err != nil {
				return IDSet{}, err
			}
		}
		return IDSet{Mode: IDSetEmpty}, nil
	}

	// IDs representation: first ID is count-1
	ids := make([]int32, int(count))
	ids[0] = int32(count - 1)
	// Read remaining count-1 IDs
	for i := 1; i < int(count); i++ {
		var id pk.VarInt
		if _, err := id.ReadFrom(r); err != nil {
			return IDSet{}, err
		}
		ids[i] = int32(id)
	}

	// Determine mode based on count
	if count == 1 {
		return IDSet{Mode: IDSetSingle, IDs: ids}, nil
	}
	return IDSet{Mode: IDSetList, IDs: ids}, nil
}
