// Command kinax inspects and manipulates the macOS UI tree via the
// Accessibility API.
//
// Usage:
//
//	kinax tree [--pid PID | --bundle BUNDLE | --focused] [--depth N]
//	kinax attr ATTR [--pid PID | --bundle BUNDLE]
//	kinax attrs [--pid PID | --bundle BUNDLE]
//	kinax actions [--pid PID | --bundle BUNDLE]
//	kinax find ROLE [TITLE] [--pid PID | --bundle BUNDLE] [--depth N]
//	kinax click TITLE [--role ROLE] [--pid PID | --bundle BUNDLE] [--depth N]
//	kinax at-point X Y
//	kinax trust [--prompt]
//	kinax version
package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/LocalKinAI/kinax-go"
)

const usage = `kinax — macOS Accessibility tree inspector / controller

COMMANDS
  tree [--pid N | --bundle ID | --focused] [--depth N]
      Print the AX tree of an app. --focused is default (active app).
      --depth limits traversal (default 6).

  attr ATTR [--pid N | --bundle ID | --focused]
      Print the value of an AX attribute on the app element
      (e.g. "AXTitle", "AXRole").

  attrs [--pid N | --bundle ID | --focused]
      List every attribute name this app element exposes.

  actions [--pid N | --bundle ID | --focused]
      List every action name the focused element responds to.

  find ROLE [TITLE] [--pid N | --bundle ID | --focused] [--depth N]
      Print all descendants of an app element with the given role
      (and optional title). Output is one per line:
      ROLE\tTITLE\tIDENTIFIER.

  click TITLE [--role ROLE] [--pid N | --bundle ID | --focused]
      Find an element by title (optional role filter) and fire AXPress.
      Exits 0 on success, 1 if not found, 2 on any other error.

  at-point X Y
      Print the AX role + title + PID of whatever is at the given
      global screen coordinates.

  trust [--prompt]
      Print 1 if Accessibility is granted, 0 otherwise. --prompt
      shows the system dialog.

  version
      Print version info.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "tree":
		cmdTree(args)
	case "attr":
		cmdAttr(args)
	case "attrs":
		cmdAttrs(args)
	case "actions":
		cmdActions(args)
	case "find":
		cmdFind(args)
	case "click":
		cmdClick(args)
	case "at-point":
		cmdAtPoint(args)
	case "trust":
		cmdTrust(args)
	case "version":
		fmt.Printf("kinax-go %s (go %s, %s/%s)\n",
			kinax.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		// Call Load() so dylib path is populated before we print it.
		_ = kinax.Load()
		fmt.Printf("dylib: %s\n", kinax.ResolvedDylibPath())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "kinax: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// ─── target selection ────────────────────────────────────────

type target struct {
	pid    int
	bundle string
}

// parseTarget pulls --pid / --bundle / --focused out of args and returns
// the remaining args plus the target.
func parseTarget(args []string) (target, []string) {
	var t target
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pid":
			if i+1 >= len(args) {
				fatalf("--pid needs a value\n")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				fatalf("--pid: %v\n", err)
			}
			t.pid = n
			i++
		case "--bundle":
			if i+1 >= len(args) {
				fatalf("--bundle needs a value\n")
			}
			t.bundle = args[i+1]
			i++
		case "--focused":
			// default behavior, explicit for clarity
		default:
			rest = append(rest, args[i])
		}
	}
	return t, rest
}

func (t target) open() (*kinax.Element, error) {
	switch {
	case t.pid != 0:
		return kinax.ApplicationByPID(t.pid)
	case t.bundle != "":
		return kinax.ApplicationByBundleID(t.bundle)
	default:
		return kinax.FocusedApplication()
	}
}

// ─── commands ────────────────────────────────────────────────

func cmdTree(args []string) {
	t, rest := parseTarget(args)
	depth := 6
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--depth" && i+1 < len(rest) {
			n, err := strconv.Atoi(rest[i+1])
			if err != nil {
				fatalf("--depth: %v\n", err)
			}
			depth = n
			i++
		}
	}
	app, err := t.open()
	check(err)
	defer app.Close()

	printTree(app, 0, depth)
}

func printTree(e *kinax.Element, indent, maxDepth int) {
	role, _ := e.Role()
	title, _ := e.Title()
	id, _ := e.Identifier()

	pad := ""
	for i := 0; i < indent; i++ {
		pad += "  "
	}
	line := fmt.Sprintf("%s%s", pad, role)
	if title != "" {
		line += fmt.Sprintf(" %q", title)
	}
	if id != "" {
		line += fmt.Sprintf(" [%s]", id)
	}
	fmt.Println(line)

	if indent >= maxDepth {
		return
	}
	kids, err := e.Children()
	if err != nil {
		return
	}
	for _, k := range kids {
		printTree(k, indent+1, maxDepth)
		k.Close()
	}
}

func cmdAttr(args []string) {
	t, rest := parseTarget(args)
	if len(rest) < 1 {
		fatalf("attr: expected ATTR\n")
	}
	app, err := t.open()
	check(err)
	defer app.Close()

	v, err := app.Attribute(rest[0])
	check(err)
	fmt.Println(v)
}

func cmdAttrs(args []string) {
	t, _ := parseTarget(args)
	app, err := t.open()
	check(err)
	defer app.Close()

	names, err := app.AttributeNames()
	check(err)
	for _, n := range names {
		fmt.Println(n)
	}
}

func cmdActions(args []string) {
	t, _ := parseTarget(args)
	app, err := t.open()
	check(err)
	defer app.Close()

	names, err := app.ActionNames()
	check(err)
	for _, n := range names {
		fmt.Println(n)
	}
}

func cmdFind(args []string) {
	t, rest := parseTarget(args)
	depth := 20
	var role, title string
	var positional []string
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--depth" && i+1 < len(rest) {
			n, err := strconv.Atoi(rest[i+1])
			if err != nil {
				fatalf("--depth: %v\n", err)
			}
			depth = n
			i++
			continue
		}
		positional = append(positional, rest[i])
	}
	if len(positional) < 1 {
		fatalf("find: expected ROLE [TITLE]\n")
	}
	role = positional[0]
	if len(positional) >= 2 {
		title = positional[1]
	}
	app, err := t.open()
	check(err)
	defer app.Close()

	var matcher kinax.Matcher
	if title != "" {
		matcher = kinax.MatchRoleAndTitle(role, title)
	} else {
		matcher = kinax.MatchRole(role)
	}
	hits := app.FindAll(matcher, depth)
	for _, h := range hits {
		r, _ := h.Role()
		ttl, _ := h.Title()
		id, _ := h.Identifier()
		fmt.Printf("%s\t%s\t%s\n", r, ttl, id)
		h.Close()
	}
}

func cmdClick(args []string) {
	t, rest := parseTarget(args)
	var role, title string
	var positional []string
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--role" && i+1 < len(rest) {
			role = rest[i+1]
			i++
			continue
		}
		positional = append(positional, rest[i])
	}
	if len(positional) < 1 {
		fatalf("click: expected TITLE\n")
	}
	title = positional[0]

	app, err := t.open()
	check(err)
	defer app.Close()

	var matcher kinax.Matcher
	if role != "" {
		matcher = kinax.MatchRoleAndTitle(role, title)
	} else {
		matcher = kinax.MatchTitle(title)
	}
	el, ok := app.FindFirst(matcher, 20)
	if !ok {
		fmt.Fprintf(os.Stderr, "click: no element titled %q\n", title)
		os.Exit(1)
	}
	defer el.Close()
	if err := el.Perform(kinax.ActionPress); err != nil {
		fmt.Fprintf(os.Stderr, "click: %v\n", err)
		os.Exit(2)
	}
}

func cmdAtPoint(args []string) {
	if len(args) != 2 {
		fatalf("at-point: expected X Y\n")
	}
	x, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		fatalf("invalid X: %v\n", err)
	}
	y, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		fatalf("invalid Y: %v\n", err)
	}
	el, err := kinax.ElementAtPoint(x, y)
	check(err)
	defer el.Close()
	role, _ := el.Role()
	title, _ := el.Title()
	fmt.Printf("%s\t%s\n", role, title)
}

func cmdTrust(args []string) {
	prompt := false
	for _, a := range args {
		if a == "--prompt" {
			prompt = true
		}
	}
	var ok bool
	if prompt {
		ok = kinax.PromptTrust()
	} else {
		ok = kinax.Trusted()
	}
	if ok {
		fmt.Println("1")
	} else {
		fmt.Println("0")
		os.Exit(1)
	}
}

// ─── helpers ─────────────────────────────────────────────────

func check(err error) {
	if err != nil {
		fatalf("error: %v\n", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
