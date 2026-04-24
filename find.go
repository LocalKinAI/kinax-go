package kinax

import "strings"

// Matcher is a predicate over an Element, used by [Element.FindFirst] /
// [Element.FindAll] to select elements during tree traversal.
//
// A Matcher may freely call accessors (Role, Title, attributes). It
// MUST NOT Close the element — ownership stays with the traversal.
type Matcher func(e *Element) bool

// MatchRole returns a Matcher that selects elements whose AXRole
// equals `role`. Use the Role* constants:
//
//	app.FindFirst(kinax.MatchRole(kinax.RoleButton), 20)
func MatchRole(role string) Matcher {
	return func(e *Element) bool {
		got, err := e.Role()
		return err == nil && got == role
	}
}

// MatchTitle selects elements whose AXTitle equals `title` exactly.
func MatchTitle(title string) Matcher {
	return func(e *Element) bool {
		got, err := e.Title()
		return err == nil && got == title
	}
}

// MatchTitleContains selects elements whose title contains `sub` as a
// substring (case-insensitive).
func MatchTitleContains(sub string) Matcher {
	lowSub := strings.ToLower(sub)
	return func(e *Element) bool {
		got, err := e.Title()
		if err != nil || got == "" {
			return false
		}
		return strings.Contains(strings.ToLower(got), lowSub)
	}
}

// MatchIdentifier selects elements with AXIdentifier == `id`. App
// developers set this for automation-stable lookup — prefer it over
// titles when available.
func MatchIdentifier(id string) Matcher {
	return func(e *Element) bool {
		got, err := e.Identifier()
		return err == nil && got == id
	}
}

// MatchRoleAndTitle selects elements matching both role and exact title.
func MatchRoleAndTitle(role, title string) Matcher {
	return func(e *Element) bool {
		r, err := e.Role()
		if err != nil || r != role {
			return false
		}
		t, err := e.Title()
		return err == nil && t == title
	}
}

// MatchAll combines matchers with AND.
func MatchAll(ms ...Matcher) Matcher {
	return func(e *Element) bool {
		for _, m := range ms {
			if !m(e) {
				return false
			}
		}
		return true
	}
}

// MatchAny combines matchers with OR.
func MatchAny(ms ...Matcher) Matcher {
	return func(e *Element) bool {
		for _, m := range ms {
			if m(e) {
				return true
			}
		}
		return false
	}
}

// FindFirst walks the descendants of e (depth-first, excluding e itself)
// and returns the first match. Traversal is bounded by `maxDepth` to
// prevent runaway on pathological trees.
//
// The returned Element is a fresh handle owned by the caller — Close it
// when done. Returns (nil, false) if no match.
//
// Note: e itself is NOT considered a match candidate. This keeps
// ownership semantics clean — the caller's existing handle is never
// returned, so there's no risk of a double-Close.
func (e *Element) FindFirst(m Matcher, maxDepth int) (*Element, bool) {
	if e == nil || e.closed.Load() || m == nil {
		return nil, false
	}
	if maxDepth <= 0 {
		return nil, false
	}
	return searchFirst(e, m, maxDepth)
}

// FindAll collects every descendant of e matching `m` (excluding e).
// Caller must Close every returned element.
func (e *Element) FindAll(m Matcher, maxDepth int) []*Element {
	if e == nil || e.closed.Load() || m == nil || maxDepth <= 0 {
		return nil
	}
	var out []*Element
	searchAll(e, m, maxDepth, &out)
	return out
}

// searchFirst descends the immediate children of parent and returns the
// first match found. On a match: we stop descending, preserve the
// matching child's handle, and Close all siblings + descendants we
// walked but didn't match.
func searchFirst(parent *Element, m Matcher, depthLeft int) (*Element, bool) {
	kids, err := parent.Children()
	if err != nil {
		return nil, false
	}
	var found *Element
	for _, k := range kids {
		if found != nil {
			// Already found — just release remaining siblings.
			k.Close()
			continue
		}
		if m(k) {
			found = k
			continue
		}
		if depthLeft > 1 {
			if hit, ok := searchFirst(k, m, depthLeft-1); ok {
				found = hit
			}
		}
		if k != found {
			k.Close()
		}
	}
	if found != nil {
		return found, true
	}
	return nil, false
}

// searchAll is like searchFirst but collects every match.
func searchAll(parent *Element, m Matcher, depthLeft int, out *[]*Element) {
	kids, err := parent.Children()
	if err != nil {
		return
	}
	for _, k := range kids {
		keep := m(k)
		if keep {
			*out = append(*out, k)
		}
		if depthLeft > 1 {
			searchAll(k, m, depthLeft-1, out)
		}
		if !keep {
			k.Close()
		}
	}
}
