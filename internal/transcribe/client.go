// Package transcribe sends audio to an OpenAI-compatible transcription
// endpoint and returns the recognized Swedish text. The endpoint is one of
// the selectable backends (a local whisper.cpp GPU server, or Berget AI as
// the cloud fallback); see PRATA-GPU-SERVER.md.
package transcribe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	model       = "KBLab/kb-whisper-large"
	language    = "sv"
	httpTimeout = 30 * time.Second
)

// Endpoint URLs for the selectable backends.
//
// HomeURL targets the home GPU server over Tailscale, so it is reachable
// from any network the client sits on (cabin, mobile hotspot, home LAN).
// WorkURL targets the work GPU server on the clinic LAN; it is left empty
// until that server is deployed and its LAN IP is known — set it then and
// rebuild (Prata follows "edit constant + recompile", no config file).
// Selecting a backend with an empty URL fails with an error cue rather than
// falling back silently.
const (
	HomeURL   = "http://100.87.6.56:8080/v1/audio/transcriptions"
	WorkURL   = ""
	BergetURL = "https://api.berget.ai/v1/audio/transcriptions"
)

// Backend identifies where transcription requests go and whether they need
// Berget authentication. Prata never switches backend silently; the active
// backend is chosen deliberately in the tray menu and shown there — a
// backend swapped under the user in a patient-data tool is a safety problem,
// not a convenience (see PRATA-GPU-SERVER.md, "Ingen automatisk failover").
type Backend struct {
	Name        string // shown in the tray menu and tooltip
	URL         string // full OpenAI-compatible transcription endpoint
	RequiresKey bool   // Berget needs a Bearer key; local GPU servers do not
}

// The selectable backends. Home and Work are local whisper.cpp GPU servers
// (no auth); Berget is the cloud fallback (Bearer-authenticated).
var (
	Home   = Backend{Name: "Hemma", URL: HomeURL, RequiresKey: false}
	Work   = Backend{Name: "Jobb", URL: WorkURL, RequiresKey: false}
	Berget = Backend{Name: "Berget", URL: BergetURL, RequiresKey: true}
)

// Backends is the selectable list, in tray-menu order.
var Backends = []Backend{Home, Work, Berget}

// BackendByName returns the predefined backend with the given name. It is
// used to resolve a persisted selection back to a Backend on startup.
func BackendByName(name string) (Backend, bool) {
	for _, b := range Backends {
		if b.Name == name {
			return b, true
		}
	}
	return Backend{}, false
}

// Client transcribes audio against the active backend.
type Client struct {
	apiKey string
	http   *http.Client

	mu     sync.Mutex
	active Backend
}

// NewClient returns a Client with the given Berget API key, defaulting to
// the Berget backend. Call SetBackend to switch. The key may be empty when
// only the local GPU backends are used.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http: &http.Client{
			Timeout: httpTimeout,
		},
		active: Berget,
	}
}

// SetBackend switches the active backend. Safe to call from any goroutine;
// the next Transcribe uses it. Prata calls this only from the deliberate
// tray selection — never as automatic failover.
func (c *Client) SetBackend(b Backend) {
	c.mu.Lock()
	c.active = b
	c.mu.Unlock()
}

// ActiveBackend returns the currently selected backend.
func (c *Client) ActiveBackend() Backend {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// transcriptionResponse mirrors the JSON shape returned by Berget for
// response_format=json. Additional fields (duration, language, segments)
// are deliberately ignored — Prata only needs the text.
type transcriptionResponse struct {
	Text string `json:"text"`
}

// Transcribe sends a WAV-encoded audio stream to the active backend and
// returns the transcribed text. The reader must contain a complete WAV file
// (RIFF header + PCM data); raw PCM will be rejected by the API.
//
// The Authorization header is sent only for backends that require it
// (Berget); the local whisper.cpp GPU servers take no auth. A backend with
// no configured URL, or Berget without a key, fails here rather than going
// out wrong on the wire.
func (c *Client) Transcribe(wav io.Reader) (string, error) {
	b := c.ActiveBackend()
	if b.URL == "" {
		return "", fmt.Errorf("backend %q has no configured URL", b.Name)
	}
	if b.RequiresKey && c.apiKey == "" {
		return "", fmt.Errorf("backend %q requires an API key but none is set", b.Name)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	fileWriter, err := mw.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(fileWriter, wav); err != nil {
		return "", fmt.Errorf("copy wav into form: %w", err)
	}

	if err := mw.WriteField("model", model); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}
	if err := mw.WriteField("language", language); err != nil {
		return "", fmt.Errorf("write language field: %w", err)
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("write response_format field: %w", err)
	}

	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, b.URL, &body)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	if b.RequiresKey {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("post to %s backend: %w", b.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%s backend returned %d: %s", b.Name, resp.StatusCode, string(errBody))
	}

	var out transcriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return normalizeTranscript(out.Text), nil
}

// normalizeTranscript collapses the per-segment line breaks the backend puts
// in the "text" field into single spaces, yielding one flowing prose block
// per dictation. Whisper (the whisper.cpp server and Berget alike) serializes
// each timing segment on its own line; those breaks fall on time-window cuts,
// not sentence boundaries, so passing them through made injected text read
// like a poem. This mirrors Diktell, which concatenates segments without a
// separator. The trailing newline that marks the end of a dictation is added
// later in cmd/prata, so it is intentionally not produced here.
func normalizeTranscript(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
