# Skin Demo

A demonstration program for the mc-agent skin downloading and management system.

## Building

```bash
cd cmd/skin-demo
go build .
```

## Usage

### List all available skins

```bash
./skin-demo -list
```

This will:
1. Download the Minecraft client jar (if not cached)
2. Extract skin files (if not already extracted)
3. List all 18 available skins (9 slim + 9 wide)

### Load a random skin

```bash
./skin-demo
```

This will load a random skin from the extracted skins and display its properties.

### Load a specific skin

```bash
./skin-demo -skin steve -model wide
./skin-demo -skin alex -model slim
./skin-demo -skin sunny -model wide
```

Available skins: alex, ari, efe, kai, makena, noor, steve, sunny, zuri
Available models: slim, wide

### Specify Minecraft version

```bash
./skin-demo -version 1.21.4
```

Default version is 1.21.5.

## Options

- `-version string` - Minecraft version (default "1.21.5")
- `-list` - Only list available skins, don't test loading
- `-skin string` - Load a specific skin by name (e.g., 'steve', 'alex')
- `-model string` - Skin model: 'slim' or 'wide' (default "wide")

## Example Output

```
2025/12/05 07:20:59 Creating skin manager...
2025/12/05 07:20:59 Initializing skin system for Minecraft 1.21.5...
2025/12/05 07:21:01 ✓ Skin system initialized successfully

=== Available Skins (18 total) ===
  • alex (slim) - skins/extracted/slim/alex.png
  • ari (slim) - skins/extracted/slim/ari.png
  • efe (slim) - skins/extracted/slim/efe.png
  ...
  • steve (wide) - skins/extracted/wide/steve.png
  • sunny (wide) - skins/extracted/wide/sunny.png
  • zuri (wide) - skins/extracted/wide/zuri.png

Summary: 9 slim, 9 wide
```

## Files Created

The demo will create the following directories:

- `./skins/` - Skin cache directory
  - `./skins/extracted/` - Extracted skin PNG files
- `./data/client-cache/` - Downloaded Minecraft client jars

These directories are cached and reused on subsequent runs.
