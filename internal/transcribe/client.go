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
// WorkURL targets the work GPU server on the clinic LAN at its fixed IP
// (10.64.3.60). It is only reachable from inside the clinic network, so
// selecting Rum1 GPU-server off-site fails with an error cue rather than falling back
// silently. Prata follows "edit constant + recompile", no config file — change
// this and rebuild if the server is re-addressed.
const (
	HomeURL   = "http://100.87.6.56:8080/v1/audio/transcriptions"
	WorkURL   = "http://10.64.3.60:8080/v1/audio/transcriptions"
	BergetURL = "https://api.berget.ai/v1/audio/transcriptions"
)

// Backend identifies where transcription requests go and whether they need
// Berget authentication. Prata never switches backend silently; the active
// backend is chosen deliberately in the tray menu and shown there — a
// backend swapped under the user in a patient-data tool is a safety problem,
// not a convenience (see PRATA-GPU-SERVER.md, "Ingen automatisk failover").
type Backend struct {
	ID          string // stable identifier for persistence and lookup (backend.txt)
	DisplayName string // shown in the tray menu, tooltip, and user-facing messages
	URL         string // full OpenAI-compatible transcription endpoint
	RequiresKey bool   // Berget needs a Bearer key; local GPU servers do not
}

// The selectable backends. Home and Work are local whisper.cpp GPU servers
// (no auth); Berget is the cloud fallback (Bearer-authenticated).
var (
	Home   = Backend{ID: "Hemma", DisplayName: "Rngv GPU-server (Tailscale)", URL: HomeURL, RequiresKey: false}
	Work   = Backend{ID: "Jobb", DisplayName: "Rum1 GPU-server", URL: WorkURL, RequiresKey: false}
	Berget = Backend{ID: "Berget", DisplayName: "Berget Ai", URL: BergetURL, RequiresKey: true}
)

// Backends is the selectable list, in tray-menu order.
var Backends = []Backend{Home, Work, Berget}

// BackendByName returns the predefined backend with the given stable ID. It is
// used to resolve a persisted selection back to a Backend on startup.
func BackendByName(id string) (Backend, bool) {
	for _, b := range Backends {
		if b.ID == id {
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
		return "", fmt.Errorf("backend %q has no configured URL", b.DisplayName)
	}
	if b.RequiresKey && c.apiKey == "" {
		return "", fmt.Errorf("backend %q requires an API key but none is set", b.DisplayName)
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
		return "", fmt.Errorf("post to %s backend: %w", b.DisplayName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%s backend returned %d: %s", b.DisplayName, resp.StatusCode, string(errBody))
	}

	var out transcriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return normalizeTranscript(out.Text), nil
}

// normalizeTranscript joins the per-segment lines the backend puts in the
// "text" field into one flowing prose block, the way Diktell does: by
// concatenating segments with no separator. Whisper (the whisper.cpp server and
// Berget alike) serializes each timing segment on its own line. A real word
// boundary already carries a leading space on the next segment, but a boundary
// that falls *inside* a word — which whisper does for long Swedish compounds,
// e.g. "Tyd" + "lighet" or "kärnenergifrå" + "gan" — does not. Replacing the
// line break with a space (a naive Fields/Join over all whitespace) therefore
// injects a space inside such a word ("Tyd lighet"); dropping the line break
// instead preserves "Tydlighet". Remaining runs of spaces/tabs are collapsed
// and the result is trimmed. The trailing newline that marks the end of a
// dictation is added later in cmd/prata, so it is intentionally not produced
// here.
func normalizeTranscript(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return strings.Join(strings.Fields(s), " ")
}
