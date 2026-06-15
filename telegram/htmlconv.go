package telegram

import (
	"fmt"
	"regexp"
	"strings"
)

// convertToTelegramHTML rewrites GFM-flavoured agent output into the limited
// HTML subset that Telegram's parse_mode=HTML accepts.
//
// Spec: https://core.telegram.org/bots/api#html-style
//
// Allowed tags: <b>, <i>, <u>, <s>, <a>, <code>, <pre>, <blockquote>,
// <span class="tg-spoiler">. Code blocks with a language hint MUST be nested
// as <pre><code class="language-XXX">…</code></pre>; standalone <code> can't
// carry a language attribute. Outside tags, &, < and > must be escaped.
//
// Scope: handles bold, inline code, fenced code blocks (with language hint for
// diff/python/etc syntax highlighting), markdown links, and blockquotes (long
// ones collapse via <blockquote expandable>). Italic and strikethrough are
// intentionally skipped — single * and _ collide with list bullets and
// snake_case identifiers in agent output and would produce false positives that
// break message rendering. Tables are already handled upstream by
// markdown.ConvertTables (which converts to fenced code or bullets), so we
// don't need table support here.
//
// Algorithm:
//  1. Extract fenced code blocks → placeholders (so their bodies aren't
//     touched by escape/format passes)
//  2. Extract blockquotes → placeholders (inner text recurses through this
//     function so inline formatting inside a quote still renders)
//  3. Extract inline code spans  → placeholders
//  4. HTML-escape remaining text
//  5. Apply **bold** and [link](url) transforms
//  6. Restore placeholders (quotes first, so a fenced-code placeholder nested
//     inside a quote body is still live for the block-restore pass)
func convertToTelegramHTML(text string) string {
	var blocks, spans, quotes []string

	text = fencedCodeRe.ReplaceAllStringFunc(text, func(match string) string {
		m := fencedCodeRe.FindStringSubmatch(match)
		blocks = append(blocks, renderPreBlock(strings.TrimSpace(m[1]), m[2]))
		return fmt.Sprintf("\x00B%d\x00", len(blocks)-1)
	})

	// Blockquotes are extracted before inline code / escaping because their
	// rendered <blockquote> form must survive the escape pass. renderBlockquote
	// recurses into convertToTelegramHTML for the inner text, so bold/code/link
	// inside a quote still work; fenced code is already a placeholder here so a
	// stray ">" in code can't be misread as a quote line.
	text = blockquoteRe.ReplaceAllStringFunc(text, func(match string) string {
		quotes = append(quotes, renderBlockquote(match))
		return fmt.Sprintf("\x00Q%d\x00", len(quotes)-1)
	})

	text = inlineCodeRe.ReplaceAllStringFunc(text, func(match string) string {
		m := inlineCodeRe.FindStringSubmatch(match)
		spans = append(spans, "<code>"+escapeHTML(m[1])+"</code>")
		return fmt.Sprintf("\x00S%d\x00", len(spans)-1)
	})

	text = escapeHTML(text)

	text = boldRe.ReplaceAllString(text, "<b>$1</b>")

	text = linkRe.ReplaceAllStringFunc(text, func(match string) string {
		m := linkRe.FindStringSubmatch(match)
		// Label is already HTML-escaped (escape pass ran before this), URL
		// also passed through escape so &/</> in href are entities. Quote
		// is the only remaining attribute-breaker.
		href := strings.ReplaceAll(m[2], "\"", "&quot;")
		return fmt.Sprintf("<a href=\"%s\">%s</a>", href, m[1])
	})

	// Quotes restore first: a blockquote body may contain a \x00B…\x00 fenced
	// code placeholder (from a "> ```" code block), which must still be live in
	// the text when the block-restore pass runs below.
	text = placeholderQuoteRe.ReplaceAllStringFunc(text, func(match string) string {
		var idx int
		fmt.Sscanf(match, "\x00Q%d\x00", &idx)
		if idx < 0 || idx >= len(quotes) {
			return match
		}
		return quotes[idx]
	})

	text = placeholderBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		var idx int
		fmt.Sscanf(match, "\x00B%d\x00", &idx)
		if idx < 0 || idx >= len(blocks) {
			return match
		}
		return blocks[idx]
	})
	text = placeholderSpanRe.ReplaceAllStringFunc(text, func(match string) string {
		var idx int
		fmt.Sscanf(match, "\x00S%d\x00", &idx)
		if idx < 0 || idx >= len(spans) {
			return match
		}
		return spans[idx]
	})

	return text
}

// renderPreBlock produces the nested <pre><code class="language-X">…</code></pre>
// form when a language hint is present, else a plain <pre>…</pre>. Body is
// HTML-escaped; trailing newline (often emitted by the markdown source after
// the closing ```) is dropped to avoid an empty line inside the rendered block.
func renderPreBlock(lang, body string) string {
	body = strings.TrimSuffix(body, "\n")
	escaped := escapeHTML(body)
	if lang == "" {
		return "<pre>" + escaped + "</pre>"
	}
	lang = sanitizeLang(lang)
	if lang == "" {
		return "<pre>" + escaped + "</pre>"
	}
	return fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", lang, escaped)
}

// sanitizeLang restricts the language hint to a safe character set in case
// the agent emits something unexpected. libprisma is permissive but Telegram
// clients vary on rejecting odd values.
func sanitizeLang(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '+', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeHTML escapes the three required characters per Telegram's spec.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// blockquoteExpandableMinLines is the line count above which a blockquote is
// rendered as <blockquote expandable> so long quotes collapse by default in the
// client (Bot API 7.4+; the HTML parse_mode handles the attribute server-side,
// so no library method is needed).
const blockquoteExpandableMinLines = 10

// renderBlockquote turns a run of markdown "> " lines into a Telegram
// <blockquote>. Leading ">" markers (including nested ">>") are stripped —
// Telegram forbids nested blockquotes, so deeper levels are flattened to one.
// The inner text is run back through convertToTelegramHTML so inline bold /
// code / link still render inside the quote; recursion terminates because the
// stripped body no longer starts lines with ">". Quotes longer than
// blockquoteExpandableMinLines use the expandable variant.
func renderBlockquote(raw string) string {
	lines := strings.Split(raw, "\n")
	inner := make([]string, 0, len(lines))
	for _, ln := range lines {
		inner = append(inner, stripQuotePrefix(ln))
	}
	body := convertToTelegramHTML(strings.Join(inner, "\n"))
	body = strings.Trim(body, "\n")
	if len(inner) > blockquoteExpandableMinLines {
		return "<blockquote expandable>" + body + "</blockquote>"
	}
	return "<blockquote>" + body + "</blockquote>"
}

// stripQuotePrefix removes leading whitespace and one or more ">" markers (each
// optionally followed by a single space) from a blockquote line, flattening
// nested quotes to a single level.
func stripQuotePrefix(s string) string {
	s = strings.TrimLeft(s, " \t")
	for strings.HasPrefix(s, ">") {
		s = s[1:]
		s = strings.TrimPrefix(s, " ")
	}
	return s
}

// (?s) dot-all so the body can span newlines; capture lang and body.
var fencedCodeRe = regexp.MustCompile("(?s)```([A-Za-z0-9_+\\-]*)\\s*\n(.*?)\n?```")

// `code` — single backticks, no newlines inside.
var inlineCodeRe = regexp.MustCompile("`([^`\n]+?)`")

// **bold** — non-greedy.
var boldRe = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)

// [text](url) — markdown link. URL stops at whitespace or close-paren.
var linkRe = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\n\s]+)\)`)

// blockquoteRe matches a run of consecutive lines that start with ">" — a
// markdown blockquote. (?m) anchors ^ at each line start; fenced code is
// already a placeholder by the time this runs, so ">" inside code is safe.
var blockquoteRe = regexp.MustCompile(`(?m)^>[^\n]*(?:\n>[^\n]*)*`)

var placeholderBlockRe = regexp.MustCompile("\x00B\\d+\x00")
var placeholderSpanRe = regexp.MustCompile("\x00S\\d+\x00")
var placeholderQuoteRe = regexp.MustCompile("\x00Q\\d+\x00")
