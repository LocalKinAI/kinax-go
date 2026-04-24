# Changelog

All notable changes to kinax-go are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning: [SemVer](https://semver.org/spec/v2.0.0.html).

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
