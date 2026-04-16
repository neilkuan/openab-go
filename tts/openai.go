package tts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenAIConfig holds configuration for the OpenAI TTS API.
type OpenAIConfig struct {
	APIKey       string // OpenAI API key
	Model        string // "tts-1", "tts-1-hd", or "gpt-4o-mini-tts"
	Voice        string // Built-in voice name (alloy, ash, ballad, coral, echo, etc.)
	Instructions string // Voice style instructions (gpt-4o-mini-tts only)
	BaseURL      string // Custom API endpoint (default: "https://api.openai.com/v1")
	TimeoutSec   int
}

// OpenAISynthesizer uses the OpenAI TTS API.
type OpenAISynthesizer struct {
	config OpenAIConfig
	client *http.Client
}

// NewOpenAISynthesizer creates a new OpenAI TTS client.
func NewOpenAISynthesizer(cfg OpenAIConfig) *OpenAISynthesizer {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "tts-1"
	}
	if cfg.Voice == "" {
		cfg.Voice = "alloy"
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 60
	}
	return &OpenAISynthesizer{
		config: cfg,
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second},
	}
}

// speechRequest is the JSON body for POST /audio/speech.
type speechRequest struct {
	Model        string `json:"model"`
	Input        string `json:"input"`
	Voice        string `json:"voice"`
	Instructions string `json:"instructions,omitempty"`
}

// Synthesize generates audio using the default configured voice.
func (s *OpenAISynthesizer) Synthesize(text string) (string, error) {
	req := speechRequest{
		Model:        s.config.Model,
		Input:        text,
		Voice:        s.config.Voice,
		Instructions: s.config.Instructions,
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(s.config.BaseURL, "/") + "/audio/speech"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	return s.saveResponse(httpReq, "mp3")
}

// saveResponse executes the request and saves audio response to a temp file.
func (s *OpenAISynthesizer) saveResponse(req *http.Request, ext string) (string, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tts API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
		return "", fmt.Errorf("tts API returned %d: %s", resp.StatusCode, string(body))
	}

	tmpDir := os.TempDir()
	localName := fmt.Sprintf("tts_%d.%s", time.Now().UnixMilli(), ext)
	localPath := filepath.Join(tmpDir, localName)

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	written, err := io.Copy(f, io.LimitReader(resp.Body, 50*1024*1024+1))
	if err != nil {
		f.Close()
		os.Remove(localPath)
		return "", fmt.Errorf("write audio: %w", err)
	}
	if written > 50*1024*1024 {
		f.Close()
		os.Remove(localPath)
		return "", fmt.Errorf("audio too large (>50MB)")
	}

	if err := f.Close(); err != nil {
		os.Remove(localPath)
		return "", fmt.Errorf("close file: %w", err)
	}

	return localPath, nil
}

// Verify interface satisfaction.
var _ Synthesizer = (*OpenAISynthesizer)(nil)
