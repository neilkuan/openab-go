package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Telegram Bot API 10.1 (2026-06-11) added Rich Messages: a structured
// formatting system that renders headings, tables, etc. natively. go-telegram/bot
// has no binding for sendRichMessage yet (latest v1.21.0 tracks Bot API 10.0),
// so we issue the raw HTTP call ourselves using the same token the adapter
// created the client with.
//
// Design mirrors the openab gateway (openabdev/openab#1106): agent markdown is
// passed through unchanged via InputRichMessage.markdown — "Rich Markdown" is
// GitHub-Flavored-Markdown compatible, so no markdown→object-tree conversion is
// needed; Telegram renders tables/headings server-side.
//
// NOTE: this path is experimental and opt-in (telegram.rich_messages). The
// exact API behaviour has not been validated against a live bot in this repo;
// any send failure falls back to the existing parse_mode=HTML path.

// telegramAPIBase is the Bot API root. A package var so tests can point it at a
// stub server.
var telegramAPIBase = "https://api.telegram.org"

// richHTTPClient is dedicated to the raw sendRichMessage call so its timeout is
// independent of go-telegram/bot's internal client.
var richHTTPClient = &http.Client{Timeout: 30 * time.Second}

// richMessageEnvelope is the minimal slice of Telegram's response we need.
type richMessageEnvelope struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

// isComplexMarkdown reports whether agent output benefits from sendRichMessage
// rather than the plain sendMessage / parse_mode=HTML path. Mirrors the openab
// heuristic: long content, ATX headings, or a GFM table separator row.
//
// Code blocks are intentionally NOT treated as complex — the HTML path keeps
// language-tagged syntax highlighting that rich messages render less well.
// Conservative by design: false positives are harmless (rich renders plain text
// fine), but we avoid switching API for ordinary prose to keep the risk surface
// small.
func isComplexMarkdown(text string) bool {
	// sendMessage hard limit is 4096 chars; rich messages support 32768.
	if len([]rune(text)) > 4096 {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimLeft(line, " \t")
		// ATX headings (h1–h6): sendMessage has zero heading support.
		switch {
		case strings.HasPrefix(t, "# "),
			strings.HasPrefix(t, "## "),
			strings.HasPrefix(t, "### "),
			strings.HasPrefix(t, "#### "),
			strings.HasPrefix(t, "##### "),
			strings.HasPrefix(t, "###### "):
			return true
		}
		// GFM table separator row, e.g. |---|:--:|.
		if len(t) >= 2 && strings.HasPrefix(t, "|") && strings.HasSuffix(t, "|") {
			if isTableSeparatorRow(t[1 : len(t)-1]) {
				return true
			}
		}
	}
	return false
}

// isTableSeparatorRow reports whether inner (the content between the outer pipes
// of a table row) is a GFM separator row: every cell non-empty and composed only
// of '-' once surrounding spaces and alignment colons are stripped.
func isTableSeparatorRow(inner string) bool {
	cells := strings.Split(inner, "|")
	for _, cell := range cells {
		c := strings.Trim(strings.TrimSpace(cell), ":")
		if c == "" {
			return false
		}
		for _, ch := range c {
			if ch != '-' {
				return false
			}
		}
	}
	return true
}

// richMessageBody builds the sendRichMessage JSON payload. message_thread_id is
// only included for forum topics (threadID != 0).
func richMessageBody(chatID int64, threadID int, markdownText string) map[string]any {
	body := map[string]any{
		"chat_id":      chatID,
		"rich_message": map[string]string{"markdown": markdownText},
	}
	if threadID != 0 {
		body["message_thread_id"] = threadID
	}
	return body
}

// sendRichMessage sends agent markdown via Bot API 10.1 sendRichMessage, passing
// the markdown through untouched. Returns an error on transport failure or a
// non-ok API response so the caller can fall back to the HTML path.
func (h *Handler) sendRichMessage(ctx context.Context, chatID int64, threadID int, markdownText string) error {
	if h.BotToken == "" {
		return fmt.Errorf("rich message: empty bot token")
	}
	buf, err := json.Marshal(richMessageBody(chatID, threadID, markdownText))
	if err != nil {
		return fmt.Errorf("rich message: marshal: %w", err)
	}
	url := fmt.Sprintf("%s/bot%s/sendRichMessage", telegramAPIBase, h.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("rich message: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := richHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("rich message: send: %w", err)
	}
	defer resp.Body.Close()

	var env richMessageEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("rich message: decode response: %w", err)
	}
	if !env.OK {
		return fmt.Errorf("rich message: api error: %s", env.Description)
	}
	return nil
}
