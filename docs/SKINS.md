# Minecraft Skin System

This document describes how to use the Minecraft skin downloading and management system in mc-agent.

## Overview

The skin system provides the ability to:
- Download the Minecraft client jar from Mojang
- Extract player skin textures from the jar
- Cache downloaded files to avoid repeated downloads
- Load and serve any skin from the extracted files
- Fetch player skins from Mojang's session servers

## Architecture

The skin system consists of three main components:

### 1. ClientJarDownloader (`agent/downloader.go`)
Downloads and extracts the Minecraft client jar:
- Fetches version metadata from Mojang's version manifest
- Downloads the client.jar for a specific Minecraft version
- Extracts skin PNG files from `assets/minecraft/textures/entity/player/`
- Caches downloaded files with configurable TTL

### 2. SkinFetcher (`agent/skins.go`)
Manages skin retrieval and caching:
- Loads skins from cache (player-specific or defaults)
- Fetches skins from Mojang's session servers (if network is enabled)
- Loads extracted local skins from PNG files
- Generates default skins as fallback

### 3. SkinManager (`agent/skin_manager.go`)
High-level API that ties everything together:
- Initializes the downloader and fetcher
- Ensures skins are available (downloads if needed)
- Provides simple methods to get skins for players
- Lists all available extracted skins

## Quick Start

### Basic Usage

```go
import "github.com/reallyoldfogie/mc-agent/agent"

// Create a skin manager
skinMgr := agent.NewSkinManager(agent.SkinManagerConfig{
    MinecraftVersion:  "1.21.5",
    CacheRoot:         "./skins",
    AllowNetwork:      true,
    ClientJarCacheDir: "./data/client-cache",
})

// Initialize (downloads and extracts skins if not cached)
if err := skinMgr.Initialize("1.21.5"); err != nil {
    log.Fatal(err)
}

// Get a skin for a player
uuid := [16]byte{...} // player UUID
props := skinMgr.GetSkinForPlayer(uuid, "PlayerName")

// Get a random extracted skin
randomProps := skinMgr.GetRandomExtractedSkin(uuid, "PlayerName")

// Get a specific skin by name
steveProps := skinMgr.GetSkinByName("steve", "wide", uuid, "PlayerName")

// List all available skins
skins, err := skinMgr.ListAvailableSkins()
for _, skin := range skins {
    fmt.Printf("%s (%s)\n", skin.Name, skin.Model)
}
```

## Available Skins

As of Minecraft 1.21.5, the following default skins are available:

**Slim model** (Alex-style, 3px arms):
- alex
- ari
- efe
- kai
- makena
- noor
- steve (slim variant)
- sunny
- zuri

**Wide model** (Steve-style, 4px arms):
- alex (wide variant)
- ari (wide variant)
- efe (wide variant)
- kai (wide variant)
- makena (wide variant)
- noor (wide variant)
- steve
- sunny (wide variant)
- zuri (wide variant)

## Configuration

### SkinManagerConfig

```go
type SkinManagerConfig struct {
    // MinecraftVersion is the version to download skins for (e.g., "1.21.5")
    MinecraftVersion string

    // CacheRoot is the base directory for all skin-related files (default: "./skins")
    CacheRoot string

    // AllowNetwork enables fetching skins from Mojang's API
    AllowNetwork bool

    // ClientJarCacheDir is where downloaded client jars are stored (default: "./data/client-cache")
    ClientJarCacheDir string
}
```

### ClientJarDownloaderConfig

```go
type ClientJarDownloaderConfig struct {
    CacheDir   string        // Where to cache downloaded files
    HTTPClient *http.Client  // Custom HTTP client (optional)
    TTLDays    int           // Cache TTL in days (default: 30)
}
```

### SkinFetcherConfig

```go
type SkinFetcherConfig struct {
    AllowNetwork bool          // Enable network fetching from Mojang
    CacheRoot    string        // Base directory for skin cache
    HTTPClient   *http.Client  // Custom HTTP client (optional)
}
```

## Directory Structure

After initialization, the following directory structure is created:

```
skins/
├── extracted/              # Extracted skins from client jar
│   ├── slim/              # Slim model skins (3px arms)
│   │   ├── alex.png
│   │   ├── steve.png
│   │   └── ...
│   └── wide/              # Wide model skins (4px arms)
│       ├── steve.png
│       ├── alex.png
│       └── ...
└── players/               # Cached player-specific skins
    └── [uuid-name.json files]

data/client-cache/
└── 1.21.5/
    └── client.jar        # Downloaded Minecraft client
```

## Skin Property Format

Skins are returned as `ProfileProperty` structs:

```go
type ProfileProperty struct {
    Name      string  // "textures"
    Value     string  // Base64-encoded JSON with texture data
    Signature string  // Optional signature for signed skins
}
```

The `Value` field contains base64-encoded JSON in this format:

```json
{
  "timestamp": 1234567890,
  "profileId": "uuid-without-dashes",
  "profileName": "PlayerName",
  "textures": {
    "SKIN": {
      "url": "data:image/png;base64,..." or "https://...",
      "metadata": {
        "model": "slim" or "wide"
      }
    }
  }
}
```

## Skin Resolution Priority

When calling `GetSkinForPlayer()`, the system tries to find a skin in this order:

1. **Cached player skin** - Previously fetched skin for this specific player UUID
2. **Network fetch** - Fetch from Mojang's session servers (if `AllowNetwork` is true)
3. **Extracted skins** - Random skin from the extracted client jar skins (final fallback)

**Note:** If no skins are available (i.e., client jar not downloaded), `Get()` returns `nil`. Make sure to call `Initialize()` first to download and extract skins.

## Advanced Usage

### Using Lower-Level APIs

You can use the individual components directly for more control:

```go
// Use ClientJarDownloader directly
downloader := agent.NewClientJarDownloader(agent.ClientJarDownloaderConfig{
    CacheDir: "./cache",
    TTLDays:  30,
})

clientJarPath, err := downloader.DownloadClientJar("1.21.5")
if err != nil {
    log.Fatal(err)
}

err = downloader.ExtractSkins(clientJarPath, "./skins/extracted")
if err != nil {
    log.Fatal(err)
}

// Use SkinFetcher directly
fetcher := agent.NewSkinFetcher(agent.SkinFetcherConfig{
    AllowNetwork: true,
    CacheRoot:    "./skins",
})

uuid := [16]byte{...}
props := fetcher.Get(uuid, "PlayerName")
```

### Loading Specific Skins

```go
// Load all available skins
skins, _ := skinMgr.ListAvailableSkins()

// Filter by model
slimSkins := []agent.LocalSkin{}
for _, skin := range skins {
    if skin.Model == "slim" {
        slimSkins = append(slimSkins, skin)
    }
}

// Get a specific skin
props := skinMgr.GetSkinByName("alex", "slim", uuid, "PlayerName")
```

## Error Handling

The skin system handles errors gracefully:
- If download fails, it uses cached files
- If extracted skins are missing, it falls back to generated defaults
- Network errors (when fetching from Mojang) fall back to local skins

Example:

```go
skinMgr := agent.NewSkinManager(agent.SkinManagerConfig{
    MinecraftVersion: "1.21.5",
    AllowNetwork:     false, // Disable network to force local skins
})

if err := skinMgr.Initialize("1.21.5"); err != nil {
    // Handle initialization error
    // Note: Initialize only fails if download is required but fails
    // If skins are already cached, it will succeed
    log.Printf("Failed to initialize: %v", err)
}

// Get a skin - will use extracted/cached/generated skins
props := skinMgr.GetSkinForPlayer(uuid, "PlayerName")
if len(props) == 0 {
    // This should rarely happen, as generated defaults are always available
    log.Println("No skin available")
}
```

## Testing

The skin system includes comprehensive tests:

```bash
# Run all skin manager tests
go test -v ./agent -run TestSkinManager -timeout 5m

# Run specific test
go test -v ./agent -run TestSkinManager_Initialize

# All tests
go test ./agent
```

## Licensing Notes

⚠️ **Important**: The skin PNG files extracted from the Minecraft client jar are Mojang's property and subject to Minecraft's EULA and asset usage guidelines. The extraction process is performed at runtime by each user, ensuring compliance with Mojang's licensing terms.

Do not redistribute the extracted skin files. Each installation should download and extract its own copy.

## Performance

- **First run**: Downloads ~50MB client jar, extracts skins (~300KB total for 18 skins)
- **Subsequent runs**: Uses cached files, near-instant initialization
- **Memory usage**: Minimal, skins are loaded on-demand
- **Disk usage**: ~50MB for client jar + ~300KB for extracted skins

## Troubleshooting

### "failed to download client jar"
- Check internet connection
- Verify Mojang's servers are accessible
- Check firewall/proxy settings

### "no skin files found in client jar"
- The downloaded jar may be corrupted
- Try deleting the cache directory and re-downloading
- Verify you're using a valid Minecraft version

### "skins directory not created"
- Check file permissions
- Verify the cache directory path is writable
- Check available disk space

## Integration Example

Here's a complete example of integrating the skin system into a bot:

```go
package main

import (
    "log"
    "github.com/reallyoldfogie/mc-agent/agent"
)

func main() {
    // Initialize skin manager
    skinMgr := agent.NewSkinManager(agent.SkinManagerConfig{
        MinecraftVersion:  "1.21.5",
        CacheRoot:         "./skins",
        AllowNetwork:      true,
        ClientJarCacheDir: "./data/client-cache",
    })

    log.Println("Initializing skin system...")
    if err := skinMgr.Initialize("1.21.5"); err != nil {
        log.Fatalf("Failed to initialize skins: %v", err)
    }

    // List available skins
    skins, _ := skinMgr.ListAvailableSkins()
    log.Printf("Loaded %d skins", len(skins))

    // Use in your bot
    botUUID := [16]byte{...}
    botName := "MyBot"

    // Get a skin for the bot
    skinProps := skinMgr.GetRandomExtractedSkin(botUUID, botName)

    // Use skinProps when connecting to the server
    // (Implementation depends on your bot framework)
}
```
