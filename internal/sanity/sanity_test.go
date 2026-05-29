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
