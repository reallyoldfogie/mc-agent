package models

// ProfileProperty represents a single game profile property such as "textures".
type ProfileProperty struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature,omitempty"`
}
