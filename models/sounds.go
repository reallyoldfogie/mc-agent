//go:build none

package models

import "encoding/json"

// SoundID represents a sound ID used in the minecraft protocol.
type SoundID int32

type Sound struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	SubtitleKey string `json:"subtitle_key,omitempty"`
}

type SoundsFileContent map[string]SoundData

type SoundData struct {
	Sounds   []SoundInfo `json:"sounds,omitempty"`
	Subtitle string      `json:"subtitle,omitempty"`
}

type SoundInfo struct {
	Name   string  `json:"name,omitempty"`
	Stream bool    `json:"stream,omitempty"`
	Volume float32 `json:"volume,omitempty"`
	Weight float32 `json:"weight,omitempty"`
}

// sounds array may be either an array of strings or an array of SoundInfo objects
func (si *SoundInfo) UnmarshalJSON(b []byte) error {
	type tmpObj struct {
		Name   string  `json:"name,omitempty"`
		Stream bool    `json:"stream,omitempty"`
		Volume float32 `json:"volume,omitempty"`
		Weight float32 `json:"weight,omitempty"`
	}

	var str string
	var obj tmpObj
	if err := json.Unmarshal(b, &str); err != nil {
		if err := json.Unmarshal(b, &obj); err != nil {
			panic(err)
		}

		si.Name = obj.Name
		si.Stream = obj.Stream
		si.Volume = obj.Volume
	} else {
		si.Name = str
	}

	return nil
}
