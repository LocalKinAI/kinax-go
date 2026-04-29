# Changelog

All notable changes to kinax-go are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning: [SemVer](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-04-29

The headline addition is **push-based AX event subscriptions via
`Observer`**. Wraps `AXObserverCreate` + `AXObserverGetRunLoopSource`
+ `CFRunLoopRun` so callers can subscribe to UI changes (focus
moves, value edits, window creates) and consume them via a Go
channel — instead of polling the AX tree every N ms looking for
deltas. The pattern is a direct port of AXSwift's `Observer` shape
done right (worker thread + condvar queue + clean Close). 5-claw
agents that previously had to re-`ui tree` after every input action
to "see what changed" can now drive off real signals.

### Added — `kinax.Observer`

```go
obs, err := kinax.NewObserver(pid)
if err != nil { ... }
defer obs.Close()

obs.Subscribe(app,
    kinax.NotifFocusedWindowChanged,
    kinax.NotifWindowCreated)

events := obs.Events(ctx, 100*time.Millisecond)
for ev := range events {
    fmt.Println(ev.Notification, ev.Element)
    ev.Element.Close()  // caller owns the freshly-CFRetain'd handle
}
```

API:

- `NewObserver(pid int) (*Observer, error)` — creates observer +
  spins a dedicated worker thread running CFRunLoopRun.
- `(o) Subscribe(elem, notifications ...string)` — register
  subscriptions; aggregates errors so one bad name doesn't abort
  the rest.
- `(o) Unsubscribe(elem, notification)` — remove a subscription.
- `(o) Next(timeout time.Duration) (*Event, error)` — block-with-
  timeout for the next event. Returns `ErrObserverTimeout` when
  empty.
- `(o) Events(ctx, pollInterval) <-chan Event` — goroutine-friendly
  streaming wrapper.
- `(o) Close()` — stop the worker, drain pending events, free
  AXObserver. Idempotent.

`Event` carries the notification name, the element it fired on
(freshly CFRetain'd, caller's responsibility to `Close()`), and a
timestamp.

Constants for the common notifications: `NotifFocusedWindowChanged`,
`NotifFocusedUIElementChanged`, `NotifValueChanged`,
`NotifTitleChanged`, `NotifWindowCreated`, `NotifMenuOpened`,
`NotifApplicationActivated`, ...

### Implementation

ObjC side appended to `objc/kinax_ax.m` (no new file — same dylib):

- `kinax_observer_create(pid, ...)` — spawns a `pthread` whose entry
  function calls `AXObserverCreate`, attaches the run-loop source
  to that thread's CFRunLoop, then `CFRunLoopRun()`'s.
- AX callback on the worker pushes notification + retained element
  + timestamp onto a FIFO guarded by `pthread_mutex_t` +
  `pthread_cond_t`.
- `kinax_observer_next` blocks the caller's thread on
  `pthread_cond_timedwait` until an event lands or timeout fires.
- `kinax_observer_close` sets the runloop to stop (with
  `CFRunLoopWakeUp` so it takes effect on idle), `pthread_join`'s,
  drains the queue (releasing leftover elements), frees everything.

Startup synchronization: a separate `pthread_cond_t` lets
`kinax_observer_create` block until the worker thread has either
successfully created the AXObserver or failed (with a copyable
error string). No polling sleep loop.

### Tests — `observer_test.go`

10 cases covering the pure-Go bits:

- `NewObserver` rejects pid<=0
- `Close` is nil-safe and idempotent
- `Subscribe` / `Unsubscribe` / `Next` on closed observer return
  `ErrObserverClosed`
- `parseEvent` handles valid JSON, NUL-terminated buffers, malformed
  input, empty buffers
- Notification name constants are stable (drift = silent
  subscription no-ops)

End-to-end (real AX event from a real app) needs TCC + a running
target process — exercise manually with `cmd/kinax` examples.

### Why this matters

Every AX-driving agent has been polling: dump the tree, diff,
react, repeat at 200-500ms cadence. That's wasted IPC and a
~half-second floor on responsiveness. Observer flips it to push:
when the user's focused window changes, your agent gets a signal
in single-digit milliseconds. KinClaw v1.7+ uses this for "agent
reacts to user activity" workflows that were structurally
impossible before.

Pairs naturally with v0.2's `Element.GetMany`: Observer tells you
*when* something changed, GetMany lets you cheaply re-fetch the
relevant attributes when it does.

### Threading note (still binding)

Apple AX is main-thread-only per
[Forums #94878](https://developer.apple.com/forums/thread/94878).
The dylib enforces this internally via the dedicated worker thread
per Observer; the AX callback fires on that thread, never on
caller's goroutine. Go callers can use Observer from any goroutine
safely — synchronization happens inside the dylib.

## [0.2.0] - 2026-04-28

The headline addition is **`Element.GetMany`** — batch attribute fetch
backed by `AXUIElementCopyMultipleAttributeValues`. This is the
biggest single performance win available in the macOS AX API: a tree
dump that previously paid an IPC round-trip per (node × attribute)
pair now pays one per node. Measured 2-5× speedup on dense Electron
/ iWork apps. Pattern harvested from AXSwift's `getMultipleAttributes`
during the cross-language survey done for KinClaw 2026-04-28.

### Added

#### `Element.GetMany(attrs ...string) (map[string]any, error)`

```go
attrs, err := el.GetMany(
    kinax.AttrRole,
    kinax.AttrTitle,
    kinax.AttrEnabled,
    kinax.AttrPosition,
)
// attrs == map[string]any{
//   "AXRole": "AXButton",
//   "AXTitle": "Save",
//   "AXEnabled": true,
//   "AXPosition": "CGPoint {x=412, y=85}",  // stringified, see note below
// }
```

What you get:

- **One IPC round-trip** for the whole batch instead of N. The
  Apple AX API was always synchronous-blocking IPC under the hood;
  collapsing N calls into 1 is the real per-node speedup.
- **Atomicity within the batch**. The fetched values are a coherent
  snapshot of the element at one point in time, not a sequence of
  N reads with arbitrary state changes between them.
- **Missing/unsupported attributes simply absent from the map**.
  No special "not present" sentinel — caller checks
  `if v, ok := attrs[name]; ok`.

What `GetMany` does NOT return:

- **Element-valued attributes** (`AXChildren`, `AXMainWindow`,
  `AXFocusedWindow`, `AXParent`). The C-level multi-fetch can't
  return handle-typed values usefully — they'd lose ownership
  semantics. Use [Element.AttributeElement] /
  [Element.AttributeElements] for those.
- **Structured AXValue** (point/size/rect/range) come back as
  descriptions ("CGPoint {x=0, y=0}"). Use [Element.AttributePoint]
  / [Element.AttributeSize] for typed access.

Implementation: ObjC side calls
`AXUIElementCopyMultipleAttributeValues(el, names, 0, &values)` —
options=0 means "don't stop on error", so the returned array has
the same count as the request, with errored slots marked as
`AXValue` of type `kAXValueAXErrorType`. The shim filters those
out, stringifies CFString / CFNumber / CFBoolean to JSON-native
shapes, and serializes the result. The Go side `json.Unmarshal`s
into `map[string]any` — strings become Go strings, booleans stay
booleans, numbers become `float64` (JSON numeric default).

### Performance

Indicative numbers (kinax-go integration test, MacBook Pro M3,
populated Cursor window, single window subtree only):

| Op                              | v0.1.0   | v0.2.0   | Speedup |
|---------------------------------|----------|----------|---------|
| Tree dump 7 attrs × ~400 nodes  | ~280 ms  | ~70 ms   | 4.0×    |
| Tree dump 4 attrs × ~150 nodes  | ~70 ms   | ~22 ms   | 3.2×    |
| Single-attribute reads          | unchanged| unchanged| —       |

(`GetMany` only helps when fetching multiple attributes per node;
single-attribute paths still go through the existing per-attr
entry points unchanged.)

### Why this matters

Tree dump is the hottest path in any AX-driving agent — it's how
the agent figures out "what's on screen right now." Faster dump
means faster planning (fewer round-trips per turn), and a faster
fallback path when the agent's first guess at a UI element doesn't
match (the next dump is cheaper to re-do). For KinClaw's `ui tree`
action specifically, this is a 1-3 second → 300-600ms swing on
heavy apps, which compounds across the multiple `ui tree` calls
the pilot makes per turn.

The pattern also makes the `ui` claw competitive with vision-based
fallbacks at finer time scales: each saved second is a second the
agent doesn't need to fall back to vision (which costs tokens).

## [0.1.0] - 2026-04-23

Initial release. Pure-Go binding to the macOS Accessibility (AX) API
via `AXUIElement` + purego + an embedded ObjC companion dylib.
Single binary, `go install`, no cgo required downstream. Part of
KinKit alongside [sckit-go](https://github.com/LocalKinAI/sckit-go),
[kinrec](https://github.com/LocalKinAI/kinrec), and
[input-go](https://github.com/LocalKinAI/input-go).

### Added

#### Element tree navigation
- `kinax.SystemWide()` — root element for hit-testing + global focus.
- `kinax.FocusedApplication()` — currently active app.
- `kinax.ApplicationByPID(pid)` / `kinax.ApplicationByBundleID(id)` —
  attach to a specific app.
- `kinax.FrontmostPID()` — PID of the foreground app.
- `kinax.ElementAtPoint(x, y)` — hit-test a global screen coordinate.
- `(*Element).Children()` / `(*Element).Parent()` —
  tree walk.
- `(*Element).Windows()` / `(*Element).MainWindow()` /
  `(*Element).FocusedWindow()` — app window navigation.
- `(*Element).FocusedElement()` — the UI element with keyboard focus.

#### Attribute readers (typed)
- `Attribute(name) (string, error)` — string-valued attributes.
- `AttributeInt(name) (int64, error)` — number-valued.
- `AttributeBool(name) (bool, error)` — CFBoolean and bool-int fallback.
- `AttributeElement(name) (*Element, error)` — nested AX elements.
- `AttributeElements(name) ([]*Element, error)` — AXChildren, AXWindows etc.
- `AttributePoint(name) (image.Point, error)` — AXValue CGPoint
  (e.g. AXPosition).
- `AttributeSize(name) (image.Point, error)` — AXValue CGSize
  (e.g. AXSize).

#### Convenience wrappers
- `(*Element).Role()` / `.Subrole()` / `.Title()` / `.Description()` /
  `.Value()` / `.Identifier()` / `.Enabled()` / `.Focused()` /
  `.Position()` / `.Size()`.

#### Introspection + action
- `(*Element).AttributeNames() ([]string, error)` —
  via JSON-serialized `AXUIElementCopyAttributeNames`.
- `(*Element).ActionNames() ([]string, error)` —
  via `AXUIElementCopyActionNames`.
- `(*Element).Perform(action string) error` — fire AXPress / etc.
- `(*Element).SetString(attr, value) error` — write string attr
  (e.g. AXValue on a text field).
- `(*Element).SetBool(attr, value) error` — e.g. AXFocused=true.

#### Semantic search
- `Matcher` type with composable combinators: `MatchAll`, `MatchAny`.
- Built-in matchers: `MatchRole`, `MatchTitle`, `MatchTitleContains`,
  `MatchIdentifier`, `MatchRoleAndTitle`.
- `(*Element).FindFirst(matcher, maxDepth)` — depth-first search
  returning a fresh handle (caller Closes).
- `(*Element).FindAll(matcher, maxDepth)` — collect every match.
- Traversal auto-Closes non-matching siblings so callers never leak
  handles during search.

#### Constants (ABI stability gates)
- **Attributes** — 30+ constants covering role/title/value/position/
  size/children/windows/focused-element and more.
- **Roles** — 30+ constants for the common AX roles (application,
  window, button, text field, menu, ...).
- **Actions** — 10 constants (AXPress, AXShowMenu, AXIncrement, ...).

#### Trust helpers
- `kinax.Trusted()` — probe Accessibility permission without prompting.
- `kinax.PromptTrust()` — show system dialog on first use.
- `kinax.RequireTrust()` — convenience: returns `ErrNotTrusted` if
  permission is missing, for hard-fail startup.

#### Packaging
- `//go:embed` universal dylib (arm64 + x86_64, ~70 KB) —
  downstream users never need `clang` / `CGO_ENABLED` / a C toolchain.
- Auto-extracts to `~/Library/Caches/kinax-go/<content-hash>/` on
  first call.
- `DylibPath` override for contributors shipping patched dylibs.
- `ResolvedDylibPath()` for diagnostics.

### CLI — `cmd/kinax`

- `kinax tree [--pid N | --bundle ID | --focused] [--depth N]` —
  indented AX tree dump with role/title/identifier per line.
- `kinax attr ATTR [--target]` — read one attribute's string value.
- `kinax attrs [--target]` — list attribute names.
- `kinax actions [--target]` — list action names.
- `kinax find ROLE [TITLE] [--target] [--depth N]` — tab-separated
  listing.
- `kinax click TITLE [--role ROLE] [--target]` — find element by
  title and fire AXPress. Exits 1 if not found, 2 on other error.
- `kinax at-point X Y` — hit-test a global coordinate, print
  role + title.
- `kinax trust [--prompt]` — permission check.
- `kinax version` — semver + Go toolchain + resolved dylib path.

### Dylib / ObjC
- 20 exported C-ABI functions covering trust, root/app lookup, element
  release, typed attribute readers, element-array reader, attribute
  and action name listing (JSON-encoded), perform, and setters.
- **Opaque handle model**: every AXUIElement is a `uintptr` held with
  a CFRetain on the ObjC side, released via `kinax_element_release`.
  Prevents ObjC-lifecycle confusion across the purego boundary.
- **JSON-encoded list attributes** — attribute names and action names
  ship as compact JSON string arrays, decoded Go-side. Avoids shipping
  a full CF→Go type system across the FFI.
- **Frontmost-app lookup via NSWorkspace** — faster and more reliable
  than AXUIElementCopyAttributeValue(system, AXFocusedApplication).

### Tests
- **Unit tests** (no dylib, no permission): Version set,
  sentinel-error distinctness (`ErrNotTrusted` / `ErrNotFound` /
  `ErrClosed` / `ErrInvalidType`), attribute / role / action constant
  AX-prefix check, matcher combinator short-circuit semantics (AND/OR
  + vacuous empty cases), closed-element method returns `ErrClosed`
  without touching the dylib, idempotent Close via atomic
  CompareAndSwap.
- **Integration tests** (build-tag `integration`, requires
  Accessibility permission + a running app): Load + ResolvedDylibPath
  populated, FrontmostPID positive, FocusedApplication returns
  AXApplication role, ApplicationByPID round-trip, SystemWide +
  FocusedElement, AttributeNames includes AXRole, FindFirst finds a
  button, ElementAtPoint hit-tests the menu bar, RequireTrust returns
  sentinel error.
- `go vet` + `staticcheck` + `golangci-lint`: **0 warnings**.

### Documentation
- `README.md` — install, permission model, library API,
  CLI reference, memory-ownership rules, combining with input-go,
  roadmap.
- `CHANGELOG.md` — this file.

### Known limitations

- macOS only.
- Write API is string + bool only (no CGPoint/CGSize setters yet).
- No AXObserver / notifications (planned v0.2).
- Float-valued attributes stringify; no dedicated `AttributeFloat` yet.
- Tested only on macOS 26.3 arm64 so far; Intel + macOS 14/15 pending CI.
