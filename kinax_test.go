package kinax

import (
	"errors"
	"testing"
)

func TestVersionSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}

func TestSentinelErrorsDistinct(t *testing.T) {
	cases := []struct {
		name string
		a, b error
	}{
		{"NotTrusted vs NotFound", ErrNotTrusted, ErrNotFound},
		{"NotFound vs Closed", ErrNotFound, ErrClosed},
		{"Closed vs InvalidType", ErrClosed, ErrInvalidType},
		{"NotTrusted vs Closed", ErrNotTrusted, ErrClosed},
	}
	for _, c := range cases {
		if errors.Is(c.a, c.b) {
			t.Errorf("%s: must be distinct", c.name)
		}
		if errors.Is(c.b, c.a) {
			t.Errorf("%s: must be distinct (reverse)", c.name)
		}
	}
}

func TestAttributeConstants(t *testing.T) {
	// These are the stable AX attribute strings from Apple's
	// AXAttributeConstants.h — regression-guard against typos.
	cases := map[string]string{
		"AttrRole":           AttrRole,
		"AttrTitle":          AttrTitle,
		"AttrValue":          AttrValue,
		"AttrChildren":       AttrChildren,
		"AttrWindows":        AttrWindows,
		"AttrFocusedWindow":  AttrFocusedWindow,
		"AttrFocusedElement": AttrFocusedElement,
		"AttrPosition":       AttrPosition,
		"AttrSize":           AttrSize,
		"AttrEnabled":        AttrEnabled,
	}
	for name, val := range cases {
		if val == "" || val[:2] != "AX" {
			t.Errorf("%s = %q; expected AX-prefixed string", name, val)
		}
	}
}

func TestRoleConstants(t *testing.T) {
	roles := []string{
		RoleApplication, RoleWindow, RoleButton, RoleTextField,
		RoleStaticText, RoleMenu, RoleMenuItem, RoleList,
	}
	for _, r := range roles {
		if r == "" || r[:2] != "AX" {
			t.Errorf("role constant %q must be AX-prefixed", r)
		}
	}
}

func TestActionConstants(t *testing.T) {
	actions := []string{
		ActionPress, ActionIncrement, ActionDecrement,
		ActionConfirm, ActionCancel, ActionShowMenu,
	}
	for _, a := range actions {
		if a == "" || a[:2] != "AX" {
			t.Errorf("action constant %q must be AX-prefixed", a)
		}
	}
}

func TestMatcherAllAndAny(t *testing.T) {
	// Test the matcher combinators without needing a real element.
	// Use a stub that returns a fixed answer.
	var stub *Element // methods are safe on nil because our Matchers check err
	// Each Matcher calls e.Role() → returns ErrClosed for nil, so returns false.

	// Confirm MatchAll short-circuits to false when any child is false.
	combined := MatchAll(
		func(*Element) bool { return true },
		func(*Element) bool { return false },
	)
	if combined(stub) {
		t.Error("MatchAll: expected false when any matcher is false")
	}

	// Confirm MatchAny short-circuits to true when any child is true.
	combined = MatchAny(
		func(*Element) bool { return false },
		func(*Element) bool { return true },
	)
	if !combined(stub) {
		t.Error("MatchAny: expected true when any matcher is true")
	}

	// Empty MatchAll is vacuously true.
	if !MatchAll()(stub) {
		t.Error("MatchAll() must be true (vacuous)")
	}
	// Empty MatchAny is vacuously false.
	if MatchAny()(stub) {
		t.Error("MatchAny() must be false (vacuous)")
	}
}

func TestCstr(t *testing.T) {
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte("hello\x00world"), "hello"},
		{[]byte("no nul"), "no nul"},
		{[]byte(""), ""},
		{[]byte("\x00"), ""},
	}
	for _, c := range cases {
		if got := cstr(c.in); got != c.want {
			t.Errorf("cstr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClosedElementMethods(t *testing.T) {
	// Verify that operations on a closed element return ErrClosed
	// without crashing or touching the dylib.
	e := &Element{handle: 0}
	e.closed.Store(true)

	if _, err := e.Attribute("AXRole"); err != ErrClosed {
		t.Errorf("Attribute on closed: got %v, want ErrClosed", err)
	}
	if _, err := e.AttributeInt("AXRole"); err != ErrClosed {
		t.Errorf("AttributeInt on closed: got %v, want ErrClosed", err)
	}
	if _, err := e.AttributeBool("AXEnabled"); err != ErrClosed {
		t.Errorf("AttributeBool on closed: got %v, want ErrClosed", err)
	}
	if _, err := e.AttributeElement("AXParent"); err != ErrClosed {
		t.Errorf("AttributeElement on closed: got %v, want ErrClosed", err)
	}
	if err := e.Perform("AXPress"); err != ErrClosed {
		t.Errorf("Perform on closed: got %v, want ErrClosed", err)
	}
	if err := e.SetString("AXValue", "x"); err != ErrClosed {
		t.Errorf("SetString on closed: got %v, want ErrClosed", err)
	}
	if err := e.SetBool("AXFocused", true); err != ErrClosed {
		t.Errorf("SetBool on closed: got %v, want ErrClosed", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	// A zero-handle Element can't actually call elementReleaseFn
	// (loadOnce hasn't fired in a no-integration test), but Close
	// guards against reuse via a CompareAndSwap. Verify that path.
	e := &Element{handle: 0}
	// First close — handle is 0, so elementReleaseFn is skipped.
	e.Close()
	// Second close should be a no-op.
	e.Close()
	if !e.closed.Load() {
		t.Error("Close did not mark element as closed")
	}
}
