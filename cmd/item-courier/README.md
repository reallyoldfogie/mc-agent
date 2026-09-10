# item-courier

Logs one shared player identity into an arbitrary number of Minecraft servers running
[`mc-item-transfer-mod`](https://github.com/reallyoldfogie/mc-item-transfer-mod) simultaneously, and
relays that mod's `item_transfer:*` plugin messages between whichever pair a given transfer names.
See `../../docs/plans/ITEM_TRANSFER_COURIER_PLAN.md` for the design and
`mc-item-transfer-mod/docs/protocol.md` for the wire protocol this implements the Go side of.

This courier never inspects the item passport (JWT) it relays — trust and item verification are
entirely the mod's job, on each server. This document is about wiring the two sides together.

## 1. Trust setup (on the Minecraft servers, not here)

Every server running `mc-item-transfer-mod` has its own Ed25519 identity and a `serverId` (in that
server's `config/item_transfer/config.json`, auto-generated as an obvious placeholder on first
boot — rename it to something meaningful before going further). Two servers must exchange public
keys before either will accept a transfer from the other. As an op on **each** server:

```
/itemtransfer pubkey
```

prints that server's own `serverId` and base64 public key. Then, on **each** server, trust the
*other* one:

```
/itemtransfer trust add <the other server's serverId> <the other server's pubkey>
```

Trust must be added in both directions — Server A trusting Server B does not imply the reverse.
Verify with `/itemtransfer trust list` on each side. (`/itemtransfer denylist add/remove/list` and
`/itemtransfer status` are also available — see that mod's own command reference.)

**The `serverId` values from this step are exactly the `label`s this courier's config uses below —
not an independently invented local name.** If Server A's `serverId` is `survival-main`, this
courier's config calls it `survival-main` too.

## 2. Courier config (here)

`item-courier` reads the same unified `mc-agent` config as `cmd/agent` (JSON, see
`../../config/settings.go`) — `-config path/to/config.json`. Two sections matter:

- `connection`/`auth`: the single player identity every server sees (name/uuid/offline/token,
  Microsoft auth cache settings) — see `../../configs/config.example.json`.
- `courier.servers`: the list of servers to connect to, one entry per `{label, address, version}`.
  `version` can be left empty to auto-detect. `label` must match that server's mod-side `serverId`
  from step 1.

Example (two servers — see `../../configs/config.example.json` for the full file including
`connection`/`auth`):

```json
{
  "connection": { "name": "ItemCourierBot", "offline": true },
  "courier": {
    "servers": [
      { "label": "survival-main", "address": "127.0.0.1:25565", "version": "" },
      { "label": "creative-vault", "address": "127.0.0.1:25566", "version": "" }
    ]
  }
}
```

Both servers must run a build of `mc-item-transfer-mod` new enough to answer
`item_transfer:handshake` with a matching `courier.ProtocolVersion` (see that constant in
`../../courier/courier.go`) — a mismatch is logged as a warning, not a hard failure, but a transfer
against a mismatched pair should be expected to misbehave.

## 3. Running it

```bash
go run ./cmd/item-courier -config path/to/config.json
```

On startup it logs into every configured server, exchanges `item_transfer:handshake` with each
(warning if any doesn't reply — check that mod is actually installed and running there), and
registers a `transfer` command on every one of them. It then idles until `Ctrl+C`.

## 4. Triggering a transfer

Three equivalent ways, per `docs/plans/ITEM_TRANSFER_COURIER_PLAN.md`'s Open Question 1 (all three
dispatch through the same underlying `courier.Courier.StartTransfer`):

- **Chat**, on either connected server, addressed to the courier's own bot name (the existing
  `>>>botName<<<` convention from `../../agent/commands.go`):
  ```
  >>>ItemCourierBot<<<transfer 5 creative-vault
  ```
  moves the item in the player's main-inventory slot 5 on whichever server that chat message was
  sent from, to `creative-vault`. The bot replies in chat when the transfer starts, completes, or
  fails.
- **Direct API**: call `courier.Courier.StartTransfer(ctx, sourceLabel, destLabel, slotID)` from any
  Go program that constructs a `courier.Courier` the way this command's `main.go` does.
- **RL/LLM executor**: register `courier.TransferAction` into whatever
  `models.ActionRegistry[models.CommandAgent]` your executor drives (e.g. the same registry
  `cmd/rl-train` builds via `actions.NewRegistry()`) — it dispatches identically to the chat path.

## Known limitations (see the plan for details)

- Only main-inventory + hotbar slots (`0`-`35`) are supported — no armor/offhand.
- Concurrent transfers *from the same source server* are correlated by a FIFO assumption
  (`courier/courier.go`'s `pendingRequest` doc comment) not yet verified against a real server under
  load — fine for one-at-a-time manual testing, worth confirming before relying on high concurrency.
- No crash-recovery: an in-flight transfer is lost if this process is killed mid-transfer (the mod
  side's own staging-timeout/return-to-owner safety net still applies on each server independently).
