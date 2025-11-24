//go:build none

package models

import "encoding/json"

// BlockID describes the numeric ID of a block.
type BlockID uint32

// Block describes information about a type of block.
type Block struct {
	ID          BlockID
	DisplayName string
	Name        string

	Hardness   float64
	Diggable   bool
	DropIDs    []uint32
	NeedsTools map[uint32]bool

	MinStateID uint64
	MaxStateID uint64

	Transparent      bool
	FilterLightLevel int
	EmitLightLevel   int
}

type ParseBlock struct {
	Name             string
	ID               int64
	IDTag            string
	DisplayName      string
	Hardness         int
	Diggable         bool
	DropIDs          []uint32
	NeedsTools       string
	MinStateID       uint64
	MaxStateID       uint64
	StateIDs         []uint64
	Transparent      bool
	FilterLightLevel int
	EmitLightLevel   int
}

type BlockFileContents map[string]BlockObject

type BlockObject struct {
	Definition BlockDefinition `json:"definition,omitempty"`
	Properties BlockProperties `json:"properties,omitempty"`
	States     []BlockState    `json:"states,omitempty"`
}

type BlockDefinition struct {
	Type               string          `json:"type,omitempty"`
	BlockSetType       string          `json:"block_set_type,omitempty"`
	Properties         BlockProperties `json:"properties,omitempty"`
	TicksToStayPressed int             `json:"ticks_to_stay_pressed,omitempty"`
}

type BlockProperties map[string]BlockProperty

type BlockProperty []string

func (bp *BlockProperty) UnmarshalJSON(b []byte) error {
	type tmpArr []string

	var str string
	var tmp tmpArr
	if err := json.Unmarshal(b, &str); err != nil {
		if err := json.Unmarshal(b, &tmp); err != nil {
			panic(err)
		}

		*bp = BlockProperty(tmp)
	} else {
		*bp = []string{str}
	}

	return nil
}

type BlockState struct {
	ID         int64           `json:"id,omitempty"`
	Default    bool            `json:"default,omitempty"`
	Properties BlockProperties `json:"properties,omitempty"`
}
