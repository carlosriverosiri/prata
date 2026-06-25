package sanity

import (
	"strings"
	"testing"
)

func TestIsDegenerate(t *testing.T) {
	cases := map[string]struct {
		in   string
		want bool
	}{
		"natural sentence is not degenerate": {
			in:   "Patienten har god rörlighet i höften och inga tecken på inflammation idag.",
			want: false,
		},
		"long repetition loop is degenerate": {
			in:   strings.Repeat("O A ", 100),
			want: true,
		},
		"short repetition below floor is not degenerate": {
			// High ratio, but under the length floor where gzip's
			// header overhead makes the ratio meaningless.
			in:   strings.Repeat("O A ", 5),
			want: false,
		},
		"empty string is not degenerate": {
			in:   "",
			want: false,
		},

		// --- Legitimate but repetitive clinical dictation must survive ---
		// These are the false-positive risk: real journal text that repeats a
		// word ("ingen", "bilateralt", "utan anmärkning") across varied content.
		// Their gzip ratios top out around 1.8 (measured 2026-06-25), well below
		// maxRatio=2.4, so they are kept. They also guard against anyone lowering
		// the threshold into false-positive territory.
		"repeated negations are kept": {
			in:   "Ingen feber, ingen frossa, ingen hosta, ingen andnöd, ingen smärta, ingen yrsel, ingen illamående, ingen kräkning, ingen diarré, ingen trötthet.",
			want: false,
		},
		"bilateral findings are kept": {
			in:   "Axel normal bilateralt, armbåge normal bilateralt, handled normal bilateralt, höft normal bilateralt, knä normal bilateralt, fotled normal bilateralt.",
			want: false,
		},
		"utan anmärkning list is kept": {
			in:   "Hjärta utan anmärkning, lungor utan anmärkning, buk utan anmärkning, hud utan anmärkning, leder utan anmärkning, lymfkörtlar utan anmärkning.",
			want: false,
		},

		// --- Phrase/sentence loops: caught when repetition is high enough ---
		"repeated sentence loop is degenerate": {
			in:   strings.Repeat("Patienten mår bra idag. ", 6),
			want: true,
		},
		"repeated short sentence loop is degenerate": {
			in:   strings.Repeat("Inga anmärkningar. ", 12),
			want: true,
		},

		// --- Known limitation (documented, not a bug) ---
		// A sentence repeated only ~4x compresses to ~1.9, below maxRatio, so the
		// gzip guard does NOT catch it. The ratio cannot be lowered to catch it
		// without discarding the legitimate repetitive cases above (which reach
		// ~1.8). Low-repetition phrase loops are short and visible to the user,
		// who can delete and re-dictate. See PRATA-REVIEW §15 #7.
		"low-repetition phrase loop slips through (known limit)": {
			in:   strings.Repeat("Patienten mår bra idag. ", 4),
			want: false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsDegenerate(tc.in); got != tc.want {
				t.Errorf("IsDegenerate(%q) = %v, want %v (ratio %.2f)", tc.in, got, tc.want, Ratio(tc.in))
			}
		})
	}
}

func TestRatioEmptyString(t *testing.T) {
	if got := Ratio(""); got != 0 {
		t.Errorf("Ratio(\"\") = %v, want 0", got)
	}
}
