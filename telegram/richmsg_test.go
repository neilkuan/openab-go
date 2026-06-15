package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsComplexMarkdown(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"plain prose", "just a normal sentence", false},
		{"bold only", "some **bold** text", false},
		{"inline code", "run `go test`", false},
		{"fenced code not complex", "```go\nfmt.Println()\n```", false},
		{"h1 heading", "# Title\nbody", true},
		{"h3 heading", "### Section", true},
		{"indented heading", "   ## Indented", true},
		{"not a heading (no space)", "#hashtag not heading", false},
		{"table separator", "| a | b |\n|---|---|\n| 1 | 2 |", true},
		{"aligned table separator", "| a | b |\n|:--|--:|", true},
		{"pipes but not separator", "a | b | c", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isComplexMarkdown(c.in); got != c.want {
				t.Fatalf("isComplexMarkdown(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestIsComplexMarkdownLongContent(t *testing.T) {
	// 4097 runes (incl. multibyte) must trip the length rule; 4096 must not.
	if isComplexMarkdown(strings.Repeat("a", 4096)) {
		t.Fatal("4096 chars should not be complex")
	}
	if !isComplexMarkdown(strings.Repeat("漢", 4097)) {
		t.Fatal("4097 runes should be complex (rune count, not bytes)")
	}
}

func TestRichMessageBody(t *testing.T) {
	// No thread → no message_thread_id key.
	b := richMessageBody(12345, 0, "# Hi")
	if _, ok := b["message_thread_id"]; ok {
		t.Fatal("message_thread_id must be omitted when threadID == 0")
	}
	rm, ok := b["rich_message"].(map[string]string)
	if !ok || rm["markdown"] != "# Hi" {
		t.Fatalf("rich_message.markdown not passed through: %#v", b["rich_message"])
	}
	if b["chat_id"] != int64(12345) {
		t.Fatalf("chat_id mismatch: %#v", b["chat_id"])
	}

	// Forum topic → message_thread_id present.
	b2 := richMessageBody(1, 99, "x")
	if b2["message_thread_id"] != 99 {
		t.Fatalf("message_thread_id should be 99, got %#v", b2["message_thread_id"])
	}
}

func TestSendRichMessageSuccess(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer srv.Close()

	orig := telegramAPIBase
	telegramAPIBase = srv.URL
	defer func() { telegramAPIBase = orig }()

	h := &Handler{BotToken: "TESTTOKEN"}
	if err := h.sendRichMessage(context.Background(), 555, 0, "# Heading\n\n| a |\n|---|"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/botTESTTOKEN/sendRichMessage" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	rm, ok := gotBody["rich_message"].(map[string]any)
	if !ok || rm["markdown"] != "# Heading\n\n| a |\n|---|" {
		t.Fatalf("server did not receive markdown payload: %#v", gotBody)
	}
}

func TestSendRichMessageAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":false,"description":"RICH_MESSAGE_NOT_SUPPORTED"}`)
	}))
	defer srv.Close()

	orig := telegramAPIBase
	telegramAPIBase = srv.URL
	defer func() { telegramAPIBase = orig }()

	h := &Handler{BotToken: "TESTTOKEN"}
	err := h.sendRichMessage(context.Background(), 1, 0, "# x")
	if err == nil {
		t.Fatal("expected error on ok:false response")
	}
	if !strings.Contains(err.Error(), "RICH_MESSAGE_NOT_SUPPORTED") {
		t.Fatalf("error should surface API description, got: %v", err)
	}
}

func TestSendRichMessageEmptyToken(t *testing.T) {
	h := &Handler{BotToken: ""}
	if err := h.sendRichMessage(context.Background(), 1, 0, "x"); err == nil {
		t.Fatal("expected error with empty bot token")
	}
}
