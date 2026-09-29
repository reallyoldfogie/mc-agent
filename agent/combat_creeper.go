package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

const creeperLootWait = 2 * time.Second

func validateCreeperWeapon(itemName string) error {
	if combat.ClassifyMeleeItem(itemName) == combat.UnknownMeleeWeapon {
		return errors.New("kill creeper: a supported melee weapon must already be selected")
	}
	return nil
}

// KillCreeperForGunpowder runs one controlled creeper encounter using the
// currently selected supported melee weapon and collects a dropped item when
// one appears. Weapon selection and inventory setup belong to the caller. A
// false result is a successful no-drop encounter; a true result means the
// agent observed an item entity, walked to it, and saw its gunpowder count
// increase. The action deliberately does not use server/RCON state, so it can
// be reused by command and RL callers.
func (a *agent) KillCreeperForGunpowder(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, errors.New("kill creeper: nil context")
	}
	if err := validateCreeperWeapon(a.combatHeldItemName()); err != nil {
		return false, err
	}

	creeperType, found := a.GetEntityTypeID("minecraft:creeper")
	if !found {
		return false, errors.New("kill creeper: creeper type not found in registry")
	}
	position, _, _, initialized := a.GetPosition()
	if !initialized {
		return false, errors.New("kill creeper: agent position not initialized")
	}
	deadline := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	var targetID int32
	for {
		if id, _, ok := a.FindNearestEntityByType(creeperType, position.X, position.Y, position.Z, false); ok {
			targetID = id
			break
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
		}
	}

	beforeGunpowder := a.InventoryCount("minecraft:gunpowder")
	combatCtx, cancelCombat := context.WithCancel(ctx)
	combatDone := make(chan error, 1)
	go func() {
		combatDone <- a.RunCombat(combatCtx, 12, false)
	}()

	var targetDead bool
	for !targetDead {
		if health, _, _, known := a.Health(); known && health <= 0 {
			cancelCombat()
			<-combatDone
			return false, errors.New("kill creeper: agent died during encounter")
		}
		info, tracked := a.GetTrackedEntities()[targetID]
		targetDead = !tracked || info.Removed || (info.MaxHealth > 0 && info.Health <= 0)
		if targetDead {
			break
		}
		select {
		case <-ctx.Done():
			cancelCombat()
			<-combatDone
			return false, ctx.Err()
		case <-deadline.C:
		}
	}
	cancelCombat()
	if err := <-combatDone; err != nil && !errors.Is(err, context.Canceled) {
		return false, fmt.Errorf("kill creeper: combat loop: %w", err)
	}

	lootDeadline := time.NewTimer(creeperLootWait)
	defer lootDeadline.Stop()
	for {
		if current := a.InventoryCount("minecraft:gunpowder"); current > beforeGunpowder {
			return true, nil
		}
		_, x, y, z, visible, err := a.FindNearestVisibleItem(ctx, 12)
		if err != nil {
			return false, fmt.Errorf("kill creeper: find dropped item: %w", err)
		}
		if visible {
			if err := a.MoveTo(ctx, x, y, z, false); err != nil {
				return false, fmt.Errorf("kill creeper: collect dropped item: %w", err)
			}
			if current := a.InventoryCount("minecraft:gunpowder"); current > beforeGunpowder {
				return true, nil
			}
			return false, errors.New("kill creeper: dropped item was not gunpowder")
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-lootDeadline.C:
			return false, nil
		case <-deadline.C:
		}
	}
}

var _ models.AgentActions = (*agent)(nil)
