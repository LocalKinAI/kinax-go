package kinax

import (
	"fmt"
	"unsafe"
)

// SystemWide returns an Element representing the system-wide AX root.
// Useful primarily to query AXFocusedApplication or to hit-test points.
// Caller must Close.
func SystemWide() (*Element, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	h := systemWideFn()
	if h == 0 {
		return nil, fmt.Errorf("kinax: system-wide element unavailable")
	}
	return wrap(h), nil
}

// FocusedApplication returns the Element for the currently active
// (frontmost) application. Returns [ErrNotFound] if there is none —
// this can happen briefly during login screens and app transitions.
// Caller must Close.
func FocusedApplication() (*Element, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	h := focusedAppFn()
	if h == 0 {
		return nil, ErrNotFound
	}
	return wrap(h), nil
}

// ApplicationByPID returns the AX Element for the running application
// with the given Unix PID. Returns a valid Element even if no such
// process exists — subsequent AX calls on it will simply fail.
// Caller must Close.
func ApplicationByPID(pid int) (*Element, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	h := appByPIDFn(int32(pid))
	if h == 0 {
		return nil, ErrNotFound
	}
	return wrap(h), nil
}

// ApplicationByBundleID returns the Element for the first running app
// with the given bundle identifier (e.g. "com.apple.Safari"). Returns
// [ErrNotFound] if no such app is running.
// Caller must Close.
func ApplicationByBundleID(bundleID string) (*Element, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	bid := append([]byte(bundleID), 0)
	pid := pidByBundleFn(unsafe.Pointer(&bid[0]))
	if pid == 0 {
		return nil, fmt.Errorf("%w: bundle %q not running", ErrNotFound, bundleID)
	}
	h := appByPIDFn(pid)
	if h == 0 {
		return nil, fmt.Errorf("%w: pid %d (bundle %q)", ErrNotFound, pid, bundleID)
	}
	return wrap(h), nil
}

// FrontmostPID returns the PID of the currently-active application.
// Returns 0 if none (rare — happens during login and app transitions).
func FrontmostPID() int {
	if err := Load(); err != nil {
		return 0
	}
	return int(frontmostPIDFn())
}

// ElementAtPoint returns the AX element at the given global screen
// coordinates. This is how you implement "inspect element under cursor"
// (hold ⌥ hover over anything → show its AX info). Returns
// [ErrNotFound] if no element is at the point (e.g. on the desktop
// wallpaper in some configurations). Caller must Close.
func ElementAtPoint(x, y float64) (*Element, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	h := elementAtPointFn(x, y)
	if h == 0 {
		return nil, ErrNotFound
	}
	return wrap(h), nil
}

// Windows returns the top-level windows of an app element. Convenience
// for (*Element).AttributeElements(AttrWindows).
func (e *Element) Windows() ([]*Element, error) {
	return e.AttributeElements(AttrWindows)
}

// MainWindow returns the app's main window (usually the frontmost).
// Returns (nil, nil) if the app has no main window right now.
func (e *Element) MainWindow() (*Element, error) {
	return e.AttributeElement(AttrMainWindow)
}

// FocusedWindow returns the app's currently-focused window.
func (e *Element) FocusedWindow() (*Element, error) {
	return e.AttributeElement(AttrFocusedWindow)
}

// FocusedElement returns the element within an app (or window) that
// currently has keyboard focus. For the system-wide element, this is
// the globally-focused UI element across all apps.
func (e *Element) FocusedElement() (*Element, error) {
	return e.AttributeElement(AttrFocusedElement)
}
