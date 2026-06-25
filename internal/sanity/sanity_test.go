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
		"short below-floor non-loop is not degenerate": {
			// Short and repetitive enough that the gzip ratio is unreliable
			// (below the length floor), but only 3 words — under looksRepeated's
			// back-to-back threshold — so neither signal fires.
			in:   "ja ja ja",
			want: false,
		},
		"short token loop is caught by repetition check": {
			// "O A O A O A O A O A" — the canonical Whisper loop. Below the gzip
			// length floor, but looksRepeated catches the back-to-back "O A".
			in:   strings.Repeat("O A ", 5),
			want: true,
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

		// --- Phrase/sentence loops: caught by gzip ratio (high repetition) ---
		"repeated sentence loop is degenerate": {
			in:   strings.Repeat("Patienten mår bra idag. ", 6),
			want: true,
		},
		"repeated short sentence loop is degenerate": {
			in:   strings.Repeat("Inga anmärkningar. ", 12),
			want: true,
		},

		// --- Low-repetition phrase loops: caught by looksRepeated, not gzip ---
		// A sentence repeated only 4x compresses to ~1.9 (under maxRatio), so the
		// gzip ratio misses it; the back-to-back multi-word repetition check
		// catches it. See PRATA-REVIEW §15 #7.
		"phrase loop repeated 4x is degenerate": {
			in:   strings.Repeat("Patienten mår bra idag. ", 4),
			want: true,
		},
		"phrase loop after real text is degenerate": {
			in:   "Patienten har ont i höger knä sedan en vecka. " + strings.Repeat("Tack så mycket för idag. ", 4),
			want: true,
		},

		// --- Accepted gaps (documented, not bugs) ---
		// A phrase repeated only 3x is ambiguous with a spoken read-back, so it is
		// left alone (looksRepeated needs 4, gzip needs ~2.4). Short and visible.
		"phrase repeated only 3x is kept": {
			in:   strings.Repeat("Patienten mår bra idag. ", 3),
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
