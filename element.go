package kinax

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"sync/atomic"
	"unsafe"
)

// Element is a reference to a single AXUIElement in the system UI tree.
// Elements are opaque handles onto a retained Core Foundation object —
// callers MUST call [Element.Close] when done to avoid leaking.
//
// Element is safe for concurrent use by multiple goroutines; all AX API
// calls are thread-safe per Apple docs.
type Element struct {
	handle uintptr
	closed atomic.Bool
}

// wrap converts a raw handle into an Element. Returns nil if handle is 0.
func wrap(handle uintptr) *Element {
	if handle == 0 {
		return nil
	}
	return &Element{handle: handle}
}

// Close releases the underlying CFTypeRef. Safe to call multiple times;
// subsequent calls are no-ops. After Close, all other methods return
// [ErrClosed].
func (e *Element) Close() {
	if e == nil || !e.closed.CompareAndSwap(false, true) {
		return
	}
	if e.handle == 0 {
		return
	}
	elementReleaseFn(e.handle)
	e.handle = 0
}

// Handle returns the opaque pointer value — useful for identity
// comparisons (though two handles may point to the same element in
// some cases — prefer comparing semantic identity via Title/Role).
func (e *Element) Handle() uintptr {
	if e == nil {
		return 0
	}
	return e.handle
}

// ─── Core attribute accessors ────────────────────────────────

// Attribute reads a string-valued attribute. The return convention:
//   - empty string + nil error → attribute absent OR value is empty string
//   - non-empty string + nil error → attribute present with value
//   - "" + non-nil error → real failure (permission, etc.)
//
// This collapses "absent" and "empty" into one case; if you need to
// distinguish, use [Element.AttributeNames] first.
func (e *Element) Attribute(name string) (string, error) {
	if e == nil || e.closed.Load() {
		return "", ErrClosed
	}
	if err := Load(); err != nil {
		return "", err
	}
	nameC := append([]byte(name), 0)
	// Stack-allocate a reasonable default buffer; resize if it overflows.
	buf := make([]byte, 512)
	rc := attrStringFn(e.handle, unsafe.Pointer(&nameC[0]),
		unsafe.Pointer(&buf[0]), int32(len(buf)))
	switch {
	case rc == 0:
		return cstr(buf), nil
	case rc > 0:
		// Need bigger buffer.
		buf = make([]byte, rc)
		rc2 := attrStringFn(e.handle, unsafe.Pointer(&nameC[0]),
			unsafe.Pointer(&buf[0]), int32(len(buf)))
		if rc2 != 0 {
			return "", fmt.Errorf("kinax: attr %q: rc=%d", name, rc2)
		}
		return cstr(buf), nil
	default:
		// -1 means "attribute not present OR AX error". We return
		// an empty value + ErrNotFound so callers can tell.
		return "", fmt.Errorf("%w: attribute %q", ErrNotFound, name)
	}
}

// AttributeInt reads a number-valued attribute as int64.
func (e *Element) AttributeInt(name string) (int64, error) {
	if e == nil || e.closed.Load() {
		return 0, ErrClosed
	}
	if err := Load(); err != nil {
		return 0, err
	}
	nameC := append([]byte(name), 0)
	var out int64
	rc := attrIntFn(e.handle, unsafe.Pointer(&nameC[0]), unsafe.Pointer(&out))
	if rc != 0 {
		return 0, fmt.Errorf("%w: attribute %q", ErrNotFound, name)
	}
	return out, nil
}

// AttributeBool reads a bool-valued attribute.
func (e *Element) AttributeBool(name string) (bool, error) {
	if e == nil || e.closed.Load() {
		return false, ErrClosed
	}
	if err := Load(); err != nil {
		return false, err
	}
	nameC := append([]byte(name), 0)
	var out int32
	rc := attrBoolFn(e.handle, unsafe.Pointer(&nameC[0]), unsafe.Pointer(&out))
	if rc != 0 {
		return false, fmt.Errorf("%w: attribute %q", ErrNotFound, name)
	}
	return out == 1, nil
}

// AttributeElement reads an element-valued attribute (e.g. AXFocusedWindow,
// AXParent) and returns a new *Element that the caller must Close.
// Returns (nil, nil) if the attribute is absent — not an error, because
// "no focused window" is a perfectly normal state.
func (e *Element) AttributeElement(name string) (*Element, error) {
	if e == nil || e.closed.Load() {
		return nil, ErrClosed
	}
	if err := Load(); err != nil {
		return nil, err
	}
	nameC := append([]byte(name), 0)
	h := attrElementFn(e.handle, unsafe.Pointer(&nameC[0]))
	if h == 0 {
		return nil, nil
	}
	return wrap(h), nil
}

// AttributePoint reads a CGPoint-valued attribute (e.g. AXPosition).
func (e *Element) AttributePoint(name string) (image.Point, error) {
	if e == nil || e.closed.Load() {
		return image.Point{}, ErrClosed
	}
	if err := Load(); err != nil {
		return image.Point{}, err
	}
	nameC := append([]byte(name), 0)
	var x, y float64
	rc := attrPointFn(e.handle, unsafe.Pointer(&nameC[0]),
		unsafe.Pointer(&x), unsafe.Pointer(&y))
	if rc != 0 {
		return image.Point{}, fmt.Errorf("%w: attribute %q", ErrNotFound, name)
	}
	return image.Point{X: int(x), Y: int(y)}, nil
}

// AttributeSize reads a CGSize-valued attribute (e.g. AXSize).
func (e *Element) AttributeSize(name string) (image.Point, error) {
	if e == nil || e.closed.Load() {
		return image.Point{}, ErrClosed
	}
	if err := Load(); err != nil {
		return image.Point{}, err
	}
	nameC := append([]byte(name), 0)
	var w, h float64
	rc := attrSizeFn(e.handle, unsafe.Pointer(&nameC[0]),
		unsafe.Pointer(&w), unsafe.Pointer(&h))
	if rc != 0 {
		return image.Point{}, fmt.Errorf("%w: attribute %q", ErrNotFound, name)
	}
	return image.Point{X: int(w), Y: int(h)}, nil
}

// AttributeElements reads an array-of-elements attribute (e.g. AXChildren,
// AXWindows). Each returned Element must be closed by the caller.
func (e *Element) AttributeElements(name string) ([]*Element, error) {
	if e == nil || e.closed.Load() {
		return nil, ErrClosed
	}
	if err := Load(); err != nil {
		return nil, err
	}
	nameC := append([]byte(name), 0)

	// First call: probe with a modest buffer. If it overflows, reallocate.
	buf := make([]uintptr, 64)
	count := int32(len(buf))
	rc := attrElementArrayFn(e.handle, unsafe.Pointer(&nameC[0]),
		unsafe.Pointer(&buf[0]), unsafe.Pointer(&count))
	if rc != 0 {
		return nil, fmt.Errorf("%w: attribute %q", ErrNotFound, name)
	}
	if int(count) > len(buf) {
		// Retry with exact size.
		for _, h := range buf {
			if h != 0 {
				elementReleaseFn(h) // release the over-retained ones
			}
		}
		buf = make([]uintptr, count)
		got := count
		rc2 := attrElementArrayFn(e.handle, unsafe.Pointer(&nameC[0]),
			unsafe.Pointer(&buf[0]), unsafe.Pointer(&got))
		if rc2 != 0 {
			return nil, fmt.Errorf("kinax: attr %q retry: rc=%d", name, rc2)
		}
		count = got
	}
	out := make([]*Element, 0, count)
	for i := 0; i < int(count); i++ {
		if buf[i] != 0 {
			out = append(out, wrap(buf[i]))
		}
	}
	return out, nil
}

// ─── Convenience accessors ───────────────────────────────────

// Role returns the element's AXRole (e.g. "AXButton"). Empty if absent.
func (e *Element) Role() (string, error) { return e.Attribute(AttrRole) }

// Subrole returns the element's AXSubrole (e.g. "AXCloseButton").
func (e *Element) Subrole() (string, error) { return e.Attribute(AttrSubrole) }

// Title returns the element's AXTitle.
func (e *Element) Title() (string, error) { return e.Attribute(AttrTitle) }

// Description returns the element's AXDescription.
func (e *Element) Description() (string, error) { return e.Attribute(AttrDescription) }

// Value returns the element's AXValue as a string (works for text fields,
// sliders with a stringified number, etc.).
func (e *Element) Value() (string, error) { return e.Attribute(AttrValue) }

// Identifier returns the element's AXIdentifier — the stable string
// ID set by the app developer for automation.
func (e *Element) Identifier() (string, error) { return e.Attribute(AttrIdentifier) }

// Enabled reports whether the element is enabled (vs. grayed out).
func (e *Element) Enabled() (bool, error) { return e.AttributeBool(AttrEnabled) }

// Focused reports whether the element currently has keyboard focus.
func (e *Element) Focused() (bool, error) { return e.AttributeBool(AttrFocused) }

// Position returns the element's top-left corner in global screen
// coordinates.
func (e *Element) Position() (image.Point, error) { return e.AttributePoint(AttrPosition) }

// Size returns the element's width and height in pixels.
func (e *Element) Size() (image.Point, error) { return e.AttributeSize(AttrSize) }

// Children returns the element's direct children. Caller must Close each.
func (e *Element) Children() ([]*Element, error) {
	return e.AttributeElements(AttrChildren)
}

// Parent returns the element's parent, or nil if it's a root.
func (e *Element) Parent() (*Element, error) {
	return e.AttributeElement(AttrParent)
}

// ─── Attribute + action introspection ────────────────────────

// AttributeNames returns all attribute names this element exposes.
// Useful for dumping the full AX state of an element during debugging.
func (e *Element) AttributeNames() ([]string, error) {
	return e.listNames(attributeNamesFn)
}

// ActionNames returns all action names this element responds to.
func (e *Element) ActionNames() ([]string, error) {
	return e.listNames(actionNamesFn)
}

func (e *Element) listNames(fn func(uintptr, unsafe.Pointer, int32) int32) ([]string, error) {
	if e == nil || e.closed.Load() {
		return nil, ErrClosed
	}
	if err := Load(); err != nil {
		return nil, err
	}
	// ObjC encodes the list as compact JSON; read into a buffer that
	// grows if undersized.
	buf := make([]byte, 4096)
	rc := fn(e.handle, unsafe.Pointer(&buf[0]), int32(len(buf)))
	if rc > 0 {
		buf = make([]byte, rc)
		rc = fn(e.handle, unsafe.Pointer(&buf[0]), int32(len(buf)))
	}
	if rc != 0 {
		return nil, errors.New("kinax: unable to list names")
	}
	s := cstr(buf)
	if s == "" {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(s), &names); err != nil {
		return nil, fmt.Errorf("kinax: parse names %q: %w", s, err)
	}
	return names, nil
}

// Perform fires the named action (e.g. AXPress). Returns nil on success
// or an error with the underlying AXError code.
func (e *Element) Perform(action string) error {
	if e == nil || e.closed.Load() {
		return ErrClosed
	}
	if err := Load(); err != nil {
		return err
	}
	nameC := append([]byte(action), 0)
	if rc := elementPerformFn(e.handle, unsafe.Pointer(&nameC[0])); rc != 0 {
		return fmt.Errorf("kinax: perform %q: AXError %d", action, rc)
	}
	return nil
}

// SetString sets a string-valued attribute. Typically used to set
// AXValue on a text field:
//
//	field.SetString(kinax.AttrValue, "new text")
func (e *Element) SetString(attr, value string) error {
	if e == nil || e.closed.Load() {
		return ErrClosed
	}
	if err := Load(); err != nil {
		return err
	}
	nameC := append([]byte(attr), 0)
	valC := append([]byte(value), 0)
	if rc := elementSetStringFn(e.handle, unsafe.Pointer(&nameC[0]),
		unsafe.Pointer(&valC[0])); rc != 0 {
		return fmt.Errorf("kinax: set %q: AXError %d", attr, rc)
	}
	return nil
}

// SetBool sets a bool-valued attribute (e.g. AXFocused=true to request
// keyboard focus).
func (e *Element) SetBool(attr string, value bool) error {
	if e == nil || e.closed.Load() {
		return ErrClosed
	}
	if err := Load(); err != nil {
		return err
	}
	nameC := append([]byte(attr), 0)
	v := int32(0)
	if value {
		v = 1
	}
	if rc := elementSetBoolFn(e.handle, unsafe.Pointer(&nameC[0]), v); rc != 0 {
		return fmt.Errorf("kinax: set %q: AXError %d", attr, rc)
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
