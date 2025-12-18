# Agent Package Refactor Plan

## Overview
- Goal: Extract all agent logic from `main.go` into a reusable `agent` package with a clear lifecycle so we can run integration tests and create multiple independent agent instances.
- Outcomes:
  - Testable core logic (unit + integration) without a CLI.
  - Multiple agents can coexist in one process.
  - `main.go` becomes a thin bootstrap for flags, config, and lifecycle control.

## Current State Summary (from main.go)
- Auth and version resolution (offline/online auth, version/protocol detection).
- Managers: `packetMgr`, `blockMgr`, `soundMgr` selection by version.
- Client/player setup: `bot.Client`, `basic.Player`, `playerlist.PlayerList`, `msg.Manager`, `world.World`, `screen.Manager`.
- Movement/pathfinding/following: `movementExecutor`, `shapeMgr`, `pathFinder`, `followManager`, `targetSelector`.
- Custom registries capture during configuration phase.
- Entity tracking: bot position, bot entity ID, tracked entities, cleanup goroutine.
- Packet/event handlers: game lifecycle, chat, world events, entity spawn/move/teleport, sounds, etc.
- Logging: rotating packet logs with `lumberjack`.
- Many globals and free functions; tightly coupled to a single running instance.

## Target Architecture
### Package Layout
- `agent/` (new): Core package.
  - `agent.go`: `Agent` type, constructor, lifecycle (`Init`, `Start`, `Close`).
  - `config.go`: `Config` struct (address, auth, version, protocol, paths, managers, toggles) + validation.
  - `handlers.go`: Event and packet handler methods on `Agent`.
  - `sound.go`: Sound packet handler and logging via SoundMgr.
  - `world_handlers.go`: Chunk load/unload callbacks.
  - `screen.go`: Screen slot change hook.
  - `tracking.go`: Entity tracking, position, cleanup ticker, helpers.
  - `registry.go`: Custom registry capture, lookup APIs.
  - `interfaces.go`: Narrow interfaces for external deps to enable fakes in tests.
- `cmd/mc-agent/main.go`: Thin CLI using flags to build `agent.Config`, start/stop the agent.

### Public API
- `type Agent interface { ... }`
- `type agent struct { ... }`
- `func New(cfg Config) (Agent, error)`
- `func (a *agent) Init(ctx context.Context) error` // set up deps, handlers, registries
- `func (a *agent) Start(ctx context.Context) error` // connect/login, start background tasks
- `func (a *agent) Close(ctx context.Context) error` // graceful shutdown, stop tasks
- Optional helper methods (for tests/tools): `SendChat`, `GetPosition`, `FollowTarget`, `TrackedEntities`, etc.

### Config and Dependency Injection
- `Config` fields:
  - Connection: `Address`, `Auth`, `Version`, `ProtocolVersion`.
  - Data paths: `MCDataGenPath`, `MCProtocolGoPath`.
  - Managers: `PacketMgr`, `BlockMgr`, `SoundMgr` (derive by version if nil).
  - Logging: optional `io.Writer` or logger.
  - Features: toggles for pathfinding/following.
- Introduce small interfaces as seams for testing (examples):
  - `Client` (JoinServerWithOptions, Events, etc.)
  - `Player`, `PlayerList`, `Chat`, `World`, `Screen`
  - Movement executor, pathfinder, follow manager
- Provide default adapters that wrap current mc-bot-go types.

### Lifecycle and Concurrency
- `Init` creates/wires dependencies, registers handlers.
- `Start` connects and kicks off goroutines (e.g., entity cleanup) tied to context.
- `Close` cancels context, removes listeners, stops goroutines, and drains.
- No package-level globals; all state is fields guarded by `sync.Mutex/RWMutex` where needed.

### Event Wiring
- Convert existing free functions (e.g., `onGameStart`, `onDisconnect`, entity handlers) to `Agent` methods.
- Register packet handlers within `Init` where IDs are resolved via `PacketMgr`.
  - Chat event handlers wired via `msg.EventsHandler` in CLI for now.
  - World and screen events wired in CLI with agent method callbacks.

## Migration Plan
1) Create `agent` package skeleton and `Config` with validation and defaulting (derive managers from version if not provided).
2) Introduce `Agent` with fields for all prior globals:
   - Client, player, player list, chat, world, screen.
   - Movement executor, pathfinding components, follow manager/selector.
   - Bot position/entity ID, tracked entities map, custom registries, logger.
3) Move helper functions to methods: position getters/setters, chat sending, tracked entity access, player UUID lookup.
4) Port packet/event handlers into `handlers.go` as methods using `Agent` fields.
5) Register handlers inside `Init` using `PacketMgr` to resolve IDs.
6) Implement background cleanup loop in `tracking.go` bound to context with intervals as constants.
7) Implement lifecycle (`Init`, `Start`, `Close`) and ensure proper teardown.
8) Replace `main.go` with `cmd/mc-agent/main.go` that:
   - Parses flags.
   - Resolves version and paths (using current helpers).
   - Builds `agent.Config`, constructs agent, calls `Init`/`Start`, handles signals, defers `Close`.
9) Update docs and README usage to point to new entrypoint and package API.

## Testing Strategy
- Unit tests for:
  - Entity tracking updates, soft-delete/un-delete behavior, and grace-period cleanup (table-driven).
  - Registry capture logic.
  - Handler reactions to key packets/events.
- Fakes for interfaces (`Client`, `PlayerList`, `Chat`, `World`, `Movement`, `Pathfinder`).
- Integration tests that:
  - Instantiate `Agent` with fakes.
  - Simulate packet events and assert state changes and method calls.
- Concurrency tests ensuring `Close` terminates background goroutines and listeners.

## Backwards Compatibility
- Preserve current command-line flags/behavior via `cmd/mc-agent`.
- Keep logging semantics (rotating files) but move injection behind `Config`.
- No changes to external packages (mc-bot-go, mc-protocol-go); only wrap and inject.

## Risks and Mitigations
- Hidden global state: audit and encapsulate as `Agent` fields; avoid singletons.
- Event ordering and registration timing: wire handlers in `Init` before `Start`.
- Version/manager mismatches: validate `Config` and fail fast with clear errors.
- Pathfinding heavy init: allow disabling via feature toggle; lazy-load state props with clear logs.
- Graceful shutdown: ensure goroutines respect context and listeners are unregistered.

## Acceptance Criteria
- Builds with new `agent` package and thin CLI.
- `main.go` logic lives inside `agent` methods; old globals removed.
- Unit tests cover tracking and handler logic; basic integration test runs with fakes.
- Able to spawn multiple `Agent` instances in tests without interference.

## Work Breakdown
- ✅ Phase 1: Package skeleton, Config, Agent struct, lifecycle stubs.
- ✅ Phase 2: Port helpers and tracking; implement cleanup loop.
- ✅ Phase 3: Port event handlers; register wiring in `Init`.
- ✅ Phase 4: Pathfinding/following integration behind feature toggles.
- ✅ Phase 5: CLI bootstrap and documentation updates.
- ✅ Phase 6: Tests (unit, fakes, integration) and polish.

## Additional Features Implemented
Beyond the original plan, the following features were added:

### Recipe System (`agent/recipes*.go`)
- Parse Update Recipes packets (property sets, stonecutter, slot displays)
- Export recipes as JSON for inspection
- Comprehensive test coverage (5 test files)
- See [RECIPES.md](RECIPES.md) for documentation

### Bow Commands (`agent/bowCommands.go`)
- Physics-based ballistic calculation for bow aiming
- Commands: `fireBow`, `fireBowAt <x> <y> <z>`
- Gravity and drag simulation matching Minecraft physics
- Standalone testing utility (`cmd/targetBow/`)
- See [BOW_COMMANDS.md](BOW_COMMANDS.md) for documentation

### Testing Framework (`testing/`)
- Docker-based integration testing
- Navigation tests (flat, pathfinding, vertical)
- Follow behavior tests
- RCON-based world manipulation
- ReplayMod recording for all tests
- See [testing/README.md](../testing/README.md) for guide

### Bug Fixes and Improvements
- Player chat packet handling (type mismatch fix)
- Declare Commands packet parsing
- Enhanced logging for game loop
- Context cancellation on HandleGame errors
- See [PLAYERCHAT_INVESTIGATION.md](PLAYERCHAT_INVESTIGATION.md)

## Migration Status
- **Legacy Code**: `main.go` moved to `cmd/legacy/main.go` (deprecated)
- **Production Entry Point**: `cmd/agent/main.go`
- **Package Location**: `github.com/reallyoldfogie/mc-agent/agent`
- **Backward Compatibility**: Full CLI flag parity maintained
