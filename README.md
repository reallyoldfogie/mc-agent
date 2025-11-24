# mc-agent

A Minecraft Java Edition bot/agent written in Go that connects to servers, handles game state, and responds to chat commands.  

**Inspiration taken from github.com/Tnze/mc-go/bot.**

## Features

- **Multi-Version Support**: Compatible with Minecraft 1.21.1 through 1.21.10
- **Microsoft Authentication**: Secure authentication via Microsoft accounts with credential caching
- **Packet Logging**: Comprehensive logging of all received packets for debugging and analysis
- **Entity Tracking**: Real-time tracking of players and entities with position updates
- **Chat Commands**: Bot responds to commands via chat with `>>>ROF_bot<<<` prefix
- **Event-Driven Architecture**: Clean separation of concerns with manager-based design
- **Version-Agnostic Protocol Handling**: Uses version managers for protocol differences

## Quick Start

### Prerequisites

- Go 1.21 or later
- Access to a Minecraft Java Edition server (1.21.1-1.21.10)
- Microsoft account for authentication (or use offline mode)

### Installation

```bash
git clone https://github.com/reallyoldfogie/mc-agent.git
cd mc-agent
go build -o mc-agent main.go
```

### Running the Bot

```bash
# Connect to a server with default settings
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
| `-version` | string | `1.21.5` | Target Minecraft version |
| `-offline` | bool | `false` | Use offline mode (no authentication) |
| `-token` | string | (cached) | Access token for offline mode |

## Chat Commands

The bot responds to commands in chat prefixed with `>>>ROF_bot<<<`:

| Command | Description |
|---------|-------------|
| `fireBow` | Fires equipped bow (experimental) |
| `startTracking` | Continuously look at nearest player |
| `stopTracking` | Stop tracking players |

**Example:**
```
>>>ROF_bot<<< startTracking
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
├── main.go                # Main bot executable
├── go.mod                 # Go module definition
│
├── bot/                   # Bot extensions and wrappers
│   ├── path/              # Pathfinding (planned)
│   ├── ptypes/            # Custom packet types
│   └── world/             # World state tracking
│
├── event_handler/         # Entity event handlers
├── models/                # Data models
├── utils/                 # Utility functions
├── data/                  # Static data files
└── logs/                  # Packet logs (auto-rotated)
```

## External Dependencies

The project integrates with several local repositories:

- **[mc-bot-go](https://github.com/reallyoldfogie/mc-bot-go)** - Bot framework (forked from Tnze/go-mc)
- **[mc-protocol-go](https://github.com/reallyoldfogie/mc-protocol-go)** - Version-specific protocol management
- **[mc-data-gen](https://github.com/reallyoldfogie/mc-data-gen)** - Block collision and shape data

## Configuration

### Authentication

By default, the bot uses Microsoft authentication. Credentials are cached in `.credCacheFile` for convenience.

To use offline mode:
```bash
./mc-agent -offline -name "MyBot"
```

### Logging

Packets are automatically logged to `./logs/` with rotation:
- Format: JSON with packet ID, name, data, and timestamp
- Rotation: 10MB max file size, 3 backups, 28-day retention
- Compression: Old logs are gzip compressed

## Planned Features

### Player Following (Ready for Implementation)

The bot will be able to follow players using intelligent pathfinding. See [FOLLOW_PLAYER_PLAN.md](FOLLOW_PLAYER_PLAN.md) for the comprehensive implementation plan.

**Planned capabilities:**
- Follow specific player by name
- Navigate around obstacles
- Handle water, ladders, and complex terrain
- Avoid dangerous blocks (lava, etc.)
- Recover from stuck states

## Development

### Building from Source

```bash
go build -o mc-agent main.go
```

### Running in Development

```bash
go run main.go -address "localhost:25565"
```

### Documentation

- **[CLAUDE.md](CLAUDE.md)** - Comprehensive codebase guide for Claude Code
- **[FOLLOW_PLAYER_PLAN.md](FOLLOW_PLAYER_PLAN.md)** - Player-following feature implementation plan
- **[MC_DATA_GEN_UPDATES_IMPACT.md](MC_DATA_GEN_UPDATES_IMPACT.md)** - Block collision data integration guide
- **[bow-fire-sequence.md](bow-fire-sequence.md)** - Bow firing packet sequence documentation

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
- 1.21.1
- 1.21.2
- 1.21.3
- 1.21.4
- 1.21.5 (default)
- 1.21.6 through 1.21.10

## Known Issues

- Position tracking was fixed for Minecraft 1.21.5+ protocol changes (TeleportID field reordering)
- Pathfinding system exists but is currently commented out pending mc-data-gen integration

## Related Projects

- [mc-bot-go](https://github.com/reallyoldfogie/mc-bot-go) - Custom bot framework
- [mc-protocol-go](https://github.com/reallyoldfogie/mc-protocol-go) - Protocol version management
- [mc-data-gen](https://github.com/reallyoldfogie/mc-data-gen) - Block data generator
