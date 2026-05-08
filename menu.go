package kinax

import (
	"fmt"
	"strings"
	"time"
)

// NavigateMenu walks an application's macOS menu bar by string path and
// triggers the leaf menu item via [Element.Perform](ActionPress).
// Path separators accepted: " > ", ">", " / ", "/", " → ", "→".
//
//	app, _ := kinax.FocusedApplication()
//	defer app.Close()
//	if err := app.NavigateMenu("Format > Cell > Conditional Highlighting"); err != nil { ... }
//
// Implementation walks AXMenuBar → AXMenuBarItem → AXMenu → AXMenuItem,
// pressing each ancestor along the way to open submenus. An 80ms settle
// is inserted between submenu opens to give AppKit time to populate the
// AXMenu's children before we look up the next path step.
//
// Errors:
//
//   - "no AXMenuBar": app is sandboxed without standard AppKit menus,
//     or it's not a GUI app.
//   - "no child titled X at step N/M": one of the path components
//     didn't match any menu / item title at that level. Check the path
//     against macOS's actual menu structure (Help → search bar shows
//     all items in the menu bar of the focused app).
//
// macOS menu items can have alternate forms reachable only with ⌥
// held. Those don't show up in this walk because AX exposes them as
// AXAlternateUIVisible elements only while ⌥ is pressed; for those,
// use [Hotkey] with the literal modifier+key from the menu's listed
// shortcut.
func (e *Element) NavigateMenu(path string) error {
	parts := splitMenuPath(path)
	if len(parts) == 0 {
		return fmt.Errorf("kinax: NavigateMenu: empty path")
	}

	menubar, err := e.AttributeElement(AttrMenuBar)
	if err != nil || menubar == nil {
		return fmt.Errorf("kinax: NavigateMenu: app has no AXMenuBar (sandboxed / non-AppKit?)")
	}
	defer menubar.Close()

	current := menubar
	owned := []*Element{} // close on exit; menubar stays via its own defer
	defer func() {
		for _, x := range owned {
			x.Close()
		}
	}()

	for i, name := range parts {
		next, err := findMenuChild(current, name)
		if err != nil {
			return fmt.Errorf("kinax: NavigateMenu: at step %d/%d (%q): %w",
				i+1, len(parts), name, err)
		}
		owned = append(owned, next)

		isLast := i == len(parts)-1
		if isLast {
			if err := next.Perform(ActionPress); err != nil {
				return fmt.Errorf("kinax: NavigateMenu: final AXPress on %q: %w", name, err)
			}
			return nil
		}
		// Open submenu. AXPress on AXMenuBarItem / AXMenuItem with
		// children pops the menu visible; subsequent Children() calls
		// see the populated AXMenuItem set.
		if err := next.Perform(ActionPress); err != nil {
			return fmt.Errorf("kinax: NavigateMenu: AXPress on %q to open submenu: %w", name, err)
		}
		// 80ms settle is empirically enough on M-series Macs; older
		// Intel + crowded menus may need more. If it becomes a
		// reliability issue we can poll for the AXMenu child instead.
		time.Sleep(80 * time.Millisecond)
		current = next
	}
	return fmt.Errorf("kinax: NavigateMenu: unreachable")
}

// MenuItemShortcut reads a menu item's keyboard equivalent. Returns
// the character (e.g. "s"), the modifier bitfield, and the virtual
// keycode (set when the shortcut is a non-character key like F-keys
// or arrows). Apple's encoding quirk: bit 3 of mods means "no ⌘"
// (i.e. ⌘ is implicit when bit 3 is clear).
//
//	char, mods, vk, err := el.MenuItemShortcut()
//	// e.g. char="s" mods=0 vk=0  → ⌘S
//	// e.g. char=""  mods=0 vk=122 → ⌘F1 (virtual key 122)
//
// Use this on AXMenuItem elements found via [NavigateMenu]'s tree
// walk or directly via [Element.FindFirst]. Returns empty/zero values
// when the element has no keyboard equivalent (most non-leaf menu
// items don't).
func (e *Element) MenuItemShortcut() (char string, mods int, vk int, err error) {
	char, _ = e.Attribute(AttrMenuItemCmdChar)
	if v, mErr := e.AttributeInt(AttrMenuItemCmdModifiers); mErr == nil {
		mods = int(v)
	}
	if v, vkErr := e.AttributeInt(AttrMenuItemCmdVirtualKey); vkErr == nil {
		vk = int(v)
	}
	if char == "" && vk == 0 {
		return "", 0, 0, fmt.Errorf("kinax: MenuItemShortcut: element has no keyboard equivalent")
	}
	return char, mods, vk, nil
}

// splitMenuPath divides a path string into its components. Tries
// known separators in priority order (" > " before ">" so spaces
// around > are eaten cleanly). Whitespace-only parts get dropped.
func splitMenuPath(p string) []string {
	for _, sep := range []string{" > ", ">", " / ", "/", " → ", "→"} {
		if strings.Contains(p, sep) {
			parts := strings.Split(p, sep)
			out := make([]string, 0, len(parts))
			for _, x := range parts {
				if t := strings.TrimSpace(x); t != "" {
					out = append(out, t)
				}
			}
			return out
		}
	}
	if t := strings.TrimSpace(p); t != "" {
		return []string{t}
	}
	return nil
}

// findMenuChild looks up a menu element by exact title within parent's
// children. Handles the AXMenuBarItem → AXMenu → AXMenuItem layering:
// when the matched element has exactly one AXMenu child, that submenu
// is returned (so the caller's next iteration walks its items),
// otherwise the leaf is returned directly.
//
// Caller owns the returned element; siblings are closed before
// returning to avoid handle leaks during deep walks.
func findMenuChild(parent *Element, title string) (*Element, error) {
	kids, err := parent.Children()
	if err != nil {
		return nil, err
	}
	for i, k := range kids {
		t, _ := k.Title()
		if t != title {
			continue
		}
		// Match. Probe one level deeper for an AXMenu container.
		subKids, _ := k.Children()
		var menuChild *Element
		for _, sk := range subKids {
			r, _ := sk.Role()
			if r == RoleMenu && menuChild == nil {
				menuChild = sk
			} else {
				sk.Close()
			}
		}
		if menuChild != nil {
			// Open the parent (so submenu populates) + return submenu.
			_ = k.Perform(ActionPress)
			k.Close()
			closeFollowing(kids, i)
			return menuChild, nil
		}
		closeFollowing(kids, i)
		return k, nil
	}
	closeFollowing(kids, -1) // close all
	return nil, fmt.Errorf("no child titled %q", title)
}

// closeFollowing closes every element AFTER index i in the slice.
// Pass i = -1 to close all. Tolerates nil entries.
func closeFollowing(els []*Element, after int) {
	for i, e := range els {
		if i <= after || e == nil {
			continue
		}
		e.Close()
	}
}
