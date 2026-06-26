package main

import "testing"

func TestExtractValue_IntConst(t *testing.T) {
	src := "// transcribeQueueDepth bounds the queue.\nconst transcribeQueueDepth = 8\n"
	got, ok := extractValue(src, reConst("transcribeQueueDepth"))
	if !ok || got != "8" {
		t.Fatalf("reConst: got %q ok=%v, want \"8\" true", got, ok)
	}
}

func TestExtractValue_Duration(t *testing.T) {
	src := "\tpasteSettleDelay = 400 * time.Millisecond\n"
	got, ok := extractValue(src, reDur("pasteSettleDelay"))
	if !ok || got != "400 * time.Millisecond" {
		t.Fatalf("reDur: got %q ok=%v, want \"400 * time.Millisecond\" true", got, ok)
	}
}

func TestExtractValue_Float(t *testing.T) {
	src := "\tmaxRatio = 2.4\n"
	got, ok := extractValue(src, reFloat("maxRatio"))
	if !ok || got != "2.4" {
		t.Fatalf("reFloat: got %q ok=%v, want \"2.4\" true", got, ok)
	}
}

func TestExtractValue_Module(t *testing.T) {
	src := "module github.com/carlosriveros/prata\n\ngo 1.26.3\n"
	got, ok := extractValue(src, facts[0].re)
	if !ok || got != "github.com/carlosriveros/prata" {
		t.Fatalf("module: got %q ok=%v", got, ok)
	}
}

func TestExtractValue_NotFound(t *testing.T) {
	if _, ok := extractValue("nothing here", reConst("absent")); ok {
		t.Fatal("expected ok=false for a missing constant")
	}
}

// reConst must not match a longer constant name that ends with the same word, or
// a different constant that merely shares a prefix.
func TestReConst_WordBoundary(t *testing.T) {
	src := "const minPhraseWords = 2\nconst minPhraseReps = 4\n"
	got, ok := extractValue(src, reConst("minPhraseReps"))
	if !ok || got != "4" {
		t.Fatalf("got %q ok=%v, want \"4\" true", got, ok)
	}
}

func TestExtractSection(t *testing.T) {
	src := "intro\n## Register\n| ID | Rejected |\n| REJ-001 | x |\nClass legend: foo\ntail"
	got, ok := extractSection(src, "| ID | Rejected", "Class legend")
	if !ok {
		t.Fatal("section not found")
	}
	if want := "| ID | Rejected |\n| REJ-001 | x |"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractSection_NoEnd(t *testing.T) {
	got, ok := extractSection("a\nSTART\nb\nc", "START", "NOPE")
	if !ok || got != "START\nb\nc" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestCountRejRows(t *testing.T) {
	src := "| ID | x |\n| REJ-001 | a |\n| REJ-002 | b |\n| REJ-010 | c |\nnot | REJ-x | row\n"
	if n := countMatches(src, rejRowRe); n != 3 {
		t.Fatalf("got %d REJ rows, want 3", n)
	}
}

func TestStripFirstHeading(t *testing.T) {
	if got := stripFirstHeading("# Title\nbody\n"); got != "body\n" {
		t.Fatalf("got %q", got)
	}
	if got := stripFirstHeading("no heading\n"); got != "no heading\n" {
		t.Fatalf("got %q", got)
	}
}

// Guard against a duplicate label slipping into the facts table.
func TestFactLabelsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range facts {
		if seen[f.label] {
			t.Fatalf("duplicate fact label: %q", f.label)
		}
		seen[f.label] = true
	}
}
