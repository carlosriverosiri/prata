// Package transcribe sends audio to Berget AI's transcription endpoint
// and returns the recognized Swedish text.
package transcribe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const (
	endpoint    = "https://api.berget.ai/v1/audio/transcriptions"
	model       = "KBLab/kb-whisper-large"
	language    = "sv"
	httpTimeout = 30 * time.Second
)

// Client transcribes audio via Berget AI.
type Client struct {
	apiKey string
	http   *http.Client
}

// NewClient returns a Client configured with the given Berget API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// transcriptionResponse mirrors the JSON shape returned by Berget for
// response_format=json. Additional fields (duration, language, segments)
// are deliberately ignored — Prata only needs the text.
type transcriptionResponse struct {
	Text string `json:"text"`
}

// Transcribe sends a WAV-encoded audio stream to Berget and returns the
// transcribed text. The reader must contain a complete WAV file
// (RIFF header + PCM data); raw PCM will be rejected by the API.
func (c *Client) Transcribe(wav io.Reader) (string, error) {
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

	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("post to berget: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("berget returned %d: %s", resp.StatusCode, string(b))
	}

	var out transcriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return out.Text, nil
}
