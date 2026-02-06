# Mod Configurations

This directory allows you to provide Fabric mod configuration files for integration tests. Configurations are automatically mounted by the testing framework, enabling fine-tuned mod behavior during testing.

## Quick Start

### 1. Create a Version Directory

```bash
# For Minecraft 1.21.5
mkdir -p testing/configs/v1_21_5

# For Minecraft 1.21.7
mkdir -p testing/configs/v1_21_7
```

### 2. Add Your Mod Config Files

Organize configs to mirror the Fabric `/data/config` directory structure:

```bash
# Example: Add a mod config file
mkdir -p testing/configs/v1_21_5/modname
cp /path/to/modname.toml testing/configs/v1_21_5/modname/

# Example: Multiple mod configs
testing/configs/v1_21_5/
├── modname1/
│   └── config.toml
├── modname2/
│   └── settings.json
└── global/
    └── shared.conf
```

### 3. Run Tests

```bash
# Tests automatically detect and load configs
./testing/run_tests.sh

# Or run a specific test
./testing/run_tests.sh -t TestWithCustomConfig
```

## Directory Structure

Configs are mounted at `/data/config` in the Docker container, mirroring the standard Fabric server structure:

```
testing/configs/
├── v1_21_5/                          # Minecraft 1.21.5 configs
│   ├── modname1/config.toml          # Mounted to /data/config/modname1/config.toml
│   ├── modname2/settings.json        # Mounted to /data/config/modname2/settings.json
│   └── global/shared.conf            # Mounted to /data/config/global/shared.conf
├── v1_21_7/                          # Minecraft 1.21.7 configs
│   └── ...
└── README.md
```

## Common Config Locations

Most Fabric mods place their configs in subdirectories under `/data/config`:

```
/data/config/
├── modname/                          # Mod-specific config folder
│   ├── modname.toml                  # Main config file
│   └── other-settings.json
├── global/                           # Shared configs
│   └── server.properties
└── advanced/                         # Advanced settings
    └── tweaks.toml
```

## Use Cases

### Mod Behavior Customization

Configure mods to behave differently during tests:

```bash
testing/configs/v1_21_5/
└── protocol-dumper/
    └── config.toml                   # Enable verbose logging, set output path
```

### Test-Specific Settings

Override default mod settings for testing scenarios:

```bash
testing/configs/v1_21_5/
├── feature-mod/
│   └── config.toml                   # Enable experimental features
├── performance-mod/
│   └── settings.json                 # Set lower performance thresholds
└── logging-mod/
    └── loggers.conf                  # Configure detailed logging
```

### Multi-Mod Coordination

Coordinate multiple mods with shared config files:

```bash
testing/configs/v1_21_5/
├── mod-a/
│   └── config.toml                   # Point to shared settings
├── mod-b/
│   └── config.toml                   # Point to shared settings
└── shared/
    └── common-settings.json          # Used by both mods
```

## Version Format

Version directories must match your Minecraft version with underscores instead of dots:

| Minecraft Version | Directory Name |
|---|---|
| 1.21.5 | `v1_21_5` |
| 1.21.7 | `v1_21_7` |
| 1.20.4 | `v1_20_4` |

## Server Data Directory

Configs persist across test runs in the server cache:

```
~/.cache/mc-agent-test/1.21.5/
├── config/                           # Your config files (mounted from testing/configs/v1_21_5/)
├── mods/                             # Mod JAR files
├── minecraft_server.1.21.5.jar       # Server JAR (downloaded once)
├── world/                            # Game world (cleared between tests)
└── logs/
```

## Troubleshooting

### Configs Not Loading

**Check that the path matches Fabric expectations:**

```bash
# Correct: Mod-specific subdirectory
testing/configs/v1_21_5/modname/config.toml
# ↓ Mounted to
/data/config/modname/config.toml

# Incorrect: Config at top level (may not be found by mod)
testing/configs/v1_21_5/modname.toml
```

**Check server logs for config loading:**

```bash
# Look for messages like:
# [modname/INFO]: Loading configuration from /data/config/modname/config.toml
```

### Version Mismatch

Ensure config directories match the test version exactly:

```bash
# Wrong: Test expects 1.21.5, but configs are for 1.21.7
testing/configs/v1_21_7/modname/config.toml  # Won't be loaded

# Right: Match the test version exactly
testing/configs/v1_21_5/modname/config.toml  # Loaded correctly
```

### Missing Mod

If a mod is installed but its config isn't found:

```bash
# Make sure the config directory exists and is named correctly
ls -la testing/configs/v1_21_5/modname/
# Should show the config files

# Verify the mod is in the mods directory
ls -la testing/mods/v1_21_5/modname*.jar
```

## Performance Considerations

- **File Count**: Each config file adds to startup time (minimal impact)
- **File Size**: Large JSON/TOML files are parsed by mods at startup
- **Validation**: Invalid config files may cause mod initialization failures

If tests are slow, check config file count and complexity:

```bash
find testing/configs/v1_21_5 -type f | wc -l
du -sh testing/configs/v1_21_5/
```

## Git Ignore

Config files are automatically ignored by git:

```bash
# These won't be committed
testing/configs/v1_21_5/modname/config.toml
```

This prevents tracking potentially sensitive configuration data.

## Framework Integration

The testing framework automatically:

- Converts version format: `1.21.5` → `v1_21_5`
- Detects config directory existence
- Mounts via Docker bind: `<absolute-path>:/data/config`
- Preserves config files across test runs (cached via server data directory)

## Notes

- The `configs` directory is completely optional - tests work fine without it
- Each Minecraft version can have independent mod configurations
- Configs are git-ignored to prevent tracking sensitive or large files
- Framework is compatible with Fabric mod configurations
- Config directory structure mirrors `/data/config` layout in container
- Mods with no configs simply use their hardcoded defaults
