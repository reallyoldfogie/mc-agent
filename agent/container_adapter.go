package agent

// // containerHelperAdapter adapts items.ContainerHelper to agent.ContainerHelper interface.
// // This handles type conversions between agent types (V3, BlockFace) and items types.
// type containerHelperAdapter struct {
// 	impl *items.ContainerHelper
// }

// // NewContainerHelperAdapter creates an adapter that wraps items.ContainerHelper.
// func NewContainerHelperAdapter(impl *items.ContainerHelper) ContainerHelper {
// 	return &containerHelperAdapter{impl: impl}
// }

// func (a *containerHelperAdapter) SetEntityIDProvider(provider EntityIDProvider) {
// 	// Wrap the agent's EntityIDProvider to match items.EntityIDProvider
// 	a.impl.SetEntityIDProvider(&entityIDProviderAdapter{provider: provider})
// }

// func (a *containerHelperAdapter) OpenContainer(pos V3, face BlockFace, timeout time.Duration) (byte, error) {
// 	// Convert agent.V3 to models.V3 and agent.BlockFace to items.BlockFace
// 	itemsPos := models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
// 	itemsFace := items.BlockFace(face)
// 	return a.impl.OpenContainer(itemsPos, itemsFace, timeout)
// }

// func (a *containerHelperAdapter) OpenEntityContainer(entityID int32, timeout time.Duration) (byte, error) {
// 	return a.impl.OpenEntityContainer(entityID, timeout)
// }

// func (a *containerHelperAdapter) CloseContainer() error {
// 	return a.impl.CloseContainer()
// }

// func (a *containerHelperAdapter) GetContainerSlotCount(windowID byte) int {
// 	return a.impl.GetContainerSlotCount(windowID)
// }

// func (a *containerHelperAdapter) TakeItemFromChest(windowID byte, chestSlot int16) error {
// 	return a.impl.TakeItemFromChest(windowID, chestSlot)
// }

// func (a *containerHelperAdapter) PutItemInChest(windowID byte, playerInventorySlot int16, chestSlot int16) error {
// 	return a.impl.PutItemInChest(windowID, playerInventorySlot, chestSlot)
// }

// func (a *containerHelperAdapter) FindItemInPlayerInventory(windowID byte, itemID int32) int16 {
// 	return a.impl.FindItemInPlayerInventory(windowID, itemID)
// }

// func (a *containerHelperAdapter) FindEmptyChestSlot(windowID byte) int16 {
// 	return a.impl.FindEmptyChestSlot(windowID)
// }

// func (a *containerHelperAdapter) GetChestRows(windowID byte) int {
// 	return a.impl.GetChestRows(windowID)
// }

// entityIDProviderAdapter adapts agent.EntityIDProvider to items.EntityIDProvider
type entityIDProviderAdapter struct {
	provider EntityIDProvider
}

func (a *entityIDProviderAdapter) GetEntityID() int32 {
	return a.provider.GetEntityID()
}
