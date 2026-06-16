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
	c := newTestClient("secret-key", Backend{ID: "Hemma", DisplayName: "Rngv GPU-server", URL: srv.URL, RequiresKey: false})
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
	c := newTestClient("", Backend{ID: "Jobb", DisplayName: "Rum1 GPU-server", URL: "", RequiresKey: false})
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
	cases := []struct{ in, want string }{
		{"hej", "hej"},
		{" inledande och avslutande \n", "inledande och avslutande"},
		{"första raden\n andra raden\n tredje raden\n", "första raden andra raden tredje raden"},
		{"a\r\nb\tc   d", "a b c d"},
		{"", ""},
		{"\n\n", ""},
	}
	for _, c := range cases {
		if got := normalizeTranscript(c.in); got != c.want {
			t.Errorf("normalizeTranscript(%q) = %q, want %q", c.in, got, c.want)
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

	c := newTestClient("", Backend{ID: "Hemma", DisplayName: "Rngv GPU-server", URL: srv.URL, RequiresKey: false})
	got, err := c.Transcribe(bytes.NewReader(EncodePCM([]byte{1, 2, 3, 4})))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	want := "Även om jag dikterar ganska långa meningar så går det fort."
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
