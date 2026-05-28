package dict

import (
	"strings"
	"testing"
)

func mustLoad(t *testing.T, s string) *Dict {
	t.Helper()
	d, err := Load(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	return d
}

func TestApplyReplacesWholeWords(t *testing.T) {
	d := mustLoad(t, "adoption = abduktion\n")
	got := d.Apply("patienten har en adoption i höften")
	want := "patienten har en abduktion i höften"
	if got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
}

func TestApplyRespectsWordBoundaries(t *testing.T) {
	d := mustLoad(t, "adoption = abduktion\n")
	// "adoptioner" must not match: the trailing "er" means there is no
	// word boundary after "adoption".
	in := "adoptioner"
	if got := d.Apply(in); got != in {
		t.Errorf("Apply changed a substring match: got %q, want %q", got, in)
	}
}

func TestApplyIgnoresCommentsAndBlankLines(t *testing.T) {
	src := "# a comment\n\n   \nadoption = abduktion\n# trailing comment\n"
	d := mustLoad(t, src)
	if got := d.Apply("adoption"); got != "abduktion" {
		t.Errorf("Apply = %q, want %q", got, "abduktion")
	}
}

func TestApplyRulesAreOrderedAndChained(t *testing.T) {
	// Rule 2 must see the output of rule 1: "a" -> "b" -> "c".
	d := mustLoad(t, "a = b\nb = c\n")
	if got := d.Apply("a"); got != "c" {
		t.Errorf("Apply = %q, want %q (rules should chain)", got, "c")
	}
}

func TestApplyReplacementWithDollarIsLiteral(t *testing.T) {
	// A '$' in the replacement must not be treated as a regexp
	// backreference; it should appear verbatim.
	d := mustLoad(t, "kod = pris$5\n")
	if got := d.Apply("kod"); got != "pris$5" {
		t.Errorf("Apply = %q, want %q", got, "pris$5")
	}
}

func TestApplyIsCaseSensitive(t *testing.T) {
	d := mustLoad(t, "adoption = abduktion\n")
	in := "Adoption"
	if got := d.Apply(in); got != in {
		t.Errorf("Apply matched case-insensitively: got %q, want %q", got, in)
	}
}

func TestLoadEmptyDictIsNoop(t *testing.T) {
	d := mustLoad(t, "# only comments\n\n")
	in := "anything goes here"
	if got := d.Apply(in); got != in {
		t.Errorf("empty dict altered text: got %q, want %q", got, in)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]string{
		"missing separator": "no equals sign here\n",
		"empty key":         "   = correction\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(strings.NewReader(src)); err == nil {
				t.Errorf("Load(%q) = nil error, want error", src)
			}
		})
	}
}
