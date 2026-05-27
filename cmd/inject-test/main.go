// Command inject-test verifies internal/inject by typing the supplied
// text into whichever window has keyboard focus 3 seconds after launch.
//
// Usage:
//
//	inject-test "text to type"
//
// Run it, then switch focus to (e.g.) Notepad within 3 seconds. The
// text should appear there.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/carlosriveros/prata/internal/inject"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: inject-test <text>")
		os.Exit(2)
	}

	fmt.Fprintln(os.Stderr, "switch focus to target window; injecting in 3s...")
	time.Sleep(3 * time.Second)

	if err := inject.Type(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "inject: %v\n", err)
		os.Exit(1)
	}
}
