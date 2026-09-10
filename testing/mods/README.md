# Mod JARs for integration tests

This directory lets integration tests run a real Fabric server with specific mods installed.
Framework.StartServer auto-mounts `testing/mods/v<version>/*.jar` to the container's `/data/mods`
when `cfg.Version == <version>` (see `getModsDir` in `framework.go`). Like `testing/configs/`, this
whole directory is git-ignored except this README and `.gitignore` — populate it locally, it's never
committed.

## Version directory naming

Same convention as `testing/configs/`: dots become underscores, prefixed with `v`.

| Minecraft Version | Directory Name |
|---|---|
| 1.21.11 | `v1_21_11` |
| 1.21.5 | `v1_21_5` |

## `TestTransferItemAcrossServers` (`transfer_item_test.go`)

Needs `testing/mods/v1_21_11/` populated with **two** jars — Fabric API is a separate
`modImplementation` dependency of `item-transfer-mod`, not bundled into its jar, so a real server
needs both:

1. **The mod itself** — build it from the sibling `mc-item-transfer-mod` repo:
   ```bash
   cd ../../mc-item-transfer-mod
   go run ./cmd/mc-mod-gen -versions 1.21.11
   cp dist/1.21.11/item-transfer-mod-*.jar ../mc-agent/testing/mods/v1_21_11/
   ```
2. **A matching Fabric API jar** — the exact version string is in that repo's `mc-mod-gen.yaml`
   (e.g. `0.141.6+1.21.11` at time of writing). If you've already built the mod once, Gradle already
   downloaded it — reuse that instead of fetching it again:
   ```bash
   find ~/.gradle/caches/modules-2/files-2.1/net.fabricmc.fabric-api/fabric-api \
     -name "fabric-api-*.jar" ! -name "*-sources.jar"
   cp <the jar that command finds> ../mc-agent/testing/mods/v1_21_11/
   ```

Do **not** also populate `testing/configs/v1_21_11/` for this specific test — see
`transfer_item_test.go`'s file doc comment for why (two server instances of the same version need
independent identities/config, which that test achieves via `ServerConfig.CacheDir` instead).
