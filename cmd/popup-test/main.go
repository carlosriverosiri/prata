// Command popup-test verifies the quick-fix popup in isolation (no F9, no
// dictionary, no inject-back). It opens a pre-filled modal text popup and
// prints the outcome to stderr. SetProcessDPIAware is called first so the
// popup's DPI sizing sees real per-monitor DPI instead of 96.
package main

import (
	"fmt"
	"os"

	"github.com/carlosriveros/prata/internal/popup"
	"github.com/carlosriveros/prata/internal/tray"
)

func main() {
	tray.SetProcessDPIAware()

	result, ok, err := popup.Prompt("kalle ankka")
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "popup error: %v\n", err)
		os.Exit(1)
	case !ok:
		fmt.Fprintln(os.Stderr, "cancelled")
	default:
		fmt.Fprintf(os.Stderr, "result: %q\n", result)
	}
}
