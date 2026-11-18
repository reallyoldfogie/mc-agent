# Running the Bot

**Recommended (via agent package):**
```bash
go run cmd/agent/main.go [flags]
```

**Legacy (direct):**
```bash
go run cmd/legacy/main.go [flags]
```

Available flags:
- `-address string` - Server address (default "127.0.0.1:25565")
- `-name string` - Player's name (default "Daze")
- `-uuid string` - Player's UUID (for offline mode)
- `-version string` - Target Minecraft version (empty = auto-detect from server)
- `-offline` - Use offline mode
- `-token string` - Access token for offline mode
- `-data-path string` - Path to mc-data-gen data directory (empty = auto-detect)
- `-protocol-path string` - Path to mc-protocol-go directory (empty = auto-detect)
- `-disable-physics` - Disable physics-based movement (uses simple teleport-style movement)
- `-clutch` - Enable clutch assist during physics movement (fast block placement)
- `-replay` - Enable ReplayMod recording (.mcpr files)
- `-replay-out string` - Replay output file path (default "session.mcpr")
- `-replay-generator string` - Replay generator string (default "mc-agent")
- `-skin-cache string` - Directory to cache player skins (default "skins")
- `-skin-net` - Allow network skin fetches from Mojang API (default off for privacy)

## Authentication

The bot uses Microsoft authentication by default via the `go-mc-ms-auth` library:
- Credentials are cached in `.credCacheFile`
- Offline mode available via `-offline` flag (requires `-name`, `-uuid`, `-token`)
- Uses MS app ID from msauth documentation (should be replaced with own ID for production)

## Graceful Shutdown

Create a stop file to trigger graceful shutdown:

```bash
# Terminal 1: Start the agent
go run cmd/agent/main.go -address server:25565

# Terminal 2: Trigger graceful shutdown
touch .agentStop
```

When the stop file is detected:
- Agent initiates graceful shutdown
- All goroutines are cleanly terminated
- Replay files are properly closed
- Stop file is automatically removed
- Process exits with status 0

## Building

```bash
go build -o bin/mc-agent ./cmd/agent
```

## Data Loading

The bot requires mc-data-gen block collision data for pathfinding. The `-data-path` flag supports both URLs and local paths:

**Local Path Usage:**
```bash
# Absolute path
./bin/mc-agent -data-path /path/to/mc-data-gen/data

# Empty uses auto-detection from sibling directories
./bin/mc-agent
```

**URL Usage:**
```bash
# Download from GitHub repository
./bin/mc-agent -data-path https://github.com/user/mc-data-gen
```

When a URL is provided:
- GitHub URLs are automatically detected and converted to archive downloads
- Data is downloaded automatically on first run
- Downloaded data is cached in `./data/mc-data-gen-cache/`

Expected data structure:
```
{data-path}/
├── 1.21.5/
│   └── blocks/
│       ├── minecraft_acacia_button.json
│       └── ...
└── ...
```
