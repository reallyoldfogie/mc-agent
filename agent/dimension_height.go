package agent

import (
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// dimensionHeightFromNBT extracts a "minecraft:dimension_type" registry
// entry's min_y/height NBT fields (both TAG_Int) and returns the
// equivalent world-Y bounds [minY, maxY] (maxY = min_y + height - 1). ok
// is false if nbt is nil or either field is missing/not an int - callers
// should keep the Overworld's default bounds in that case rather than
// treating it as an error.
func dimensionHeightFromNBT(nbt *protocol_models.NBTCompound) (minY, maxY int32, ok bool) {
	if nbt == nil {
		return 0, 0, false
	}

	minYVal, found := nbt.Get("min_y")
	if !found {
		return 0, 0, false
	}
	minYInt, isInt := minYVal.(*protocol_models.NBTInt)
	if !isInt {
		return 0, 0, false
	}

	heightVal, found := nbt.Get("height")
	if !found {
		return 0, 0, false
	}
	heightInt, isInt := heightVal.(*protocol_models.NBTInt)
	if !isInt {
		return 0, 0, false
	}

	return minYInt.Value, minYInt.Value + heightInt.Value - 1, true
}

// getDimensionHeightBounds looks up dimensionName's world-Y build height
// range [minY, maxY] from the already-received "minecraft:dimension_type"
// registry data. This reads mc-bot-go's own Client.RegistryData() - which
// independently parses and stores each registry entry's full NBT payload
// during the configuration phase - rather than mc-agent's own
// ParseRegistryData/onRegistryDataCallback path, which only keeps a
// simplified name->index map and discards the NBT entirely. ok is false if
// the registry hasn't arrived yet, dimensionName isn't a known entry, or
// its NBT is missing min_y/height.
func (a *agent) getDimensionHeightBounds(dimensionName string) (minY, maxY int32, ok bool) {
	if a.client == nil {
		return 0, 0, false
	}
	registries := a.client.RegistryData()
	if registries == nil {
		return 0, 0, false
	}
	dimTypes, found := registries["minecraft:dimension_type"]
	if !found || dimTypes == nil {
		return 0, 0, false
	}
	entry, found := dimTypes.ByName[dimensionName]
	if !found || entry == nil {
		return 0, 0, false
	}
	return dimensionHeightFromNBT(entry.NBT)
}
