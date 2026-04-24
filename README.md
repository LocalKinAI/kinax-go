# kinax-go

Pure-Go binding to the macOS Accessibility (AX) API.

Navigate and manipulate the system-wide UI tree — inspect running
applications, find buttons by semantic identity, read text field
contents, click elements without hardcoded pixel coordinates.

Single binary, zero `cgo`, `go install`-able. Built on `AXUIElement`
via `purego` + an embedded companion dylib — the same pattern as
[`sckit-go`](https://github.com/LocalKinAI/sckit-go),
[`kinrec`](https://github.com/LocalKinAI/kinrec), and
[`input-go`](https://github.com/LocalKinAI/input-go).

`kinax-go` is part of **KinKit** — the pure-Go macOS system library
family powering the [LocalKin](https://localkin.ai) agent swarm. It
pairs with `input-go`: `kinax-go` *sees* the UI, `input-go` *moves*
it.

```bash
go install github.com/LocalKinAI/kinax-go/cmd/kinax@latest

kinax tree --bundle com.apple.Safari --depth 4
kinax find AXButton --bundle com.apple.Safari
kinax click "Continue"
kinax at-point 200 100
```

## Features

- **Element tree navigation** — `Children`, `Parent`, `Windows`,
  `FocusedWindow`, `FocusedElement`, `AttributeElement`.
- **Typed attribute readers** — `Attribute` (string), `AttributeInt`,
  `AttributeBool`, `AttributePoint` (CGPoint), `AttributeSize` (CGSize),
  `AttributeElements` (array).
- **Attribute + action introspection** — `AttributeNames`,
  `ActionNames` return live lists from the element.
- **Semantic search** — `FindFirst` / `FindAll` with composable
  matchers (`MatchRole`, `MatchTitle`, `MatchTitleContains`,
  `MatchIdentifier`, `MatchAll`, `MatchAny`).
- **Hit testing** — `ElementAtPoint(x, y)` returns whatever AX
  element is at the given global coords (what Accessibility Inspector
  shows when you hover).
- **App targeting** — `FocusedApplication`, `ApplicationByPID`,
  `ApplicationByBundleID`, `SystemWide`.
- **Action + write** — `Perform("AXPress")`, `SetString`, `SetBool`.
- **No cgo**: downstream projects stay pure Go. The ObjC companion
  dylib is `//go:embed`ded (~70 KB universal arm64+x86_64) and
  extracted to `~/Library/Caches` on first call.

## Install

```bash
# CLI
go install github.com/LocalKinAI/kinax-go/cmd/kinax@latest

# Library
go get github.com/LocalKinAI/kinax-go
```

Requires macOS 12+ and Go 1.22+.

## Permission

macOS requires the invoking binary to be listed in
**System Settings → Privacy & Security → Accessibility** for `AX*`
calls to succeed. Unlike `input-go` (which silently no-ops without
permission), `kinax-go` returns real errors — every accessor will
fail until permission is granted.

```go
if err := kinax.RequireTrust(); err != nil {
    kinax.PromptTrust() // shows system dialog
    log.Fatal("grant Accessibility permission, then rerun")
}
```

## Library usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/LocalKinAI/kinax-go"
)

func main() {
    if err := kinax.Load(); err != nil {
        log.Fatal(err)
    }
    if err := kinax.RequireTrust(); err != nil {
        log.Fatal(err)
    }

    // Attach to Safari (must be running)
    app, err := kinax.ApplicationByBundleID("com.apple.Safari")
    if err != nil { log.Fatal(err) }
    defer app.Close()

    // Walk all windows
    wins, _ := app.Windows()
    for _, w := range wins {
        t, _ := w.Title()
        fmt.Println("window:", t)
        w.Close()
    }

    // Find every text field, print its value
    fields := app.FindAll(kinax.MatchRole(kinax.RoleTextField), 30)
    for _, f := range fields {
        v, _ := f.Value()
        fmt.Println("field:", v)
        f.Close()
    }

    // Click the "New Tab" button
    if btn, ok := app.FindFirst(kinax.MatchTitle("New Tab"), 20); ok {
        defer btn.Close()
        btn.Perform(kinax.ActionPress)
    }
}
```

## CLI usage

```bash
# Dump the AX tree of the focused app (default target)
kinax tree --depth 4

# Dump a specific app's tree
kinax tree --bundle com.apple.Safari --depth 5
kinax tree --pid 1234

# Inspect one attribute of the app element
kinax attr AXTitle --bundle com.apple.Safari

# List every attribute or action the app exposes
kinax attrs --focused
kinax actions --focused

# Find every button (optionally with a specific title)
kinax find AXButton --bundle com.apple.Safari
kinax find AXButton "New Tab" --bundle com.apple.Safari --depth 25

# Click a button by title
kinax click "Continue"
kinax click "OK" --role AXButton --bundle com.apple.SystemPreferences

# Hit-test a screen coordinate
kinax at-point 200 100
# → AXMenuBar    Apple

# Permission
kinax trust              # prints 1 or 0
kinax trust --prompt     # shows system dialog
```

## Memory ownership

Every `*Element` returned by `kinax-go` wraps a retained
`CFTypeRef`. The caller **must** call `(*Element).Close` — forgetting
to leaks a handle for the process lifetime.

Traversal helpers (`FindFirst`, `FindAll`) return fresh handles; any
siblings they walk past are closed automatically. You only need to
close what's returned to you.

```go
// Correct
wins, _ := app.Windows()
for _, w := range wins {
    defer w.Close()
}

// Also correct — find returns a fresh handle
if btn, ok := app.FindFirst(kinax.MatchTitle("OK"), 20); ok {
    defer btn.Close()
    btn.Perform(kinax.ActionPress)
}
```

## Combining with input-go

`kinax` finds the element; `input` drives the cursor there. This is
the natural UI-automation loop:

```go
import (
    "github.com/LocalKinAI/kinax-go"
    "github.com/LocalKinAI/input-go"
)

btn, _ := app.FindFirst(kinax.MatchTitle("Save"), 20)
defer btn.Close()

pos, _ := btn.Position()
size, _ := btn.Size()
cx := float64(pos.X + size.X/2)
cy := float64(pos.Y + size.Y/2)

input.MoveSmooth(ctx, cx, cy, 300*time.Millisecond)
input.ClickAt(ctx, cx, cy)
```

For many cases `btn.Perform(kinax.ActionPress)` is enough — clicking
via AX is faster and doesn't require Accessibility permission on the
binary that runs `input.Click` (though it does require it on the one
that runs `kinax.Perform`). Use `input-go` for pixel-level drag
gestures, hover highlights, and scrolling.

## How it works

`kinax-go` follows the **embedded dylib pattern** documented in
Paper #9 of [localkin.dev/papers](https://www.localkin.dev/papers/embedded-dylib).

```
Go code  ─── purego.Dlopen ────► libkinax_sync.dylib (embedded)
                                     │
                                     └──► AXUIElement* APIs
```

- `objc/kinax_ax.m` — ~450 LOC ObjC shim exposing 20 C-ABI functions
  (`kinax_element_attr_string`, `kinax_element_perform`, etc.).
- `internal/dylib/libkinax_sync.dylib` — universal Mach-O, committed.
- Opaque `uintptr` handles for elements; `CFRetain` on the ObjC side,
  `CFRelease` via `kinax_element_release` when Go Closes.
- JSON encoding for list attributes (attribute names, action names) —
  avoids shipping a full CF→Go type system across the FFI.

## Known limitations (v0.1)

- **macOS only.** The Accessibility API is macOS-specific — no
  cross-platform ambitions.
- **Read-heavy API.** Writing is limited to string and bool attribute
  sets. CGPoint/CGSize/CGRect AXValue setters (e.g. to move a window
  programmatically) are deferred to v0.2.
- **No observers / notifications.** `AXObserverRef` +
  `kAXFocusedWindowChangedNotification` etc. are planned for v0.2 —
  they enable agents to *react* to UI changes rather than polling.
- **Numeric attributes only as int64.** Float-valued attributes
  (`AXValue` on sliders) currently string-stringify. Dedicated
  `AttributeFloat` planned for v0.2.
- **Single main thread assumption** for some CF calls. In practice
  `kinax` works from any goroutine because we don't use CFRunLoop.
- **Tested only on macOS 26.3 arm64** so far; Intel + macOS 14/15
  verification pending CI.

## Roadmap

- **v0.2** — AXObserver subscription (`OnNotification` callback), more
  typed setters (`SetPoint`, `SetSize`), `AttributeFloat`.
- **v0.3** — helpers for common idioms: `WaitForWindow(bundleID, title, timeout)`,
  `TypeInFieldLabeled(app, label, text)`, "did the UI change" snapshots.
- **Cross-app automation recipes** — Safari URL read, screenshot a
  specific window via sckit-go + AXFrame, etc.

## Contributing

```bash
git clone https://github.com/LocalKinAI/kinax-go
cd kinax-go
make dylib            # rebuild universal Mach-O after ObjC changes
make test             # unit tests (no Accessibility permission needed)
make test-integration # requires Accessibility permission
make lint             # go vet + staticcheck + golangci-lint
```

## License

MIT. See `LICENSE`.

## See also

- [`sckit-go`](https://github.com/LocalKinAI/sckit-go) — ScreenCaptureKit (screen pixels).
- [`kinrec`](https://github.com/LocalKinAI/kinrec) — screen + audio recorder.
- [`input-go`](https://github.com/LocalKinAI/input-go) — mouse + keyboard synthesis.
- [Embedded Dylib paper](https://www.localkin.dev/papers/embedded-dylib) — the architectural pattern.
