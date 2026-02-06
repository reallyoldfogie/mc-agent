package models

// WorldOperations provides access to world state for block queries.
type WorldOperations interface {
	GetWorld() World
	BlockNameAt(ix, iy, iz int) string
}
