# Adding a New Action

A chat command (`moveTo`, `fireBow`, `equip`, ...) is a `models.Action[models.CommandAgent]`
registered in `actions/registry.go`'s `RegisterDefaults`. This document is the checklist for
adding one, including the extra steps needed for it to also be usable by the RL policy
(`rlenv`, `github.com/reallyoldfogie/cRL-go`) rather than just from chat.

## Quick checklist

1. Does the action need a capability `models.CommandAgent` doesn't have yet? If so, add it
   there (or a narrower embedded interface) and implement it on the real agent.
2. Write the `Action` type in `actions/commands.go`.
3. Register it in `actions/registry.go`'s `RegisterDefaults`.
4. Add it to `helpText` in `actions/commands.go`.
5. Add it to CLAUDE.md's command list.
6. *(Optional)* Wire it into `rlenv` if the RL policy should be able to use it.
7. Build, vet, test, gofmt.

## 1. New capability on `models.CommandAgent`?

`actions/commands.go` only ever calls methods on `models.CommandAgent` (`models/command_agent.go`)
— it has no access to the concrete `agent` type. If the action needs something the interface
doesn't expose yet (a new movement primitive, a new query), add the method there first (or to
one of its embedded interfaces — `MovementAgent`, `ChatOperations` — or a new small interface,
if it's conceptually separate, the way `models.HealthProvider` was split out), then implement it
on the real type in `agent/*.go`.

**This is the step that fans out.** `models.CommandAgent` is a single interface, so every Go type
that implements it must grow the new method too, or the build breaks with a clear
`missing method X` compiler error naming exactly what's absent. As of this writing there are
exactly two implementers to update:

- `agent/*.go` — the real implementation.
- `rlenv/fake_agent_test.go`'s `fakeAgent` — a from-scratch test double that exercises the real
  `actions.NewRegistry()` dispatch path end to end (not a partial mock), so it must implement the
  *entire* interface, not just the methods a given test happens to exercise. `rlenv/environment_test.go`
  has a compile-time assertion (`var _ models.CommandAgent = (*fakeAgent)(nil)`) that fails loudly
  if it falls behind. Check `grep -rn "models.CommandAgent = " --include="*.go" .` for any other
  implementers before assuming this list is exhaustive — it can grow.

Skip this step entirely if the action only recombines existing `CommandAgent` methods (most
actions do).

## 2. Write the Action

In `actions/commands.go`:

```go
type YourAction struct{}

func (YourAction) Name() string  { return "youraction" } // lowercased on Register
func (YourAction) Usage() string { return "yourAction <arg>" }
func (YourAction) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	// parse/validate args, agent.SendChat(...) a usage message and
	// return models.Done(nil), nil on bad input (existing convention:
	// invalid *arguments* are reported via chat, not a returned error)
	...
}
```

`Execute` returns `(models.Completion, error)` (`models/completion.go`) — `error` is a
dispatch-time failure, and the `Completion` reports whether the action's actual effect (which
may still be running after `Execute` returns) succeeded:

- **Synchronous action** (the effect finishes before `Execute` returns — most actions):
  return `models.Done(err)`.
- **Action that launches a goroutine** (movement, anything that shouldn't block the chat
  dispatcher): construct one with `models.NewCompletion()`, launch the goroutine, and call the
  returned `resolve` function exactly once with that goroutine's outcome:

  ```go
  completion, resolve := models.NewCompletion()
  go func() {
  	err := agent.SomeLongRunningThing(ctx, ...)
  	resolve(err)
  }()
  return completion, nil
  ```

  This is what lets a caller (`rlenv.Environment.Step`, in particular) `completion.Wait(ctx)` for
  the real "did it finish" signal instead of guessing. See `models/completion.go`'s doc comment.

## 3. Register it

In `actions/registry.go`'s `RegisterDefaults`:

```go
reg.Register(YourAction{})
```

## 4. Update `helpText`

`actions/commands.go`'s `helpText` constant is what the in-chat `help` command prints. Add your
command to it in the same `name <args> - description` style as its neighbors.

## 5. Update CLAUDE.md

CLAUDE.md's "Command System" section lists every command for whoever (human or Claude) is
working in this repo next. Add a line there too — it's a second, separately-maintained list from
`helpText`, so both need updating by hand.

## 6. (Optional) Wire it into `rlenv` for RL usability

Most actions do **not** need this — `rlenv` only exposes a small, deliberately curated subset of
actions to the RL policy (see `rlenv/action.go`'s doc comment and
`docs/plans/RL_POLICY_INTEGRATION_PLAN.md` item 3). Do this only if the new action is something
you want a trained policy to be able to choose.

- `rlenv/action.go`: add an `rl.Action` constant to the `iota` block, which bumps `NumActions`
  automatically.
- Extend the action→dispatch mapping (currently `movementTarget`, which resolves a movement
  action to `moveto` coordinates — a differently-shaped action will need its own resolution
  logic, not necessarily that same function) to cover the new action.
- If step 1 added a new `CommandAgent` method, `rlenv/fake_agent_test.go`'s `fakeAgent` already
  needs a stub for it (see step 1) — nothing extra here beyond that.
- Consider whether `rlenv/observation.go`'s feature vector needs to grow so the policy can
  actually perceive whatever state makes this action meaningful (e.g. an inventory count for a
  `craft` action). Growing `observationSize` changes the fixed contract any already-trained
  checkpoint was trained against — see the Open Questions in
  `docs/plans/RL_POLICY_INTEGRATION_PLAN.md` about goal/observation schema versioning before
  doing this casually.

## 7. Verify

```bash
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test $(go list ./... | grep -v '/testing')   # testing/ is a slow, hours-long integration suite - see testing/README.md
gofmt -l <files you touched>
```

## Streamlining ideas (not implemented)

This process has several manual, easy-to-forget sync points. None of these are done yet; they're
recorded here as concrete options if the friction becomes worth fixing:

- **Generate `helpText` from the registry instead of hand-maintaining it.** `ActionRegistry`
  already holds every registered action's `Name()`/`Usage()`; `Help.Execute` could format its
  output from that at runtime instead of from a separately-maintained string constant. This
  removes step 4 entirely and makes `help`'s output impossible to let drift from what's actually
  registered. Would need `ActionRegistry[T]` to expose an enumeration method (e.g. `List() []Action[T]`),
  which it doesn't today.
- **Have CLAUDE.md and the README's command table point at generated output instead of a second
  hand-copied list.** Once `help`'s output is generated (above), CLAUDE.md/README could say "see
  the `help` command's output" instead of maintaining a third copy of the same list that goes
  stale on its own schedule (this document found CLAUDE.md's list already missing several
  merged commands, and using the name of an interface — `Command` — that doesn't exist).
- **The step-1 interface fan-out is inherent to Go, not really a process bug.** A codegen step
  that scaffolds a blank stub for every implementer when `CommandAgent` gains a method is
  possible but is a fair amount of machinery for what's already a same-day compiler error naming
  the exact missing method and its signature — likely not worth building.
- **A shared arg-parsing helper for the common `<x> <y> <z>` case.** `MoveTo`, `LineTo`,
  `MoveToAndSneak`, `LineToAndSneak`, `FindPath`, `FlyTo`, and part of `FireBowAt` each hand-roll
  the same three-`parseFloat`-calls-plus-per-coordinate-chat-error block. A shared
  `parseXYZArgs(args []string) (x, y, z float64, err error)` wouldn't remove a step from this
  checklist, but it's a real, low-risk simplification of step 2 worth doing independently.
