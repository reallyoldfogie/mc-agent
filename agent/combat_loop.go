package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// RunCombat runs the first conservative autonomous combat loop. It dispatches
// ordinary melee, version-aware spear Jab, and ranged projectile decisions
// through their respective execution adapters.
func (a *agent) RunCombat(ctx context.Context, radius float64, includeNeutral bool) error {
	return a.RunCombatWithPolicy(ctx, radius, combat.TargetPolicy{
		IncludePlayers: false,
		IncludeNeutral: includeNeutral,
	})
}

// RunCombatWithPolicy runs autonomous combat with an explicit target policy.
// Player targeting is opt-in at this boundary to prevent accidental friendly
// fire for callers that need PvE-only behavior.
func (a *agent) RunCombatWithPolicy(ctx context.Context, radius float64, policy combat.TargetPolicy) error {
	if ctx == nil {
		return fmt.Errorf("combat loop: nil context")
	}
	if err := a.EnterManualMode(); err != nil {
		return fmt.Errorf("combat loop: enter manual mode: %w", err)
	}
	defer a.ExitManualMode()

	controller := combat.NewController()
	currentWeapon := combat.NoWeapon
	lastState := combat.Idle
	lastTargetID := int32(-1)
	lastTargetVisible := false
	shieldActive := false
	shieldRestoreSlot := int16(-1)
	timer := time.NewTimer(a.combatTickInterval())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-timer.C:
			targets, err := a.RankCombatTargetsWithPolicy(ctx, radius, policy)
			if err != nil {
				return fmt.Errorf("combat loop: rank targets: %w", err)
			}
			health, _, _, known := a.Health()
			if !known {
				health = 20
			}
			obs := combat.Observation{
				Health: health, MaxHealth: 20, EnemyCount: len(targets), CurrentWeapon: currentWeapon,
				ShieldAvailable: a.hasCombatShield(), ShieldActive: shieldActive,
			}
			a.populateCombatMovementState(&obs)
			if len(targets) > 0 {
				target := targets[0]
				obs.HasTarget, obs.TargetVisible = true, target.Visible
				obs.TargetDistance = target.Distance
				obs.TargetDirectionX, obs.TargetDirectionZ = target.DirectionX, target.DirectionZ
				obs.TargetIsRanged = combatTargetIsRanged(target.TypeName)
				obs.TargetIsExplosive = combatTargetIsExplosive(target.TypeName)
				if target.Visible {
					if err := a.TurnTowards(ctx, target.X, target.Y+1.0, target.Z); err != nil {
						a.logf("[combat] target facing failed: entity=%d error=%v", target.EntityID, err)
					}
				}
				if target.EntityID != lastTargetID || target.Visible != lastTargetVisible {
					a.logf("[combat] target selected: entity=%d type=%s category=%d distance=%.2f visible=%v health=%.1f/%.1f",
						target.EntityID, target.TypeName, target.Category, target.Distance, target.Visible, target.Health, target.MaxHealth)
				}
				lastTargetID = target.EntityID
				lastTargetVisible = target.Visible
			} else if lastTargetID != -1 {
				a.logf("[combat] target lost: entity=%d", lastTargetID)
				lastTargetID = -1
				lastTargetVisible = false
			}

			decision, intent := controller.Next(now, obs)
			switch decision.ShieldAction {
			case combat.RaiseShield:
				if hand, slot, ok := a.combatShieldLocation(); ok && hand == models.MainHand {
					a.heldSlotMu.RLock()
					currentSlot, currentSet := a.heldSlot, a.heldSlotSet
					a.heldSlotMu.RUnlock()
					if !currentSet {
						a.logf("[combat] shield raise skipped: current hotbar slot is unknown")
						break
					}
					shieldRestoreSlot = currentSlot
					if currentSlot != slot {
						if err := a.SelectHotbarSlot(ctx, slot); err != nil {
							a.logf("[combat] shield slot selection failed: %v", err)
							break
						}
					}
				}
				if err := a.setCombatShield(ctx, true); err != nil {
					a.logf("[combat] shield raise failed: %v", err)
				} else {
					shieldActive = true
				}
			case combat.LowerShield:
				if err := a.setCombatShield(ctx, false); err != nil {
					a.logf("[combat] shield lower failed: %v", err)
				} else {
					shieldActive = false
					if shieldRestoreSlot >= 0 {
						if err := a.SelectHotbarSlot(ctx, shieldRestoreSlot); err != nil {
							a.logf("[combat] shield weapon restore failed: %v", err)
						}
						shieldRestoreSlot = -1
					}
				}
			}
			if decision.State != lastState {
				a.logf("[combat] state transition: %d -> %d weapon=%d targets=%d health=%.1f/%.1f",
					lastState, decision.State, decision.Weapon, len(targets), health, obs.MaxHealth)
				lastState = decision.State
			}
			currentWeapon = decision.Weapon
			if len(targets) > 0 {
				target := targets[0]
				environment, err := a.CombatMovementEnvironment(ctx, models.V3{X: target.X, Y: target.Y, Z: target.Z}, combatHazard(target.TypeName))
				if err != nil {
					return fmt.Errorf("combat loop: environment: %w", err)
				}
				environment.ExplosiveTarget = combatTargetIsExplosive(target.TypeName)
				intent = combat.FilterMovement(intent, environment)
			}
			if err := a.ApplyCombatMovement(ctx, intent); err != nil {
				return fmt.Errorf("combat loop: movement: %w", err)
			}
			if decision.Attack && decision.Weapon == combat.MeleeWeapon && len(targets) > 0 {
				critical := decision.Critical && a.combatCriticalEligible()
				if err := a.executeCombatMeleeAttack(ctx, targets[0]); err == nil {
					a.logf("[combat] melee attack succeeded: entity=%d critical=%v", targets[0].EntityID, critical)
					controller.CommitAttack(time.Now())
				} else {
					a.logf("[combat] melee attack failed: entity=%d critical=%v error=%v", targets[0].EntityID, critical, err)
				}
			}
			if decision.Attack && decision.Weapon == combat.RangedWeapon && len(targets) > 0 {
				var lastErr error
				weapons, err := a.availableCombatProjectileWeapons(ctx)
				if err != nil {
					lastErr = err
				}
				for _, weapon := range weapons {
					request, err := rangedRequestForTarget(targets[0], weapon)
					if err != nil {
						lastErr = err
						continue
					}
					if err := a.ExecuteRangedAttack(ctx, request); err == nil {
						controller.CommitAttack(time.Now())
						lastErr = nil
						break
					} else {
						lastErr = err
					}
				}
				if lastErr != nil {
					a.logf("[combat loop] ranged attack failed for all available weapons: %v", lastErr)
				}
			}
			timer.Reset(a.combatTickInterval())
		}
	}
}

// availableCombatProjectileWeapons returns the stable fallback order filtered
// by the synchronized player inventory. This avoids repeatedly attempting to
// equip absent weapons on every combat tick and keeps the final error tied to
// an actually available weapon.
func (a *agent) availableCombatProjectileWeapons(ctx context.Context) ([]combat.ProjectileWeapon, error) {
	available := make(map[string]bool, 3)
	for _, weapon := range []combat.ProjectileWeapon{combat.Bow, combat.Crossbow, combat.Trident} {
		itemName, _, err := projectileForCombatWeapon(weapon)
		if err != nil {
			return nil, err
		}
		_, found, err := a.FindSlotWith(ctx, itemName, -2)
		if err != nil {
			return nil, fmt.Errorf("combat ranged inventory: check %s: %w", itemName, err)
		}
		if found {
			available[itemName] = true
		}
	}
	return filterCombatProjectileWeapons(available), nil
}

// populateCombatMovementState copies authoritative movement/status facts into
// the transport-independent combat observation. If the executor does not
// expose its physics state, the zero velocity leaves Critical false rather
// than guessing that a critical is possible.
func (a *agent) populateCombatMovementState(obs *combat.Observation) {
	if obs == nil {
		return
	}
	a.movementMu.RLock()
	executor := a.moveExec
	a.movementMu.RUnlock()
	if stateProvider, ok := executor.(interface{ GetPhysicsState() models.PhysicsState }); ok {
		if state := stateProvider.GetPhysicsState(); state != nil {
			obs.OnGround = state.OnGround()
			obs.VerticalVelocity = state.Velocity().Y
			obs.InWater = state.IsInWater()
		}
	}
	obs.OnVehicle = a.IsMounted()
	_, obs.HasBlindness = a.GetOwnActiveEffect("minecraft:blindness")
	if executor != nil {
		obs.Sprinting = executor.IsSprinting()
	}
}

// combatCriticalEligible re-reads the movement/status state immediately before
// melee dispatch. There is no critical-hit bit in the interaction packet: the
// server derives critical-hit eligibility from the movement state at the time
// it handles the attack. Rechecking here prevents stale controller decisions
// from being reported as critical while preserving ordinary AttackEntity
// dispatch when the state has changed between ticks.
func (a *agent) combatCriticalEligible() bool {
	var obs combat.Observation
	a.populateCombatMovementState(&obs)
	return combat.CriticalHitAllowed(obs)
}

// executeCombatMeleeAttack keeps ordinary melee dispatch unchanged while
// routing a held spear through its version-aware executor. Spear Jab uses the
// same primary interaction as a normal melee attack, but the adapter must
// validate the 1.21.11+ item and reach semantics before sending it.
func (a *agent) executeCombatMeleeAttack(ctx context.Context, target combat.Target) error {
	itemName := a.combatHeldItemName()
	switch combat.ClassifyMeleeItem(itemName) {
	case combat.MaceMeleeWeapon:
		if a.maceSmashActive.Load() {
			return nil
		}
		return a.MaceAttackAt(ctx, target.EntityID, itemName)
	case combat.SpearMeleeWeapon:
		break
	default:
		return a.AttackEntity(ctx, target.EntityID, false)
	}
	request, err := spearJabRequestForTarget(target, itemName)
	if err != nil {
		return err
	}
	return a.ExecuteSpearAttack(ctx, request)
}

func (a *agent) combatHeldItemName() string {
	a.heldSlotMu.RLock()
	heldSlot, set := a.heldSlot, a.heldSlotSet
	a.heldSlotMu.RUnlock()
	if !set {
		return ""
	}
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		return ""
	}
	itemID, count, ok := slots.ResolveSlot(-2, mcscreen.HotbarSlotStart+heldSlot)
	if !ok || count <= 0 {
		return ""
	}
	return normalizeItemName(itemMgr.GetItemNameByID(itemID))
}

// combatShieldLocation returns the preferred shield hand and, for a main-hand
// shield, its hotbar slot. Off-hand is preferred because it does not disturb
// the selected combat weapon.
func (a *agent) combatShieldLocation() (models.Hand, int16, bool) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		return models.MainHand, -1, false
	}
	isShield := func(index int16) bool {
		itemID, count, ok := slots.ResolveSlot(-2, index)
		return ok && count > 0 && normalizeItemName(itemMgr.GetItemNameByID(itemID)) == "minecraft:shield"
	}
	if isShield(45) { // player off-hand
		return models.OffHand, -1, true
	}
	for slot := minHotbarSlot; slot <= maxHotbarSlot; slot++ {
		if isShield(36 + slot) {
			return models.MainHand, slot, true
		}
	}
	return models.MainHand, -1, false
}

func (a *agent) hasCombatShield() bool {
	_, _, ok := a.combatShieldLocation()
	return ok
}

func combatTargetIsRanged(typeName string) bool {
	switch normalizeItemName(typeName) {
	case "minecraft:skeleton", "minecraft:stray", "minecraft:bogged", "minecraft:breeze", "skeleton", "stray", "bogged", "breeze":
		return true
	default:
		return false
	}
}

// setCombatShield emits the same generic use-item/release sequence used by a
// vanilla client. The off-hand item is known to be a shield by
// hasCombatShield; no version-specific shield packet exists.
func (a *agent) setCombatShield(ctx context.Context, active bool) error {
	if ctx == nil {
		return fmt.Errorf("shield action: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.versionHandler == nil {
		return fmt.Errorf("shield action: missing version handler")
	}
	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return fmt.Errorf("shield action: action handler not available")
	}
	conn, err := a.getPacketWriter()
	if err != nil {
		return err
	}
	_, yaw, pitch, ok := a.GetPosition()
	if active && !ok {
		return fmt.Errorf("shield action: agent position not initialized")
	}
	hand, _, shieldAvailable := a.combatShieldLocation()
	if !shieldAvailable {
		return fmt.Errorf("shield action: shield is no longer equipped")
	}
	return sendCombatShieldAction(actions, conn, hand, active, yaw, pitch, a.getNextSequence())
}

// SetCombatShield exposes the same validated shield packet sequence used by
// the autonomous combat loop.
func (a *agent) SetCombatShield(ctx context.Context, active bool) error {
	return a.setCombatShield(ctx, active)
}

// sendCombatShieldAction is kept separate from agent state so packet dispatch
// can be tested against each version handler without a live server.
func sendCombatShieldAction(actions models.ActionHandler, conn models.PacketWriter, hand models.Hand, active bool, yaw, pitch float64, sequence int32) error {
	if active {
		return actions.SendUseItem(conn, hand, sequence, yaw, pitch)
	}
	return actions.SendPlayerAction(conn, common.PlayerActionReleaseUseItem, 0, 0, 0, 0, sequence)
}

// combatTickInterval returns the current server tick interval. The packet
// handler updates serverTickRateBits when the server changes its tick rate;
// an unset rate retains the existing vanilla-tick fallback.
func (a *agent) combatTickInterval() time.Duration {
	rate := float64(math.Float32frombits(a.serverTickRateBits.Load()))
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		rate = vanillaTickRate
	}
	return time.Duration(float64(time.Second) / rate)
}

func combatHazard(typeName string) bool {
	category := combat.Classify(typeName)
	if category != combat.Hostile {
		return false
	}
	return typeName == "minecraft:creeper" || typeName == "creeper" ||
		typeName == "minecraft:skeleton" || typeName == "skeleton" ||
		typeName == "minecraft:stray" || typeName == "stray"
}

func combatTargetIsExplosive(typeName string) bool {
	switch normalizeItemName(typeName) {
	case "minecraft:creeper", "creeper":
		return true
	default:
		return false
	}
}
