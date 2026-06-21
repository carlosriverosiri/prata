package transcribe

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient returns a Client whose http.Client is the default one, with
// the given key and active backend, without going through NewClient's Berget
// default.
func newTestClient(apiKey string, active Backend) *Client {
	c := NewClient(apiKey)
	c.SetBackend(active)
	return c
}

func TestTranscribeBergetSendsAuthAndFields(t *testing.T) {
	var gotAuth string
	var gotModel, gotLanguage, gotFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		gotModel = r.FormValue("model")
		gotLanguage = r.FormValue("language")
		gotFormat = r.FormValue("response_format")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hej"}`))
	}))
	defer srv.Close()

	c := newTestClient("secret-key", Backend{ID: "Berget", DisplayName: "Berget Ai", URL: srv.URL, RequiresKey: true})
	text, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "hej" {
		t.Errorf("text = %q, want %q", text, "hej")
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-key")
	}
	if gotModel != model {
		t.Errorf("model = %q, want %q", gotModel, model)
	}
	if gotLanguage != language {
		t.Errorf("language = %q, want %q", gotLanguage, language)
	}
	if gotFormat != "json" {
		t.Errorf("response_format = %q, want %q", gotFormat, "json")
	}
}

func TestTranscribeLocalBackendSendsNoAuth(t *testing.T) {
	var gotAuth string
	var hadAuthHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuthHeader = r.Header["Authorization"]
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"lokalt"}`))
	}))
	defer srv.Close()

	// A key is present but the local backend must never send it.
	c := newTestClient("secret-key", Backend{ID: "Hemma", DisplayName: "Rngv GPU-server (Tailscale)", URL: srv.URL, RequiresKey: false})
	text, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "lokalt" {
		t.Errorf("text = %q, want %q", text, "lokalt")
	}
	if hadAuthHeader {
		t.Errorf("Authorization header was sent (%q), want none for a local backend", gotAuth)
	}
}

func TestTranscribeEmptyURLFails(t *testing.T) {
	c := newTestClient("", Backend{ID: "Jobb", DisplayName: "LAN GPU-server", URL: "", RequiresKey: false})
	_, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err == nil {
		t.Fatal("Transcribe with empty URL: want error, got nil")
	}
	if !strings.Contains(err.Error(), "no configured URL") {
		t.Errorf("error = %q, want it to mention the missing URL", err)
	}
}

func TestTranscribeBergetWithoutKeyFails(t *testing.T) {
	c := newTestClient("", Berget)
	_, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err == nil {
		t.Fatal("Berget without key: want error, got nil")
	}
	if !strings.Contains(err.Error(), "requires an API key") {
		t.Errorf("error = %q, want it to mention the missing key", err)
	}
}

func TestNormalizeTranscript(t *testing.T) {
	cases := []struct {
		in, want        string
		trimmedSegments bool
	}{
		{in: "hej", want: "hej"},
		{in: " inledande och avslutande \n", want: "inledande och avslutande"},
		// Local whisper.cpp (untrimmed segments): boundaries at real word
		// boundaries carry their own leading space, so the spacing is preserved.
		{in: "första raden\n andra raden\n tredje raden\n", want: "första raden andra raden tredje raden"},
		// Local: segment boundary inside a word — the continuation has no leading
		// space, so the line break must vanish without inserting one.
		{in: "Tyd\nlighet", want: "Tydlighet"},
		{in: "a\r\nb\tc   d", want: "ab c d"},
		// Local: real captured server output where whisper split "Tydlighet"
		// across a segment boundary. Must read "Tydlighet", not "Tyd lighet".
		{
			in:   " Nu ska jag testa röstinspelaren med kärnenergifrågan. Tyd\nlighet, små, enligt, akromeoplastik.\n",
			want: "Nu ska jag testa röstinspelaren med kärnenergifrågan. Tydlighet, små, enligt, akromeoplastik.",
		},
		// Berget (trimmedSegments=true): each sentence is a trimmed line, so the
		// line break is the only separator and MUST become a space. Without this,
		// sentences glue together ("förluster.Ungdomarna").
		{
			in:              "Vi talar om sorg och förluster.\nUngdomarna kommer få berätta.\nVi inleder med oss vuxna.",
			want:            "Vi talar om sorg och förluster. Ungdomarna kommer få berätta. Vi inleder med oss vuxna.",
			trimmedSegments: true,
		},
		{in: "", want: ""},
		{in: "\n\n", want: ""},
	}
	for _, c := range cases {
		if got := normalizeTranscript(c.in, c.trimmedSegments); got != c.want {
			t.Errorf("normalizeTranscript(%q, %v) = %q, want %q", c.in, c.trimmedSegments, got, c.want)
		}
	}
}

// TestTranscribeNormalizesSegmentedText mirrors the real symptom: Whisper
// returns each timing segment on its own line, and Transcribe must collapse
// those into one flowing prose block (like Diktell), not a poem.
func TestTranscribeNormalizesSegmentedText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":" Även om jag dikterar\n ganska långa meningar\n så går det fort.\n"}`))
	}))
	defer srv.Close()

	c := newTestClient("", Backend{ID: "Hemma", DisplayName: "Rngv GPU-server (Tailscale)", URL: srv.URL, RequiresKey: false})
	got, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	want := "Även om jag dikterar ganska långa meningar så går det fort."
	if got != want {
		t.Errorf("Transcribe = %q, want %q", got, want)
	}
}

// TestTranscribeJoinsMidWordSegmentSplit reproduces the särskrivning symptom:
// whisper occasionally places a segment boundary inside a long word, so the
// continuation segment has no leading space ("Tyd" + "lighet"). Transcribe must
// join them into "Tydlighet", never "Tyd lighet". The \n below is a JSON escape
// the decoder turns into a real newline, exactly as the GPU server sends it.
func TestTranscribeJoinsMidWordSegmentSplit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":" med kärnenergifrågan. Tyd\nlighet, små, enligt.\n"}`))
	}))
	defer srv.Close()

	c := newTestClient("", Backend{ID: "Hemma", DisplayName: "Rngv GPU-server (Tailscale)", URL: srv.URL, RequiresKey: false})
	got, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	want := "med kärnenergifrågan. Tydlighet, små, enligt."
	if got != want {
		t.Errorf("Transcribe = %q, want %q", got, want)
	}
}

// TestTranscribeBergetKeepsSentenceSpaces reproduces the Berget regression:
// Berget trims each segment line, so a sentence boundary lands as a bare
// newline with no leading space on the next line ("förluster." + "Ungdomarna").
// With TrimmedSegments=true, Transcribe must turn that break into a space so
// sentences do not glue together ("förluster.Ungdomarna").
func TestTranscribeBergetKeepsSentenceSpaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"Vi talar om sorg och förluster.\nUngdomarna kommer få berätta.\n"}`))
	}))
	defer srv.Close()

	c := newTestClient("secret-key", Backend{ID: "Berget", DisplayName: "Berget Ai", URL: srv.URL, RequiresKey: true, TrimmedSegments: true})
	got, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	want := "Vi talar om sorg och förluster. Ungdomarna kommer få berätta."
	if got != want {
		t.Errorf("Transcribe = %q, want %q", got, want)
	}
}

func TestBackendByName(t *testing.T) {
	for _, want := range Backends {
		got, ok := BackendByName(want.ID)
		if !ok {
			t.Errorf("BackendByName(%q) not found", want.ID)
			continue
		}
		if got != want {
			t.Errorf("BackendByName(%q) = %+v, want %+v", want.ID, got, want)
		}
	}
	if _, ok := BackendByName("Nonexistent"); ok {
		t.Error("BackendByName(unknown) = ok, want not found")
	}
}
