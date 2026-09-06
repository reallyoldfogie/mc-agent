package models

import (
	"context"
	"io"
	"time"
)

type AgentActions interface { // Action helpers (used by plan runner)
	MoveTo(ctx context.Context, x, y, z float64, notifyChat bool) error
	LineTo(ctx context.Context, x, y, z float64, notifyChat bool) error
	MoveForward(ctx context.Context, distance float64) error
	MoveUp(ctx context.Context, distance float64) error
	FindPath(ctx context.Context, x, y, z float64) (*Path, error)
	ExecutePath(ctx context.Context, path *Path) error
	LookAt(ctx context.Context, x, y, z float64) error
	TurnTowards(ctx context.Context, x, y, z float64) error
	Follow(ctx context.Context, target string) error
	StopFollow(ctx context.Context) error
	FollowStatus(ctx context.Context) string
	ChatEvents(ctx context.Context) <-chan string
	HasLineOfSight(ctx context.Context, x, y, z float64) (bool, error)

	// CanInteractFromPosition reports whether a bot standing at
	// (fromX, fromY, fromZ) would have line-of-sight to interact with the
	// block at (targetX, targetY, targetZ) - unlike HasLineOfSight, it does
	// not use the agent's own current position, so callers can evaluate a
	// hypothetical standing position before actually moving there. See
	// FindInteractPosition (interact_position.go).
	CanInteractFromPosition(ctx context.Context, fromX, fromY, fromZ, targetX, targetY, targetZ float64) (bool, error)
	FindVisibleEntity(ctx context.Context, entityTypeID int32, maxDistance float64) (entityID int32, x, y, z float64, found bool, err error)
	FindVisibleBlock(ctx context.Context, blockName string, maxDistance int) (x, y, z float64, found bool, err error)
	FindAllVisibleBlocksInSphere(ctx context.Context, radius int) ([]VisibleBlockInfo, error)
	OpenContainerAt(ctx context.Context, x, y, z float64, face BlockFace, timeout time.Duration) (byte, error)
	UseItemOnBlock(ctx context.Context, x, y, z float64, face BlockFace, hand Hand) error
	UseItemOnEntity(ctx context.Context, entityID int32, hand Hand, sneaking bool) error
	SelectHotbarSlot(ctx context.Context, slot int16) error
	FindSlotWith(ctx context.Context, itemName string, windowID int) (slot int, found bool, err error)

	// WaitForHotbarItem waits for a specific item to appear in the hotbar.
	// This is useful after RCON commands that place items, as there may be inventory sync delays.
	// Returns the slot index when found, or error if timeout/context cancelled.
	WaitForHotbarItem(ctx context.Context, itemName string, maxWaitMS int) (slot int16, err error)

	// SwitchToItem finds an item by name anywhere in the player's inventory and equips it.
	// If the item is already in the hotbar, it selects that slot directly.
	// If the item is in the main inventory, it swaps it into a hotbar slot and selects it.
	// Returns (true, nil) if the item was found and equipped, (false, nil) if not found,
	// or (false, err) on error.
	SwitchToItem(ctx context.Context, itemName string) (bool, error)
	LogInventory(output io.Writer)

	MineBlockAt(ctx context.Context, pos V3, face BlockFace) error
	// PlaceBlockAt(ctx context.Context, pos V3, blockName string) error
	// AttackEntity(ctx context.Context, entityID int32) error
	// InteractWithEntity(ctx context.Context, entityID int32) error

	// Bow firing actions
	//Deprecated: Use FireBowAt for targeted firing
	FireBow(ctx context.Context) error

	FireBowAt(ctx context.Context, x, y, z float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error)
	FireBowWithPitch(ctx context.Context, pitch, yaw float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error) // Returns []models.TrajectoryPoint

	// Projectile actions
	// projectileType: models.ProjectileType value
	ThrowProjectileAt(ctx context.Context, projectileType ProjectileType, x, y, z float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error)

	// UseFireworkRocket sends a plain "use item" interaction with the
	// currently held item, mirroring the real client action that triggers
	// FireworkRocketItem.use()'s gliding-boost path when a firework rocket
	// is held while gliding. See the agent package implementation's doc
	// comment for why no aiming/targeting is involved.
	UseFireworkRocket() error

	// HasActiveFireworkBoost reports whether a firework rocket used while
	// gliding is currently attached to the agent's own entity and boosting
	// its velocity. See models.MountedEntityPositionGetter.HasActiveFireworkBoost
	// for the full doc comment — exposed here too so callers driving a
	// flight (deciding when to fire another rocket as the current one's
	// boost runs out) don't need access to the narrower internal interface.
	HasActiveFireworkBoost() bool
}
