package dict

import (
	"os"
	"path/filepath"
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

// seedDict writes initial into a temp dictionary file and points
// PRATA_DICT_PATH at it so Save/Reload resolve there. An empty initial
// leaves the file absent (to exercise Save creating it). Returns the
// path.
func seedDict(t *testing.T, initial string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary-corrections.txt")
	if initial != "" {
		if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
			t.Fatalf("seed dictionary: %v", err)
		}
	}
	t.Setenv("PRATA_DICT_PATH", path)
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
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

func TestSaveAppendsNewRule(t *testing.T) {
	path := seedDict(t, "# header\nfoo = bar\n")
	ok, err := Save("baz", "qux")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if !ok {
		t.Fatal("Save = false, want true (a new rule should be written)")
	}
	want := "# header\nfoo = bar\nbaz = qux\n"
	if got := readFile(t, path); got != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestSaveReplacesExistingKeyInPlace(t *testing.T) {
	path := seedDict(t, "# header\nfoo = bar\nhello = world\n")
	ok, err := Save("foo", "BAR")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if !ok {
		t.Fatal("Save = false, want true")
	}
	got := readFile(t, path)
	want := "# header\nfoo = BAR\nhello = world\n"
	if got != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
	// The key must not be duplicated: exactly one "foo =" line.
	if n := strings.Count(got, "foo ="); n != 1 {
		t.Errorf("found %d %q lines, want 1 (in-place replace, no duplicate)", n, "foo =")
	}
}

func TestSaveCreatesMissingFile(t *testing.T) {
	path := seedDict(t, "") // no file on disk yet
	ok, err := Save("foo", "bar")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if !ok {
		t.Fatal("Save = false, want true")
	}
	if got := readFile(t, path); got != "foo = bar\n" {
		t.Errorf("file = %q, want %q", got, "foo = bar\n")
	}
}

func TestSaveIgnoresEmptyAndIdentity(t *testing.T) {
	path := seedDict(t, "foo = bar\n")
	cases := [][2]string{
		{"", "x"},
		{"x", ""},
		{"   ", "x"},
		{"same", "same"},
		{" same ", "same"}, // identity after trimming
	}
	for _, c := range cases {
		ok, err := Save(c[0], c[1])
		if err != nil {
			t.Fatalf("Save(%q, %q) returned error: %v", c[0], c[1], err)
		}
		if ok {
			t.Errorf("Save(%q, %q) = true, want false (no write)", c[0], c[1])
		}
	}
	if got := readFile(t, path); got != "foo = bar\n" {
		t.Errorf("file changed by ignored rules: %q", got)
	}
}

func TestSavePreservesCommentsAndBlankLines(t *testing.T) {
	path := seedDict(t, "# c1\n\nfoo = bar\n\n# c2\n")
	ok, err := Save("new", "rule")
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if !ok {
		t.Fatal("Save = false, want true")
	}
	want := "# c1\n\nfoo = bar\n\n# c2\nnew = rule\n"
	if got := readFile(t, path); got != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestApplyMatchesKeyEndingInSwedishVowel(t *testing.T) {
	// "blå" ends in 'å'; under ASCII \b the trailing boundary never held,
	// so this never matched. A following space is now a proper boundary.
	d := mustLoad(t, "blå = grön\n")
	if got := d.Apply("en blå bil"); got != "en grön bil" {
		t.Errorf("Apply = %q, want %q", got, "en grön bil")
	}
}

func TestApplyDoesNotMatchKeyAsWordSuffix(t *testing.T) {
	// "blå" must not match inside "himmelsblå": the preceding 's' is a
	// word character, so it is not a whole word.
	d := mustLoad(t, "blå = grön\n")
	if got := d.Apply("himmelsblå"); got != "himmelsblå" {
		t.Errorf("Apply overmatched a word suffix: got %q, want unchanged", got)
	}
}

func TestApplyMatchesKeyStartingInSwedishVowel(t *testing.T) {
	d := mustLoad(t, "öra = öron\n")
	if got := d.Apply("ett öra"); got != "ett öron" {
		t.Errorf("Apply = %q, want %q", got, "ett öron")
	}
	// Inside "höra" the leading 'h' is a word char: not a whole word.
	if got := d.Apply("höra"); got != "höra" {
		t.Errorf("Apply overmatched inside word: got %q, want unchanged", got)
	}
}

func TestApplyNoOvermatchAcrossSwedishBoundary(t *testing.T) {
	// The classic ASCII-\b bug: 'å' was treated as a word separator, so
	// "sken" wrongly matched inside "påsken". It must not anymore.
	d := mustLoad(t, "sken = ljus\n")
	if got := d.Apply("påsken"); got != "påsken" {
		t.Errorf("Apply overmatched across 'å': got %q, want unchanged", got)
	}
}

func TestApplyAsciiBoundaryUnchanged(t *testing.T) {
	// ASCII behavior must be identical to the old \b: "neer" is a
	// substring of "Generera" but not a whole word.
	d := mustLoad(t, "neer = NEER\n")
	if got := d.Apply("Generera"); got != "Generera" {
		t.Errorf("Apply changed an ASCII substring: got %q, want unchanged", got)
	}
}

func TestApplyReplacesAllAdjacentOccurrences(t *testing.T) {
	d := mustLoad(t, "foo = bar\n")
	if got := d.Apply("foo foo foo"); got != "bar bar bar" {
		t.Errorf("Apply = %q, want %q", got, "bar bar bar")
	}
	// Joined without a separator is one longer word, not two: no match.
	if got := d.Apply("foofoo"); got != "foofoo" {
		t.Errorf("Apply overmatched joined words: got %q, want unchanged", got)
	}
}

func TestReloadPicksUpSavedRule(t *testing.T) {
	seedDict(t, "foo = bar\n")

	// Build a running Dict from the seeded file (path field stays empty,
	// like cmd/prata's dict.Load(f); Reload then resolves PRATA_DICT_PATH).
	f, err := os.Open(os.Getenv("PRATA_DICT_PATH"))
	if err != nil {
		t.Fatalf("open seeded dict: %v", err)
	}
	d, err := Load(f)
	f.Close()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := d.Apply("baz"); got != "baz" {
		t.Fatalf("precondition: Apply(%q) = %q, want unchanged", "baz", got)
	}

	if ok, err := Save("baz", "qux"); err != nil || !ok {
		t.Fatalf("Save: ok=%v err=%v", ok, err)
	}

	if err := d.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := d.Apply("baz"); got != "qux" {
		t.Errorf("after Reload, Apply(%q) = %q, want %q", "baz", got, "qux")
	}
}
