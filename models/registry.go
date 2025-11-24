//go:build none

package models

import (
	"encoding/json"
	"io"
)

type Registries map[string]Registry

type Registry struct {
	ID      string                `json:"protocol_id,omitempty"`
	Entries map[string]ProtocolID `json:"entries,omitempty"`
}

type ProtocolID struct {
	ID int64 `json:"protocol_id,omitempty"`
}

func (reg *Registries) ReadFrom(r io.Reader) (numRead int64, err error) {
	json.NewDecoder(r).Decode(reg)
	return numRead, err
}

/*
"minecraft:sound_event": {
    "entries": {
      "minecraft:ambient.basalt_deltas.additions": {
        "protocol_id": 8
      },
      "minecraft:ambient.basalt_deltas.loop": {
        "protocol_id": 9
      },
	  },
    "protocol_id": 1
},

*/
