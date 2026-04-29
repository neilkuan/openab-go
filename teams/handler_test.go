package teams

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neilkuan/quill/acp"
	"github.com/neilkuan/quill/command"
	"github.com/neilkuan/quill/platform"
)

func TestStripBotMention(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		botID    string
		entities []Entity
		want     string
	}{
		{
			name:  "simple mention",
			text:  "<at>QuillBot</at> hello world",
			botID: "bot-123",
			entities: []Entity{
				{Type: "mention", Mentioned: &Account{ID: "bot-123"}, Text: "<at>QuillBot</at>"},
			},
			want: "hello world",
		},
		{
			name:  "mention in middle",
			text:  "hey <at>QuillBot</at> do something",
			botID: "bot-123",
			entities: []Entity{
				{Type: "mention", Mentioned: &Account{ID: "bot-123"}, Text: "<at>QuillBot</at>"},
			},
			want: "hey  do something",
		},
		{
			name:     "no mention",
			text:     "plain text message",
			botID:    "bot-123",
			entities: nil,
			want:     "plain text message",
		},
		{
			name:  "mention of different user",
			text:  "<at>OtherUser</at> hello",
			botID: "bot-123",
			entities: []Entity{
				{Type: "mention", Mentioned: &Account{ID: "other-456"}, Text: "<at>OtherUser</at>"},
			},
			want: "<at>OtherUser</at> hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBotMention(tt.text, tt.botID, tt.entities)
			if got != tt.want {
				t.Errorf("stripBotMention() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBotMentioned(t *testing.T) {
	botID := "bot-123"

	tests := []struct {
		name     string
		entities []Entity
		want     bool
	}{
		{
			name: "bot mentioned",
			entities: []Entity{
				{Type: "mention", Mentioned: &Account{ID: "bot-123"}},
			},
			want: true,
		},
		{
			name: "other user mentioned",
			entities: []Entity{
				{Type: "mention", Mentioned: &Account{ID: "other-456"}},
			},
			want: false,
		},
		{
			name:     "no entities",
			entities: nil,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBotMentioned(botID, tt.entities)
			if got != tt.want {
				t.Errorf("isBotMentioned() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildSessionKey(t *testing.T) {
	got := buildSessionKey("19:abc@thread.tacv2;messageid=123")
	want := "teams:19:abc@thread.tacv2;messageid=123"
	if got != want {
		t.Errorf("buildSessionKey() = %s, want %s", got, want)
	}
}

func TestExtensionForContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        string
	}{
		{"png", "image/png", ".png"},
		{"jpeg", "image/jpeg", ".jpg"},
		{"jpg alias", "image/jpg", ".jpg"},
		{"mp3", "audio/mpeg", ".mp3"},
		{"ogg voice", "audio/ogg", ".ogg"},
		{"pdf", "application/pdf", ".pdf"},
		{"with charset suffix", "text/plain; charset=utf-8", ".txt"},
		{"upper case", "IMAGE/PNG", ".png"},
		{"csv", "text/csv", ".csv"},
		{"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{"xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
		{"pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
		{"doc legacy", "application/msword", ".doc"},
		{"xls legacy", "application/vnd.ms-excel", ".xls"},
		{"ppt legacy", "application/vnd.ms-powerpoint", ".ppt"},
		{"empty", "", ""},
		{"unknown falls through", "application/x-made-up", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extensionForContentType(tt.contentType)
			if got != tt.want {
				t.Errorf("extensionForContentType(%q) = %q, want %q", tt.contentType, got, tt.want)
			}
		})
	}
}

// TestIsDownloadableAttachment verifies that we only attempt to download
// attachments that actually carry a fetchable file. Teams routinely sends
// non-file attachments alongside formatted text (e.g. a `text/html`
// rendering for messages with @mentions, or Adaptive Cards in invokes);
// these have no `contentUrl` and must not be passed through to the HTTP
// downloader, which otherwise errors out with "unsupported protocol scheme".
func TestIsDownloadableAttachment(t *testing.T) {
	tests := []struct {
		name string
		att  Attachment
		want bool
	}{
		{
			name: "image with https url",
			att:  Attachment{ContentType: "image/png", ContentURL: "https://example.com/a.png"},
			want: true,
		},
		{
			name: "audio with https url",
			att:  Attachment{ContentType: "audio/ogg", ContentURL: "https://example.com/a.ogg"},
			want: true,
		},
		{
			name: "file with https url",
			att:  Attachment{ContentType: "application/pdf", ContentURL: "https://example.com/a.pdf"},
			want: true,
		},
		{
			name: "empty contentUrl is skipped",
			att:  Attachment{ContentType: "image/png", ContentURL: ""},
			want: false,
		},
		{
			name: "text/html mention rendering is skipped",
			att:  Attachment{ContentType: "text/html", Content: "<div>...</div>"},
			want: false,
		},
		{
			name: "adaptive card is skipped",
			att:  Attachment{ContentType: "application/vnd.microsoft.card.adaptive", Content: map[string]any{}},
			want: false,
		},
		{
			name: "thumbnail card is skipped",
			att:  Attachment{ContentType: "application/vnd.microsoft.card.thumbnail", Content: map[string]any{}},
			want: false,
		},
		{
			name: "non-http scheme is skipped",
			att:  Attachment{ContentType: "image/png", ContentURL: "file:///etc/passwd"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDownloadableAttachment(tt.att); got != tt.want {
				t.Errorf("isDownloadableAttachment(%+v) = %v, want %v", tt.att, got, tt.want)
			}
		})
	}
}

// TestSendModeCard_BuildsAdaptiveCardAttachment verifies sendModeCard sends
// a POST with an Adaptive Card attachment.
func TestSendModeCard_BuildsAdaptiveCardAttachment(t *testing.T) {
	cap := &captureUpdate{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&cap.body)
		_, _ = w.Write([]byte(`{"id":"sent-1"}`))
	}))
	defer ts.Close()
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "mock", "expires_in": 3600})
	}))
	defer tokenServer.Close()

	auth := &BotAuth{appID: "a", appSecret: "s", tenantID: "t", tokenURL: tokenServer.URL}
	client := NewBotClient(auth)

	h := &Handler{Client: client}
	listing := command.ModeListing{
		Current:   "kiro_default",
		Available: []acp.ModeInfo{{ID: "kiro_default"}, {ID: "kiro_spec"}},
	}
	a := &Activity{ServiceURL: ts.URL, Conversation: Conversation{ID: "conv-X"}}

	h.sendModeCard(a, "teams:conv-X", listing)

	if cap.method != http.MethodPost {
		t.Fatalf("expected POST, got %s", cap.method)
	}
	if len(cap.body.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(cap.body.Attachments))
	}
	att := cap.body.Attachments[0]
	if att.ContentType != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("ContentType = %q", att.ContentType)
	}
}

// TestSendModelCard_BuildsAdaptiveCardAttachment verifies sendModelCard sends
// a POST with an Adaptive Card attachment (symmetric to sendModeCard).
func TestSendModelCard_BuildsAdaptiveCardAttachment(t *testing.T) {
	cap := &captureUpdate{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&cap.body)
		_, _ = w.Write([]byte(`{"id":"sent-1"}`))
	}))
	defer ts.Close()
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "mock", "expires_in": 3600})
	}))
	defer tokenServer.Close()

	auth := &BotAuth{appID: "a", appSecret: "s", tenantID: "t", tokenURL: tokenServer.URL}
	client := NewBotClient(auth)

	h := &Handler{Client: client}
	listing := command.ModelListing{
		Current:   "claude-opus-4.6",
		Available: []acp.ModelInfo{{ID: "claude-opus-4.6"}, {ID: "claude-sonnet-4.5"}},
	}
	a := &Activity{ServiceURL: ts.URL, Conversation: Conversation{ID: "conv-X"}}

	h.sendModelCard(a, "teams:conv-X", listing)

	if cap.method != http.MethodPost {
		t.Fatalf("expected POST, got %s", cap.method)
	}
	if len(cap.body.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(cap.body.Attachments))
	}
	att := cap.body.Attachments[0]
	if att.ContentType != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("ContentType = %q", att.ContentType)
	}
}

// jpegMagicBytes is a minimal valid JPEG header — enough for
// http.DetectContentType to classify the file as image/jpeg.
var jpegMagicBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}

// pngMagicBytes is the canonical PNG signature.
var pngMagicBytes = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// TestEnsureFileExtension covers the post-download filename fix that
// keeps Discord/Telegram/Teams aligned: when the original attachment
// metadata does not yield an extension (Teams Bot Framework often sends
// `name=""` for inline mobile photos), we fall back to the content-type
// hint, then to magic-byte sniffing. kiro-cli's `read` tool with
// `mode=Image` rejects extensionless paths, so this is the difference
// between "agent sees the image" and "agent says it cannot read the file".
func TestEnsureFileExtension(t *testing.T) {
	dir := t.TempDir()

	t.Run("already has extension is returned unchanged", func(t *testing.T) {
		path := filepath.Join(dir, "photo.jpg")
		if err := os.WriteFile(path, jpegMagicBytes, 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ensureFileExtension(path, "image/jpeg")
		if err != nil {
			t.Fatalf("ensureFileExtension: %v", err)
		}
		if got != path {
			t.Errorf("expected unchanged path, got %q", got)
		}
	})

	t.Run("uses content-type hint when filename has no extension", func(t *testing.T) {
		path := filepath.Join(dir, "no_ext_with_hint")
		if err := os.WriteFile(path, jpegMagicBytes, 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ensureFileExtension(path, "image/jpeg")
		if err != nil {
			t.Fatalf("ensureFileExtension: %v", err)
		}
		if !strings.HasSuffix(got, ".jpg") {
			t.Errorf("expected .jpg suffix, got %q", got)
		}
		if _, statErr := os.Stat(got); statErr != nil {
			t.Errorf("renamed file does not exist: %v", statErr)
		}
		if _, statErr := os.Stat(path); statErr == nil {
			t.Error("original extensionless path still exists after rename")
		}
	})

	t.Run("falls back to magic bytes when content-type is empty", func(t *testing.T) {
		path := filepath.Join(dir, "no_ext_no_hint")
		if err := os.WriteFile(path, pngMagicBytes, 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ensureFileExtension(path, "")
		if err != nil {
			t.Fatalf("ensureFileExtension: %v", err)
		}
		if !strings.HasSuffix(got, ".png") {
			t.Errorf("expected .png suffix from magic bytes, got %q", got)
		}
	})

	t.Run("falls back to magic bytes when content-type is unknown", func(t *testing.T) {
		path := filepath.Join(dir, "no_ext_bad_hint")
		if err := os.WriteFile(path, jpegMagicBytes, 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ensureFileExtension(path, "application/x-made-up")
		if err != nil {
			t.Fatalf("ensureFileExtension: %v", err)
		}
		if !strings.HasSuffix(got, ".jpg") {
			t.Errorf("expected .jpg suffix from magic bytes, got %q", got)
		}
	})

	t.Run("unknown binary falls through to runtime mime DB", func(t *testing.T) {
		// http.DetectContentType returns application/octet-stream for binary
		// payloads it cannot classify, and Go's mime package maps that to
		// .bin — fine for agents like kiro which only need *some* extension
		// to drive their read-tool routing. The contract: we always end up
		// with an extension on a binary file, even if not the "right" one.
		path := filepath.Join(dir, "totally_unknown_blob")
		if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03, 0x00, 0x00, 0xAB, 0xCD}, 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ensureFileExtension(path, "")
		if err != nil {
			t.Fatalf("ensureFileExtension: %v", err)
		}
		if filepath.Ext(got) == "" {
			t.Errorf("expected some extension to be inferred for binary blob, got %q", got)
		}
	})
}

// TestBuildPromptContent_AttachedImagesBlock confirms the image-path text
// format used by Discord/Telegram is reachable from Teams — i.e. once
// imagePaths are passed in, the prompt embeds them as
// `<attached_images>` so the agent can use its read tool.
func TestBuildPromptContent_AttachedImagesBlock(t *testing.T) {
	got := buildPromptContent(
		"hello",
		[]string{"/tmp/x/123_photo.jpg", "/tmp/x/124_b.png"},
		nil,
		nil,
	)
	if !strings.Contains(got, "<attached_images>") {
		t.Errorf("expected <attached_images> block, got: %s", got)
	}
	if !strings.Contains(got, "/tmp/x/123_photo.jpg") || !strings.Contains(got, "/tmp/x/124_b.png") {
		t.Errorf("expected both image paths in output, got: %s", got)
	}
	if !strings.Contains(got, "Please read and analyze") {
		t.Errorf("expected agent instruction line, got: %s", got)
	}
}

// TestBuildPromptContent_NoAttachments returns the prompt unmodified when
// no images / files / transcriptions are present — guards against a
// regression where an empty `<attached_images>` block leaks into plain
// text replies.
func TestBuildPromptContent_NoAttachments(t *testing.T) {
	got := buildPromptContent("just text", nil, nil, nil)
	if got != "just text" {
		t.Errorf("expected plain prompt unchanged, got: %q", got)
	}
}

// TestBuildPromptContent_FileBlockStillRendered — files use the shared
// platform.FormatFileBlock; images going through the text-path must not
// accidentally suppress that block.
func TestBuildPromptContent_FileBlockStillRendered(t *testing.T) {
	got := buildPromptContent(
		"see file",
		[]string{"/tmp/x/img.png"},
		nil,
		[]platform.FileAttachment{{Filename: "spec.pdf", LocalPath: "/tmp/x/spec.pdf"}},
	)
	if !strings.Contains(got, "/tmp/x/img.png") {
		t.Errorf("expected image path, got: %s", got)
	}
	if !strings.Contains(got, "spec.pdf") {
		t.Errorf("expected file block to mention spec.pdf, got: %s", got)
	}
}
