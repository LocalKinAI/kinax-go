package kinax

import (
	"errors"
	"testing"
	"time"
)

func TestObserver_InvalidPID(t *testing.T) {
	cases := []int{0, -1, -100}
	for _, pid := range cases {
		_, err := NewObserver(pid)
		if err == nil {
			t.Errorf("NewObserver(%d) should error, got nil", pid)
		}
	}
}

func TestObserver_CloseIdempotent(t *testing.T) {
	// We can't safely create a real observer in unit tests (would need
	// TCC + a real PID), so just exercise the nil/closed paths.
	var o *Observer
	o.Close() // nil receiver, must not panic

	o2 := &Observer{}
	o2.Close()
	o2.Close() // idempotent

	if !o2.closed.Load() {
		t.Error("Close should set closed=true")
	}
}

func TestObserver_OperationsOnClosed(t *testing.T) {
	o := &Observer{}
	o.Close()

	if _, err := o.Next(time.Millisecond); !errors.Is(err, ErrObserverClosed) {
		t.Errorf("Next on closed = %v, want ErrObserverClosed", err)
	}
	if err := o.Subscribe(nil); !errors.Is(err, ErrObserverClosed) {
		t.Errorf("Subscribe on closed = %v, want ErrObserverClosed", err)
	}
	if err := o.Unsubscribe(nil, "AXValueChanged"); !errors.Is(err, ErrObserverClosed) {
		t.Errorf("Unsubscribe on closed = %v, want ErrObserverClosed", err)
	}
}

func TestObserver_PID(t *testing.T) {
	o := &Observer{pid: 42}
	if o.PID() != 42 {
		t.Errorf("PID() = %d, want 42", o.PID())
	}

	var nilObs *Observer
	if nilObs.PID() != 0 {
		t.Errorf("nil.PID() should return 0, got %d", nilObs.PID())
	}
}

func TestParseEvent_ValidJSON(t *testing.T) {
	// Mock dylib output. Element handle is the raw uintptr value the
	// dylib CFRetain'd. parseEvent should NOT release it — caller
	// owns it via Event.Element.Close().
	raw := []byte(`{"notification":"AXFocusedWindowChanged","element_handle":140735312345,"timestamp_ms":1714431234567}` + "\x00trailing junk")
	ev, err := parseEvent(raw)
	if err != nil {
		t.Fatalf("parseEvent error: %v", err)
	}
	if ev.Notification != "AXFocusedWindowChanged" {
		t.Errorf("Notification = %q", ev.Notification)
	}
	if ev.Element == nil {
		t.Fatal("Element is nil")
	}
	if ev.Element.handle != 140735312345 {
		t.Errorf("Element.handle = %d, want 140735312345", ev.Element.handle)
	}
	if ev.Timestamp.UnixMilli() != 1714431234567 {
		t.Errorf("Timestamp = %v", ev.Timestamp)
	}
}

func TestParseEvent_Empty(t *testing.T) {
	_, err := parseEvent([]byte{})
	if err == nil {
		t.Error("parseEvent on empty buffer should error")
	}
	_, err = parseEvent([]byte{0, 0, 0})
	if err == nil {
		t.Error("parseEvent on all-NUL buffer should error")
	}
}

func TestParseEvent_Malformed(t *testing.T) {
	_, err := parseEvent([]byte("not json\x00"))
	if err == nil {
		t.Error("parseEvent on non-JSON should error")
	}
}

func TestNotificationConstants(t *testing.T) {
	// Sanity: the well-known notification names are exact strings the
	// macOS AX framework expects. Drift here would make subscriptions
	// silently no-op.
	cases := map[string]string{
		NotifFocusedWindowChanged:    "AXFocusedWindowChanged",
		NotifFocusedUIElementChanged: "AXFocusedUIElementChanged",
		NotifValueChanged:            "AXValueChanged",
		NotifTitleChanged:            "AXTitleChanged",
		NotifWindowCreated:           "AXWindowCreated",
		NotifMenuOpened:              "AXMenuOpened",
		NotifApplicationActivated:    "AXApplicationActivated",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}
