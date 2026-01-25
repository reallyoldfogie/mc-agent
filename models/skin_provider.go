package models

// SkinProvider returns skin/texture properties for a given player UUID/name.
type SkinProvider interface {
	Get(uuid [16]byte, name string) []ProfileProperty
}
