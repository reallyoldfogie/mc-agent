# Architecture Reference

Full per-package file map and dependency detail. For the short "what repo do I touch" version,
see `CLAUDE.md`'s Related Repositories section.

## External Dependencies

The project relies on several local packages with `replace` directives in `go.mod`:

- **`github.com/reallyoldfogie/mc-bot-go`** - Bot framework
  - Provides `bot.Client`, `basic.Player`, `msg.Manager`, `playerlist.PlayerList`, `screen.Manager`, `world.World`
  - Forked/customized from Tnze/go-mc

- **`github.com/reallyoldfogie/mc-protocol-go`** - Version-specific protocol management
  - Provides `BlockMgr`, `SoundMgr`, `PacketMgr` for different Minecraft versions
  - Models for packet structures and logging

- **`github.com/reallyoldfogie/mc-data-gen/loader`** (v0.0.3) - Block collision data
  - Version-specific block collision and shape data
  - Block properties: passable, solid, climbable, water, lava, slab, stair, etc.
  - Authoritative collision data extracted directly from Minecraft's internal systems

- **`github.com/reallyoldfogie/mc-replay-go`** (v0.0.3) - Replay recording
  - ReplayMod .mcpr file generation
  - Packet recording and skin embedding

- **`github.com/reallyoldfogie/mc-client-test-go`** - Integration testing framework
  - RCON helpers for world manipulation
  - Test framework subsystems

Key external libraries:
- `github.com/Tnze/go-mc` v1.20.3 - Base Minecraft protocol library
- `github.com/maxsupermanhd/go-mc-ms-auth` - Microsoft authentication
- `github.com/stretchr/testify` - Testing assertions
- `github.com/pkg/errors` - Error handling
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation

## Core Components (agent package)

The agent package (`agent/`) is the primary implementation with an event-driven architecture:

**Lifecycle Management:**
- `agent.go` - Agent lifecycle, initialization, and main loop
- `config.go` - Configuration structure with all options
- `stop_file.go` - Graceful shutdown via file watcher

**Position & Entity Tracking:**
- `tracking.go` - Bot position and entity tracking
- `handlers.go` - Packet handlers for entity movement, teleportation

**Chat & Commands:**
- `commands.go` - Chat command parsing and dispatch
- `command_actions.go` - Command execution
- `game_chat.go` - Game event handlers (death, disconnect, health changes)

**Movement & Navigation:**
- Integrates with `pathfinding/` for HPA* pathfinding
- Integrates with `movement/` for physics-based execution
- Integrates with `following/` for player following

**Containers & Inventory:**
- `containers.go` - Container interaction (chests, furnaces, crafting tables)
- `containerEvents.go` - Container event handling
- `screen.go` - GUI/screen management
- `hotbar.go` - Hotbar slot management

**Recipes:**
- `recipes.go` and related files - Crafting recipe system

**Replay & Skins:**
- `replay_mirror.go` - ReplayMod recording support
- `skin_manager.go`, `skins.go` - Player skin fetching and caching

**Plan Execution:**
- `plan_runner.go` - Multi-step plan execution framework
- `plan/` subdirectory - Plan step definitions

## Pathfinding System (HPA*)

The `pathfinding/` package implements Hierarchical Pathfinding A* (HPA*):

**Core Files:**
- `hpa_pathfinder.go` - Main HPA* pathfinder implementation
- `hpa_builder.go` - Graph construction
- `hpa_graph.go` - Abstract graph structure
- `hpa_cluster.go` - Cluster management (divides world into regions)
- `hpa_entrance_grouping.go` - Entrance/exit detection between clusters
- `hpa_updates.go` - Dynamic graph updates as world changes

**Supporting Files:**
- `a_star.go` - Standard A* for intra-cluster pathfinding
- `epea_star.go` - Enhanced Partial Expansion A* variant
- `movement.go` - Movement validation (walkable, jumpable, etc.)
- `blocks.go` - Block collision detection
- `collision_check.go` - AABB collision checking
- `input_generator.go` - Movement input generation for path execution
- `config.go` - Pathfinding configuration
- `types.go` - Path types and node definitions

See `HPA_USAGE_EXAMPLE.md` for usage.

## Physics Engine

The `physics/` package provides Minecraft physics simulation:

- `state.go` - Physics state (position, velocity, acceleration)
- `constants.go` - Physics constants (gravity, friction, etc.)
- `rotation.go` - Yaw/pitch calculations for looking at targets
- `projectile.go` - Projectile physics (arrows, etc.)
- `fall_damage.go` - Fall damage calculation
- `clutch.go` - Clutch mechanics (fast block placement during falls)

## Movement Execution

The `movement/` package handles movement command execution:

- `executor.go` - Main movement executor with smart Y-coordinate handling
- `physics_executor.go` - Physics-based movement with tick simulation
- `physics_adapters.go` - Adapters for physics system integration
- `packets.go` - Movement packet building
- `factory.go` - Executor factory for creating executors

## Following System

The `following/` package implements player following:

- `manager.go` - Follow behavior management and state machine
- `target.go` - Target tracking (player position updates)
- `state.go` - Follow state (active, paused, stopped)
- `config.go` - Follow configuration (distance thresholds, update intervals)

## Project Structure

```
mc-agent/
├── CLAUDE.md                        # Project guidance (short version; this file for detail)
├── README.md                        # Project documentation
├── AGENTS.md                        # Agent configuration documentation
├── go.mod                           # Go module with replace directives
├── go.sum                           # Go module checksums
│
├── cmd/                             # Application entry points
│   ├── agent/main.go                # RECOMMENDED entry point
│   ├── legacy/main.go               # Legacy entry point (deprecated)
│   ├── skin-demo/                   # Skin caching demo
│   └── targetBow/                   # Bow targeting demo
│
├── agent/                           # Core agent package (75+ files)
│   ├── agent.go                     # Agent lifecycle and orchestration
│   ├── config.go                    # Configuration structure
│   ├── interfaces.go                # Agent interfaces
│   ├── adapters.go                  # Adapter implementations
│   ├── tracking.go                  # Position and entity tracking
│   ├── handlers.go                  # Packet handlers
│   ├── commands.go                  # Chat command handling
│   ├── command_actions.go           # Command execution
│   ├── game_chat.go                 # Game event handlers
│   ├── containers.go                # Container interaction
│   ├── containerEvents.go           # Container events
│   ├── screen.go                    # Screen/GUI handling
│   ├── hotbar.go                    # Hotbar management
│   ├── recipes*.go                  # Recipe system (multiple files)
│   ├── bowCommands.go               # Bow firing commands
│   ├── clutch.go                    # Clutch mechanics
│   ├── plan_runner.go               # Plan execution
│   ├── replay_mirror.go             # Replay recording
│   ├── skin_manager.go              # Skin management
│   ├── stop_file.go                 # Graceful shutdown
│   ├── plan/                        # Plan step definitions
│   └── *_test.go                    # Comprehensive test coverage
│
├── actions/                         # Command action definitions
│   ├── commands.go                  # Command implementations
│   ├── registry.go                  # Action registry
│   └── action_registry_impl.go      # Registry implementation
│
├── pathfinding/                     # HPA* Pathfinding (22 files)
│   ├── hpa_pathfinder.go            # Main HPA* implementation
│   ├── hpa_builder.go               # Graph builder
│   ├── hpa_graph.go                 # Graph structure
│   ├── hpa_cluster.go               # Cluster management
│   ├── hpa_entrance_grouping.go     # Entrance detection
│   ├── hpa_updates.go               # Dynamic updates
│   ├── a_star.go                    # A* algorithm
│   ├── epea_star.go                 # EPEA* variant
│   ├── movement.go                  # Movement validation
│   ├── blocks.go                    # Block collision
│   ├── collision_check.go           # AABB collision
│   ├── input_generator.go           # Movement inputs
│   ├── config.go                    # Configuration
│   └── types.go                     # Type definitions
│
├── movement/                        # Movement execution (8 files)
│   ├── executor.go                  # Main executor
│   ├── physics_executor.go          # Physics-based movement
│   ├── physics_adapters.go          # Physics adapters
│   ├── packets.go                   # Movement packets
│   └── factory.go                   # Executor factory
│
├── physics/                         # Physics engine (14 files)
│   ├── state.go                     # Physics state
│   ├── constants.go                 # Physics constants
│   ├── rotation.go                  # Rotation calculations
│   ├── projectile.go                # Projectile physics
│   ├── fall_damage.go               # Fall damage
│   ├── clutch.go                    # Clutch mechanics
│   └── types.go                     # Type definitions
│
├── following/                       # Player following (5 files)
│   ├── manager.go                   # Follow manager
│   ├── target.go                    # Target tracking
│   ├── state.go                     # Follow state
│   └── config.go                    # Configuration
│
├── items/                           # Item/inventory management (8 files)
│   ├── inventory.go                 # Inventory operations
│   ├── container_helper.go          # Container operations
│   ├── button_click.go              # GUI button clicking
│   ├── usage.go                     # Item usage
│   └── container_finder.go          # Container discovery
│
├── models/                          # Interfaces and types (66 files)
│   ├── agent.go                     # Agent interface
│   ├── client.go                    # Client interface
│   ├── world*.go                    # World interfaces
│   ├── screen*.go                   # Screen interfaces
│   ├── inventory*.go                # Inventory interfaces
│   ├── path*.go                     # Path interfaces
│   ├── physics*.go                  # Physics interfaces
│   ├── plan*.go                     # Plan interfaces
│   └── ...                          # Many more interface definitions
│
├── bot/                             # Bot framework extensions
│   ├── client.go                    # Client wrapper
│   ├── path/                        # Pathfinding primitives
│   ├── ptypes/                      # Packet type definitions
│   └── world/                       # World state
│       └── entity/player/           # Player entity
│
├── utils/                           # Utilities (6 files)
│   ├── utils.go                     # General utilities
│   ├── data_loader.go               # Data loading
│   ├── line.go                      # Line/ray utilities
│   ├── registry_downloader.go       # Registry downloading
│   └── minecraft_data_cache.go      # Data caching
│
├── testing/                         # Integration tests (40+ files)
│   ├── framework.go                 # Test framework
│   ├── rcon_helpers.go              # RCON helpers
│   ├── world_builder.go             # World construction
│   ├── navigation_*_test.go         # Navigation tests
│   ├── container_*_test.go          # Container tests
│   ├── follow_test.go               # Follow tests
│   └── README.md                    # Testing documentation
│
├── event_handler/                   # Legacy event handlers
│   └── entityEventHandlers.go       # Entity event processing
│
├── docs/                            # Documentation
│   ├── BOW_COMMANDS.md              # Bow firing commands
│   ├── CONTAINER_INTERACTION_GUIDE.md # Container system guide
│   ├── HPA_USAGE_EXAMPLE.md         # HPA* usage examples
│   ├── RECIPES.md                   # Recipe system
│   ├── SKINS.md                     # Skin management
│   ├── SNEAKING_MECHANICS_NOTES.md  # Sneaking behavior
│   ├── STRUCTURE_LOADER.md          # Structure-block .nbt loading/building
│   ├── TELEMETRY_ARCHITECTURE.md    # Telemetry system
│   ├── bow-fire-sequence.md         # Bow timing details
│   └── archive/                     # Historical documentation (130+ files)
│
├── code-archive/                    # Legacy implementations
│   ├── old/                         # Original legacy code
│   ├── src/                         # Intermediate version
│   └── phys/                        # Physics experiments
│
├── data/                            # Downloaded data cache
├── logs/                            # Packet logs (gitignored)
├── replays/                         # Replay recordings
├── skins/                           # Cached player skins
└── tmp/                             # Temporary files
```
