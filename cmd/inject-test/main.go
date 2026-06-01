// Command inject-test verifies internal/inject by typing the supplied
// text into whichever window has keyboard focus 3 seconds after launch.
//
// Usage:
//
//	inject-test [-mode clipboard|unicode|auto] [-nl] "text to type"
//
// Run it, then switch focus to (e.g.) Notepad within 3 seconds. The text
// should appear there. -mode selects the injection route: "clipboard"
// (default) uses inject.Type (clipboard + Ctrl+V); "unicode" uses
// inject.TypeUnicode (single-call SendInput + KEYEVENTF_UNICODE); "auto"
// uses inject.TypeAuto, which routes on the foreground window's class and
// logs the chosen route. -nl replaces literal "\n" in the argument with a
// real newline (useful where the shell does not).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/carlosriveros/prata/internal/inject"
)

func main() {
	mode := flag.String("mode", "clipboard", `injection mode: "clipboard" (inject.Type, default), "unicode" (inject.TypeUnicode), or "auto" (inject.TypeAuto, class-based routing)`)
	nl := flag.Bool("nl", false, `interpret literal "\n" in the text as a newline (handy where the shell does not)`)
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: inject-test [-mode clipboard|unicode|auto] [-nl] <text>")
		os.Exit(2)
	}
	text := flag.Arg(0)
	if *nl {
		text = strings.ReplaceAll(text, "\\n", "\n")
	}

	var typeFn func(string) error
	switch *mode {
	case "clipboard":
		typeFn = inject.Type
	case "unicode":
		typeFn = inject.TypeUnicode
	case "auto":
		typeFn = inject.TypeAuto
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q (want clipboard, unicode, or auto)\n", *mode)
		os.Exit(2)
	}

	fmt.Fprintln(os.Stderr, "switch focus to target window; injecting in 3s...")
	time.Sleep(3 * time.Second)

	// Diagnostic for the class-based routing: report which window will
	// receive the injection and whether its class could be read.
	class, ok := inject.ForegroundWindowClass()
	fmt.Fprintf(os.Stderr, "foreground class=%q (ok=%v)\n", class, ok)

	// In auto mode, surface the route TypeAuto will take. TypeAuto re-reads
	// the class internally; this mirrors that decision from the read above.
	if *mode == "auto" {
		route := "clipboard"
		if ok && inject.IsSendInputSafeClass(class) {
			route = "unicode"
		}
		fmt.Fprintf(os.Stderr, "route=%s\n", route)
	}

	if err := typeFn(text); err != nil {
		fmt.Fprintf(os.Stderr, "inject: %v\n", err)
		os.Exit(1)
	}
}
