# Server Output Files

This directory is used to mount server output files into the test container so they can be accessed from the host after test execution.

## Quick Start

1. Create a version-specific subdirectory: `v1_21_8/`
2. Configure server to write output files to `/data/output` in the container
3. Framework automatically mounts this directory at test start
4. Files are accessible in `outputs/v1_21_8/` after server stops

## Directory Structure

```
outputs/
├── v1_21_5/          # 1.21.5 server outputs
├── v1_21_8/          # 1.21.8 server outputs
└── v1_21_X/          # Other versions...
```

## Common Use Cases

### Protocol Dumps
Mount a directory where mods write protocol data:

```bash
mkdir -p outputs/v1_21_8/protocol_dumps
# Server writes to /data/output/protocol_dumps
# Accessible at outputs/v1_21_8/protocol_dumps/
```

### Data Exports
Export game data or test results:

```bash
mkdir -p outputs/v1_21_8/exports
# Server writes to /data/output/exports
```

### Debug Logs
Capture detailed debug information:

```bash
mkdir -p outputs/v1_21_8/debug
# Server writes to /data/output/debug
```

## How It Works

1. **Framework Detection**: When you call `StartServer()`, the framework checks for `outputs/v{version}/`
2. **Automatic Mounting**: If the directory exists, it's mounted to `/data/output` in the container
3. **Docker Bind Mount**: `<host-path>:/data/output`
4. **Version Format**: Dots are replaced with underscores (`1.21.8` → `v1_21_8`)

## Configuration in Mods

Mods can write to `/data/output` which maps to `outputs/v{version}/`:

```java
// Example: protocol-dumper mod
File outputDir = new File("/data/output");
outputDir.mkdirs();
File dumpFile = new File(outputDir, "decode_ops.jsonl");
```

## Git Ignore

The `.gitignore` prevents tracking:
- Version-specific directories (`/v*/`)
- Log files (`*.log`)
- JSON files (`*.json`)
- JSONL files (`*.jsonl`)
- Text files (`*.txt`)

Only the directory structure is tracked, not the generated output files.

## Troubleshooting

**Files not appearing in outputs/**
- Check if `outputs/v1_21_X/` directory exists
- Verify server has write permissions to `/data/output`
- Check server logs for file write errors

**Permission denied errors**
- Docker container runs as UID 1000
- Ensure host directory is readable/writable by the container user
- Check with: `ls -la outputs/v1_21_X/`

**Too many files**
- Output files accumulate from multiple test runs
- Clean up: `rm -rf outputs/v1_21_X/*`
- Or use `TEST_KEEP_SERVER_DATA=` to preserve specific runs

## Performance Notes

- Output mounting adds minimal overhead
- Files are written directly to host filesystem
- Large files (protocol dumps) can be several MB
- No caching of output files between runs (always fresh)

## Advanced Usage

### Custom Output Paths

If you want outputs in a different location, you can specify a custom path when creating the test instance. The automatic mounting uses `outputs/v{version}` by convention.

### Multiple Output Directories

Each version can have its own isolated output directory:
- `outputs/v1_21_5/` - all 1.21.5 outputs
- `outputs/v1_21_8/` - all 1.21.8 outputs
- Subdirectories organize by test type or mod

### Parallel Test Isolation

When running tests in parallel, each creates a separate container with its own mounted output directory, preventing file conflicts.

## Framework Integration

The framework calls `getOutputDir(version)` which:

1. Converts version format (`1.21.8` → `v1_21_8`)
2. Checks if `outputs/v1_21_8/` exists
3. Returns absolute path if found
4. Returns empty string if not found (no mount)
5. Passes to Docker as bind mount: `<path>:/data/output`

See `framework.go` for implementation details.

## Related Documentation

- [Mods README](../mods/README.md) - Mod loading and protocol dumper setup
- [Configs README](../configs/README.md) - Configuration file mounting
- [Testing README](../README.md) - Overview of testing framework
