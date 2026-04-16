package tts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestOpenAISynthesizer_Synthesize_Success(t *testing.T) {
	fakeAudio := []byte("fake-mp3-audio-data")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/speech" {
			t.Errorf("expected /audio/speech, got %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing Bearer token")
		}

		var req speechRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		if req.Model != "tts-1" {
			t.Errorf("expected model 'tts-1', got %q", req.Model)
		}
		if req.Voice != "nova" {
			t.Errorf("expected voice 'nova', got %v", req.Voice)
		}

		w.Write(fakeAudio)
	}))
	defer server.Close()

	synth := NewOpenAISynthesizer(OpenAIConfig{
		APIKey:  "test-key",
		Model:   "tts-1",
		Voice:   "nova",
		BaseURL: server.URL,
	})

	audioPath, err := synth.Synthesize("Hello test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(audioPath)

	data, _ := os.ReadFile(audioPath)
	if string(data) != string(fakeAudio) {
		t.Errorf("unexpected audio content")
	}
}

func TestOpenAISynthesizer_Synthesize_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	synth := NewOpenAISynthesizer(OpenAIConfig{
		APIKey:  "bad-key",
		BaseURL: server.URL,
	})

	_, err := synth.Synthesize("test")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got %q", err.Error())
	}
}

func TestNewOpenAISynthesizer_Defaults(t *testing.T) {
	synth := NewOpenAISynthesizer(OpenAIConfig{APIKey: "test-key"})

	if synth.config.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base URL, got %q", synth.config.BaseURL)
	}
	if synth.config.Model != "tts-1" {
		t.Errorf("expected default model 'tts-1', got %q", synth.config.Model)
	}
	if synth.config.Voice != "alloy" {
		t.Errorf("expected default voice 'alloy', got %q", synth.config.Voice)
	}
}

var _ Synthesizer = (*OpenAISynthesizer)(nil)
