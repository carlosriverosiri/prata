// Command sanity-test is a dev-only calibration tool for the
// internal/sanity package. It prints the gzip compression ratio and
// IsDegenerate verdict for a fixed set of representative dictation
// examples, so the 2.4 threshold can be eyeballed before the guard is
// trusted in production.
//
// Usage:
//
//	go run ./cmd/sanity-test/
package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/carlosriveros/prata/internal/sanity"
)

type example struct {
	label string
	text  string
}

var examples = []example{
	{"natural short", "Patienten har god rörlighet i höften utan smärta vid abduktion."},
	{"natural long", "Vänster axel har full passiv rörlighet men aktiv abduktion begränsas till nittio grader av smärta över rotatorkuffen, utan tecken på instabilitet eller neurologiskt bortfall."},
	{"phone digits", "Telefonnummer noll sju noll fyra åtta fyra åtta fyra åtta."},
	{"number stream", "noll sju noll fyra åtta fyra åtta fyra åtta noll ett två tre fyra fem sex sju åtta nio noll noll noll noll noll"},
	{"personnummer", "Personnummer nitton sjuttiotvå noll tre femton fyra åtta noll noll."},
	{"degenerate loop", strings.Repeat("O A ", 100)},
	{"pure repetition", strings.Repeat("O", 300)},
}

func main() {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "label\tratio\tverdict\tbytes\tpreview")
	fmt.Fprintln(w, "-----\t-----\t-------\t-----\t-------")
	for _, ex := range examples {
		ratio := sanity.Ratio(ex.text)
		verdict := "ok"
		if sanity.IsDegenerate(ex.text) {
			verdict = "DEGEN"
		}
		fmt.Fprintf(w, "%s\t%.2f\t%s\t%d\t%s\n",
			ex.label, ratio, verdict, len(ex.text), preview(ex.text, 50))
	}
	w.Flush()
}

// preview returns the first n runes of s, appending "..." when truncated.
func preview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
