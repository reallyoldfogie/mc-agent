package movement

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// PhysicsMode represents the operational mode of the physics executor.
type PhysicsMode int

const (
	// PhysicsModeIdle - Agent is idle, just simulate physics (gravity, forces)
	PhysicsModeIdle PhysicsMode = iota
	// PhysicsModeNavigating - Agent is following a path
	PhysicsModeNavigating
	// PhysicsModeManual - Agent is under external control (for special actions)
	PhysicsModeManual
)

// String returns the name of the physics mode.
func (pm PhysicsMode) String() string {
	switch pm {
	case PhysicsModeIdle:
		return "Idle"
	case PhysicsModeNavigating:
		return "Navigating"
	case PhysicsModeManual:
		return "Manual"
	default:
		return "Unknown"
	}
}

// StuckRecoveryFn is a callback function for stuck recovery.
// It receives the current position and the remaining goal position.
// It should attempt to re-pathfind from the current position to the goal.
// Returns the new path if recovery succeeded, nil if recovery failed.
type StuckRecoveryFn func(currentPos models.V3, goalPos models.V3) *pathfinding.Path

// PhysicsMovementExecutor implements MovementExecutor using physics simulation.
// It runs continuously at 20 TPS, always simulating physics (gravity, forces, etc.)
// and sending position updates to the server.
type PhysicsMovementExecutor struct {
	// Base executor for packet sending
	movementPacketSender *movementPacketSender

	// Physics simulation
	physicsState  models.PhysicsState
	inputGen      models.InputGenerator
	world         models.PhysicsWorld
	worldManager  models.World // For accessing world time and other state
	shapeProvider physics.BlockShapeProvider

	// Continuous operation state
	mode      PhysicsMode
	modeMu    sync.RWMutex
	running   bool
	runningMu sync.RWMutex
	isDead    atomic.Bool // True while the player is dead; suppresses position updates
	stopChan  chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc

	// Path navigation state
	currentPath *pathfinding.Path
	currentStep int
	pathMu      sync.RWMutex
	pathDone    chan struct{} // Signals when path is complete

	// Timing
	tickRate time.Duration // Time per tick (default: 50ms for 20 TPS)

	// Server correction tracking
	predictionErrors []float64 // Recent prediction errors for logging
	maxErrorHistory  int       // Max errors to track (default: 100)

	// Optional clutch handling
	clutchCallback func(physics.ClutchPlan)
	lastClutchTime time.Time
	clutchCooldown time.Duration

	// Optional telemetry recording for testing
	telemetryRecorder models.MovementTelemetryRecorder

	// Stuck detection and recovery
	stepStartTime     time.Time       // When current step started
	stepStartPos      models.V3       // Position when current step started
	lastProgressPos   models.V3       // Last position where meaningful progress was made
	lastProgressTime  time.Time       // Time of last progress
	stuckThreshold    time.Duration   // How long without progress before considered stuck
	stuckRecovery     StuckRecoveryFn // Callback for recovery when stuck
	stuckRecoveryLock sync.Mutex      // Prevent concurrent recovery attempts
	isRecovering      bool            // Whether we're currently in recovery

	// Mounted/riding state
	mountedEntityMu      sync.RWMutex
	mountedEntityID      int32 // -1 = not mounted
	entityPositionGetter models.MountedEntityPositionGetter
	versionHandler       models.VersionHandler
	recoveryAttempt      int           // Which recovery stage we're on (0=none, 1=sideways, 2=repath)
	sidewaysDirection    int           // Which sideways direction to try (-1=left, 1=right)
	sidewaysStartTime    time.Time     // When sideways recovery started
	sidewaysTimeout      time.Duration // How long to try sideways before giving up

	// Manual input state (thread-safe)
	manualInputsMu sync.RWMutex  // Protects manual inputs
	manualInputs   models.Inputs // Current manual inputs for PhysicsModeManual

	// dismountRequested is set when the agent sends a dismount (PRESS_SHIFT_KEY)
	// and cleared when the server acknowledges via SetPassengers (SetDismounted).
	// While true, every SendVehicleInput forces sneak=true to prevent the
	// server's tickRiding() from seeing isSneaking()=false due to a race
	// with updateInput() clearing the sneaking flag.
	dismountRequested bool

	// Riding velocity state (reset on mount/dismount).
	// Maintained across ticks so the server sees smooth acceleration rather
	// than an immediate jump to full speed every tick.
	ridingVelX float64
	ridingVelZ float64
	ridingVelY float64 // Y velocity for gravity accumulation (minecart off-rail, camel dash)

	// ridingGroundY records the entity's Y position the last time it was on the
	// ground. Used to detect landing after a camel dash lunge: when the entity
	// falls back to or below this Y, it has landed.
	ridingGroundY float64

	// minecartCoyoteTicks counts consecutive ticks a ridden minecart has gone
	// without a detected rail while still near the last rail height. It lets a
	// brief detection gap at a rail-segment transition (or a not-yet-loaded rail
	// block) coast on the last rail height for a few ticks instead of immediately
	// free-falling; once the grace window is exceeded the cart falls normally.
	minecartCoyoteTicks int
	// minecartLastRailGroundY is the Y a ridden minecart last rested on a rail,
	// used as the coast height during the coyote grace window above.
	minecartLastRailGroundY float64

	// lastRidingJumpState tracks the jump input from the previous tick to detect
	// jump transitions. Used for camel dash charging.
	lastRidingJumpState bool

	// sentVehicleMoves is a ring buffer of the most recent VehicleMove positions
	// we sent while controlling a vehicle. Some server versions (observed on
	// 1.21.9/1.21.10) re-broadcast the controlling passenger's own moves back as
	// RelEntityMove packets a few ticks late; applying those stale echoes as
	// corrections snaps the vehicle backwards mid-motion (worst during a camel
	// dash at ~2 blocks/tick). SyncMountedPosition drops any sync that matches a
	// position in this buffer. Guarded by sentVehicleMovesMu.
	sentVehicleMovesMu   sync.Mutex
	sentVehicleMoves     [sentVehicleMovesSize]models.V3
	sentVehicleMovesTime [sentVehicleMovesSize]time.Time
	sentVehicleMovesLen  int
	sentVehicleMovesIdx  int

	// firstVehicleMoveAt/firstVehicleMovePos record the very first VehicleMove
	// position sent after mounting, kept outside the ring buffer above (which
	// only covers ~1s of history) purely for diagnostics: if a later
	// server-side entity sync fails to match anything in the ring buffer but
	// matches this mount-time snapshot, isVehicleMoveEcho logs exactly how
	// stale that echo is, distinguishing a genuinely (if unusually) late
	// server echo from an unrelated correction. See
	// docs/PIG_MOUNT_SYNC_SNAPBACK_INVESTIGATION.md.
	firstVehicleMoveAt  time.Time
	firstVehicleMovePos models.V3

	// Horse (and other JumpingMount) charge-on-hold / release-to-fire jump state.
	// Mirrors the vanilla client (ClientPlayerEntity.tickMovement) where holding the
	// jump key accumulates charge via the MountJumpStrength ramp and releasing fires
	// the jump. The handler arms pendingJumpStrength on release; tickControlled
	// (here, the on-ground check in handleRidingModeHorse) applies the velocity on
	// the next on-ground tick, matching AbstractHorseEntity.tickControlled.
	horseCharging            bool    // true while the jump key is held and accumulating charge
	horseJumpChargeTicks     int     // ticks the jump key has been held (for the ramp)
	horsePendingJumpStrength float64 // strength armed by a release; applied on next on-ground tick

	// camelState holds the camel-specific pose, dash, and charge state.
	// Non-nil only when mounted on a camel or camel_husk; nil for other vehicles.
	camelState *models.CamelState

	// nautilusState holds the nautilus-specific eased vehicle yaw and dash state.
	// Non-nil only when mounted on a nautilus or zombie_nautilus; nil otherwise.
	nautilusState *models.NautilusState

	// onDashReady is an optional callback invoked when the camel's dash cooldown
	// reaches zero and a new dash can be initiated. Clients use this to know
	// when they can trigger a lunge/jump again.
	onDashReady func()

	// Strider/pig SaddledComponent boost state. Mirrors Java SaddledComponent:
	// boost is triggered by "using" (right-click) the warped_fungus_on_a_stick /
	// carrot_on_a_stick, providing a temporary sinusoidal speed multiplier.
	saddleBoosted    bool // Whether a boost is currently active
	saddleBoostTime  int  // Current tick within the boost
	saddleBoostTotal int  // Total boost duration in ticks

	// Boat-only: angular velocity (degrees/tick) preserved between ticks so
	// turning has momentum, mirroring vanilla AbstractBoatEntity.yawVelocity
	// (extractedSrc 1.21.10/net/minecraft/entity/vehicle/AbstractBoatEntity.java).
	boatYawVelocity float64

	// lastVelMultiplier records the per-tick drag constant used on the most
	// recent riding tick (boat or horse). Exposed via GetRidingDragMultiplier
	// so tests can read the actual value instead of guessing.
	lastVelMultiplier float64

	// onPositionUpdate is an optional callback invoked after every position
	// update sent to the server. Used by the replay mirror to record position
	// snapshots for auto-camera timeline generation.
	onPositionUpdate func(x, y, z float64, yaw, pitch float64)

	// Mount/dismount callbacks for vehicle pathfinding
	onMountRequired    func(ctx context.Context, entityID int32) error
	onDismountRequired func(ctx context.Context) error
	waitingForMount    bool
	waitingForDismount bool
}

// NewPhysicsMovementExecutor creates a new physics-based movement executor.
func NewPhysicsMovementExecutor(
	ctx context.Context,
	client bot.Client,
	packetMgr protocol_models.PacketMgr,
	getBotPos func() (models.V3, float64, float64, bool),
	setBotPos func(models.V3, float64, float64),
	getBotEntityID func() int32,
	world physics.World,
	shapeProvider physics.BlockShapeProvider,
	worldManager models.World,
) *PhysicsMovementExecutor {
	// Create movement packet sender
	movementPacketSender := &movementPacketSender{
		client:         client,
		packetMgr:      packetMgr,
		getBotPosition: getBotPos,
		setBotPosition: setBotPos,
		getBotEntityID: getBotEntityID,
		isSprinting:    false,
		isSneaking:     false,
		onPacketSent:   nil,
	}

	// Create physics state
	physicsState := physics.NewState(shapeProvider)

	// Initialize physics state from current bot position
	botPos, yaw, pitch, initialized := getBotPos()
	if initialized {
		physicsState.SetPosition(
			botPos,
			yaw,
			pitch,
			true, // Assume on ground initially
		)
	}

	childCtx, cancel := context.WithCancel(ctx)

	return &PhysicsMovementExecutor{
		movementPacketSender: movementPacketSender,
		physicsState:         physicsState,
		inputGen:             pathfinding.NewInputGenerator(),
		world:                world,
		worldManager:         worldManager,
		shapeProvider:        shapeProvider,
		mode:                 PhysicsModeIdle,
		running:              false,
		stopChan:             make(chan struct{}),
		ctx:                  childCtx,
		cancel:               cancel,
		currentPath:          nil,
		currentStep:          0,
		pathDone:             nil,
		tickRate:             50 * time.Millisecond, // 20 TPS
		predictionErrors:     make([]float64, 0, 100),
		maxErrorHistory:      100,
		clutchCooldown:       500 * time.Millisecond,
		stuckThreshold:       3 * time.Second, // Default: stuck if no progress for 3 seconds
		sidewaysTimeout:      2 * time.Second, // Try sideways recovery for 2 seconds before re-pathing
		sidewaysDirection:    1,               // Start with right
		mountedEntityID:      -1,              // Not mounted initially
	}
}

func (pe *PhysicsMovementExecutor) SetVelocity(x, y, z float64) error {
	// Directly set the velocity in the physics state
	pe.physicsState.SetVelocity(models.V3{X: x, Y: y, Z: z})
	return nil
}

// SetMounted transitions the executor to mounted/riding mode.
// The executor will then send vehicle movement packets instead of player position packets.
func (pe *PhysicsMovementExecutor) SetMounted(vehicleEntityID int32) error {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()
	pe.mountedEntityID = vehicleEntityID
	pe.ridingVelX = 0
	pe.ridingVelZ = 0
	pe.ridingVelY = 0
	pe.boatYawVelocity = 0
	pe.minecartCoyoteTicks = 0
	pe.minecartLastRailGroundY = 0

	// Seed the physics state with the entity's current tracked position (and yaw
	// for boats) so that the very first handleRidingMode tick sends a VehicleMove
	// that matches the vehicle's actual location. Without this, the first tick
	// would use the agent's pre-mount walking position, which the server would
	// reject by sending an empty SetPassengers (dismounting the agent).
	// For boats specifically, vanilla also syncs the player's yaw to the boat's
	// yaw on first mount so the initial VehicleMove carries the correct heading.
	if pe.entityPositionGetter != nil {
		if entX, entY, entZ, found := pe.entityPositionGetter.GetMountedEntityPosition(vehicleEntityID); found {
			_, yaw, pitch, _ := pe.physicsState.GetPosition()
			// Align yaw to the vehicle's facing direction on initial mount.
			// This is especially important for boats: the server validates that
			// the first VehicleMove heading matches the boat's orientation.
			if entYaw, yawFound := pe.entityPositionGetter.GetMountedEntityYaw(vehicleEntityID); yawFound {
				yaw = entYaw
			}
			pe.physicsState.SetPosition(models.V3{X: entX, Y: entY, Z: entZ}, yaw, pitch, false)
			log.Printf("[SetMounted] Physics state seeded from entity %d at (%.2f, %.2f, %.2f) yaw=%.2f", vehicleEntityID, entX, entY, entZ, yaw)

			// For minecarts, initialize velocity from the server's entity velocity.
			if et, typeFound := pe.entityPositionGetter.GetMountedEntityType(vehicleEntityID); typeFound {
				if pe.entityPositionGetter.IsMountedEntityMinecart(et) {
					// Get server velocity (encoded in protocol units, 1 unit = 1/8000 blocks/tick)
					velX, _, velZ, velFound := pe.entityPositionGetter.GetEntityVelocity(vehicleEntityID)
					if velFound {
						pe.ridingVelX = velX / 8000.0
						pe.ridingVelZ = velZ / 8000.0
						log.Printf("[SetMounted] Minecart velocity initialized from server: (%.4f, %.4f) blocks/tick", pe.ridingVelX, pe.ridingVelZ)
					}
				}
			}
		}
	}

	pe.lastRidingJumpState = false // Reset jump state on mount
	pe.clearSentVehicleMoves()     // Fresh echo-detection history for the new vehicle
	pe.saddleBoosted = false       // Reset boost state on mount
	pe.saddleBoostTime = 0
	pe.saddleBoostTotal = 0

	// Reset horse charge state on mount.
	pe.horseCharging = false
	pe.horseJumpChargeTicks = 0
	pe.horsePendingJumpStrength = 0

	// Initialize camel state if mounted on a camel/camel_husk
	pe.camelState = nil
	if pe.entityPositionGetter != nil {
		if entityTypeID, found := pe.entityPositionGetter.GetMountedEntityType(vehicleEntityID); found {
			if pe.entityPositionGetter.IsMountedEntityCamel(entityTypeID) {
				// Get actual world time from server; falls back to 0 if not yet synchronized
				worldTime := int64(0)
				if pe.worldManager != nil {
					if age, ok := pe.worldManager.GetWorldAge(); ok {
						worldTime = age
					}
				}
				isCamelHusk := false
				if huskChecker, ok := pe.entityPositionGetter.(interface{ IsMountedEntityCamelHusk(int32) bool }); ok {
					isCamelHusk = huskChecker.IsMountedEntityCamelHusk(entityTypeID)
				}
				pe.camelState = models.NewCamelState(worldTime, isCamelHusk)
				log.Printf("[SetMounted] Camel state initialized (husk=%v, worldTime=%d) for entity %d", isCamelHusk, worldTime, vehicleEntityID)
			}
		}
	}

	// Initialize nautilus state if mounted on a nautilus/zombie_nautilus. The
	// nautilus's own yaw is seeded from the tracked entity yaw (it eases toward
	// the rider's look yaw each tick), and the zombie flag selects the default
	// movement speed (1.1 vs 1.0) used when the server omits movement_speed.
	pe.nautilusState = nil
	if pe.entityPositionGetter != nil {
		if entityTypeID, found := pe.entityPositionGetter.GetMountedEntityType(vehicleEntityID); found {
			if pe.entityPositionGetter.IsMountedEntityNautilus(entityTypeID) {
				var initialYaw float64
				if entYaw, yawFound := pe.entityPositionGetter.GetMountedEntityYaw(vehicleEntityID); yawFound {
					initialYaw = entYaw
				} else {
					_, stateYaw, _, _ := pe.physicsState.GetPosition()
					initialYaw = stateYaw
				}
				isZombie := pe.entityPositionGetter.IsMountedEntityZombieNautilus(entityTypeID)
				pe.nautilusState = models.NewNautilusState(initialYaw, isZombie)
				log.Printf("[SetMounted] Nautilus state initialized (zombie=%v, yaw=%.1f) for entity %d", isZombie, initialYaw, vehicleEntityID)
			}
		}
	}

	log.Printf("[SetMounted] Agent mounted on entity %d (mode stays %s)", vehicleEntityID, pe.GetMode())
	return nil
}

// NotifyVehiclePose applies a server-reported EntityPose change to whatever
// vehicle-specific state the executor maintains. Today this only affects the
// camel pose machinery; other vehicle kinds are silently no-op.
//
// Callers (typically the agent's metadata handler) invoke this whenever the
// mounted entity emits a Pose metadata update, and once at mount time so the
// camel's CamelState reflects the actual server-side pose instead of the
// hardcoded "standing" assumption in NewCamelState.
//
// poseName is the lowercased EntityPose enum name resolved from the loaded
// poses.json registry; ordinal is the raw wire varint and is kept for logging.
func (pe *PhysicsMovementExecutor) NotifyVehiclePose(entityID int32, poseName string, ordinal int32) {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	if pe.mountedEntityID != entityID {
		return
	}
	if pe.camelState == nil {
		return
	}

	worldTime := int64(0)
	if pe.worldManager != nil {
		if age, ok := pe.worldManager.GetWorldAge(); ok {
			worldTime = age
		}
	}

	switch poseName {
	case "sitting":
		if !pe.camelState.IsSitting() {
			pe.camelState.StartSitting(worldTime)
			log.Printf("[NotifyVehiclePose] Camel %d: server reports SITTING (ord=%d), applied StartSitting", entityID, ordinal)
		}
	case "standing":
		if pe.camelState.IsSitting() {
			pe.camelState.StartStanding(worldTime)
			log.Printf("[NotifyVehiclePose] Camel %d: server reports STANDING (ord=%d), applied StartStanding", entityID, ordinal)
		}
	default:
		// Camels only ever report sitting/standing for our purposes. Other
		// poses (e.g. dying, swimming) we leave alone — Java treats those
		// as orthogonal to the sit/stand state machine.
		log.Printf("[NotifyVehiclePose] Camel %d: ignoring non-sit/stand pose %q (ord=%d)", entityID, poseName, ordinal)
	}
}

// SetDismounted transitions the executor back from mounted mode to normal movement mode.
// The executor will resume sending player position packets.
func (pe *PhysicsMovementExecutor) SetDismounted() error {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()
	pe.mountedEntityID = -1
	pe.ridingVelX = 0
	pe.ridingVelZ = 0
	pe.ridingVelY = 0
	pe.boatYawVelocity = 0
	pe.minecartCoyoteTicks = 0
	pe.minecartLastRailGroundY = 0
	pe.dismountRequested = false
	pe.lastRidingJumpState = false // Reset jump state on dismount
	pe.clearSentVehicleMoves()     // Clear echo-detection history on dismount
	pe.camelState = nil            // Clear camel state on dismount
	pe.nautilusState = nil         // Clear nautilus state on dismount
	pe.saddleBoosted = false       // Clear boost state on dismount
	pe.saddleBoostTime = 0
	pe.saddleBoostTotal = 0

	// Clear horse charge state on dismount.
	pe.horseCharging = false
	pe.horseJumpChargeTicks = 0
	pe.horsePendingJumpStrength = 0
	log.Printf("[SetDismounted] Agent dismounted from vehicle (mode stays %s)", pe.GetMode())
	return nil
}

// NotifyDismountRequested marks that a dismount has been requested but not yet
// acknowledged by the server. While set, handleRidingMode forces sneak=true in
// every SendVehicleInput to prevent the server's updateInput() from clearing
// the sneaking flag before tickRiding() can process the dismount.
func (pe *PhysicsMovementExecutor) NotifyDismountRequested() {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()
	pe.dismountRequested = true
	log.Printf("[NotifyDismountRequested] Dismount flag set — sneak will be forced until server acknowledges")
}

// SetPacketCallback sets an optional callback for packet interception.
func (pe *PhysicsMovementExecutor) SetPacketCallback(callback func(pkt interface{})) {
	pe.movementPacketSender.SetPacketCallback(callback)
}

// SetMovementHandler sets an optional version-specific movement handler.
// This forwards to the base executor for version-aware packet construction.
func (pe *PhysicsMovementExecutor) SetMovementHandler(handler models.MovementHandler) {
	pe.movementPacketSender.SetMovementHandler(handler)
}

// SetVersionHandler sets the version handler for sending version-specific packets (used for riding).
func (pe *PhysicsMovementExecutor) SetVersionHandler(handler models.VersionHandler) {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()
	pe.versionHandler = handler
}

// SetMountedEntityPositionGetter sets the interface for retrieving mounted entity positions.
func (pe *PhysicsMovementExecutor) SetMountedEntityPositionGetter(getter models.MountedEntityPositionGetter) {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()
	pe.entityPositionGetter = getter
}

// GetMountedEntityID returns the entity ID of the currently mounted vehicle, or -1 if not mounted.
func (pe *PhysicsMovementExecutor) GetMountedEntityID() int32 {
	pe.mountedEntityMu.RLock()
	defer pe.mountedEntityMu.RUnlock()
	return pe.mountedEntityID
}

// SetMountedVelocity updates the riding velocity from a server velocity packet.
// Call when ClientboundEntityVelocity is received for the mounted entity.
// velocity is in protocol units (1/8000 blocks per tick), and is converted to blocks/tick.
func (pe *PhysicsMovementExecutor) SetMountedVelocity(velX, velY, velZ float64) {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	// Only update if mounted
	if pe.mountedEntityID < 0 {
		return
	}

	// Convert from protocol units (1/8000) to blocks/tick
	pe.ridingVelX = velX / 8000.0
	pe.ridingVelZ = velZ / 8000.0

	log.Printf("[SetMountedVelocity] Server velocity for entity %d: (%.4f, %.4f) blocks/tick",
		pe.mountedEntityID, pe.ridingVelX, pe.ridingVelZ)
}

// TriggerSaddleBoost activates the SaddledComponent speed boost for striders/pigs.
// Mirrors Java SaddledComponent.boost(Random): sets a random boost duration and
// enables the sinusoidal speed multiplier. Returns false if a boost is already active.
// Called when the player "uses" (right-clicks) a warped_fungus_on_a_stick or
// carrot_on_a_stick while riding.
func (pe *PhysicsMovementExecutor) TriggerSaddleBoost() bool {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	if pe.saddleBoosted {
		return false
	}

	pe.saddleBoosted = true
	pe.saddleBoostTime = 0
	// Java: random.nextInt(841) + 140
	// We use a fixed midpoint value for deterministic behavior on the client side.
	// The actual server-side duration may differ, but the client does not receive it.
	pe.saddleBoostTotal = physics.StriderBoostMinDuration + physics.StriderBoostRandomRange/2
	log.Printf("[TriggerSaddleBoost] Boost activated: duration=%d ticks", pe.saddleBoostTotal)
	return true
}

// SetDashReadyCallback sets an optional callback invoked when the camel's dash
// cooldown reaches zero and a new dash/lunge can be initiated.
// This allows clients to know when they can trigger a jump again.
func (pe *PhysicsMovementExecutor) SetDashReadyCallback(callback func()) {
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()
	pe.onDashReady = callback
}

// SetClutchCallback sets an optional callback for clutch planning signals.
func (pe *PhysicsMovementExecutor) SetClutchCallback(callback func(plan physics.ClutchPlan)) {
	pe.clutchCallback = callback
}

// SetTelemetryRecorder sets an optional telemetry recorder for testing.
func (pe *PhysicsMovementExecutor) SetTelemetryRecorder(recorder models.MovementTelemetryRecorder) {
	pe.telemetryRecorder = recorder
}

// SetStuckRecoveryCallback sets a callback for stuck recovery.
// When the executor detects the agent is stuck (no progress for stuckThreshold),
// it will call this callback with the current position and goal position.
// The callback should attempt to re-pathfind and return the new path, or nil if recovery failed.
func (pe *PhysicsMovementExecutor) SetStuckRecoveryCallback(callback StuckRecoveryFn) {
	pe.stuckRecovery = callback
}

// SetStuckThreshold sets how long without progress before the agent is considered stuck.
func (pe *PhysicsMovementExecutor) SetStuckThreshold(threshold time.Duration) {
	pe.stuckThreshold = threshold
}

// SetMountCallbacks sets the callbacks for mount/dismount actions during vehicle pathfinding.
func (pe *PhysicsMovementExecutor) SetMountCallbacks(
	onMount func(ctx context.Context, entityID int32) error,
	onDismount func(ctx context.Context) error,
) {
	pe.onMountRequired = onMount
	pe.onDismountRequired = onDismount
}

// SendPosition sends a position update to the server.
// Also syncs the physics state to prevent the tick loop from sending stale positions.
func (pe *PhysicsMovementExecutor) SendPosition(x, y, z float64, onGround bool) error {
	// Sync physics state FIRST so tick loop doesn't send stale position
	_, currentYaw, currentPitch, _ := pe.physicsState.GetPosition()
	pe.physicsState.SetPosition(
		models.V3{X: x, Y: y, Z: z},
		currentYaw,
		currentPitch,
		onGround,
	)
	return pe.movementPacketSender.SendPosition(x, y, z, onGround)
}

// SendPositionAndRotation sends a combined position and rotation update.
// Also syncs the physics state to prevent the tick loop from sending stale positions.
func (pe *PhysicsMovementExecutor) SendPositionAndRotation(x, y, z float64, yaw, pitch float64, onGround bool) error {
	// Sync physics state FIRST so tick loop doesn't send stale position.
	// mountedEntityMu serializes this write with a riding tick's
	// read-compute-write cycle: a handler holds the lock from reading the
	// rotation through applyRidingTickState's write-back, so without it a
	// mid-tick external rotation change (e.g. TurnTowards) would be clobbered
	// by the tick's stale write-back. Released before any network I/O.
	pe.mountedEntityMu.Lock()
	mounted := pe.mountedEntityID >= 0
	pe.physicsState.SetPosition(
		models.V3{X: x, Y: y, Z: z},
		yaw,
		pitch,
		onGround,
	)
	pe.mountedEntityMu.Unlock()

	pe.syncManualRotation(yaw, pitch)

	if mounted {
		// A passenger must not send player position packets (the server ignores
		// or rejects them). Send rotation only, like a vanilla riding client;
		// the riding tick carries the new heading in its per-tick Look and
		// VehicleMove packets.
		return pe.movementPacketSender.SendRotation(yaw, pitch, onGround)
	}
	return pe.movementPacketSender.SendPositionAndRotation(x, y, z, yaw, pitch, onGround)
}

// SendRotation sends a rotation update.
// Also syncs the physics state to prevent the tick loop from sending stale rotation.
func (pe *PhysicsMovementExecutor) SendRotation(yaw, pitch float64, onGround bool) error {
	// Sync physics state FIRST so tick loop doesn't send stale rotation.
	// mountedEntityMu guards against a riding tick writing back a stale
	// rotation (see SendPositionAndRotation); released before network I/O.
	pe.mountedEntityMu.Lock()
	pos, _, _, _ := pe.physicsState.GetPosition()
	pe.physicsState.SetPosition(
		pos,
		yaw,
		pitch,
		onGround,
	)
	pe.mountedEntityMu.Unlock()

	pe.syncManualRotation(yaw, pitch)

	return pe.movementPacketSender.SendRotation(yaw, pitch, onGround)
}

// syncManualRotation mirrors an externally set rotation into the manual-mode
// look inputs. ResetManualInputs captures a concrete yaw/pitch on entering
// manual mode, so without this a later external rotation change would be
// rate-limited back toward the stale value by applyLookInputs on every
// manual walking tick.
func (pe *PhysicsMovementExecutor) syncManualRotation(yaw, pitch float64) {
	pe.manualInputsMu.Lock()
	pe.manualInputs.Yaw = yaw
	pe.manualInputs.Pitch = pitch
	pe.manualInputsMu.Unlock()
}

// MoveTowards is not used by physics executor (use ExecutePath instead).
func (pe *PhysicsMovementExecutor) MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error) {
	return 0, 0, 0, fmt.Errorf("MoveTowards not supported by physics executor, use ExecutePath instead")
}

// LookAt rotates the bot to look at target coordinates.
func (pe *PhysicsMovementExecutor) LookAt(targetX, targetY, targetZ float64, onGround bool) error {
	return pe.movementPacketSender.LookAt(targetX, targetY, targetZ, onGround)
}

// StartSprinting sends a command to start sprinting.
func (pe *PhysicsMovementExecutor) StartSprinting() error {
	return pe.movementPacketSender.StartSprinting()
}

// StopSprinting sends a command to stop sprinting.
func (pe *PhysicsMovementExecutor) StopSprinting() error {
	return pe.movementPacketSender.StopSprinting()
}

// IsSprinting returns true if the bot is currently sprinting.
func (pe *PhysicsMovementExecutor) IsSprinting() bool {
	return pe.movementPacketSender.IsSprinting()
}

// StartSneaking sends a command to start sneaking.
func (pe *PhysicsMovementExecutor) StartSneaking() error {
	return pe.movementPacketSender.StartSneaking()
}

// StopSneaking sends a command to stop sneaking.
func (pe *PhysicsMovementExecutor) StopSneaking() error {
	return pe.movementPacketSender.StopSneaking()
}

// IsSneaking returns true if the bot is currently sneaking.
func (pe *PhysicsMovementExecutor) IsSneaking() bool {
	return pe.movementPacketSender.IsSneaking()
}

// HandleServerCorrection updates physics state from server position correction.
// This should be called when receiving ClientboundPosition packets from the server.
func (pe *PhysicsMovementExecutor) HandleServerCorrection(x, y, z float64, yaw, pitch float64, onGround bool) {
	// Calculate prediction error before syncing
	currentPos, currentYaw, currentPitch, _ := pe.physicsState.GetPosition()
	deltaX := x - currentPos.X
	deltaY := y - currentPos.Y
	deltaZ := z - currentPos.Z
	deltaYaw := yaw - currentYaw
	deltaPitch := pitch - currentPitch
	predictionError := deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ

	// Track prediction error
	pe.trackPredictionError(predictionError)

	// Sync physics state with server
	pe.physicsState.SetPosition(
		models.V3{X: x, Y: y, Z: z},
		yaw,
		pitch,
		onGround,
	)

	// Log significant corrections
	if predictionError > 0.01 || math.Abs(deltaYaw) > 1.0 || math.Abs(deltaPitch) > 1.0 {
		log.Printf("[PhysicsExecutor] Server correction: pos Δ(%.3f, %.3f, %.3f) yaw Δ%.2f pitch Δ%.2f error²=%.6f",
			deltaX, deltaY, deltaZ, deltaYaw, deltaPitch, predictionError)
	}
}

// SyncWithServer is an alias for HandleServerCorrection for backward compatibility.
func (pe *PhysicsMovementExecutor) SyncWithServer(x, y, z float64, yaw, pitch float64, onGround bool) {
	pe.HandleServerCorrection(x, y, z, yaw, pitch, onGround)
}

// ridingCorrectionVelocityResetThresholdSq is the squared horizontal distance
// (blocks²) between the server's authoritative vehicle position and our
// predicted position beyond which SyncRidingPosition discards accumulated
// riding velocity. Routine corrections are a small fraction of a block; only a
// large divergence indicates our prediction is genuinely wrong. Zeroing
// velocity on every routine correction prevents slow mounts (e.g. a camel) from
// ever ramping up to terminal speed, leaving them effectively stuck a fraction
// of a block short of where they were headed. 0.25 = 0.5² comfortably preserves
// per-tick corrections while still resetting on a genuine desync.
const ridingCorrectionVelocityResetThresholdSq = 0.25

// ridingCorrectionVerticalResetThreshold is the vertical distance (blocks)
// between the server's authoritative vehicle height and our predicted height
// beyond which a riding correction discards accumulated vertical velocity.
// A large vertical gap means our height prediction is wrong (e.g. we free-fell
// off a ledge while the server rests the vehicle on a block); keeping the stale
// downward velocity would make the next tick dive again and thrash against the
// correction. Routine per-tick syncs are well under this threshold.
const ridingCorrectionVerticalResetThreshold = 0.5

// sentVehicleMovesSize is how many recent VehicleMove positions are kept for
// echo detection — one second of history at 20 TPS, comfortably more than the
// few-tick latency of observed server echoes.
const sentVehicleMovesSize = 20

// vehicleMoveEchoEpsilon is the per-axis tolerance when matching an incoming
// mounted-entity sync against recently sent VehicleMove positions.
// RelEntityMove deltas are quantized to 1/4096 blocks and the entity tracker
// accumulates them, so an echo can drift slightly from the exact doubles we
// sent. A genuine server correction this close to a position we just sent
// would be inconsequential to apply anyway.
const vehicleMoveEchoEpsilon = 0.05

// recordSentVehicleMove remembers a VehicleMove position we sent so later
// server echoes of it can be recognized and ignored.
func (pe *PhysicsMovementExecutor) recordSentVehicleMove(pos models.V3) {
	pe.sentVehicleMovesMu.Lock()
	defer pe.sentVehicleMovesMu.Unlock()
	now := time.Now()
	if pe.firstVehicleMoveAt.IsZero() {
		pe.firstVehicleMoveAt = now
		pe.firstVehicleMovePos = pos
	}
	pe.sentVehicleMoves[pe.sentVehicleMovesIdx] = pos
	pe.sentVehicleMovesTime[pe.sentVehicleMovesIdx] = now
	pe.sentVehicleMovesIdx = (pe.sentVehicleMovesIdx + 1) % sentVehicleMovesSize
	if pe.sentVehicleMovesLen < sentVehicleMovesSize {
		pe.sentVehicleMovesLen++
	}
}

// isVehicleMoveEcho reports whether an incoming mounted-entity position matches
// a VehicleMove we recently sent (i.e. the server relaying our own movement
// back to us rather than issuing a correction).
func (pe *PhysicsMovementExecutor) isVehicleMoveEcho(x, y, z float64) bool {
	pe.sentVehicleMovesMu.Lock()
	defer pe.sentVehicleMovesMu.Unlock()
	for i := 0; i < pe.sentVehicleMovesLen; i++ {
		sent := pe.sentVehicleMoves[i]
		if math.Abs(x-sent.X) <= vehicleMoveEchoEpsilon &&
			math.Abs(y-sent.Y) <= vehicleMoveEchoEpsilon &&
			math.Abs(z-sent.Z) <= vehicleMoveEchoEpsilon {
			log.Printf("[isVehicleMoveEcho] Matched ring-buffer entry age=%s pos=(%.2f,%.2f,%.2f)",
				time.Since(pe.sentVehicleMovesTime[i]), sent.X, sent.Y, sent.Z)
			return true
		}
	}
	// Diagnostic: no match in the ~1s ring buffer. Check whether this is a
	// late echo of the very first VehicleMove we sent after mounting (the
	// mount-time snapshot never expires from this check, unlike the ring
	// buffer) so we can measure exactly how stale a non-matching packet is.
	// See docs/PIG_MOUNT_SYNC_SNAPBACK_INVESTIGATION.md next step #1.
	if !pe.firstVehicleMoveAt.IsZero() &&
		math.Abs(x-pe.firstVehicleMovePos.X) <= vehicleMoveEchoEpsilon &&
		math.Abs(y-pe.firstVehicleMovePos.Y) <= vehicleMoveEchoEpsilon &&
		math.Abs(z-pe.firstVehicleMovePos.Z) <= vehicleMoveEchoEpsilon {
		log.Printf("[isVehicleMoveEcho] NOT recognized as echo, but matches mount-time snapshot from %s ago (pos=(%.2f,%.2f,%.2f)); ring buffer holds %d entries spanning %s",
			time.Since(pe.firstVehicleMoveAt), pe.firstVehicleMovePos.X, pe.firstVehicleMovePos.Y, pe.firstVehicleMovePos.Z,
			pe.sentVehicleMovesLen, pe.sentVehicleMovesHistorySpanLocked())
	} else if pe.sentVehicleMovesLen > 0 {
		log.Printf("[isVehicleMoveEcho] NOT recognized as echo: pos=(%.2f,%.2f,%.2f), ring buffer holds %d entries spanning %s (oldest=%s ago, newest=%s ago)",
			x, y, z, pe.sentVehicleMovesLen, pe.sentVehicleMovesHistorySpanLocked(),
			time.Since(pe.oldestSentVehicleMoveTimeLocked()), time.Since(pe.newestSentVehicleMoveTimeLocked()))
	}
	return false
}

// oldestSentVehicleMoveTimeLocked returns the timestamp of the oldest entry
// currently in the ring buffer. Callers must hold sentVehicleMovesMu.
func (pe *PhysicsMovementExecutor) oldestSentVehicleMoveTimeLocked() time.Time {
	if pe.sentVehicleMovesLen == 0 {
		return time.Time{}
	}
	oldestIdx := pe.sentVehicleMovesIdx
	if pe.sentVehicleMovesLen == sentVehicleMovesSize {
		// Buffer is full; the slot the next write will land on holds the oldest entry.
		return pe.sentVehicleMovesTime[oldestIdx]
	}
	// Buffer not yet full; the oldest entry is at index 0.
	return pe.sentVehicleMovesTime[0]
}

// newestSentVehicleMoveTimeLocked returns the timestamp of the most recently
// recorded entry. Callers must hold sentVehicleMovesMu.
func (pe *PhysicsMovementExecutor) newestSentVehicleMoveTimeLocked() time.Time {
	if pe.sentVehicleMovesLen == 0 {
		return time.Time{}
	}
	newestIdx := (pe.sentVehicleMovesIdx - 1 + sentVehicleMovesSize) % sentVehicleMovesSize
	return pe.sentVehicleMovesTime[newestIdx]
}

// sentVehicleMovesHistorySpanLocked reports how much wall-clock time the
// current ring-buffer contents cover. Callers must hold sentVehicleMovesMu.
func (pe *PhysicsMovementExecutor) sentVehicleMovesHistorySpanLocked() time.Duration {
	if pe.sentVehicleMovesLen == 0 {
		return 0
	}
	return pe.newestSentVehicleMoveTimeLocked().Sub(pe.oldestSentVehicleMoveTimeLocked())
}

// clearSentVehicleMoves empties the echo-detection history (on mount/dismount).
func (pe *PhysicsMovementExecutor) clearSentVehicleMoves() {
	pe.sentVehicleMovesMu.Lock()
	defer pe.sentVehicleMovesMu.Unlock()
	pe.sentVehicleMovesLen = 0
	pe.sentVehicleMovesIdx = 0
	pe.firstVehicleMoveAt = time.Time{}
	pe.firstVehicleMovePos = models.V3{}
}

// SyncRidingPosition applies a server-authoritative vehicle position correction
// while the executor is in riding mode. The corrected position is always
// synced, but accumulated horizontal riding velocity is preserved across
// routine corrections — momentum is physical and independent of the position
// fix, and if our prediction really is heading into a wall the next tick's
// collision resolution will clamp it. Velocity (and boat yaw velocity) is only
// discarded on a large divergence (see ridingCorrectionVelocityResetThresholdSq),
// which indicates the prediction is genuinely wrong (e.g. a teleport).
// Should be called when a ClientboundMoveVehicle packet is received.
func (pe *PhysicsMovementExecutor) SyncRidingPosition(x, y, z float64, yaw, pitch float64) {
	currentPos, currentYaw, currentPitch, _ := pe.physicsState.GetPosition()

	// Horizontal divergence between the server's authoritative position and our
	// prediction. Only a large gap should cost us our accumulated momentum.
	deltaX := x - currentPos.X
	deltaZ := z - currentPos.Z
	horizontalDivergenceSq := deltaX*deltaX + deltaZ*deltaZ

	pe.mountedEntityMu.Lock()
	if horizontalDivergenceSq > ridingCorrectionVelocityResetThresholdSq {
		pe.ridingVelX = 0
		pe.ridingVelZ = 0
		pe.boatYawVelocity = 0
	}
	// A large vertical divergence means our height prediction disagrees with the
	// server (e.g. we free-fell while it rests the vehicle on a block). Discard
	// the stale vertical velocity and rail coyote state so the next tick settles
	// at the corrected height instead of immediately re-applying the fall.
	if math.Abs(y-currentPos.Y) > ridingCorrectionVerticalResetThreshold {
		pe.ridingVelY = 0
		pe.minecartCoyoteTicks = 0
		pe.minecartLastRailGroundY = y
	}
	pe.mountedEntityMu.Unlock()

	// Use corrected yaw/pitch if non-zero, otherwise preserve current rotation
	newYaw := currentYaw
	newPitch := currentPitch
	if yaw != 0 || pitch != 0 {
		newYaw = yaw
		newPitch = pitch
	}

	pos := models.V3{X: x, Y: y, Z: z}
	pe.physicsState.SetPosition(
		pos,
		newYaw,
		newPitch,
		false,
	)
	pe.movementPacketSender.setBotPosition(pos, newYaw, newPitch)
	log.Printf("[SyncRidingPosition] Vehicle position corrected by server to %s yaw=%.2f (divergence²=%.4f)", pos, newYaw, horizontalDivergenceSq)
}

// SyncMountedPosition applies a server-authoritative mounted entity position correction.
// Called when entity movement packets (RelEntityMove, MoveEntityPosRot) arrive for the mounted entity.
// Unlike SyncRidingPosition, this preserves riding velocity (it's a passive sync, not a full reset).
// The position comes from the entity tracker which aggregates server move packets.
func (pe *PhysicsMovementExecutor) SyncMountedPosition(x, y, z float64) {
	// Ignore stale echoes of our own VehicleMove packets (see sentVehicleMoves).
	if pe.isVehicleMoveEcho(x, y, z) {
		log.Printf("[SyncMountedPosition] Ignoring echo of our own vehicle move: (%.2f, %.2f, %.2f)", x, y, z)
		return
	}

	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	// Only sync if mounted
	if pe.mountedEntityID < 0 {
		return
	}

	pos := models.V3{X: x, Y: y, Z: z}
	currentPos, currentYaw, currentPitch, _ := pe.physicsState.GetPosition()

	// If the server's authoritative height diverges sharply from our prediction,
	// discard the stale vertical riding velocity (and rail coyote state) so the
	// next tick settles at the corrected height instead of re-applying the
	// accumulated fall and thrashing against the server. This keeps a cart that
	// has run off the end of a track from oscillating between the server's
	// resting height and the client's free-fall.
	if math.Abs(y-currentPos.Y) > ridingCorrectionVerticalResetThreshold {
		pe.ridingVelY = 0
		pe.minecartCoyoteTicks = 0
		pe.minecartLastRailGroundY = y
	}

	pe.physicsState.SetPosition(
		pos,
		currentYaw,
		currentPitch,
		false,
	)

	pe.movementPacketSender.setBotPosition(pos, currentYaw, currentPitch)
	log.Printf("[SyncMountedPosition] Mounted entity %d position synced to %s", pe.mountedEntityID, pos)
}

// SyncMountedPositionWithRotation applies a server-authoritative mounted entity position and rotation correction.
// Called when entity movement packets with rotation (MoveEntityPosRot) arrive for the mounted entity.
func (pe *PhysicsMovementExecutor) SyncMountedPositionWithRotation(x, y, z, yaw, pitch float64) {
	// Ignore stale echoes of our own VehicleMove packets (see sentVehicleMoves).
	if pe.isVehicleMoveEcho(x, y, z) {
		log.Printf("[SyncMountedPositionWithRotation] Ignoring echo of our own vehicle move: (%.2f, %.2f, %.2f)", x, y, z)
		return
	}

	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	// Only sync if mounted
	if pe.mountedEntityID < 0 {
		return
	}

	pos := models.V3{X: x, Y: y, Z: z}
	pe.physicsState.SetPosition(
		pos,
		yaw,
		pitch,
		false,
	)
	pe.movementPacketSender.setBotPosition(pos, yaw, pitch)
	log.Printf("[SyncMountedPositionWithRotation] Mounted entity %d position synced to %s yaw=%.1f° pitch=%.1f°", pe.mountedEntityID, pos, yaw, pitch)
}

// NotifyDead pauses position updates to the server.
// Call when the player dies to prevent sending invalid falling positions that corrupt playerdata.
func (pe *PhysicsMovementExecutor) NotifyDead() {
	pe.isDead.Store(true)
}

// NotifyRespawned resumes position updates to the server.
// Call when the server confirms the respawn position via onClientboundPosition.
func (pe *PhysicsMovementExecutor) NotifyRespawned() {
	pe.isDead.Store(false)
}

// trackPredictionError records a prediction error for monitoring.
func (pe *PhysicsMovementExecutor) trackPredictionError(error float64) {
	if len(pe.predictionErrors) >= pe.maxErrorHistory {
		// Remove oldest error
		pe.predictionErrors = pe.predictionErrors[1:]
	}
	pe.predictionErrors = append(pe.predictionErrors, error)
}

// GetAveragePredictionError returns the average squared prediction error.
func (pe *PhysicsMovementExecutor) GetAveragePredictionError() float64 {
	if len(pe.predictionErrors) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, err := range pe.predictionErrors {
		sum += err
	}
	return sum / float64(len(pe.predictionErrors))
}

// ExecutePath executes a complete path using the continuous physics system.
// The executor must be started with Start() before calling this method.
// Deprecated: Use ExecutePathWithContext instead to pass a parent context.
func (pe *PhysicsMovementExecutor) ExecutePath(path *pathfinding.Path) error {
	return pe.ExecutePathWithContext(context.Background(), path)
}

// ExecutePathWithContext executes a complete path using the continuous physics system,
// respecting the parent context's deadline. This allows callers with time constraints
// to interrupt path execution.
func (pe *PhysicsMovementExecutor) ExecutePathWithContext(ctx context.Context, path *pathfinding.Path) error {
	if err := pe.SetPath(path); err != nil {
		return err
	}

	timeout := time.Duration(len(path.Steps)) * 30 * time.Second

	// Wait for path completion, respecting the parent context's deadline.
	// If the parent context has a tighter deadline than our calculated timeout,
	// context.WithTimeout will use the earlier deadline.
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	completed, err := pe.WaitForPathCompletion(execCtx, 0)
	if !completed && err == nil {
		// Timeout occurred but no error - treat as success for backward compatibility
		return nil
	}
	return err
}

// GetCurrentPosition returns the current physics state position.
func (pe *PhysicsMovementExecutor) GetCurrentPosition() (x, y, z float64) {
	pos, _, _, _ := pe.physicsState.GetPosition()
	return pos.X, pos.Y, pos.Z
}

// GetPhysicsState returns the internal physics state for advanced usage.
func (pe *PhysicsMovementExecutor) GetPhysicsState() models.PhysicsState {
	return pe.physicsState
}

// Start begins the continuous physics simulation loop.
// This should be called once during agent initialization.
func (pe *PhysicsMovementExecutor) Start() {
	pe.runningMu.Lock()
	if pe.running {
		pe.runningMu.Unlock()
		log.Printf("[PhysicsExecutor] Already running, ignoring Start()")
		return
	}
	pe.running = true
	pe.runningMu.Unlock()

	log.Printf("[PhysicsExecutor] Starting continuous physics loop at 20 TPS")

	go pe.continuousTickLoop()
}

// Stop halts the continuous physics simulation loop.
// This should be called during agent shutdown.
func (pe *PhysicsMovementExecutor) Stop() {
	pe.runningMu.Lock()
	if !pe.running {
		pe.runningMu.Unlock()
		log.Printf("[PhysicsExecutor] Not running, ignoring Stop()")
		return
	}
	pe.running = false
	pe.runningMu.Unlock()

	log.Printf("[PhysicsExecutor] Stopping continuous physics loop")

	// Signal stop and cancel context
	close(pe.stopChan)
	pe.cancel()
}

// GetMode returns the current operational mode.
func (pe *PhysicsMovementExecutor) GetMode() PhysicsMode {
	pe.modeMu.RLock()
	defer pe.modeMu.RUnlock()
	return pe.mode
}

// SetMode sets the operational mode.
func (pe *PhysicsMovementExecutor) SetMode(mode PhysicsMode) {
	pe.modeMu.Lock()
	defer pe.modeMu.Unlock()

	if pe.mode != mode {
		log.Printf("[PhysicsExecutor] Mode changed: %s → %s", pe.mode, mode)
		pe.mode = mode
	}
}

// EnterManualMode switches executor to manual input control mode.
// Manual mode allows frame-by-frame control via SetManual* methods.
// If already in manual mode, this is a no-op.
// Preserves current sprint and sneak state from the previous mode.
func (pe *PhysicsMovementExecutor) EnterManualMode() error {
	pe.modeMu.Lock()
	defer pe.modeMu.Unlock()

	if pe.mode == PhysicsModeManual {
		// Already in manual mode - no-op
		return nil
	}

	oldMode := pe.mode
	pe.mode = PhysicsModeManual
	log.Printf("[PhysicsExecutor] Mode changed: %s → Manual", oldMode)

	// Initialize manual inputs with current state
	// Preserve sprint/sneak state from movementPacketSender, get rotation from physics state
	_, yaw, pitch, _ := pe.physicsState.GetPosition()

	pe.manualInputsMu.Lock()
	pe.manualInputs = models.Inputs{
		ThrottleX:      0.0,
		ThrottleZ:      0.0,
		Yaw:            yaw,
		Pitch:          pitch,
		Jump:           false,
		Sprint:         pe.movementPacketSender.IsSprinting(),
		Sneak:          pe.movementPacketSender.IsSneaking(),
		ClimbDirection: 0.0,
	}
	pe.manualInputsMu.Unlock()

	return nil
}

// ExitManualMode switches executor to idle mode from manual mode.
// If not in manual mode, this is a no-op.
// Preserves current sprint and sneak state to maintain agent stability.
func (pe *PhysicsMovementExecutor) ExitManualMode() error {
	pe.modeMu.Lock()
	defer pe.modeMu.Unlock()

	if pe.mode != PhysicsModeManual {
		// Not in manual mode - no-op
		return nil
	}

	pe.mode = PhysicsModeIdle
	log.Printf("[PhysicsExecutor] Mode changed: Manual → Idle")

	// Apply final manual inputs state to movementPacketSender to preserve sprint/sneak state
	pe.manualInputsMu.RLock()
	shouldSprint := pe.manualInputs.Sprint
	shouldSneak := pe.manualInputs.Sneak
	pe.manualInputsMu.RUnlock()

	// Apply sprint state
	if shouldSprint && !pe.movementPacketSender.IsSprinting() {
		if err := pe.StartSprinting(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to start sprinting on exit: %v", err)
		}
	} else if !shouldSprint && pe.movementPacketSender.IsSprinting() {
		if err := pe.StopSprinting(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to stop sprinting on exit: %v", err)
		}
	}

	// Apply sneak state
	if shouldSneak && !pe.movementPacketSender.IsSneaking() {
		if err := pe.StartSneaking(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to start sneaking on exit: %v", err)
		}
	} else if !shouldSneak && pe.movementPacketSender.IsSneaking() {
		if err := pe.StopSneaking(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to stop sneaking on exit: %v", err)
		}
	}

	// Clear manual inputs
	pe.manualInputsMu.Lock()
	pe.manualInputs = models.Inputs{}
	pe.manualInputsMu.Unlock()

	return nil
}

// IsManualMode returns whether currently in manual mode.
func (pe *PhysicsMovementExecutor) IsManualMode() bool {
	pe.modeMu.RLock()
	defer pe.modeMu.RUnlock()
	return pe.mode == PhysicsModeManual
}

// SetManualInputs sets all manual inputs at once.
// Works in manual mode and while mounted (for vehicle control).
func (pe *PhysicsMovementExecutor) SetManualInputs(inputs models.Inputs) error {
	pe.manualInputsMu.Lock()
	pe.manualInputs = inputs
	pe.manualInputsMu.Unlock()

	return nil
}

// GetManualInputs returns current manual inputs.
func (pe *PhysicsMovementExecutor) GetManualInputs() models.Inputs {
	pe.manualInputsMu.RLock()
	defer pe.manualInputsMu.RUnlock()
	return pe.manualInputs
}

// SetManualThrottle sets the movement direction for next tick.
// westEastThrottle: X-axis movement (-1.0 to +1.0, negative=west, positive=east)
// northSouthThrottle: Z-axis movement (-1.0 to +1.0, negative=north, positive=south)
// Works in manual mode and while mounted (for vehicle steering).
func (pe *PhysicsMovementExecutor) SetManualThrottle(westEastThrottle, northSouthThrottle float64) error {
	// Clamp values to [-1.0, 1.0]
	westEastThrottle = math.Max(-1.0, math.Min(1.0, westEastThrottle))
	northSouthThrottle = math.Max(-1.0, math.Min(1.0, northSouthThrottle))

	pe.manualInputsMu.Lock()
	pe.manualInputs.ThrottleX = westEastThrottle
	pe.manualInputs.ThrottleZ = northSouthThrottle
	pe.manualInputsMu.Unlock()

	log.Printf("[PhysicsExecutor] Manual throttle set: X=%.2f Z=%.2f", westEastThrottle, northSouthThrottle)

	return nil
}

// SetManualRotation sets yaw and pitch for looking direction.
// yaw: horizontal look direction (degrees, 0=south, 90=west, 180=north, 270=east)
// pitch: vertical look direction (degrees, -90=up, 0=forward, 90=down)
// Pass math.NaN() for yaw or pitch to keep current value.
// Only effective in manual walking mode: riding handlers derive their heading
// from the physics state, not from these inputs. To steer while mounted, use
// SendRotation/SendPositionAndRotation (e.g. via the agent's TurnTowards),
// which write the rotation into the physics state under mountedEntityMu.
func (pe *PhysicsMovementExecutor) SetManualRotation(yaw, pitch float64) error {
	pe.manualInputsMu.Lock()
	// Only update yaw if not NaN (allows "don't change" semantics)
	if !math.IsNaN(yaw) {
		pe.manualInputs.Yaw = yaw
	}
	// Only update pitch if not NaN
	if !math.IsNaN(pitch) {
		pe.manualInputs.Pitch = pitch
	}
	pe.manualInputsMu.Unlock()

	log.Printf("[PhysicsExecutor] Manual rotation set: yaw=%.2f pitch=%.2f", yaw, pitch)

	return nil
}

// SetManualJump sets whether jump button is pressed.
// Works in manual mode and while mounted.
func (pe *PhysicsMovementExecutor) SetManualJump(enabled bool) error {
	pe.manualInputsMu.Lock()
	pe.manualInputs.Jump = enabled
	pe.manualInputsMu.Unlock()

	return nil
}

// SetManualSprint sets whether sprint button is pressed.
// Works in manual mode and while mounted.
func (pe *PhysicsMovementExecutor) SetManualSprint(enabled bool) error {
	pe.manualInputsMu.Lock()
	pe.manualInputs.Sprint = enabled
	pe.manualInputsMu.Unlock()

	return nil
}

// SetManualSneak sets whether sneak button is pressed.
// Works in manual mode and while mounted.
func (pe *PhysicsMovementExecutor) SetManualSneak(enabled bool) error {
	pe.manualInputsMu.Lock()
	pe.manualInputs.Sneak = enabled
	pe.manualInputsMu.Unlock()

	return nil
}

// SetManualClimbDirection sets the ladder/vine climb direction.
// direction: +1.0=climb up, -1.0=climb down, 0.0=no climb
// Clamps to valid range [-1.0, 1.0].
// Works in manual mode (ladder/vine climbing).
func (pe *PhysicsMovementExecutor) SetManualClimbDirection(direction float64) error {
	// Clamp to valid range
	direction = math.Max(-1.0, math.Min(1.0, direction))

	pe.manualInputsMu.Lock()
	pe.manualInputs.ClimbDirection = direction
	pe.manualInputsMu.Unlock()

	return nil
}

// ResetManualInputs clears all manual inputs to zero.
// Sets yaw/pitch to current position state to preserve look direction.
// Works in manual mode and while mounted.
func (pe *PhysicsMovementExecutor) ResetManualInputs() error {
	// Get current rotation from physics state
	_, yaw, pitch, _ := pe.physicsState.GetPosition()

	pe.manualInputsMu.Lock()
	pe.manualInputs = models.Inputs{
		ThrottleX:      0.0,
		ThrottleZ:      0.0,
		Yaw:            yaw,
		Pitch:          pitch,
		Jump:           false,
		Sprint:         false,
		Sneak:          false,
		ClimbDirection: 0.0,
	}
	pe.manualInputsMu.Unlock()

	return nil
}

// continuousTickLoop is the main physics simulation loop that runs at 20 TPS.
func (pe *PhysicsMovementExecutor) continuousTickLoop() {
	ticker := time.NewTicker(pe.tickRate)
	defer ticker.Stop()

	log.Printf("[PhysicsExecutor] Continuous tick loop started")

	for {
		select {
		case <-pe.stopChan:
			log.Printf("[PhysicsExecutor] Continuous tick loop stopped")
			return

		case <-pe.ctx.Done():
			log.Printf("[PhysicsExecutor] Continuous tick loop stopped (context cancelled)")
			return

		case <-ticker.C:
			pe.tick()
		}
	}
}

// tick performs one physics simulation tick.
// Riding (mounted) state is orthogonal to mode: mode generates inputs, then
// the mounted flag decides whether those inputs drive vehicle packets or
// walking physics.
func (pe *PhysicsMovementExecutor) tick() {
	// Check mounted state (orthogonal to mode)
	pe.mountedEntityMu.RLock()
	isMounted := pe.mountedEntityID >= 0
	pe.mountedEntityMu.RUnlock()

	// Get current mode
	pe.modeMu.RLock()
	mode := pe.mode
	pe.modeMu.RUnlock()

	// (A) Generate inputs based on mode — identical whether mounted or not
	var inputs physics.Inputs
	switch mode {
	case PhysicsModeIdle:
		inputs = pe.generateIdleInputs()
	case PhysicsModeNavigating:
		inputs = pe.generateNavigationInputs()
	case PhysicsModeManual:
		inputs = pe.generateManualInputs()
	default:
		inputs = physics.Inputs{} // Zero inputs
	}

	if isMounted {
		// (B) Riding: translate mode-generated inputs into vehicle packets
		log.Printf("[PhysicsExecutor][tick] Tick inputs (mounted): mode=%s throttle=(%.2f, %.2f) yaw=%.2f pitch=%.2f jump=%t sneak=%t",
			mode, inputs.ThrottleX, inputs.ThrottleZ, inputs.Yaw, inputs.Pitch, inputs.Jump, inputs.Sneak)
		pe.handleRidingTick(inputs)
	} else {
		// (C) Walking: run physics sim + send player position
		log.Printf("[PhysicsExecutor][tick] Tick inputs: mode=%s throttle=(%.2f, %.2f) yaw=%.2f pitch=%.2f jump=%t sprint=%t sneak=%t climbDir=%.2f",
			mode, inputs.ThrottleX, inputs.ThrottleZ, inputs.Yaw, inputs.Pitch, inputs.Jump, inputs.Sprint, inputs.Sneak, inputs.ClimbDirection)

		pe.applyMovementState(inputs)

		if err := pe.physicsState.Tick(inputs, pe.world); err != nil {
			log.Printf("[PhysicsExecutor] Physics tick error: %v", err)
			return
		}

		pe.recordTelemetry(inputs)
		pe.checkClutch()
		pe.sendPositionUpdate()
	}
}

// generateIdleInputs generates inputs for idle mode (just physics, no movement).
func (pe *PhysicsMovementExecutor) generateIdleInputs() physics.Inputs {
	// Get current position for pitch
	pos, _, pitch, _ := pe.physicsState.GetPosition()

	// Preserve explicit sneak state from StartSneaking() calls (e.g., from stabilizeSneaking)
	// This is different from the navigation mode feedback loop issue - in idle mode,
	// we WANT to preserve explicit sneak commands so the agent holds position.
	shouldSneak := pe.IsSneaking()

	// Also auto-sneak if on a climbable block (ladder/vine) to hold position
	if !shouldSneak {
		blockAtPlayer, _ := pe.world.GetBlockStatus(
			int(pos.X),
			int(pos.Y),
			int(pos.Z),
		)
		if pe.shapeProvider.IsClimbable(blockAtPlayer) {
			shouldSneak = true
		}
	}

	// Return zero throttle - agent just stands still and is affected by physics
	return physics.Inputs{
		ThrottleX: 0.0,
		ThrottleZ: 0.0,
		Yaw:       math.NaN(), // Don't change yaw
		Pitch:     pitch,      // Keep current pitch
		Jump:      false,
		Sprint:    false,
		Sneak:     shouldSneak,
	}
}

// generateNavigationInputs generates inputs for navigation mode (path following).
func (pe *PhysicsMovementExecutor) generateNavigationInputs() physics.Inputs {
	pe.pathMu.RLock()

	// Check if we have a path
	if pe.currentPath == nil || pe.currentStep >= len(pe.currentPath.Steps) {
		pe.pathMu.RUnlock()
		// Path complete, switch to idle
		pe.modeMu.Lock()
		oldMode := pe.mode
		pe.mode = PhysicsModeIdle
		pe.modeMu.Unlock()

		if oldMode != PhysicsModeIdle {
			log.Printf("[PhysicsExecutor] Mode changed: %s → Idle (path complete)", oldMode)
		}

		// Signal path completion
		if pe.pathDone != nil {
			select {
			case pe.pathDone <- struct{}{}:
			default: // Don't block if already signaled
			}
		}

		return pe.generateIdleInputs()
	}

	// Get current step and goal
	step := pe.currentPath.Steps[pe.currentStep]
	stepNum := pe.currentStep
	totalSteps := len(pe.currentPath.Steps)
	goalPos := pe.currentPath.GoalPos
	pe.pathMu.RUnlock()

	pos, _, _, _ := pe.physicsState.GetPosition()
	currentPos := models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}

	// Check if current step is complete
	// For JumpToClimb, also verify the agent actually grabbed the climbable
	// (not just passed through the completion zone while falling)
	isComplete := pathfinding.IsComplete(pos, step)
	if isComplete && step.Movement == pathfinding.JumpToClimb {
		vel := pe.physicsState.GetVelocity()
		// If Y velocity is significantly negative (falling), don't mark complete
		// Agent must be stable or moving upward to have grabbed the climbable
		if vel.Y < -0.1 {
			isComplete = false
			log.Printf("[PhysicsExecutor] JumpToClimb completion deferred: Y velocity %.3f < -0.1 (falling)", vel.Y)
		}
	}

	if isComplete {
		// Log step completion with agent name
		agentName := "<unknown>"
		if pe.movementPacketSender != nil && pe.movementPacketSender.client != nil {
			agentName = pe.movementPacketSender.client.Name()
		}
		log.Printf("[PhysicsExecutor %s] Step %d/%d complete: %s to (%.0f, %.0f, %.0f)",
			agentName, stepNum+1, totalSteps, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)

		// Record step completion in telemetry
		if pe.telemetryRecorder != nil {
			pe.telemetryRecorder.RecordStep(step.Movement.String())
		}

		// Advance to next step - reset progress tracking
		pe.pathMu.Lock()
		pe.currentStep++
		pe.stepStartTime = time.Now()
		pe.stepStartPos = models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
		pe.lastProgressPos = pe.stepStartPos
		pe.lastProgressTime = time.Now()

		// Check if path is complete
		if pe.currentStep >= len(pe.currentPath.Steps) {
			pe.pathMu.Unlock()
			pe.modeMu.Lock()
			pe.mode = PhysicsModeIdle
			pe.modeMu.Unlock()

			agentName := "<unknown>"
			if pe.movementPacketSender != nil && pe.movementPacketSender.client != nil {
				agentName = pe.movementPacketSender.client.Name()
			}
			log.Printf("[PhysicsExecutor %s] Path complete!", agentName)

			// Signal path completion
			if pe.pathDone != nil {
				select {
				case pe.pathDone <- struct{}{}:
				default:
				}
			}

			return pe.generateIdleInputs()
		}

		step = pe.currentPath.Steps[pe.currentStep]
		pe.pathMu.Unlock()
	} else {
		// Check for progress - are we getting closer to the target?
		distToTarget := currentPos.DistanceTo(step.Position)
		lastDistToTarget := models.V3{X: pe.lastProgressPos.X, Y: pe.lastProgressPos.Y, Z: pe.lastProgressPos.Z}.DistanceTo(step.Position)

		// If we've made meaningful progress (moved at least 0.1 blocks closer), update tracking and reset recovery
		if lastDistToTarget-distToTarget > 0.1 {
			pe.lastProgressPos = models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
			pe.lastProgressTime = time.Now()
			// Progress made - reset recovery state
			if pe.recoveryAttempt > 0 {
				log.Printf("[PhysicsExecutor] Progress made during recovery - resuming normal navigation")
				pe.recoveryAttempt = 0
			}
		}

		// Check for stuck condition
		if pe.stuckThreshold > 0 && time.Since(pe.lastProgressTime) > pe.stuckThreshold {
			// TODO: Add forward jump up attempt as part of recovery (make sure the agent is facing the direction of travel, so we don't jump off un-necessarily).
			// TODO: If we fall back to pathfinding, we need to not completely abandon the current path, maybe?
			//       path following validation can fail when we replace the current path because we loose the
			//       telemetry (i.e. if jumps are required, and we jumped but got stuck, and the new path doesn't
			//       have/need jumps, the telemetry check fails)

			// Are we already in sideways recovery?
			switch pe.recoveryAttempt {
			case 1:
				// Check if sideways recovery has timed out
				if time.Since(pe.sidewaysStartTime) > pe.sidewaysTimeout {
					log.Printf("[PhysicsExecutor] Sideways recovery timed out after %v - attempting re-pathfind",
						pe.sidewaysTimeout)
					pe.recoveryAttempt = 2
					pe.attemptRepathRecovery(currentPos, goalPos)
				}
				// Still in sideways recovery - generate sideways inputs below
			case 0:
				// First stuck detection
				log.Printf("[PhysicsExecutor] STUCK DETECTED at step %d/%d: no progress for %v (pos: %.2f, %.2f, %.2f -> target: %.0f, %.0f, %.0f)",
					stepNum+1, totalSteps, pe.stuckThreshold,
					pos.X, pos.Y, pos.Z, step.Position.X, step.Position.Y, step.Position.Z)

				// For vertical movements (climbing, swimming up, stairs), skip sideways recovery
				// Sideways movement on these surfaces causes the agent to fall off
				// THIS SHOULD BE IMPROVED TO CHECK FOR THE ABILITY TO MOVE SIDEWAYS SAFELY, RATHER THAN JUST SKIPPING IT.
				skipSideways := step.Movement == pathfinding.Climb ||
					step.Movement == pathfinding.SwimUp ||
					step.Movement == pathfinding.AscendStairs ||
					step.Movement == pathfinding.DescendStairs ||
					step.Movement == pathfinding.Descend ||
					step.Movement == pathfinding.AscendJump ||
					step.Movement == pathfinding.DiagonalAscend
				if skipSideways {
					log.Printf("[PhysicsExecutor] %s movement - skipping sideways recovery, going straight to re-pathfind", step.Movement)
					pe.recoveryAttempt = 2
					pe.attemptRepathRecovery(currentPos, goalPos)
				} else {
					// Start sideways recovery for non-climbing movements
					log.Printf("[PhysicsExecutor] Starting sideways recovery (attempt 1)")
					pe.recoveryAttempt = 1
					pe.sidewaysStartTime = time.Now()
					// Alternate sideways direction each time we get stuck
					pe.sidewaysDirection = -pe.sidewaysDirection
				}
			}
			// recoveryAttempt == 2 means we already tried re-pathing, don't keep retrying
		}
	}

	// Handle vehicle mounting/dismounting
	switch step.Movement {
	case pathfinding.MountVehicle:
		return pe.handleMountStep(step)
	case pathfinding.DismountVehicle:
		return pe.handleDismountStep(step)
	}

	// Generate inputs - either normal navigation or sideways recovery
	if pe.recoveryAttempt == 1 {
		return pe.generateSidewaysRecoveryInputs(step)
	}

	// Normal navigation or post-repath
	// Let the input generator be authoritative on sneaking state.
	// The previous OR with pe.IsSneaking() created a feedback loop where
	// sneaking could never be disabled once enabled (e.g., from sideways recovery).
	// This caused agents to get stuck on ladders since sneaking prevents descent.
	inputs := pe.inputGen.GenerateInputs(pe.physicsState, step, 0)
	if inputs.Jump {
		_, _, _, onGround := pe.physicsState.GetPosition()
		if !onGround {
			log.Printf("[PhysicsExecutor] Jump input while not onGround at step %d/%d (%s) pos=(%.2f, %.2f, %.2f) target=(%.2f, %.2f, %.2f)",
				stepNum+1, totalSteps, step.Movement,
				pos.X, pos.Y, pos.Z, step.Position.X, step.Position.Y, step.Position.Z)
		}
	}
	return inputs
}

// generateSidewaysRecoveryInputs generates inputs to move sideways (perpendicular to movement direction)
// to try to clear an obstacle and resume the current path.
// It intelligently chooses which side to move toward based on which direction brings us closer to the target.
func (pe *PhysicsMovementExecutor) generateSidewaysRecoveryInputs(step pathfinding.PathStep) physics.Inputs {
	pos, _, pitch, _ := pe.physicsState.GetPosition()

	// Calculate direction to target
	deltaX := step.Position.X - pos.X
	deltaZ := step.Position.Z - pos.Z
	dist := math.Sqrt(deltaX*deltaX + deltaZ*deltaZ)

	if dist < 0.01 {
		// Very close to target, use normal inputs
		return pe.inputGen.GenerateInputs(pe.physicsState, step, 0)
	}

	// Normalize direction to target
	dirX := deltaX / dist
	dirZ := deltaZ / dist

	// Calculate both perpendicular directions (sideways)
	// Rotate 90 degrees: (x, z) -> (-z, x) for right, (z, -x) for left
	rightX := -dirZ
	rightZ := dirX
	leftX := dirZ
	leftZ := -dirX

	// Use random offset between 0.1 and 0.5 to break ties when rightDist == leftDist
	offset := 0.1 + rand.Float64()*0.4

	// Calculate potential positions after moving sideways
	rightPosX := pos.X + rightX*offset
	rightPosZ := pos.Z + rightZ*offset
	leftPosX := pos.X + leftX*offset
	leftPosZ := pos.Z + leftZ*offset

	// Check if sideways blocks have ground - don't move toward air
	// This prevents falling off narrow paths during recovery
	rightBlockX := int(math.Floor(pos.X + rightX))
	rightBlockZ := int(math.Floor(pos.Z + rightZ))
	leftBlockX := int(math.Floor(pos.X + leftX))
	leftBlockZ := int(math.Floor(pos.Z + leftZ))

	rightHasGround := pe.hasSolidBlockAt(rightBlockX, int(pos.Y)-1, rightBlockZ)
	leftHasGround := pe.hasSolidBlockAt(leftBlockX, int(pos.Y)-1, leftBlockZ)

	// Current block bounds (with safety margin)
	currentBlockMinX := math.Floor(pos.X) + 0.3
	currentBlockMaxX := math.Floor(pos.X) + 0.7
	currentBlockMinZ := math.Floor(pos.Z) + 0.3
	currentBlockMaxZ := math.Floor(pos.Z) + 0.7

	// If sideways direction leads to air, clamp movement to stay within current block
	if !rightHasGround {
		rightPosX = math.Max(currentBlockMinX, math.Min(currentBlockMaxX, rightPosX))
		rightPosZ = math.Max(currentBlockMinZ, math.Min(currentBlockMaxZ, rightPosZ))
	}
	if !leftHasGround {
		leftPosX = math.Max(currentBlockMinX, math.Min(currentBlockMaxX, leftPosX))
		leftPosZ = math.Max(currentBlockMinZ, math.Min(currentBlockMaxZ, leftPosZ))
	}

	// Check which sideways direction brings us closer to the target
	rightDistToTarget := math.Sqrt((step.Position.X-rightPosX)*(step.Position.X-rightPosX) +
		(step.Position.Z-rightPosZ)*(step.Position.Z-rightPosZ))
	leftDistToTarget := math.Sqrt((step.Position.X-leftPosX)*(step.Position.X-leftPosX) +
		(step.Position.Z-leftPosZ)*(step.Position.Z-leftPosZ))

	// Choose the direction that gets us closer to the target
	// Prefer direction with ground; if both or neither have ground, use distance
	var sidewaysX, sidewaysZ float64
	var chooseRight bool
	var useSneak bool
	if !rightHasGround || !leftHasGround {
		useSneak = true
	}

	if rightHasGround && !leftHasGround {
		chooseRight = true
	} else if leftHasGround && !rightHasGround {
		chooseRight = false
	} else {
		// Both have ground or both don't - use distance comparison
		chooseRight = rightDistToTarget < leftDistToTarget
	}

	if chooseRight {
		sidewaysX = rightX
		sidewaysZ = rightZ
		log.Printf("[PhysicsExecutor] Sideways recovery: moving RIGHT toward target (right dist=%.2f, left dist=%.2f, rightGround=%v, leftGround=%v)",
			rightDistToTarget, leftDistToTarget, rightHasGround, leftHasGround)
	} else {
		sidewaysX = leftX
		sidewaysZ = leftZ
		log.Printf("[PhysicsExecutor] Sideways recovery: moving LEFT toward target (right dist=%.2f, left dist=%.2f, rightGround=%v, leftGround=%v)",
			rightDistToTarget, leftDistToTarget, rightHasGround, leftHasGround)
	}

	// Combine: move mostly sideways with minimal forward momentum
	// Reduced forward momentum (0.1) to prevent falling off during recovery
	throttleX := sidewaysX*0.9 + dirX*0.1
	throttleZ := sidewaysZ*0.9 + dirZ*0.1

	// Normalize throttle
	throttleMag := math.Sqrt(throttleX*throttleX + throttleZ*throttleZ)
	if throttleMag > 0.01 {
		throttleX /= throttleMag
		throttleZ /= throttleMag
	}

	// Calculate yaw to face the combined direction
	// NOTE: Trajectory is simulated in local space with Z=forward, X=0
	// Yaw formula must convert from world delta to firing direction
	// atan2(-dx, dz) accounts for the coordinate system rotation
	yaw := math.Atan2(-throttleX, throttleZ) * 180.0 / math.Pi

	return physics.Inputs{
		ThrottleX: throttleX,
		ThrottleZ: throttleZ,
		Yaw:       yaw,
		Pitch:     pitch,
		Jump:      false, // Could add jump for step-up situations
		Sprint:    false,
		Sneak:     useSneak, // Only sneak for edge safety, don't preserve previous state
	}
}

// hasSolidBlockAt checks if there's a solid block at the given position.
func (pe *PhysicsMovementExecutor) hasSolidBlockAt(x, y, z int) bool {
	stateID, exists := pe.world.GetBlockStatus(x, y, z)
	if !exists {
		return false // Unknown block treated as no ground
	}
	return pe.shapeProvider.IsSolid(stateID)
}

// attemptRepathRecovery attempts to recover from a stuck situation by re-pathfinding.
// This is called after sideways recovery has failed.
func (pe *PhysicsMovementExecutor) attemptRepathRecovery(currentPos, goalPos models.V3) {
	pe.stuckRecoveryLock.Lock()
	if pe.isRecovering {
		pe.stuckRecoveryLock.Unlock()
		return
	}
	pe.isRecovering = true
	pe.stuckRecoveryLock.Unlock()

	defer func() {
		pe.stuckRecoveryLock.Lock()
		pe.isRecovering = false
		pe.stuckRecoveryLock.Unlock()
	}()

	if pe.stuckRecovery == nil {
		log.Printf("[PhysicsExecutor] Re-path recovery skipped: no recovery callback set")
		pe.lastProgressTime = time.Now()
		pe.recoveryAttempt = 0 // Reset so we can try again later
		return
	}

	// Log the step we are stuck on (if any) for debugging.
	pe.pathMu.RLock()
	stepDesc := "<none>"
	stepIdx := pe.currentStep
	if pe.currentPath != nil && pe.currentStep < len(pe.currentPath.Steps) {
		stepDesc = pe.currentPath.Steps[pe.currentStep].Movement.String()
	}
	pe.pathMu.RUnlock()

	log.Printf("[PhysicsExecutor] Repath recovery (attempt %d) at step %d (%s): pos=(%.2f, %.2f, %.2f) goal=(%.2f, %.2f, %.2f)",
		pe.recoveryAttempt, stepIdx, stepDesc,
		currentPos.X, currentPos.Y, currentPos.Z, goalPos.X, goalPos.Y, goalPos.Z)

	log.Printf("[PhysicsExecutor] Attempting re-path recovery from (%.2f, %.2f, %.2f) to goal (%.0f, %.0f, %.0f)",
		currentPos.X, currentPos.Y, currentPos.Z, goalPos.X, goalPos.Y, goalPos.Z)

	// Call the recovery callback to re-pathfind
	newPath := pe.stuckRecovery(currentPos, goalPos)
	if newPath == nil || !newPath.Found || len(newPath.Steps) == 0 {
		log.Printf("[PhysicsExecutor] Re-path recovery failed: no valid path found from current position")
		// Reset progress tracking to avoid immediate re-trigger
		pe.lastProgressTime = time.Now()
		pe.recoveryAttempt = 0 // Reset so we can try sideways again later
		return
	}

	log.Printf("[PhysicsExecutor] Re-path recovery succeeded: new path with %d steps", len(newPath.Steps))

	// Set the new path and reset recovery state
	pe.pathMu.Lock()
	pe.currentPath = newPath
	pe.currentStep = 0
	pe.stepStartTime = time.Now()
	pos, _, _, _ := pe.physicsState.GetPosition()
	pe.stepStartPos = models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
	pe.lastProgressPos = pe.stepStartPos
	pe.lastProgressTime = time.Now()
	pe.recoveryAttempt = 0 // Reset recovery state after successful re-path
	pe.pathMu.Unlock()

	log.Printf("[PhysicsExecutor] New path set:")
	for i, step := range newPath.Steps {
		log.Printf("[PhysicsExecutor]   Step %d: %s to (%.0f, %.0f, %.0f)",
			i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
	}
}

// generateManualInputs generates inputs for manual mode (external control).
func (pe *PhysicsMovementExecutor) generateManualInputs() physics.Inputs {
	pe.manualInputsMu.RLock()
	inputs := pe.manualInputs
	pe.manualInputsMu.RUnlock()

	// If yaw is NaN, preserve current yaw from physics state
	if math.IsNaN(inputs.Yaw) {
		_, yaw, _, _ := pe.physicsState.GetPosition()
		inputs.Yaw = yaw
	}

	// If pitch is NaN, preserve current pitch from physics state
	if math.IsNaN(inputs.Pitch) {
		_, _, pitch, _ := pe.physicsState.GetPosition()
		inputs.Pitch = pitch
	}

	// Sanitize NaN yaw/pitch that may have come from physics state
	// This can happen in rare cases where physics state is corrupted or uninitialized
	if math.IsNaN(inputs.Yaw) {
		log.Printf("[PhysicsExecutor][generateManualInputs] WARNING: yaw is NaN, defaulting to 0. This may indicate a physics state initialization issue.")
		inputs.Yaw = 0
	}
	if math.IsNaN(inputs.Pitch) {
		log.Printf("[PhysicsExecutor][generateManualInputs] WARNING: pitch is NaN, defaulting to 0. This may indicate a physics state initialization issue.")
		inputs.Pitch = 0
	}

	log.Printf("[PhysicsExecutor][generateManualInputs] Manual inputs: throttleX=%.2f throttleZ=%.2f yaw=%.2f pitch=%.2f jump=%v sprint=%v sneak=%v climbDir=%.2f",
		inputs.ThrottleX, inputs.ThrottleZ, inputs.Yaw, inputs.Pitch,
		inputs.Jump, inputs.Sprint, inputs.Sneak, inputs.ClimbDirection)

	return inputs
}

// handleMountStep handles a MountVehicle step - triggers the mount action and waits for completion.
func (pe *PhysicsMovementExecutor) handleMountStep(step pathfinding.PathStep) physics.Inputs {
	// If not yet waiting, trigger the mount action
	if !pe.waitingForMount {
		pe.waitingForMount = true
		if pe.onMountRequired != nil {
			go func() {
				err := pe.onMountRequired(pe.ctx, step.VehicleEntityID)
				if err != nil {
					log.Printf("[PhysicsExecutor] Mount failed: %v", err)
				}
			}()
		}
		log.Printf("[PhysicsExecutor] Mounting vehicle (entityID=%d)...", step.VehicleEntityID)
		return pe.generateIdleInputs() // No movement while mounting
	}

	// Check if mount is complete
	pe.mountedEntityMu.RLock()
	isMounted := pe.mountedEntityID == step.VehicleEntityID
	pe.mountedEntityMu.RUnlock()

	if isMounted {
		// Mount complete, advance to next step
		pe.waitingForMount = false
		pe.pathMu.Lock()
		pe.currentStep++
		pe.stepStartTime = time.Now()
		pos, _, _, _ := pe.physicsState.GetPosition()
		pe.stepStartPos = models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
		pe.lastProgressPos = pe.stepStartPos
		pe.lastProgressTime = time.Now()
		pe.pathMu.Unlock()

		log.Printf("[PhysicsExecutor] Mount complete, advancing to next step")
		return pe.generateIdleInputs()
	}

	// Still waiting for mount
	return pe.generateIdleInputs()
}

// handleDismountStep handles a DismountVehicle step - triggers the dismount action and waits for completion.
func (pe *PhysicsMovementExecutor) handleDismountStep(step pathfinding.PathStep) physics.Inputs {
	// If not yet waiting, trigger the dismount action
	if !pe.waitingForDismount {
		pe.waitingForDismount = true
		if pe.onDismountRequired != nil {
			go func() {
				err := pe.onDismountRequired(pe.ctx)
				if err != nil {
					log.Printf("[PhysicsExecutor] Dismount failed: %v", err)
				}
			}()
		}
		log.Printf("[PhysicsExecutor] Dismounting vehicle...")
		return pe.generateIdleInputs() // No movement while dismounting
	}

	// Check if dismount is complete
	pe.mountedEntityMu.RLock()
	isDismounted := pe.mountedEntityID == -1
	pe.mountedEntityMu.RUnlock()

	if isDismounted {
		// Dismount complete, advance to next step
		pe.waitingForDismount = false
		pe.pathMu.Lock()
		pe.currentStep++
		pe.stepStartTime = time.Now()
		pos, _, _, _ := pe.physicsState.GetPosition()
		pe.stepStartPos = models.V3{X: pos.X, Y: pos.Y, Z: pos.Z}
		pe.lastProgressPos = pe.stepStartPos
		pe.lastProgressTime = time.Now()
		pe.pathMu.Unlock()

		log.Printf("[PhysicsExecutor] Dismount complete, advancing to next step")
		return pe.generateIdleInputs()
	}

	// Still waiting for dismount
	return pe.generateIdleInputs()
}

// handleRidingTick handles one tick while the agent is mounted on a vehicle.
// It receives mode-generated inputs from tick() and translates them into
// vehicle control packets. Riding is orthogonal to PhysicsMode — the mode
// determines how inputs are generated (idle=zero, manual=user, navigating=path)
// and this function consumes them to drive the vehicle.
//
// Vanilla protocol flow (per ClientPlayerEntity.tick in extractedSrc 1.21.10/
// net/minecraft/client/network/ClientPlayerEntity.java:209-235):
//
//	if (this.hasVehicle()) {
//	  this.networkHandler.sendPacket(new PlayerMoveC2SPacket.LookAndOnGround(
//	      this.getYaw(), this.getPitch(), ...));
//	  Entity entity = this.getRootVehicle();
//	  if (entity != this && entity.isLogicalSideForUpdatingMovement()) {
//	      this.networkHandler.sendPacket(VehicleMoveC2SPacket.fromVehicle(entity));
//	      this.sendSprintingPacket();
//	  }
//	}
//
// For boats, left/right (ThrottleX) rotates the boat via angular velocity
// matching vanilla AbstractBoatEntity.yawVelocity, and forward/backward
// (ThrottleZ) thrusts along the boat's current heading.
//
// For horses/camels/donkeys, ThrottleX rotates the entity's yaw by
// horseTurnDegsPerTick per tick, and ThrottleZ thrusts forward.
//
// For pigs and striders, the entity's yaw equals the player's yaw directly
// (Java setRotation); the agent steers by changing its own look direction
// via TurnTowards. ThrottleZ>0 (forward=true) applies full forward
// acceleration; ThrottleX is not used for yaw in these entities.
func (pe *PhysicsMovementExecutor) handleRidingTick(inputs models.Inputs) {
	// Get mounted entity ID and version handler
	pe.mountedEntityMu.RLock()
	mountedEntityID := pe.mountedEntityID
	versionHandler := pe.versionHandler
	pe.mountedEntityMu.RUnlock()

	if versionHandler == nil {
		log.Printf("[handleRidingTick] Version handler not set")
		return
	}

	if pe.entityPositionGetter == nil {
		log.Printf("[handleRidingTick] Entity position getter not set, cannot send vehicle updates")
		return
	}

	// Verify the mounted entity is still tracked.
	_, _, _, found := pe.entityPositionGetter.GetMountedEntityPosition(mountedEntityID)
	if !found {
		log.Printf("[handleRidingTick] Mounted entity %d not found", mountedEntityID)
		return
	}

	// Determine vehicle type to dispatch to the appropriate handler.
	var isBoat, isMinecart, isCamel, isNautilus, isPig, isStrider, isDonkey, isMule bool
	if et, found := pe.entityPositionGetter.GetMountedEntityType(mountedEntityID); found {
		isBoat = pe.entityPositionGetter.IsMountedEntityBoat(et)
		isMinecart = pe.entityPositionGetter.IsMountedEntityMinecart(et)
		isCamel = pe.entityPositionGetter.IsMountedEntityCamel(et)
		isNautilus = pe.entityPositionGetter.IsMountedEntityNautilus(et)
		isPig = pe.entityPositionGetter.IsMountedEntityPig(et)
		isStrider = pe.entityPositionGetter.IsMountedEntityStrider(et)
		isDonkey = pe.entityPositionGetter.IsMountedEntityDonkey(et)
		isMule = pe.entityPositionGetter.IsMountedEntityMule(et)
	}

	// Use any non-zero throttle to determine direction.
	forward := inputs.ThrottleZ > 0.01
	backward := inputs.ThrottleZ < -0.01
	right := inputs.ThrottleX > 0.01
	left := inputs.ThrottleX < -0.01
	jump := inputs.Jump
	sneak := inputs.Sneak

	// If a dismount has been requested, force sneak=true in every packet
	// until the server acknowledges.
	pe.mountedEntityMu.RLock()
	if pe.dismountRequested {
		sneak = true
	}
	pe.mountedEntityMu.RUnlock()

	log.Printf("[handleRidingTick] Vehicle input: forward=%v backward=%v left=%v right=%v jump=%v sneak=%v (throttle: X=%.2f Z=%.2f)",
		forward, backward, left, right, jump, sneak, inputs.ThrottleX, inputs.ThrottleZ)

	// Vanilla ClientPlayerEntity.tick() always sends a LookAndOnGround packet
	// before VehicleMove while riding.
	_, currentYaw, currentPitch, _ := pe.physicsState.GetPosition()
	if err := versionHandler.Play().Movement().SendRotation(
		pe.movementPacketSender.client.Conn(),
		currentYaw, currentPitch,
		false,
	); err != nil {
		log.Printf("[handleRidingTick] Failed to send player look packet: %v", err)
	}

	// Dispatch to vehicle-type-specific handler.
	// Each handler is in its own file (riding_*.go) for isolation.
	// All handlers return a ridingTickResult so that syncRidingPhysicsState
	// can be called once here, preventing future handlers from accidentally
	// skipping the sync.
	var result ridingTickResult
	switch {
	case isBoat:
		result = pe.handleRidingModeBoat(versionHandler, forward, backward, left, right, sneak)
	case isMinecart:
		result = pe.handleRidingModeMinecart(versionHandler, forward, backward, sneak)
	case isCamel:
		result = pe.handleRidingModeCamel(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	case isNautilus:
		result = pe.handleRidingModeNautilus(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	case isPig:
		result = pe.handleRidingModePig(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	case isStrider:
		result = pe.handleRidingModeStrider(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	case isDonkey:
		result = pe.handleRidingModeDonkey(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	case isMule:
		result = pe.handleRidingModeMule(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	default:
		// Horse and any unknown rideable entity
		result = pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, pe.entityPositionGetter)
	}

	// Publish the computed result (physics state + VehicleMove packet) here,
	// AFTER the handler has returned and released mountedEntityMu, so the
	// executor never holds the lock across a network send.
	sendRidingMove(pe, versionHandler, result)

	// Sync physics state fields that would normally be maintained by Tick()
	// but are skipped during riding because handlers bypass Tick().
	syncRidingPhysicsState(pe, result.NewPos, result.OnGround, result.Sneak)
}

// ── Riding handler methods are in separate files: ──
// riding_boat.go      — handleRidingModeBoat + boat helpers
// riding_minecart.go  — handleRidingModeMinecart + rail helpers
// riding_camel.go     — handleRidingModeCamel (camel-specific physics)
// riding_horse.go     — handleRidingModeHorse (horse/donkey/mule/fallback)
// riding_pig.go       — handleRidingModePig (stub)
// riding_strider.go   — handleRidingModeStrider (stub)
// riding_nautilus.go  — handleRidingModeNautilus (stub)
// riding_common.go    — shared helpers (sendRidingMove, water physics, etc.)

// GetRidingVelocity returns the executor's current horizontal riding velocity
// (blocks/tick). Returns (0, 0) when not mounted.
func (pe *PhysicsMovementExecutor) GetRidingVelocity() (float64, float64) {
	pe.mountedEntityMu.RLock()
	defer pe.mountedEntityMu.RUnlock()
	return pe.ridingVelX, pe.ridingVelZ
}

// GetRidingDragMultiplier returns the per-tick velocity multiplier used on
// the most recent riding tick.
func (pe *PhysicsMovementExecutor) GetRidingDragMultiplier() float64 {
	pe.mountedEntityMu.RLock()
	defer pe.mountedEntityMu.RUnlock()
	return pe.lastVelMultiplier
}

// GetCamelState returns the current camel state if mounted on a camel,
// or (nil, false) if not mounted or mounted on a different vehicle type.
func (pe *PhysicsMovementExecutor) GetCamelState() (*models.CamelState, bool) {
	pe.mountedEntityMu.RLock()
	defer pe.mountedEntityMu.RUnlock()
	if pe.camelState == nil {
		return nil, false
	}
	return pe.camelState, true
}
