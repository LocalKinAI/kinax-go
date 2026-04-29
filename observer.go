package kinax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"
)

// Observer subscribes to AX notifications (focus changes, value edits,
// window creates, etc.) on a per-pid scope. Each Observer owns a
// dedicated worker thread inside the embedded dylib that runs a
// CFRunLoop — Apple's AX API is thread-pinned, so we can't share one
// across goroutines.
//
// Lifecycle:
//
//	obs, err := kinax.NewObserver(pid)
//	defer obs.Close()
//
//	obs.Subscribe(elem, kinax.NotifFocusedWindowChanged, kinax.NotifValueChanged)
//
//	// Drain events into a channel:
//	events := obs.Events(ctx)
//	for ev := range events {
//	    fmt.Println(ev.Notification, ev.Element)
//	    ev.Element.Close()  // caller owns the element handle
//	}
//
// Or block-on-next-with-timeout directly:
//
//	ev, err := obs.Next(500 * time.Millisecond)
//	if errors.Is(err, ErrObserverTimeout) { continue }
//
// Close() is the only safe way to stop the worker thread. Calling
// Close() twice is a no-op. After Close the Observer is unusable.
type Observer struct {
	handle uintptr
	pid    int
	closed atomic.Bool
}

// Standard AX notification names. Use these constants rather than
// string literals so typos fail at compile time. The full list is in
// <HIServices/AXNotificationConstants.h>; this is the subset most
// agent use-cases need.
const (
	NotifMainWindowChanged    = "AXMainWindowChanged"
	NotifFocusedWindowChanged = "AXFocusedWindowChanged"
	NotifFocusedUIElementChanged = "AXFocusedUIElementChanged"
	NotifWindowCreated        = "AXWindowCreated"
	NotifWindowResized        = "AXWindowResized"
	NotifWindowMoved          = "AXWindowMoved"
	NotifWindowMiniaturized   = "AXWindowMiniaturized"
	NotifWindowDeminiaturized = "AXWindowDeminiaturized"
	NotifApplicationActivated   = "AXApplicationActivated"
	NotifApplicationDeactivated = "AXApplicationDeactivated"
	NotifValueChanged         = "AXValueChanged"
	NotifTitleChanged         = "AXTitleChanged"
	NotifSelectedTextChanged  = "AXSelectedTextChanged"
	NotifSelectedChildrenChanged = "AXSelectedChildrenChanged"
	NotifMenuOpened           = "AXMenuOpened"
	NotifMenuClosed           = "AXMenuClosed"
	NotifAnnouncementRequested = "AXAnnouncementRequested"
)

// Event is one AX notification fired by the system, marshaled from the
// dylib's worker thread to Go's caller. The Element handle is freshly
// CFRetain'd by the dylib — caller owns it and MUST Close() to release.
type Event struct {
	Notification string    // e.g. "AXFocusedWindowChanged"
	Element      *Element  // the element the notification fired on (often a window)
	Timestamp    time.Time // when the dylib received the callback
}

// ErrObserverTimeout is returned from Next() when no event arrived
// within the timeout. Callers typically loop on this.
var ErrObserverTimeout = errors.New("kinax: observer next: timeout")

// ErrObserverClosed is returned from Next() / Subscribe() if Close()
// has already been called.
var ErrObserverClosed = errors.New("kinax: observer is closed")

// NewObserver creates an Observer for the given process. Returns
// (nil, error) if the process doesn't exist or AX permission is
// missing. The caller MUST Close() to free the underlying worker
// thread + AXObserverRef.
func NewObserver(pid int) (*Observer, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	if pid <= 0 {
		return nil, fmt.Errorf("kinax: NewObserver: invalid pid %d", pid)
	}
	errBuf := make([]byte, 256)
	h := observerCreateFn(int32(pid),
		unsafe.Pointer(&errBuf[0]), int32(len(errBuf)))
	if h == 0 {
		return nil, fmt.Errorf("kinax: NewObserver(pid=%d): %s", pid, cstr(errBuf))
	}
	return &Observer{handle: h, pid: pid}, nil
}

// Subscribe registers one or more AX notifications to fire on the
// given element. Common pattern: subscribe at the application root
// (kinax.ApplicationByPID) for top-level events like
// AXFocusedWindowChanged, or at a specific element for AXValueChanged.
//
// Errors are aggregated — one bad notification name doesn't abort the
// rest, but the returned error mentions every failure.
func (o *Observer) Subscribe(elem *Element, notifications ...string) error {
	if o == nil || o.closed.Load() {
		return ErrObserverClosed
	}
	if elem == nil || elem.handle == 0 {
		return fmt.Errorf("kinax: Subscribe: nil element")
	}
	if len(notifications) == 0 {
		return nil
	}
	var failures []string
	for _, n := range notifications {
		nameC := append([]byte(n), 0)
		rc := observerSubscribeFn(o.handle, elem.handle, unsafe.Pointer(&nameC[0]))
		if rc != 0 {
			failures = append(failures, fmt.Sprintf("%s (AXError %d)", n, rc))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("kinax: Subscribe failed for: %v", failures)
	}
	return nil
}

// Unsubscribe removes a previous subscription. Safe to call on a
// notification that was never subscribed (returns nil).
func (o *Observer) Unsubscribe(elem *Element, notification string) error {
	if o == nil || o.closed.Load() {
		return ErrObserverClosed
	}
	if elem == nil || elem.handle == 0 {
		return fmt.Errorf("kinax: Unsubscribe: nil element")
	}
	nameC := append([]byte(notification), 0)
	rc := observerUnsubscribeFn(o.handle, elem.handle, unsafe.Pointer(&nameC[0]))
	if rc != 0 {
		return fmt.Errorf("kinax: Unsubscribe(%s): AXError %d", notification, rc)
	}
	return nil
}

// Next blocks for up to `timeout` waiting for the next event. If no
// event arrives within the timeout, returns (nil, ErrObserverTimeout).
// On success returns the event; the Element inside is a fresh
// CFRetain'd handle that the caller MUST Close() to release.
//
// timeout=0 polls without blocking (returns immediately if queue is
// empty). negative timeout is treated as 0.
func (o *Observer) Next(timeout time.Duration) (*Event, error) {
	if o == nil || o.closed.Load() {
		return nil, ErrObserverClosed
	}
	if timeout < 0 {
		timeout = 0
	}
	timeoutMs := int32(timeout / time.Millisecond)

	buf := make([]byte, 1024)
	rc := observerNextFn(o.handle, timeoutMs,
		unsafe.Pointer(&buf[0]), int32(len(buf)))
	switch {
	case rc == 0:
		return parseEvent(buf)
	case rc == -1:
		return nil, ErrObserverTimeout
	case rc == -2:
		return nil, ErrObserverClosed
	case rc > 0:
		// Buffer too small. Resize + retry.
		buf = make([]byte, rc)
		rc2 := observerNextFn(o.handle, 0,
			unsafe.Pointer(&buf[0]), int32(len(buf)))
		if rc2 != 0 {
			return nil, fmt.Errorf("kinax: observer next retry rc=%d", rc2)
		}
		return parseEvent(buf)
	default:
		return nil, fmt.Errorf("kinax: observer next rc=%d", rc)
	}
}

// Events is a goroutine-friendly wrapper around Next: streams every
// event into a channel until ctx is cancelled or the Observer is
// closed. The channel is closed when the streamer exits.
//
// pollInterval is the timeout passed to each Next call — small values
// are responsive but burn CPU; the default (100ms) is a good baseline.
// Pass <=0 for the default.
//
// CALLER must Close() each Event.Element when done to release the
// underlying CFTypeRef; otherwise CF memory accumulates.
func (o *Observer) Events(ctx context.Context, pollInterval time.Duration) <-chan Event {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for {
			if ctx.Err() != nil {
				return
			}
			ev, err := o.Next(pollInterval)
			if err != nil {
				if errors.Is(err, ErrObserverTimeout) {
					continue
				}
				return
			}
			select {
			case out <- *ev:
			case <-ctx.Done():
				ev.Element.Close()
				return
			}
		}
	}()
	return out
}

// Close stops the worker thread, drains any pending events (releasing
// their element handles), and frees the Observer. Idempotent — safe
// to call multiple times. After Close, all other methods return
// ErrObserverClosed.
//
// Close blocks until the worker thread has fully exited (typically
// <50ms — bounded by the time CFRunLoopStop takes effect).
func (o *Observer) Close() {
	if o == nil || !o.closed.CompareAndSwap(false, true) {
		return
	}
	if o.handle != 0 {
		observerCloseFn(o.handle)
		o.handle = 0
	}
}

// PID returns the process this Observer is bound to. Useful for logs.
func (o *Observer) PID() int {
	if o == nil {
		return 0
	}
	return o.pid
}

// observerEventJSON matches the dylib's serialization shape. Stay
// in lockstep with kinax_observer_next's snprintf format string.
type observerEventJSON struct {
	Notification  string `json:"notification"`
	ElementHandle uint64 `json:"element_handle"`
	TimestampMs   int64  `json:"timestamp_ms"`
}

func parseEvent(buf []byte) (*Event, error) {
	end := len(buf)
	for i, b := range buf {
		if b == 0 {
			end = i
			break
		}
	}
	if end == 0 {
		return nil, fmt.Errorf("kinax: empty event buffer")
	}
	var raw observerEventJSON
	if err := json.Unmarshal(buf[:end], &raw); err != nil {
		return nil, fmt.Errorf("kinax: parse event %q: %w", buf[:end], err)
	}
	return &Event{
		Notification: raw.Notification,
		Element:      wrap(uintptr(raw.ElementHandle)),
		Timestamp:    time.UnixMilli(raw.TimestampMs),
	}, nil
}

