# mc-agent

A Minecraft Java Edition bot/agent written in Go that connects to servers, handles game state, and responds to chat commands.

The agent core is now a reusable Go package (`github.com/reallyoldfogie/mc-agent/agent`), and the CLI lives at `cmd/mc-agent`. This refactor enables integration tests and multiple agent instances in one process.

## Features

- **Multi-Version Support**: Compatible with Minecraft 1.21 through 1.21.8
- **Microsoft Authentication**: Secure authentication via Microsoft accounts with credential caching
- **Replay Recording**: Full ReplayMod .mcpr recording with bot visibility and skin texture embedding
- **Skin Management**: Automatic download and extraction of Minecraft client skins with caching
- **Recipe System**: Parse and export server recipes (property sets, stonecutter, slot displays)
- **Bow Firing**: Physics-based ballistic calculation for accurate bow aiming
- **Packet Logging**: Comprehensive logging of all received packets for debugging and analysis
- **Entity Tracking**: Real-time tracking of players and entities with position updates
- **Player Following**: Intelligent pathfinding-based following with obstacle avoidance
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

The bot responds to commands in chat prefixed with `>>>BOTNAME<<<` (e.g., `>>>ROF_bot<<<`). Examples:

| Command | Description |
|---------|-------------|
| `help` | Show available commands |
| `pos` | Print current position |
| `say <text>` | Echo text back to chat |
| `follow <player>` | Start following a player by name (requires path data) |
| `stopFollow` | Stop following |
| `followStatus` | Show follow status |
| `startTracking` | Track nearest player with periodic updates |
| `stopTracking` | Stop tracking |
| `fireBow` | Fire equipped bow (basic) |
| `fireBowAt <x> <y> <z>` | Fire bow at target coordinates with ballistic calculation |

Example:
```
>>>ROF_bot<<< follow Steve
>>>ROF_bot<<< pos
```

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
├── cmd/mc-agent/main.go   # Thin CLI bootstrap
├── agent/                 # Reusable agent package (lifecycle, handlers, tracking)
├── go.mod                 # Go module definition
│
├── actions/               # Action registry and command execution
├── items/                 # Item management, inventory operations, container helpers
├── models/                # Interfaces, types, and domain models
├── pathfinding/           # A*, EPEA*, and HPA* pathfinding implementations
├── physics/               # Physics engine (movement, projectiles, collision)
├── movement/              # Movement execution and physics-based movement
├── following/             # Player following system
├── versions/              # Version-specific protocol handlers
│   ├── common/            # Shared interfaces for all versions
│   └── v1_21_*/           # Per-version implementations (1.21.1-1.21.8)
├── scripts/               # Automation scripts (add_version.sh, verify_versions.sh)
├── testing/               # Integration testing framework (Docker-based)
│
├── bot/                   # Bot extensions and wrappers
│   └── world/             # World state tracking
│
├── utils/                 # Utility functions
├── data/                  # Static data files
└── logs/                  # Packet logs (auto-rotated)
```

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

- **[CLAUDE.md](CLAUDE.md)** - Comprehensive codebase guide for Claude Code
- **[docs/ADDING_NEW_VERSION.md](docs/ADDING_NEW_VERSION.md)** - Guide for adding new Minecraft version support
- **[docs/ADDING_NEW_ACTION.md](docs/ADDING_NEW_ACTION.md)** - Checklist for adding a new chat command/action, including RL (`rlenv`) wiring
- **[docs/BOW_COMMANDS.md](docs/BOW_COMMANDS.md)** - Bow firing commands and ballistics documentation
- **[docs/HPA_USAGE_EXAMPLE.md](docs/HPA_USAGE_EXAMPLE.md)** - HPA* pathfinding usage guide
- **[docs/CONTAINER_INTERACTION_GUIDE.md](docs/CONTAINER_INTERACTION_GUIDE.md)** - Container/inventory interaction guide
- **[scripts/README.md](scripts/README.md)** - Version automation scripts documentation
- **[testing/README.md](testing/README.md)** - Integration testing framework documentation

## Legacy Code

The repository includes legacy implementations in:
- `old/` - Original implementation with activity system
- `src/` - Intermediate version

These are kept for reference when porting features to the current implementation.

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
