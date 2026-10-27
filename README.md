# mc-agent

A Minecraft Java Edition bot/agent written in Go that connects to servers, handles game state, and responds to chat commands.

The agent core is now a reusable Go package (`github.com/reallyoldfogie/mc-agent/agent`), and the CLI lives at `cmd/mc-agent`. This refactor enables integration tests and multiple agent instances in one process.

## Features

- **Multi-Version Support**: Compatible with the supported Minecraft 1.21.x releases through 26.1
- **Microsoft Authentication**: Secure authentication via Microsoft accounts with credential caching
- **Replay Recording**: Full ReplayMod .mcpr recording with bot visibility and skin texture embedding
- **Skin Management**: Automatic download and extraction of Minecraft client skins with caching
- **Recipe System**: Parse and export server recipes (property sets, stonecutter, slot displays)
- **Bow Firing**: Physics-based ballistic calculation for accurate bow aiming
- **Autonomous PvP/Combat**: Target selection, melee/ranged combat, shields, armor/equipment tracking, retreat behavior, spear support, and hazardous-target handling
- **Multi-Agent Combat**: Team-aware targeting, match seeding/reset, and a real-player safety gate so several bots can fight each other (or a human) on one shared server - see [Multi-Agent Combat Training & Competition](#multi-agent-combat-training--competition)
- **RL Action Space**: `rlenv` exposes most chat actions as a fixed-size action space for an RL policy, including multi-step chains and buried-block mining - see [RL Action Space](#rl-action-space-rlenv)
- **Structure Building**: Loads vanilla Structure Block `.nbt` templates and builds them via real block placement, with a material-list "shopping list" export
- **Packet Logging**: Comprehensive logging of all received packets for debugging and analysis
- **Entity Tracking**: Real-time tracking of players and entities with position updates
- **Player Following**: Intelligent pathfinding-based following (A*/EPEA*/HPA*) with obstacle avoidance
- **Chat Commands**: Bot responds to commands via chat with `>>>ROF_bot<<<` prefix
- **Event-Driven Architecture**: Clean separation of concerns with manager-based design
- **Version-Specific Protocol Handling**: Modular per-version handlers for protocol differences
- **Version Automation**: Scripts to easily add support for new Minecraft versions
- **Integration Testing**: Docker-based testing framework for navigation and following behavior

## Quick Start

### Prerequisites

- Go 1.21 or later
- Access to a Minecraft Java Edition server (1.21-26.1)
- Microsoft account for authentication (or use offline mode)

### Installation

```bash
git clone https://github.com/reallyoldfogie/mc-agent.git
cd mc-agent
go build -o mc-agent ./cmd/mc-agent
```

### Running the Bot

```bash
# Connect to a server with default settings (auto-detects version if -version not set)
./mc-agent

# Connect to a specific server
./mc-agent -address "myserver.com:25565"

# Use a custom bot name
./mc-agent -name "MyBot"

# Use offline mode
./mc-agent -offline -name "OfflineBot"

# Specify Minecraft version
./mc-agent -version "1.21.5"
```

### Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-address` | string | `127.0.0.1:25565` | Server address and port |
| `-name` | string | `Daze` | Bot's player name |
| `-uuid` | string | (generated) | Player UUID |
| `-version` | string | `1.21.5` | Target Minecraft version (leave empty to auto-detect) |
| `-offline` | bool | `false` | Use offline mode (no authentication) |
| `-token` | string | (cached) | Access token for offline mode |

## Chat Commands

The bot responds to commands in chat prefixed with `>>>BOTNAME<<<` (e.g., `>>>ROF_bot<<<`). This is
the full, current set — every action registered in `actions/registry.go`'s `RegisterDefaults` (the
source of truth; see [Adding a New Action](#adding-a-new-action) below for keeping this list in
sync). Two internal-only variants (`movetoquiet`, `movetoandsneakquiet`) exist purely for `rlenv`'s
own dispatch and are omitted here — see [RL Action Space](#rl-action-space-rlenv).

### Movement & pathfinding

| Command | Description |
|---------|-------------|
| `help` | Show available commands |
| `pos` | Print current position |
| `say <text>` | Echo text back to chat |
| `testMove` | Test movement (+1 block on X) |
| `moveTo <x> <y> <z>` | Pathfind to coordinates |
| `lineTo <x> <y> <z>` | Straight-line movement (flat worlds only) |
| `moveForward <distance>` | Move forward by distance |
| `moveUp <distance>` | Move up by distance |
| `moveUpAndSneak <distance>` | Move up and enable sneaking |
| `moveToAndSneak <x> <y> <z>` | Pathfind and sneak at destination |
| `lineToAndSneak <x> <y> <z>` | Straight-line move and sneak at destination |
| `stopSneak` | Stop sneaking |
| `findPath <x> <y> <z>` | Calculate a path without moving |
| `testPath` | Test the pathfinding system |
| `follow [<player>]` | Follow a player by name (nearest player if omitted) |
| `stopFollow` | Stop following |
| `followStatus` | Show follow status |
| `startTracking` | Continuously look at the nearest player |
| `stopTracking` | Stop tracking |

### Combat

| Command | Description |
|---------|-------------|
| `fireBow` | Fire equipped bow in current direction (basic) |
| `fireBowAt <x> <y> <z> \| fireBowAt nearest \| fireBowAt <player>` | Aim and fire a bow with ballistic calculation at coordinates, the nearest target, or a named player |
| `attackEntity <entityID> [sneaking]` | Perform a validated direct melee attack |
| `shield <raise\|lower>` | Raise or lower the equipped shield |
| `fireCrossbowAt <entityID>` | Fire a crossbow at a tracked entity |
| `throwTridentAt <entityID>` | Throw a trident at a tracked entity |
| `spearJab <entityID> <itemName>` | Perform a spear Jab |
| `spearCharge <entityID> <itemName> <holdMs> <engagedMs> <tiredMs> [minSpeed] [minAlignment]` | Perform a spear Charge (caller already in range) |
| `spearChargeWithApproach <entityID> <itemName> <holdMs> <engagedMs> <tiredMs> [minSpeed] [minAlignment]` | Like `spearCharge`, but closes distance into range first |
| `maceAttack <entityID> <itemName>` | Perform a normal Mace attack |
| `maceSmash <entityID> <itemName>` | Wait for the falling-attack window and perform a Mace smash (caller already in range) |
| `maceSmashWithApproach <entityID> <itemName>` | Like `maceSmash`, but closes distance into range first |
| `runCombat [radius] [includeNeutral]` | Run autonomous combat - melee, ranged weapons, shields, spear support |
| `runCombatWithPolicy <radius> [includePlayers] [includeNeutral]` | Run autonomous combat with explicit target-category policy (PvP opt-in via `includePlayers`) |
| `killCreeperForGunpowder` | Fight a creeper, keep a safe distance, collect gunpowder when it drops (caller must equip a melee weapon first) |

### Vehicles & flight

| Command | Description |
|---------|-------------|
| `mount <entityID \| entityType>` | Mount a nearby vehicle/mob by entity ID or type |
| `dismount` | Dismount the current vehicle |
| `vehicleJump [power]` | Jump while mounted (e.g. horse) |
| `flyTo <x> <y> <z>` | Elytra flight to coordinates (takeoff, cruise, land) |
| `fly` / `land` | Toggle creative/spectator-style flying (distinct from `flyTo`'s elytra glide) |

### World interaction, inventory & crafting

| Command | Description |
|---------|-------------|
| `mine <x> <y> <z> \| mine <blockName> [radius]` | Mine a block at exact coordinates, or the nearest visible block matching a name |
| `lookAround [radius]` | List every visible entity/item within radius blocks (default 16) and their positions |
| `pickUpNearbyItem [maxDistance]` | Walk to the nearest visible dropped item within maxDistance blocks (default 8); vanilla auto-collects on arrival |
| `pickUpItem <x> <y> <z> \| pickUpItem <itemName> [maxDistance]` | Walk to and collect a *specific* item, by position or by matching contents |
| `craft <itemName>` | Craft an item, using the player's 2x2 grid or a nearby crafting table's 3x3 grid as the recipe requires |
| `place <itemName>` | Place a block from inventory on a standable cell beside the bot |
| `buildStructure <path> <x> <y> <z>` | Load a vanilla Structure Block `.nbt` template and build it via real block placement, origin at `<x> <y> <z>` (see [docs/STRUCTURE_LOADER.md](docs/STRUCTURE_LOADER.md)) |
| `materialList <structurePath> [exportPath]` | Report (and, with `exportPath`, persist to JSON) the materials a structure needs |
| `equip <item>` | Find an item in inventory and wear/wield it |
| `useItem [offhand]` | Use/right-click the currently held item |

### Camera & planning

| Command | Description |
|---------|-------------|
| `followCam <playerName> [maxDistance]` | Spectator-mode camera auto-follow via RCON |
| `stopFollowCam` | Stop camera auto-follow |
| `planStatus` | Show multi-step plan execution status |
| `planStop` | Stop the current plan |

Example:
```
>>>ROF_bot<<< follow Steve
>>>ROF_bot<<< pos
```

## RL Action Space (`rlenv`)

`rlenv` wraps most of the chat actions above into a fixed-size action space an RL policy can drive
directly, picking a task per episode (go to a point, mine/craft/place/pickup, multi-step chains like
"gather logs → craft planks and a table → place it → craft a goal item," combat, mob riding, elytra
flight, and player-following) rather than always practicing the same skill. Two commands above exist
*only* for this - `movetoquiet`/`movetoandsneakquiet` - identical to `moveto`/`movetoandsneak` but
without the chat announcements, which would otherwise spam (and risk a kick for) every step of a live
rollout.

Combat's RL wiring reuses `runCombatWithPolicy`'s own target-category policy, including the opt-in
PvP/team-aware targeting (`Config.CombatIncludePlayers`/`CombatIncludeNeutral`) multi-agent training
and tournament competition (see the next section) build on directly - no separate RL-only combat
code path.

Not every chat command above is wired into `rlenv` - most don't need to be (see
[Adding a New Action](#adding-a-new-action)'s step 4). The policy-facing task list, action encoding,
and curriculum design live in `rlenv/` and the sibling `mc-rsi-trainer` repo's `pkg/curriculum`; see
[docs/ADDING_NEW_ACTION.md](docs/ADDING_NEW_ACTION.md) for what wiring a new action into `rlenv`
actually involves.

## Multi-Agent Combat Training & Competition

Beyond single-bot PvE/PvP combat (above), the agent supports **multiple bots fighting each other**
on one shared server - the primitives `mc-rsi-trainer`'s `pkg/pvp` (self-play training) and
`pkg/tournament` (frozen-checkpoint tournaments) build on:

- **Team-aware targeting**: `RankCombatTargetsWithPolicy` excludes same-team entities from
  `Config.CombatIncludePlayers`'s target pool, so two teammates sharing an arena don't rank each
  other as a hostile target.
- **`SeedMatchTeam`**: assigns a vanilla scoreboard team to a participant via RCON.
- **`SeedMatchReset`**: resets a participant's health/weapon/position between matches/episodes.
- **`VerifyPlayerNearPosition`**: the safety gate every seeding helper above calls first - refuses
  to touch anyone outside a configured radius, so an unmanaged (real player) participant sharing the
  arena is never affected by another match's own reset.
- **Inventory snapshot save/restore**: captures a real player's exact inventory (including
  enchanted/named items) before handing them a fixed combat loadout, and restores it afterward.

These are mc-agent-side primitives only; the actual training loop, match barrier, bracket
orchestrator, and live dashboard live in `mc-rsi-trainer`'s `pkg/pvp`/`pkg/tournament`/`pkg/liveview`
and its `cmd/rsi-pvp-train`/`cmd/rsi-pvp-compete` entrypoints - see that repo's README and
[`docs/examples/README.md`](https://github.com/reallyoldfogie/mc-rsi-trainer/blob/main/docs/examples/README.md)
for a worked sample config.

## Architecture

The bot uses an event-driven architecture with specialized managers:

- **Client** (`*bot.Client`) - Network connection and packet handling
- **Player** (`*basic.Player`) - Player state with event callbacks
- **PlayerList** (`*playerlist.PlayerList`) - Tracks online players
- **Chat Handler** (`*msg.Manager`) - Chat message handling
- **World Manager** (`*world.World`) - Chunk loading/unloading
- **Screen Manager** (`*screen.Manager`) - Inventory/container management

### Project Structure

```
mc-agent/
├── cmd/                   # Entry points: cmd/agent (recommended), cmd/legacy, cmd/skin-demo, ...
├── agent/                 # Reusable agent package (lifecycle, handlers, tracking)
├── go.mod                 # Go module definition
│
├── actions/               # Chat-command action registry and execution
├── rlenv/                 # RL action space: task selection, action encoding, chain/composite tasks
├── items/                 # Item management, inventory operations, container helpers
├── models/                # Interfaces, types, and domain models
├── pathfinding/           # A*, EPEA*, and HPA* pathfinding implementations
├── physics/               # Physics engine (movement, projectiles, collision)
├── movement/              # Movement execution and physics-based movement
├── following/             # Player following system
├── combat/                # Combat primitives (targeting, weapons, team-aware PvP)
├── structure/             # Structure Block .nbt loading/building, material lists
├── handler_versions/      # Version-specific protocol handlers
│   ├── common/            # Shared interfaces for all versions
│   └── v1_21_*/, v26_1/   # Per-version implementations
├── config/, configs/      # Settings types and loading
├── courier/               # Cross-server item transfer
├── scripts/               # Automation scripts (add_version.sh, verify_versions.sh)
├── testing/               # Integration testing framework (Docker-based)
│
├── bot/                   # Bot extensions and wrappers
│   └── world/             # World state tracking
│
├── world/                 # World state (blocks, entities, light/biome/border)
├── utils/                 # Utility functions
├── internal/              # Internal-only helpers
├── data/                  # Static data files
└── logs/                  # Packet logs (auto-rotated, gitignored)
```

(`code-archive/` - legacy implementations kept for local reference - and `docs/plans/`/`docs/archive/`
- working design notes and session history - are gitignored: present on a machine that's been
developing this project, not in a fresh clone.)

### Version-Specific Protocol Handlers

The bot uses a modular architecture for handling Minecraft protocol differences across versions. Each supported version has its own handler package in `versions/`:

```
versions/
├── common/       # Shared interfaces (VersionHandler, LoginHandler, etc.)
├── v1_21_1/      # Minecraft 1.21.1 implementation
├── v1_21_2/      # Minecraft 1.21.2 implementation
...
└── v1_21_8/      # Minecraft 1.21.8 implementation
```

**Key Components:**
- **VersionHandler**: Main interface for version-specific behavior
- **LoginHandler**: Login phase packet handling
- **ConfigurationHandler**: Configuration phase handling
- **PlayHandler**: Play phase sub-handlers (movement, entities, containers, chat, world)

**Adding New Versions:**
Use the automation script to add support for new Minecraft versions:

```bash
# Add support for a new version
./scripts/add_version.sh 1.21.9 773

# Specify source version to copy from
./scripts/add_version.sh 1.21.9 773 1.21.8
```

See [docs/ADDING_NEW_VERSION.md](docs/ADDING_NEW_VERSION.md) for detailed instructions.

## External Dependencies

The project integrates with several local repositories:

- **[mc-bot-go](https://github.com/reallyoldfogie/mc-bot-go)** - Bot framework (forked from Tnze/go-mc)
- **[mc-protocol-go](https://github.com/reallyoldfogie/mc-protocol-go)** - Version-specific protocol management
- **[mc-data-gen](https://github.com/reallyoldfogie/mc-data-gen)** - Block collision and shape data
- **[mc-replay-go](https://github.com/reallyoldfogie/mc-replay-go)** - ReplayMod .mcpr recording and playback

## Configuration

### Authentication

By default, the bot uses Microsoft authentication. Credentials are cached in `.credCacheFile` for convenience.

To use offline mode:
```bash
./mc-agent -offline -name "MyBot"
```

### Logging

Packets are automatically logged to `./logs/` with rotation (via lumberjack):
- Format: JSON with packet ID, name, data, and timestamp
- Rotation: 10MB max file size, 3 backups, 28-day retention
- Compression: Old logs are gzip compressed

### Skin Management

The bot includes a comprehensive skin management system that can download and extract player skins from the Minecraft client jar.

**Features:**
- Automatic download of Minecraft client jar for any version
- Extraction of all 18 default player skins (9 slim + 9 wide models)
- Smart caching to avoid re-downloading
- Load skins by name or get random skins
- Fallback to Mojang's session servers for player-specific skins

**Available skins:** alex, ari, efe, kai, makena, noor, steve, sunny, zuri (in both slim and wide models)

See [docs/SKINS.md](docs/SKINS.md) for detailed documentation.

### Combat testing status

Combat has deterministic unit coverage plus multi-version Docker integration coverage for melee, shields, ranged weapons, armor/equipment tracking, target death/respawn, multiple targets, hazardous creepers, and the reusable creeper loot action. The current integration matrix covers the supported versions listed by `models.StandardVersionTests`.

Multi-agent bot-vs-bot PvP (team-aware targeting, match seeding/reset, the real-player safety gate,
inventory snapshot save/restore - see [Multi-Agent Combat Training & Competition](#multi-agent-combat-training--competition)
above) is also live-verified: unit coverage for each primitive, plus live tests confirming the
proximity guard actually refuses an out-of-range player and a save -> fixed-loadout -> restore round
trip reproduces a real inventory exactly, including an enchanted/named item.

**Try the demo:**
```bash
cd cmd/skin-demo
go build .
./skin-demo -list              # List all available skins
./skin-demo -skin steve         # Load Steve skin
./skin-demo -skin alex -model slim  # Load slim Alex skin
```

### Replay Recording

The bot includes a complete ReplayMod recording system that captures gameplay sessions in `.mcpr` format for later playback in the ReplayMod.

**Features:**
- Records all clientbound packets to ReplayMod-compatible `.mcpr` files
- Bot appears as a visible entity in replays with correct position and rotation
- Automatic skin texture embedding for accurate player rendering
- Supports both legacy and modern file formats (1.20.2+ with login/config phases)
- Configurable output path and metadata

**Usage:**
```bash
# Enable replay recording
./mc-agent -address server:25565 -replay

# Custom output file
./mc-agent -address server:25565 -replay -replay-out my-session.mcpr

# Custom generator metadata
./mc-agent -address server:25565 -replay -replay-generator "MyBot v1.0"
```

**Replay flags:**
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-replay` | bool | `false` | Enable ReplayMod recording |
| `-replay-out` | string | `session.mcpr` | Output file path for replay |
| `-replay-generator` | string | `mc-agent` | Generator metadata string |

**Technical details:**
- Movement mirroring: Converts serverbound movement packets to synthetic clientbound packets so the bot appears in replays
- Texture embedding: Fetches player skin properties and embeds them in PlayerInfo packets
- Fallback support: Can reuse textures from existing `.mcpr` files when offline

Generated `.mcpr` files can be opened in Minecraft with ReplayMod installed for cinematic playback, debugging, and analysis.

## Advanced Features

### Recipe System

The bot can parse and store the server's `Update Recipes` packet, which includes:
- **Property Sets**: Item groups (e.g., `minecraft:wood` containing all wood types)
- **Stonecutter Recipes**: Input items and their possible stonecutter outputs
- **Slot Displays**: Complex item display types (tags, stacks, smithing trims, composites)

**API Access:**
```go
// Get last Update Recipes payload
payload, ok := agent.LastUpdateRecipes()

// Export as JSON for inspection
json, ok, err := agent.ExportLastUpdateRecipesAsJSON(true)
```

See [docs/RECIPES.md](docs/RECIPES.md) for detailed documentation.

### Physics System

The bot includes a comprehensive physics engine that accurately simulates Minecraft mechanics:

**Core Features:**
- **Movement Physics**: Accurate player movement with collision detection, gravity, friction
- **Rotation Rate Limiting**: Anti-cheat compliant rotation (11°/tick yaw, 7°/tick pitch)
- **Projectile Physics**: Ballistic simulation for arrows, snowballs, eggs, ender pearls, potions, tridents
- **Fall Damage**: Accurate fall damage calculation with water detection and special blocks
- **Movement Prediction**: Simulate N ticks ahead for path validation
- **Collision Avoidance**: Pre-check paths before execution
- **Item Usage**: Block placement and item usage with sequence number tracking

**Movement Types:**
- Traverse, Sprint, Sneak, Ascend, Descend, Jump, Diagonal, Swim, Climb, and more
- Each type has accurate cost and speed parameters

**Projectile Types Supported:**
- Arrow, Snowball, Egg, Ender Pearl, Splash Potion, Trident, Fishing Bobber
- Each with accurate gravity, drag, and speed constants

**Key Capabilities:**
- Water landing detection (0 damage from any height)
- Special block damage reduction (hay bale, slime, honey, powder snow)
- Anti-cheat safe rotation and sequence numbers
- Pathfinding integration for safe navigation

See `physics/` and `models/physics*.go` for implementation details.

### Bow Firing with Ballistics

The bot includes physics-based bow aiming that calculates pitch, yaw, and power to hit targets:
- Simulates arrow trajectory (gravity, drag)
- Iterates through pitch/power combinations to find optimal shot
- Accounts for bot eye height and target position
- Now powered by generalized projectile physics system

**Commands:**
- `>>>bot<<< fireBow` - Fire bow in current direction
- `>>>bot<<< fireBowAt 100 64 200` - Aim and fire at coordinates

**Ballistics Algorithm:**
- Initial speed: `3.1 * powerFactor` (0.1-1.0)
- Gravity: 0.05 blocks/tick²
- Drag: 0.99/tick
- Simulation: 400 ticks max

See [docs/BOW_COMMANDS.md](docs/BOW_COMMANDS.md) for implementation details.

### Player Following

The bot can follow players using intelligent pathfinding with HPA* (Hierarchical Path-finding A*).

**Capabilities:**
- Follow specific player by name
- Navigate around obstacles using A*, EPEA*, or HPA* algorithms
- Handle water, ladders, and complex terrain
- Avoid dangerous blocks (lava, etc.)
- Recover from stuck states
- Dynamic world updates (handles blocks being placed/broken)

See [docs/HPA_USAGE_EXAMPLE.md](docs/HPA_USAGE_EXAMPLE.md) for pathfinding usage.

## Development

### Building from Source

```bash
go build -o mc-agent ./cmd/mc-agent
```

### Running in Development

```bash
go run ./cmd/mc-agent -address "localhost:25565"
```

### Adding a New Action

Chat commands (`moveTo`, `fireBow`, `equip`, ...) are `models.Action` values registered in
`actions/registry.go`. Adding one is roughly:

1. Add any new capability it needs to `models.CommandAgent` and implement it on the real agent.
2. Write the `Action` (`Name`/`Usage`/`Execute`) in `actions/commands.go`, returning a
   `models.Completion` (`models.Done(nil)` if it finishes synchronously, `models.NewCompletion()`
   if it launches a goroutine).
3. Register it in `actions/registry.go`, and add it to `helpText` and CLAUDE.md's command list.
4. Optionally wire it into `rlenv` (`rlenv/action.go`) so the RL policy can use it too — most
   actions don't need this.

See **[docs/ADDING_NEW_ACTION.md](docs/ADDING_NEW_ACTION.md)** for the full checklist, including
the interface fan-out that step 1 triggers and what "wire it into `rlenv`" actually involves.

### Documentation

- **CLAUDE.md** - Comprehensive codebase guide for Claude Code (gitignored - present when
  developing locally with Claude Code, not in a fresh clone)
- **[docs/ADDING_NEW_VERSION.md](docs/ADDING_NEW_VERSION.md)** - Guide for adding new Minecraft version support
- **[docs/ADDING_NEW_ACTION.md](docs/ADDING_NEW_ACTION.md)** - Checklist for adding a new chat command/action, including RL (`rlenv`) wiring
- **[docs/BOW_COMMANDS.md](docs/BOW_COMMANDS.md)** - Bow firing commands and ballistics documentation
- **[docs/HPA_USAGE_EXAMPLE.md](docs/HPA_USAGE_EXAMPLE.md)** - HPA* pathfinding usage guide
- **[docs/CONTAINER_INTERACTION_GUIDE.md](docs/CONTAINER_INTERACTION_GUIDE.md)** - Container/inventory interaction guide
- **[docs/STRUCTURE_LOADER.md](docs/STRUCTURE_LOADER.md)** - Structure Block `.nbt` loading/building and material-list export
- **[scripts/README.md](scripts/README.md)** - Version automation scripts documentation
- **[testing/README.md](testing/README.md)** - Integration testing framework documentation

## Legacy Code

Legacy implementations are kept locally for reference when porting features, under `code-archive/`
(gitignored - not present in a fresh clone):
- `code-archive/old/` - Original implementation with activity system
- `code-archive/src/` - Intermediate version
- `code-archive/phys/` - Physics experiment archive

## License

See [LICENSE](LICENSE) file for details.

## Contributing

This is a personal project, but suggestions and bug reports are welcome via GitHub issues.

## Acknowledgments

- Built on [Tnze/go-mc](https://github.com/Tnze/go-mc) - Minecraft protocol library
- Uses [go-mc-ms-auth](https://github.com/maxsupermanhd/go-mc-ms-auth) - Microsoft authentication
- Packet logging via [lumberjack](https://github.com/natefinch/lumberjack) - Log rotation

## Version Support

Currently tested and working with Minecraft Java Edition:
- 1.21.1 (protocol 767)
- 1.21.2 (protocol 768)
- 1.21.3 (protocol 768)
- 1.21.4 (protocol 769)
- 1.21.5 (protocol 770, default)
- 1.21.6 (protocol 771)
- 1.21.7 (protocol 772)
- 1.21.8 (protocol 772)

Each version has a dedicated handler in `versions/v1_21_X/`. See [docs/ADDING_NEW_VERSION.md](docs/ADDING_NEW_VERSION.md) for adding new versions.

## Known Issues

- Position tracking was fixed for Minecraft 1.21.5+ protocol changes (TeleportID field reordering)

## Related Projects

- [mc-bot-go](https://github.com/reallyoldfogie/mc-bot-go) - Custom bot framework
- [mc-protocol-go](https://github.com/reallyoldfogie/mc-protocol-go) - Protocol version management
- [mc-data-gen](https://github.com/reallyoldfogie/mc-data-gen) - Block data generator
