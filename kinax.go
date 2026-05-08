// Package kinax is a pure-Go binding to the macOS Accessibility (AX) API.
//
// kinax lets Go programs read and manipulate the system-wide UI tree —
// inspecting running applications, finding buttons, reading text field
// contents, clicking on elements by semantic identity rather than pixel
// coordinates. It's the foundation for UI automation agents, screen
// readers, and accessibility-aware tools.
//
// # Quick start
//
//	ctx := context.Background()
//	if err := kinax.Load(); err != nil { log.Fatal(err) }
//	if err := kinax.RequireTrust(); err != nil { log.Fatal(err) }
//
//	app, _ := kinax.FocusedApplication()
//	defer app.Close()
//
//	// Walk windows
//	wins, _ := app.Windows()
//	for _, w := range wins {
//	    title, _ := w.Title()
//	    fmt.Println(title)
//	    w.Close()
//	}
//
//	// Click a button by title
//	if btn, ok := app.FindFirst(kinax.MatchTitle("OK")); ok {
//	    btn.Perform(kinax.ActionPress)
//	    btn.Close()
//	}
//
// # Permission
//
// macOS requires the invoking binary to be listed in System Settings →
// Privacy & Security → Accessibility for AX calls to succeed. Without
// permission, every call returns an error (unlike `input-go`, where
// events silently no-op). Use [Trusted] / [PromptTrust] / [RequireTrust].
//
// # Memory ownership
//
// Every Element returned by kinax wraps a retained CFTypeRef. Callers
// MUST call (*Element).Close to release it. Forgetting to Close leaks
// a handle for the lifetime of the process — on the order of tens of
// bytes per element, but it adds up if you're walking the UI tree on
// a timer.
//
// # Dylib placement
//
// kinax-go ships a universal (arm64+x86_64) companion dylib via
// //go:embed. On the first call into the package, the embedded bytes
// are extracted to ~/Library/Caches/kinax-go/<hash>/libkinax_sync.dylib
// and Dlopened. Set [DylibPath] to a non-empty value before the first
// call if you ship a custom-built or patched dylib.
package kinax

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/LocalKinAI/kinax-go/internal/dylib"
	"github.com/ebitengine/purego"
)

// Version is the semantic-version tag of this package.
// Kept in sync with git tags; updated per release.
const Version = "0.4.0"

// DylibPath is an optional override for the location of libkinax_sync.dylib.
// Default (empty): extract embedded copy to cache directory.
var DylibPath = ""

// ─── Dylib handle ────────────────────────────────────────────

var (
	loadOnce sync.Once
	loadErr  error

	axTrustedFn        func(int32) int32
	systemWideFn       func() uintptr
	focusedAppFn       func() uintptr
	appByPIDFn         func(int32) uintptr
	frontmostPIDFn     func() int32
	pidByBundleFn      func(unsafe.Pointer) int32
	elementAtPointFn   func(float64, float64) uintptr
	elementReleaseFn   func(uintptr)
	attrStringFn       func(uintptr, unsafe.Pointer, unsafe.Pointer, int32) int32
	attrIntFn          func(uintptr, unsafe.Pointer, unsafe.Pointer) int32
	attrBoolFn         func(uintptr, unsafe.Pointer, unsafe.Pointer) int32
	attrElementFn      func(uintptr, unsafe.Pointer) uintptr
	attrPointFn        func(uintptr, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int32
	attrSizeFn         func(uintptr, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int32
	attrElementArrayFn func(uintptr, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int32
	attrManyFn         func(uintptr, unsafe.Pointer, unsafe.Pointer, int32) int32

	// AXObserver — push-based UI event subscriptions (v0.3+).
	observerCreateFn      func(int32, unsafe.Pointer, int32) uintptr
	observerSubscribeFn   func(uintptr, uintptr, unsafe.Pointer) int32
	observerUnsubscribeFn func(uintptr, uintptr, unsafe.Pointer) int32
	observerNextFn        func(uintptr, int32, unsafe.Pointer, int32) int32
	observerCloseFn       func(uintptr)
	attributeNamesFn   func(uintptr, unsafe.Pointer, int32) int32
	actionNamesFn      func(uintptr, unsafe.Pointer, int32) int32
	elementPerformFn   func(uintptr, unsafe.Pointer) int32
	elementSetStringFn func(uintptr, unsafe.Pointer, unsafe.Pointer) int32
	elementSetBoolFn   func(uintptr, unsafe.Pointer, int32) int32
)

// Load explicitly loads the companion dylib. Idempotent.
func Load() error {
	loadOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			loadErr = fmt.Errorf("kinax: macOS-only (runtime.GOOS=%s)", runtime.GOOS)
			return
		}
		path, err := resolveDylib()
		if err != nil {
			loadErr = err
			return
		}
		resolvedPathMu.Lock()
		resolvedPath = path
		resolvedPathMu.Unlock()

		h, err := purego.Dlopen(path, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("kinax: dlopen %q: %w", path, err)
			return
		}
		purego.RegisterLibFunc(&axTrustedFn, h, "kinax_ax_trusted")
		purego.RegisterLibFunc(&systemWideFn, h, "kinax_system_wide")
		purego.RegisterLibFunc(&focusedAppFn, h, "kinax_focused_application")
		purego.RegisterLibFunc(&appByPIDFn, h, "kinax_app_by_pid")
		purego.RegisterLibFunc(&frontmostPIDFn, h, "kinax_frontmost_pid")
		purego.RegisterLibFunc(&pidByBundleFn, h, "kinax_pid_by_bundle")
		purego.RegisterLibFunc(&elementAtPointFn, h, "kinax_element_at_point")
		purego.RegisterLibFunc(&elementReleaseFn, h, "kinax_element_release")
		purego.RegisterLibFunc(&attrStringFn, h, "kinax_element_attr_string")
		purego.RegisterLibFunc(&attrIntFn, h, "kinax_element_attr_int")
		purego.RegisterLibFunc(&attrBoolFn, h, "kinax_element_attr_bool")
		purego.RegisterLibFunc(&attrElementFn, h, "kinax_element_attr_element")
		purego.RegisterLibFunc(&attrPointFn, h, "kinax_element_attr_point")
		purego.RegisterLibFunc(&attrSizeFn, h, "kinax_element_attr_size")
		purego.RegisterLibFunc(&attrElementArrayFn, h, "kinax_element_attr_element_array")
		purego.RegisterLibFunc(&attrManyFn, h, "kinax_element_attr_many")
		purego.RegisterLibFunc(&observerCreateFn, h, "kinax_observer_create")
		purego.RegisterLibFunc(&observerSubscribeFn, h, "kinax_observer_subscribe")
		purego.RegisterLibFunc(&observerUnsubscribeFn, h, "kinax_observer_unsubscribe")
		purego.RegisterLibFunc(&observerNextFn, h, "kinax_observer_next")
		purego.RegisterLibFunc(&observerCloseFn, h, "kinax_observer_close")
		purego.RegisterLibFunc(&attributeNamesFn, h, "kinax_element_attribute_names")
		purego.RegisterLibFunc(&actionNamesFn, h, "kinax_element_action_names")
		purego.RegisterLibFunc(&elementPerformFn, h, "kinax_element_perform")
		purego.RegisterLibFunc(&elementSetStringFn, h, "kinax_element_set_string")
		purego.RegisterLibFunc(&elementSetBoolFn, h, "kinax_element_set_bool")
	})
	return loadErr
}

var (
	resolvedPath   string
	resolvedPathMu sync.RWMutex
)

// ResolvedDylibPath returns the path Load used (or would use) to Dlopen
// the dylib. Intended for diagnostics.
func ResolvedDylibPath() string {
	resolvedPathMu.RLock()
	defer resolvedPathMu.RUnlock()
	return resolvedPath
}

func resolveDylib() (string, error) {
	if DylibPath != "" {
		if _, err := os.Stat(DylibPath); err != nil {
			return "", fmt.Errorf("kinax: DylibPath override %q: %w", DylibPath, err)
		}
		return DylibPath, nil
	}
	return extractEmbedded()
}

func extractEmbedded() (string, error) {
	if len(dylib.Bytes) == 0 {
		return "", errors.New("kinax: embedded dylib is empty (build issue — make dylib)")
	}
	h := sha256.Sum256(dylib.Bytes)
	hashPrefix := hex.EncodeToString(h[:8])

	baseCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("kinax: locate cache dir: %w", err)
	}
	cacheDir := filepath.Join(baseCache, "kinax-go", hashPrefix)
	target := filepath.Join(cacheDir, "libkinax_sync.dylib")

	if existing, err := os.ReadFile(target); err == nil && len(existing) == len(dylib.Bytes) {
		return target, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("kinax: mkdir %s: %w", cacheDir, err)
	}
	tmp, err := os.CreateTemp(cacheDir, "libkinax_sync-*.dylib.tmp")
	if err != nil {
		return "", fmt.Errorf("kinax: create tmp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(dylib.Bytes); err != nil {
		cleanup()
		return "", fmt.Errorf("kinax: write dylib: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		cleanup()
		return "", fmt.Errorf("kinax: chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("kinax: close tmp: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("kinax: rename: %w", err)
	}
	return target, nil
}

// ─── Sentinel errors ─────────────────────────────────────────

// ErrNotTrusted is returned when the current process lacks Accessibility
// permission. This is the dominant error shape for kinax — virtually
// every call fails without permission.
var ErrNotTrusted = errors.New("kinax: accessibility permission not granted")

// ErrNotFound is returned when a requested element, attribute, or app
// isn't present. Callers typically want to distinguish this from a
// real error (e.g. permission denied).
var ErrNotFound = errors.New("kinax: not found")

// ErrClosed is returned when a method is called on an Element after Close.
var ErrClosed = errors.New("kinax: element closed")

// ErrInvalidType is returned when an attribute exists but has the wrong
// type for the requested accessor (e.g. calling AttributeInt on a
// string attribute).
var ErrInvalidType = errors.New("kinax: attribute has wrong type")

// ─── Trust helpers ───────────────────────────────────────────

// Trusted returns true if the current process has Accessibility
// permission. Does not prompt.
func Trusted() bool {
	if err := Load(); err != nil {
		return false
	}
	return axTrustedFn(0) == 1
}

// PromptTrust triggers the system "wants to control your computer"
// dialog if permission has not been granted. Returns the resulting
// trust state (true if already granted, false otherwise — including
// the common case where the user hasn't yet responded to the dialog).
func PromptTrust() bool {
	if err := Load(); err != nil {
		return false
	}
	return axTrustedFn(1) == 1
}

// RequireTrust returns nil if Accessibility permission is granted, or
// [ErrNotTrusted] otherwise. Convenience for callers that want a hard
// fail at startup rather than opaque failures deep in a traversal.
func RequireTrust() error {
	if !Trusted() {
		return ErrNotTrusted
	}
	return nil
}
