package rlenv

import (
	"math/rand"
	"time"
)

// Config configures an Environment. There is deliberately no notion of a
// task *type* here (see doc.go): TargetOffset is the one task this
// environment knows how to pose, "reach a point offset from wherever Reset
// found the bot."
type Config struct {
	// TargetOffset is added to the bot's position at Reset to produce this
	// episode's target (Environment.targetX/Y/Z). Using an offset from the
	// live position, rather than a fixed absolute world coordinate, is a
	// deliberate workaround for not having a real reset/teleport mechanism
	// yet (RSI_TRAINING_PLAN.md item 2, "Episode reset strategy" — still
	// unverified whether mc-agent's RCON dependency is wired up for this):
	// every Reset poses a reachable, bot-relative task without needing to
	// move the bot anywhere.
	TargetOffset [3]float64

	// ArrivalThreshold is the distance (in blocks) within which the bot
	// counts as having reached a target.
	ArrivalThreshold float64

	// StepTimeout bounds how long one Step waits for a dispatched
	// movement action's models.Completion to resolve before giving up on
	// *this step* (see environment.go's Step doc comment). The dispatched
	// action may still be running in the background past this deadline —
	// StepTimeout bounds one Step call, not the action itself.
	StepTimeout time.Duration

	// MineTargetBlock, if set, is the block name (e.g. "minecraft:stone")
	// this episode's mining task targets — enables ActionMine, following
	// TargetOffset's "environment poses the task" pattern
	// (docs/plans/RL_ACTION_SPACE_EXPANSION.md Phase 2a option (a)). Empty
	// (the default) means this Environment instance doesn't pose a mining
	// task: ActionMine becomes a safe no-op, the same way ActionWait always
	// is, rather than an error — see action.go's resolveDispatch.
	MineTargetBlock string

	// MineSearchRadius bounds FindVisibleBlock's search when locating the
	// nearest visible MineTargetBlock instance, in blocks. Ignored if
	// MineTargetBlock is empty. <= 0 defers to FindVisibleBlock's own
	// built-in default (8 blocks, see agent/actions.go) rather than being
	// validated here.
	MineSearchRadius int

	// MineRequiredTool, if set, is an item name (e.g.
	// "minecraft:wooden_pickaxe") DefaultEpisodeSeeder ensures is in the
	// bot's inventory before this episode's mining task starts (via
	// SeedGiveItem) — see docs/plans/14-buried-block-mining-and-memory-map.md
	// (mc-rsi-trainer) "Part 3": mc-agent's own mining code has no tool-tier
	// gating, but the live server does, and the generic "mined" completion
	// check alone can't tell a correctly-tooled success from a bare-handed
	// one that drops nothing. Ignored if MineTargetBlock is empty, and only
	// meaningful if the seeder in use actually implements ToolSeedAgent
	// (DefaultEpisodeSeeder does; a seeder that doesn't is free to ignore
	// this field entirely rather than fail the episode over it).
	MineRequiredTool string

	// MineRequiredToolEnchantments, if non-empty, asks DefaultEpisodeSeeder
	// to give MineRequiredTool pre-enchanted (via SeedGiveEnchantedItem)
	// instead of plain (SeedGiveItem) — keys are enchantment IDs (e.g.
	// "efficiency" or "minecraft:efficiency" — normalized the same
	// lenient way item names already are), values their level. Exists
	// because a
	// live validation run found digging through solid ground is
	// dominated by movement/pathing overhead, not break time, but a
	// faster break time is still free, safe, and compounds with any later
	// fix to that overhead — see
	// docs/plans/14-buried-block-mining-and-memory-map.md's follow-up
	// notes (mc-rsi-trainer). Ignored if MineRequiredTool is empty, or if
	// the seeder/agent in use doesn't implement EnchantedToolSeedAgent
	// (DefaultEpisodeSeeder then falls back to ToolSeedAgent's plain
	// SeedGiveItem, exactly as if this field were unset — never fails the
	// episode just because enchanting isn't available).
	MineRequiredToolEnchantments map[string]int

	// MineTargetBuried, if true, tells DefaultEpisodeSeeder that
	// MineTargetBlock is already present somewhere in the world (e.g.
	// baked into a shared clone-reset template's underground stone band —
	// docs/plans/14-buried-block-mining-and-memory-map.md, mc-rsi-trainer)
	// rather than something it needs to place. Caught live, against a
	// real server: without this, DefaultEpisodeSeeder's default behavior
	// (SeedNearbyBlock, which places a visible instance beside the bot
	// whenever FindVisibleBlock doesn't already see one) placed a second,
	// immediately-visible instance right next to the bot every episode —
	// the buried instance is never visible by design, so it always looked
	// "missing" — defeating burial entirely. This mirrors the composite
	// SeedAtGoal episode's own fix for the identical problem one level up
	// (Environment.resetAttempt already blanks MineTargetBlock/
	// CraftTargetItem before calling Seeder for exactly this reason), but
	// expressed as its own flag since a buried mine task isn't a
	// composite goto episode. Ignored if MineTargetBlock is empty; has no
	// effect on MineRequiredTool's own seeding, which still runs.
	MineTargetBuried bool

	// SleepSearchRadius, if > 0, enables ActionSleep: FindNearestBed
	// searches this many blocks for a bed to sleep in. Unlike
	// MineTargetBlock/CraftTargetItem, there is no companion "target
	// name" field — a bed isn't identified by name (it comes in 16
	// colors, see models.BlockShapeManager.IsBed), so SleepSearchRadius
	// alone both enables the capability and bounds the search, following
	// MineSearchRadius's own shape. <= 0 (the default) means this
	// Environment instance doesn't pose a sleep task: ActionSleep becomes
	// a safe no-op, the same way ActionMine/ActionCraft do when
	// unconfigured — see action.go's resolveDispatch/actionLegal. Because
	// ActionSleep is structurally always available (unlike ActionMine/
	// ActionCraft, which additionally need a non-empty target field),
	// this single field is what makes the whole feature inert everywhere
	// until something explicitly turns it on — see
	// docs/plans/SLEEP_ACTION_PLAN.md (local/untracked).
	SleepSearchRadius int

	// PortalSearchRadius, if > 0, enables ActionUsePortal:
	// FindNearestPortal searches this many blocks for a
	// nether_portal/end_portal/end_gateway block to walk into. Same shape
	// as SleepSearchRadius's own doc comment: no companion "target name"
	// field (a portal isn't identified by name — any of the three counts),
	// so this single field both enables the capability and bounds the
	// search. <= 0 (the default) means this Environment instance doesn't
	// pose a dimension-transfer task: ActionUsePortal becomes a safe
	// no-op, the same way ActionMine/ActionCraft/ActionSleep do when
	// unconfigured. Note that reward.go's portalRewardBonus is
	// deliberately NOT gated on this field (mirrors sleptThroughNightBonus's
	// own lack of a Config gate) — a dimension change is rewarded whenever
	// observed, whether or not this instance posed it as the task, same
	// "judge from world-state deltas" philosophy as every other reward in
	// this package.
	PortalSearchRadius int

	// LightPortalSearchRadius, if > 0, enables ActionLightPortal:
	// FindPortalFrame searches this many blocks for an unlit nether
	// portal frame (any vanilla size, 2x3 to 21x21 interior) to ignite.
	// Same shape as PortalSearchRadius's own doc comment: no companion
	// "target name" field, since a frame isn't identified by name either.
	// <= 0 (the default) means this Environment instance doesn't pose a
	// portal-lighting task: ActionLightPortal becomes a safe no-op.
	// Unlike PortalSearchRadius/SleepSearchRadius's reward (deliberately
	// NOT Config-gated), reward.go's lightPortalRewardBonus IS
	// effectively gated by this field: the frame it judges "did this
	// ignite" against (Environment.portalFrame) is only ever tracked
	// while this is > 0 (see refreshPortalFrameTarget) — there's no
	// frame-identifying signal to compare against otherwise, unlike
	// GetDimension()/GetTimeOfDay(), which are always trackable.
	LightPortalSearchRadius int

	// CraftTargetItem, if set, is the item name (e.g. "minecraft:stick")
	// this episode's crafting task targets — enables ActionCraft, following
	// MineTargetBlock's exact "environment poses the task" pattern
	// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1a). Empty (the default)
	// means this Environment instance doesn't pose a crafting task:
	// ActionCraft becomes a safe no-op, not an error — see action.go's
	// resolveDispatch. Independent of MineTargetBlock/TargetOffset: one
	// instance can pose a move-to target, a mine task, and a craft task
	// simultaneously.
	CraftTargetItem string

	// CraftSearchRadius, if > 0, makes the craft task need a crafting table
	// within this many blocks: the craftReady observation feature (and so
	// ActionCraft's mask) is true only while a table is visible that close,
	// on top of the ingredients being held. It is the crafting counterpart of
	// MineSearchRadius gating ActionMine, and exists for the same reason:
	// mc-agent's craft action searches 32 blocks for a table and walks there
	// itself, so without this gate a table 20 blocks away is crafted at
	// without the policy ever travelling. Only meaningful for a recipe that
	// needs a table; for one that fits the 2x2 inventory grid no table is
	// ever seeded and the craft would never become ready. <= 0 (the default)
	// applies no table condition. Ignored if CraftTargetItem is empty.
	CraftSearchRadius int

	// PickupTargetItem, if set, is the item name (e.g. "minecraft:stick")
	// this episode's item-pickup task targets — enables ActionPickUpItem,
	// following MineTargetBlock's exact "environment poses the task"
	// pattern (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 2). Empty
	// (the default) means this Environment instance doesn't pose a pickup
	// task: ActionPickUpItem becomes a safe no-op, not an error — see
	// action.go's resolveDispatch. Unlike MineTargetBlock, there's no
	// position-based reward check possible here (the item entity simply
	// disappears once collected — there's no "did this position's content
	// change" the way a mined block's position still exists, now as air):
	// reward is judged from an InventoryCount increase instead, the same
	// shape CraftTargetItem's own reward already uses.
	PickupTargetItem string

	// PickupSearchRadius bounds FindNearestVisibleItemOfType's search when
	// locating the nearest visible PickupTargetItem instance, in blocks.
	// Ignored if PickupTargetItem is empty. <= 0 means unbounded (mirrors
	// FindNearestVisibleItemOfType's own "maxDistance <= 0 disables the
	// limit" convention, unlike MineSearchRadius's "<=0 defers to a built-in
	// default" — FindNearestVisibleItemOfType has no such built-in default
	// to defer to) — see DefaultConfig's own default for a reasonable value.
	PickupSearchRadius int

	// PickupAnyItem, if true (and PickupTargetItem is empty), poses the
	// pickup task as "collect whatever's nearest, regardless of content" —
	// the generalized form of the "pickupnearbyitem" chat command, mirroring
	// FindNearestVisibleItem's own unfiltered search (the predecessor
	// FindNearestVisibleItemOfType was built on top of — see
	// docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 1). Mutually
	// exclusive with PickupTargetItem in practice: if both are set,
	// PickupTargetItem's name-filtered resolution takes precedence (see
	// refreshPickupTarget).
	PickupAnyItem bool

	// PlaceTargetItem, if set, is the item name (e.g. "minecraft:dirt")
	// this episode's standalone placement task targets — enables
	// ActionPlace outside a chain episode, following MineTargetBlock's
	// exact "environment poses the task" pattern. Ignored while
	// ChainStage > 0, which keeps its own existing hardcoded-crafting-table
	// behavior unchanged (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item
	// 2). Empty (the default, and the only option outside a chain episode
	// before this field existed) means no standalone placement task.
	PlaceTargetItem string

	// FlyTargetOffset is added to the bot's position at Reset to produce
	// this episode's elytra-flight target (Environment.flyTargetX/Y/Z) —
	// the flight-task analogue of TargetOffset, but a genuine 3D offset
	// (altitude included). Only the X/Z components ever drive arrival
	// (see FlyArrivalRadius): agent.FlyTo's own landing behavior ignores
	// the Y it's given — elytra flight can't hover, so it always lands on
	// the ground under the requested X/Z — and Environment's own arrival
	// check mirrors that. Ignored unless FlyTargetActive is also true.
	FlyTargetOffset [3]float64

	// FlyTargetActive enables the flight task — ActionFlyTo becomes legal
	// (once an elytra is equipped) when true, a permanent no-op when
	// false (the default). A separate explicit flag, not "FlyTargetOffset
	// != zero": a zero offset is itself a legitimate configuration (the
	// target is wherever the bot took off from) — the identical ambiguity
	// TargetOffset's own doc comment already documents and
	// GoToTargetDisabled was added to resolve. Flight starts with this
	// lesson already applied rather than repeating it
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3).
	FlyTargetActive bool

	// FlyArrivalRadius bounds how close, horizontally (X/Z only, matching
	// FlyTo's own landing behavior), the bot must land to count as
	// "arrived." <= 0 defers to FlyTo's own built-in arrival radius; see
	// DefaultConfig's own default for the exact value it mirrors.
	FlyArrivalRadius float64

	// RideVehicleType, if set, is the entity type name (e.g.
	// "minecraft:horse") this episode's mob-riding task targets — enables
	// ActionMount/ActionDismount/ActionVehicleJump, following
	// MineTargetBlock's exact "environment poses the task" pattern
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 4). Empty (the
	// default) means no riding task: all three actions become safe
	// no-ops — see action.go's resolveDispatch.
	RideVehicleType string

	// RideSearchRadius bounds FindNearestVisibleEntityOfType's search when
	// locating the nearest visible RideVehicleType instance, in blocks
	// (also the radius SeedVehicle checks before summoning one — see
	// DefaultEpisodeSeeder). Ignored if RideVehicleType is empty. <= 0
	// means unbounded. No existing chat-command default to mirror here —
	// unlike mine/pickup, "mount <entityType>" (MountNearest) has no
	// radius concept of its own at all — see DefaultConfig's own default
	// for the value chosen instead.
	RideSearchRadius int

	// CombatTargetMobType, if set, is the entity type name (e.g.
	// "minecraft:zombie") this episode's combat task targets — enables
	// ActionCombat, following MineTargetBlock's exact "environment poses
	// the task" pattern (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 5). Empty (the default) means no combat task. Bot death is
	// already handled generically regardless of this field — see
	// reward.go's computeReward (diedThisStep ends every episode with
	// deathPenalty) — Combat doesn't need, and doesn't add, any
	// special-cased death handling of its own.
	CombatTargetMobType string

	// CombatRadius bounds both FindNearestVisibleEntityOfType's search
	// when locating the nearest visible CombatTargetMobType instance
	// (also the radius SeedCombatMob checks before summoning one) and the
	// radius ActionCombat's own "runcombatwithpolicy <radius> ..."
	// dispatch passes through to agent.RunCombatWithPolicy. <= 0 defers
	// to DefaultConfig's own default, which matches actions/commands.go's
	// own runCombat default radius (16).
	CombatRadius float64

	// CombatIncludePlayers, if true, lets ActionCombat target real
	// connected players as well as hostile mobs
	// (combat.TargetPolicy.IncludePlayers) — see that type's own doc
	// comment ("Players are opt-in so callers do not accidentally enable
	// friendly fire"). Default false: a trained policy must never be able
	// to attack real players unless an episode explicitly asks for it —
	// this is a product/safety decision, not an engineering one (docs/
	// plans/RL_ACTION_SPACE_EXPANSION_III.md's own "Deferred" section,
	// closed out once the user made this call explicitly — see commit
	// history around 2026-10-24).
	CombatIncludePlayers bool

	// CombatIncludeNeutral, if true, lets ActionCombat target neutral
	// mobs (e.g. enderman, wolf) unprovoked, not just hostile ones
	// (combat.TargetPolicy.IncludeNeutral) — mirrors
	// actions/commands.go's own "runcombat [radius] [includeNeutral]"
	// parameter, which this field now also drives. Default false: no
	// safety concern on CombatIncludePlayers' level, but still an
	// explicit opt-in rather than a surprise behavior change for any
	// episode that already configures Config.CombatTargetMobType.
	CombatIncludeNeutral bool

	// CreeperGunpowderActive, if true, enables
	// ActionKillCreeperForGunpowder — a complete, already-built encounter
	// (agent.KillCreeperForGunpowder) that fights the nearest creeper with
	// whatever melee weapon is already equipped and collects a gunpowder
	// drop if one appears, following MineTargetBlock's exact "environment
	// poses the task" pattern. Unlike CombatTargetMobType, there is no mob-
	// type field: the method itself is hardcoded to creepers by design
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5). Empty/false
	// (the default) means no creeper-gunpowder task. Bot death is already
	// handled generically regardless of this field — same reasoning as
	// CombatTargetMobType's own doc comment.
	CreeperGunpowderActive bool

	// CreeperGunpowderRadius bounds FindNearestVisibleEntityOfType's
	// search for the nearest visible creeper (also the radius
	// SeedCreeperForGunpowder checks before summoning one). <= 0 defers to
	// DefaultConfig's own default — no existing chat-command default to
	// mirror (the real method's own search loop is unbounded, polling
	// until a creeper happens to be found), same reasoning as
	// RideSearchRadius's own doc comment.
	CreeperGunpowderRadius float64

	// SpearChargeTargetMobType, if set, is the entity type name this
	// episode's spear-charge task targets — enables
	// ActionSpearChargeWithApproach, following CombatTargetMobType's exact
	// "environment poses the task" pattern. Unlike CombatTargetMobType,
	// dispatch is at Environment's own already-resolved entity ID (mirrors
	// ActionMount's by-ID dispatch, not by-name — see ActionMine's own doc
	// comment on why): agent.SpearChargeAtWithApproach needs one specific
	// targetID, unlike RunCombat's own self-ranking loop. Empty (the
	// default) means no spear-charge task (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 6). Bot death is already
	// handled generically regardless of this field — same reasoning as
	// CombatTargetMobType's own doc comment.
	SpearChargeTargetMobType string

	// SpearChargeRadius bounds both FindNearestVisibleEntityOfType's
	// search when locating the nearest visible SpearChargeTargetMobType
	// instance and the radius SeedSpearMob checks before summoning one.
	// <= 0 defers to DefaultConfig's own default — no existing
	// chat-command default to mirror, same reasoning as RideSearchRadius's
	// own doc comment.
	SpearChargeRadius float64

	// MaceSmashTargetMobType, if set, is the entity type name this
	// episode's mace-smash task targets — enables
	// ActionMaceSmashWithApproach, following SpearChargeTargetMobType's
	// exact "environment poses the task" pattern, same by-ID dispatch
	// reasoning (agent.MaceSmashAtWithApproach needs one specific
	// targetID too). Empty (the default) means no mace-smash task (docs/
	// plans/RL_ACTION_SPACE_EXPANSION_III.md item 7). Bot death is
	// already handled generically regardless of this field.
	MaceSmashTargetMobType string

	// MaceSmashRadius bounds both FindNearestVisibleEntityOfType's
	// search when locating the nearest visible MaceSmashTargetMobType
	// instance and the radius SeedMaceMob/SeedElevatedPlatform check
	// before summoning/building. <= 0 defers to DefaultConfig's own
	// default, same reasoning as SpearChargeRadius's own doc comment.
	MaceSmashRadius float64

	// FollowTargetPlayerName, if set, is the player name (not an entity
	// type/block to search for — agent.Follow's own target-selection only
	// resolves players by name, following/manager.go's Start(targetPlayerName
	// string)) this episode's following task targets — enables
	// ActionFollow/ActionStopFollow, following MineTargetBlock's exact
	// "environment poses the task" pattern. Empty (the default) means no
	// following task.
	//
	// Real precondition, not solvable by this package: a second real
	// connected player (or bot) with this name must already be in the
	// session. There is no RCON way to seed "another player exists" — see
	// docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 4 for the full
	// reasoning and why this isn't worked around.
	FollowTargetPlayerName string

	// EquipTargetItem, if set, is the item name (e.g. "minecraft:elytra")
	// this episode's equip task wants held (a tool/weapon) or worn (armor/
	// elytra) — enables ActionEquip and ActionUseItem, following
	// MineTargetBlock's exact "environment poses the task" pattern
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 1). Empty (the
	// default) means this Environment instance doesn't pose an equip task:
	// both actions become safe no-ops, the same way ActionMine/ActionCraft
	// already do for an unconfigured instance — see action.go's
	// resolveDispatch. ActionUseItem is gated on this field too, not given
	// its own independent enable flag: using an item only makes sense once
	// something is equipped to use, so this one field poses both halves of
	// the task (see that plan document's own note on why ActionUseItem gets
	// no standalone reward component).
	EquipTargetItem string

	// UseItemOffhand selects which hand ActionUseItem sends "useItem" for
	// — true dispatches "useitem offhand", false (the default) dispatches
	// "useitem" (main hand). Ignored if EquipTargetItem is empty.
	UseItemOffhand bool

	// Seeder, if set, is called once per Reset to prepare the world for
	// this episode's configured task(s) — see EpisodeSeeder's doc comment
	// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4) for exactly what this
	// does and doesn't cover. nil (the default) leaves Reset's existing
	// behavior unchanged. Requires the Environment's agent to also satisfy
	// SeedAgent — Reset returns an error if Seeder is set but it doesn't.
	Seeder EpisodeSeeder

	// ResetOrigin, if set, is where Environment.Reset teleports the bot
	// (via RCON — see ResetAgent) before capturing this episode's
	// origin/target, replacing TargetOffset's own doc comment's "wherever
	// Reset found the bot" workaround with an actual reset. nil (the
	// default) leaves that workaround in place, unchanged, for callers
	// without RCON wired up (e.g. unit tests, or a chat-command-driven
	// session). Requires the Environment's agent to also satisfy
	// ResetAgent — Reset returns an error if ResetOrigin is set but it
	// doesn't.
	//
	// Found necessary, not merely nice-to-have: without a real teleport, a
	// policy that converges on a non-progressing action has no way to
	// escape that spot across episode boundaries either — every Reset
	// just re-poses the same relative target from the same stuck
	// position, so the observation the policy sees never changes and
	// neither does its behavior. Traced live to a REINFORCE gradient
	// collapse (a collaborative debugging session with cRL-go, 2026-09-08)
	// — StuckTimeout below addresses the within-episode half of that,
	// this field addresses the across-episode half.
	ResetOrigin *[3]float64

	// StuckTimeout, if > 0, ends the current episode (Step returns
	// Done=true) once this many consecutive Steps have produced a
	// bit-for-bit identical observation vector to the previous Step's — a
	// generic, task-agnostic "no progress of any kind is happening"
	// signal (position, health/food, mine/craft state all unchanged).
	// Found necessary by the same debugging session ResetOrigin's doc
	// comment references: without this, a stalled policy could run an
	// entire episode producing a batch of identical (observation, action,
	// advantage) triples whose REINFORCE gradient contributions cancel to
	// the float32 noise floor. 0 (the default) disables the check,
	// matching the previous unbounded (episode_len-only) behavior.
	StuckTimeout int

	// MaxConsecutiveStepTimeouts, if > 0, ends the current episode once that
	// many dispatched Steps in a row hit StepTimeout without their action
	// resolving. StuckTimeout can't catch a wedged bot whose position jitters
	// by float noise each step (found live: a bot straddling the edge of a
	// slot it could never descend into, re-dispatching a craft that timed out
	// every step), so its episode ran the whole step budget - about 17
	// minutes at a 10s StepTimeout - while every other environment waited on
	// it at the epoch barrier. A step whose action resolves resets the count;
	// steps that dispatch nothing leave it unchanged. 0 (the default)
	// disables the check.
	MaxConsecutiveStepTimeouts int

	// ClearAreaRadius, if > 0 (and ResetOrigin is set and the agent is an
	// AreaClearer), makes each Reset fill with air the box extending this
	// many blocks either side of ResetOrigin in X and Z, from ResetOrigin's
	// own Y (the bot's feet level, i.e. the first cell above the ground) up
	// ClearAreaHeight further blocks - leaving the ground itself untouched.
	// For flat training worlds; in real terrain it would delete trees and
	// hills. Keep (2r+1)^2 * (height+1) under 32,768 (vanilla's fill limit).
	// 0 (the default) disables it. Ignored if CloneAreaFrom is also set -
	// see its own doc comment for why that's the better choice when it's
	// available at all.
	ClearAreaRadius int
	// ClearAreaHeight is how many blocks above ResetOrigin's Y to clear;
	// 4 if left 0 while ClearAreaRadius is set.
	ClearAreaHeight int

	// CloneAreaFrom, if set (and ResetOrigin is set and the agent is an
	// AreaCloner), replaces ClearAreaRadius's "fill air above a boundary
	// computed from ResetOrigin" with "clone a known-good template region
	// onto this episode's working area" each Reset: a pristine reference
	// patch - ground layers and the air space above them - copied as one
	// rigid unit. This heals ground damage the same pass that clears
	// leftover placed-block debris, and - the actual point of it over
	// ClearAreaRadius - never computes a destroy/preserve boundary from a
	// live, possibly-imprecise position reading: CloneAreaFrom's own Y is
	// used for the destination too, unconditionally, never ResetOrigin's.
	// Found worth building live 2026-10-04: ClearAreaRadius's origin-Y
	// floor rounding (since fixed - see clearArea's history) ate through
	// an entire 3-block soil layer down to bedrock on two of ten bots over
	// several days; cloning removes that whole hazard class rather than
	// making one instance of it less likely.
	//
	// Same flat-world precondition ClearAreaRadius already documents,
	// stated more precisely here because this field's correctness depends
	// on it directly: every working area this episode's ResetOrigin could
	// ever land in must share CloneAreaFrom's own ground height exactly.
	// CloneAreaFrom's own X/Z must be a location no ResetOrigin (now or
	// after -parallel-envs scales up) will ever land a working area on or
	// near - e.g., mc-rsi-trainer's shared-server working areas
	// (pkg/parallelenv.WorkingAreaOffset) only ever move along X, so
	// anywhere on the Z axis is guaranteed untouched regardless of how
	// many environments get added later.
	CloneAreaFrom *[3]float64
	// CloneAreaRadius is CloneAreaFrom's half-width/depth in blocks (both
	// the source template and every destination working area use the same
	// radius); 0 (the default) falls back to ClearAreaRadius if that's
	// set, otherwise disables cloning.
	CloneAreaRadius int
	// CloneAreaHeight is how many blocks above CloneAreaFrom's own Y to
	// include - the air-clearing portion, same role as ClearAreaHeight; 4
	// if left 0 while CloneAreaFrom is set.
	CloneAreaHeight int
	// CloneAreaDepth is how many blocks below CloneAreaFrom's own Y to
	// include - the ground-healing portion ClearAreaRadius never had; 3
	// if left 0 while CloneAreaFrom is set (this run's own soil depth -
	// stop short of bedrock, which never changes so never needs healing).
	CloneAreaDepth int

	// TimePenalty is the reward subtracted every step, independent of outcome
	// (see reward.go's timePenalty for what it's for). 0 means the built-in
	// default, 0.01. The default is too small to matter next to a +10 success
	// bonus: a policy that wastes ten steps loses 0.1, so nothing pushes it
	// to be efficient (found live: 38-70% of all steps were the Wait action,
	// and mine_far episodes took ~10.7 steps where ~3 suffice). Must not be
	// negative.
	TimePenalty float32

	// NormalizeObservation scales every observation feature to roughly
	// [-1, 1] (see NormalizedObservation), instead of handing the policy raw
	// blocks and degrees. A network fed raw values (yaw up to +-180, distances
	// of tens of blocks) against ~0.1-magnitude weights saturates its first
	// layer immediately, which with a learning rate that isn't tiny kills the
	// ReLU units outright - one such run had every second-layer unit dead after
	// two generations and never learned again. Off by default so a policy
	// trained on the raw layout keeps working; a checkpoint trained with it on
	// must be run with it on.
	NormalizeObservation bool

	// ChainStage, if non-zero, makes this a multi-step crafting-chain
	// episode (chain.go): 1 gather a log, 2 ... craft planks and a table,
	// 3 ... place the table, 4 ... craft CraftTargetItem (a chainGoals item)
	// at the placed table, 5 (ChainMine) ... then find and mine
	// ChainMineTargetBlock. Enables ActionPlace, makes ActionCraft craft the
	// next missing link toward the stage's goal instead of CraftTargetItem
	// directly, replaces the mine and craft bonuses with milestone rewards,
	// and adds the inventory-count features to the observation. Needs
	// MineTargetBlock (the tree's log), SeedAtGoal and a Seeder whose agent
	// implements ChainSeedAgent and FarSeedAgent (the tree is seeded out of
	// sight, and the inventory and stray drops cleared each episode).
	ChainStage int

	// ChainMineTargetBlock, meaningful only at ChainStage ChainMine, is the
	// block name (e.g. "minecraft:coal_ore") the chain switches to tracking
	// once its goal item (a pickaxe) is actually crafted — see
	// Environment.mineTargetBlockInUse. Kept separate from MineTargetBlock,
	// which the gather stage keeps using for the tree's log throughout, so
	// a chain_buried_mining episode's two mine targets (log, then buried
	// block) don't collide (docs/plans/14-buried-block-mining-and-memory-map.md,
	// mc-rsi-trainer).
	ChainMineTargetBlock string

	// CollectDrops makes a mine step also collect the drops it produced, so
	// the mined item is in the inventory when the step's observation and
	// reward are computed. Breaking a block leaves an item entity on the
	// ground that the bot only collects if it happens to be within about a
	// block of where it lands (drops land up to a block and a half away, and
	// cannot be collected for half a second of server time), so without this
	// a task whose next step needs the mined item - crafting planks from a
	// log - fails at random. Off by default: existing mine tasks do not use
	// the item, and collecting costs a fraction of a second per mine step.
	// Needs an agent implementing models.ItemCollector; without one it is a
	// no-op.
	CollectDrops bool

	// SeedAtGoal is Config's copy of TaskOverride.SeedAtGoal (set per
	// episode by Config.TaskSelector; may also be set statically).
	SeedAtGoal bool

	// Jitter adds a uniform-random offset in [-Jitter[i], +Jitter[i]] to
	// axis i of both ResetOrigin (if set) and TargetOffset, drawn fresh
	// every Reset — see JitterSeed for the source. [3]float64{} (the
	// default) disables jitter entirely: every axis stays exactly as
	// configured, matching pre-Jitter behavior bit-for-bit.
	//
	// Found necessary, not merely nice-to-have, by the same debugging
	// session ResetOrigin/StuckTimeout's own doc comments reference: fixing
	// the across-episode "stuck forever" bug (ResetOrigin) and the
	// within-episode one (StuckTimeout) turned out not to be sufficient for
	// a policy to actually learn anything. A fixed ResetOrigin/TargetOffset
	// pair, once the policy converges toward a single dominant action, has
	// every episode replay the *exact* same trajectory — still an
	// exact-repeat, gradient-cancelling batch every epoch, just now bounded
	// instead of unbounded. Jitter closes that gap: even a fully
	// deterministic policy now sees a genuinely different (origin, target)
	// pair, and therefore a genuinely different observation, every episode
	// — confirmed live: without Jitter, GradientNorm stayed pinned at the
	// float32 noise floor for 1999 of 2000 epochs even with ResetOrigin and
	// StuckTimeout both configured.
	Jitter [3]float64

	// JitterSeed seeds Jitter's random source (one per Environment,
	// created in New). Zero is a perfectly valid, deterministic seed
	// (matches math/rand.NewSource(0)) — not a sentinel for "disabled";
	// Jitter's own zero value ([3]float64{}) is what disables jitter, not
	// this field. A fixed JitterSeed makes a training run's episode
	// starting conditions reproducible run to run, the same way
	// cRL-go/crlconfig's own Train.Seed does for the policy side.
	JitterSeed int64

	// GoToTargetDisabled, if true, makes ActionGoToTarget illegal this
	// episode (Environment.actionLegal, ActionMask, resolveDispatch),
	// mirroring how an empty MineTargetBlock/CraftTargetItem already makes
	// ActionMine/ActionCraft illegal. Unlike those two, ActionGoToTarget has
	// no natural "unset" representation on TargetOffset itself — TargetOffset's
	// own doc comment already establishes that its zero value ([3]float64{})
	// is a legitimate, intentional configuration ("the target is the bot's
	// own Reset position"), not a sentinel for "no goto task" — so this
	// needed its own explicit field rather than overloading TargetOffset.
	// Named so its zero value (false) preserves today's behavior exactly:
	// every existing caller that never sets this field keeps
	// ActionGoToTarget unconditionally legal, unchanged. See TaskSelector
	// below for the mechanism that actually sets this per episode.
	GoToTargetDisabled bool

	// ArriveSneaking, if true, makes ActionGoToTarget dispatch
	// "movetoandsneakquiet" instead of "movetoquiet" — the bot ends the
	// movement sneaking at the destination, instead of standing normally.
	// Purely a dispatch-time choice: arrival detection itself (Step's own
	// distance-based check) is unaffected either way, so there's no
	// separate reward for this (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
	// item 3). False (the default) preserves today's behavior exactly.
	ArriveSneaking bool

	// TaskSelector, if set, is called once per Reset (after any
	// Config.ResetOrigin teleport, before this episode's target/jitter is
	// computed) to choose which task(s) are active this specific episode,
	// promoting docs/plans/RL_TRAINING_LOOP_PLAN.md's live-verified
	// newAlternatingMineOrCraftSeeder proof-of-concept
	// (../mc-rsi-trainer/docs/plans/01-curriculum-generator.md item 4) into
	// a reusable capability: its TaskOverride return value's five fields
	// (GoToTargetDisabled, TargetOffset, MineTargetBlock, MineSearchRadius,
	// CraftTargetItem) overwrite this Config's own same-named fields for the
	// rest of that episode — every other field (ArrivalThreshold,
	// StepTimeout, Seeder, ResetOrigin, StuckTimeout, Jitter, JitterSeed,
	// TaskSelector itself) is untouched, so a TaskSelector only has to
	// describe what varies, not repeat every mechanics field it doesn't
	// care about. nil (the default) leaves Reset's existing static-Config
	// behavior completely unchanged — see Environment.Reset.
	//
	// Deliberately mutates this Environment's own Config in place at Reset
	// rather than computing a separate "effective" config alongside it: the
	// original static values passed to New are not meant to be recovered
	// once a TaskSelector starts choosing per-episode — the whole point is
	// that it fully owns task selection from that point on — and every
	// other read site in this package already reads Config's task fields
	// directly (e.g. Environment.actionLegal, refreshMineTarget), so this
	// way nothing else needed to change to pick up per-episode overrides.
	TaskSelector TaskSelector
}

// TaskSelector chooses which task(s) are active for one episode. episode
// is the 0-indexed count of Reset calls made against this Environment so
// far (0 for the very first episode); rng is this Environment's own
// jitter random source (Config.JitterSeed), shared rather than given a
// separate seed so a fixed JitterSeed still makes an entire run's episode
// conditions — task selection included — reproducible end to end. See
// Config.TaskSelector's own doc comment for exactly how the returned
// TaskOverride is applied.
type TaskSelector func(episode int, rng *rand.Rand) TaskOverride

// TaskOverride is TaskSelector's return value: the subset of Config that
// can vary per episode. Leaving a field at its zero value disables that
// task for the episode — GoToTargetDisabled: false is the one exception,
// since false already means "not disabled" (see its own doc comment on
// Config); a TaskSelector that wants GoToTarget active this episode must
// set TargetOffset to a real value AND leave GoToTargetDisabled false,
// exactly mirroring how MineTargetBlock/CraftTargetItem must be
// explicitly non-empty to enable their tasks.
type TaskOverride struct {
	// CollectDrops is Config.CollectDrops for this episode.
	CollectDrops bool

	// ChainStage is Config.ChainStage for this episode.
	ChainStage int

	// ChainMineTargetBlock is Config.ChainMineTargetBlock for this episode.
	ChainMineTargetBlock string

	// SeedAtGoal, for a composite episode (goto active alongside mine or
	// craft), places the mine block or crafting table next to the goto
	// target instead of beside the bot, so the bot has to travel to find
	// it. Ignored unless the goto task and a mine or craft task are both
	// active. See Environment.seedFarTargets.
	SeedAtGoal bool

	GoToTargetDisabled bool
	// ArriveSneaking is Config.ArriveSneaking for this episode.
	ArriveSneaking               bool
	TargetOffset                 [3]float64
	MineTargetBlock              string
	MineSearchRadius             int
	MineRequiredTool             string
	MineRequiredToolEnchantments map[string]int
	MineTargetBuried             bool
	SleepSearchRadius            int
	PortalSearchRadius           int
	LightPortalSearchRadius      int
	CraftTargetItem              string
	CraftSearchRadius            int

	// EquipTargetItem is Config.EquipTargetItem for this episode.
	EquipTargetItem string

	// PickupTargetItem is Config.PickupTargetItem for this episode.
	PickupTargetItem string
	// PickupSearchRadius is Config.PickupSearchRadius for this episode.
	PickupSearchRadius int
	// PickupAnyItem is Config.PickupAnyItem for this episode.
	PickupAnyItem bool

	// PlaceTargetItem is Config.PlaceTargetItem for this episode.
	PlaceTargetItem string

	// FlyTargetOffset is Config.FlyTargetOffset for this episode.
	FlyTargetOffset [3]float64
	// FlyTargetActive is Config.FlyTargetActive for this episode.
	FlyTargetActive bool
	// FlyArrivalRadius is Config.FlyArrivalRadius for this episode.
	FlyArrivalRadius float64

	// RideVehicleType is Config.RideVehicleType for this episode.
	RideVehicleType string
	// RideSearchRadius is Config.RideSearchRadius for this episode.
	RideSearchRadius int

	// CombatTargetMobType is Config.CombatTargetMobType for this episode.
	CombatTargetMobType string
	// CombatRadius is Config.CombatRadius for this episode.
	CombatRadius float64
	// CombatIncludePlayers is Config.CombatIncludePlayers for this episode.
	CombatIncludePlayers bool
	// CombatIncludeNeutral is Config.CombatIncludeNeutral for this episode.
	CombatIncludeNeutral bool

	// CreeperGunpowderActive is Config.CreeperGunpowderActive for this
	// episode.
	CreeperGunpowderActive bool
	// CreeperGunpowderRadius is Config.CreeperGunpowderRadius for this
	// episode.
	CreeperGunpowderRadius float64

	// SpearChargeTargetMobType is Config.SpearChargeTargetMobType for this
	// episode.
	SpearChargeTargetMobType string
	// SpearChargeRadius is Config.SpearChargeRadius for this episode.
	SpearChargeRadius float64

	// MaceSmashTargetMobType is Config.MaceSmashTargetMobType for this
	// episode.
	MaceSmashTargetMobType string
	// MaceSmashRadius is Config.MaceSmashRadius for this episode.
	MaceSmashRadius float64

	// FollowTargetPlayerName is Config.FollowTargetPlayerName for this
	// episode.
	FollowTargetPlayerName string
}

// DefaultConfig returns reasonable production defaults; TargetOffset must
// still be set by the caller (there's no sensible default for "how far
// away should the goal be"). MineTargetBlock is left empty — set it
// explicitly to opt an Environment instance into posing a mining task.
func DefaultConfig() Config {
	return Config{
		ArrivalThreshold: 1.5,
		StepTimeout:      10 * time.Second,
		// Matches actions/commands.go's own mineSearchRadius default for
		// the "mine <blockName>" chat command — kept as a separate literal
		// here since that constant is unexported.
		MineSearchRadius: 32,
		// Matches actions/commands.go's own pickUpItemDefaultMaxDistance
		// default for the "pickUpItem <itemName>" chat command — same
		// reasoning as MineSearchRadius above.
		PickupSearchRadius: 8,
		// Matches agent.FlyTo's own internal arrivalRadius constant
		// (agent/flight.go) — same reasoning as MineSearchRadius above.
		FlyArrivalRadius: 5,
		// No existing chat-command default to mirror (see
		// RideSearchRadius's own doc comment) — chosen generously for a
		// vehicle-sized target, larger than a block/item's own defaults.
		RideSearchRadius: 16,
		// Matches actions/commands.go's own runCombat default radius —
		// same reasoning as MineSearchRadius above.
		CombatRadius: 16,
		// No existing chat-command default to mirror (see
		// CreeperGunpowderRadius's own doc comment) — matches CombatRadius's
		// own default, a reasonable search distance for a hostile mob.
		CreeperGunpowderRadius: 16,
		// No existing chat-command default to mirror (see
		// SpearChargeRadius's own doc comment) — matches CombatRadius's
		// own default, same reasoning.
		SpearChargeRadius: 16,
		// No existing chat-command default to mirror (see
		// MaceSmashRadius's own doc comment) — matches CombatRadius's own
		// default, same reasoning.
		MaceSmashRadius: 16,
	}
}

func (c Config) validate() error {
	if c.ArrivalThreshold <= 0 {
		return errArrivalThreshold
	}
	if c.StepTimeout <= 0 {
		return errStepTimeout
	}
	if c.TimePenalty < 0 {
		return errTimePenalty
	}
	if c.ChainStage < 0 || c.ChainStage > ChainMine {
		return errChainStage
	}
	return nil
}
