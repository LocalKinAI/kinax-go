//go:build integration

package kinax_test

// Integration tests — require Accessibility permission AND at least
// one app running. Execute with:
//
//	make test-integration
//
// or:
//
//	go test -tags integration ./...
//
// On a fresh binary, the first run pops the Accessibility prompt.
// Grant permission, then rerun.

import (
	"testing"

	"github.com/LocalKinAI/kinax-go"
)

func TestLoad(t *testing.T) {
	if err := kinax.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kinax.ResolvedDylibPath() == "" {
		t.Fatal("ResolvedDylibPath is empty after Load")
	}
}

func TestTrustedReturnsSomething(t *testing.T) {
	_ = kinax.Trusted()
}

func TestFocusedApplicationUsuallyExists(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	app, err := kinax.FocusedApplication()
	if err != nil {
		t.Skipf("no focused app (CI / login screen?): %v", err)
	}
	defer app.Close()

	role, err := app.Role()
	if err != nil {
		t.Fatalf("Role: %v", err)
	}
	if role != kinax.RoleApplication {
		t.Errorf("focused element role = %q, want AXApplication", role)
	}
}

func TestFrontmostPIDPositive(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	pid := kinax.FrontmostPID()
	if pid <= 0 {
		t.Errorf("FrontmostPID = %d, expected > 0", pid)
	}
}

func TestApplicationByPID(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	pid := kinax.FrontmostPID()
	if pid == 0 {
		t.Skip("no frontmost app")
	}
	app, err := kinax.ApplicationByPID(pid)
	if err != nil {
		t.Fatalf("ApplicationByPID(%d): %v", pid, err)
	}
	defer app.Close()

	// Should be able to list windows (may be empty for background apps).
	_, err = app.Windows()
	if err != nil {
		t.Logf("Windows: %v (often normal for apps without windows)", err)
	}
}

func TestSystemWideFocusedElement(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	sw, err := kinax.SystemWide()
	if err != nil {
		t.Fatalf("SystemWide: %v", err)
	}
	defer sw.Close()

	fe, err := sw.FocusedElement()
	if err != nil {
		t.Logf("FocusedElement: %v (may be absent between app switches)", err)
		return
	}
	if fe != nil {
		defer fe.Close()
		if r, err := fe.Role(); err == nil {
			t.Logf("globally-focused element role=%q", r)
		}
	}
}

func TestAttributeNamesNonEmpty(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	app, err := kinax.FocusedApplication()
	if err != nil {
		t.Skipf("no focused app: %v", err)
	}
	defer app.Close()

	names, err := app.AttributeNames()
	if err != nil {
		t.Fatalf("AttributeNames: %v", err)
	}
	if len(names) < 3 {
		t.Errorf("expected ≥3 attributes on app element, got %v", names)
	}
	// App elements typically expose AXRole at minimum.
	var haveRole bool
	for _, n := range names {
		if n == kinax.AttrRole {
			haveRole = true
			break
		}
	}
	if !haveRole {
		t.Errorf("AXRole missing from app attribute names: %v", names)
	}
}

func TestFindFirstFindsButton(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	app, err := kinax.FocusedApplication()
	if err != nil {
		t.Skipf("no focused app: %v", err)
	}
	defer app.Close()

	// Look for any button; nearly every app has at least one.
	btn, ok := app.FindFirst(kinax.MatchRole(kinax.RoleButton), 20)
	if !ok {
		t.Skip("no button found in focused app (unusual — skipping)")
	}
	defer btn.Close()
	role, _ := btn.Role()
	if role != kinax.RoleButton {
		t.Errorf("FindFirst returned non-button: %q", role)
	}
}

func TestElementAtPointCurrentCursorIsh(t *testing.T) {
	if !kinax.Trusted() {
		t.Skip("Accessibility permission not granted")
	}
	// Point at upper-left of screen; usually hits the menu bar.
	el, err := kinax.ElementAtPoint(20, 5)
	if err != nil {
		t.Logf("ElementAtPoint: %v", err)
		return
	}
	defer el.Close()
	role, _ := el.Role()
	t.Logf("element at (20, 5) has role=%q", role)
}

func TestRequireTrustReturnsSentinel(t *testing.T) {
	err := kinax.RequireTrust()
	if err != nil && err != kinax.ErrNotTrusted {
		t.Errorf("RequireTrust returned unexpected error: %v", err)
	}
}
