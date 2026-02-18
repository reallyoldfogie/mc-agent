package physics

// GetBlockSlipperiness is a POC that returns the slipperiness multiplier for a given block id.
// By default it returns the base Slipperiness constant. Known special cases
// (ice, slime block) are handled here; expand mappings as real block ids
// (from mc-data-gen or block registry) become available.

// TODO: The mapping below currently uses hard-coded historical numeric block
// IDs. Block numeric IDs vary across Minecraft versions and should NOT be
// hard-coded here. Replace this with a version-aware mapping (for example by
// using a block registry from `mc-protocol-go`) so the
// correct slipperiness is returned for the running server version.
// There may be slipperyness information in the shape provider, which might be able to be used instead.
func GetBlockSlipperiness(blockID uint32) float64 {
	switch blockID {
	// Common vanilla numeric IDs (historical): Ice (79), Packed Ice (174)
	case 79, 174:
		return IceSlipperiness
	// Slime block historical ID: 165 (kept for compatibility with numeric tests)
	case 165:
		return SlimeBlockSlipperiness
	default:
		return Slipperiness
	}
}
