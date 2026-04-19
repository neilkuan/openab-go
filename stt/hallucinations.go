package stt

import (
	"log/slog"
	"regexp"
	"strings"
)

// rawHallucinations are phrases the Whisper model frequently produces on
// silent, noisy, or truncated audio. They originate from YouTube-style
// captions in Whisper's training data rather than real speech.
// Reference: https://github.com/openai/whisper/discussions/928
var rawHallucinations = []string{
	// Chinese (Traditional / Simplified)
	"字幕由Amara.org社區提供",
	"字幕由 Amara.org 社區提供",
	"字幕由Amara.org社区提供",
	"字幕由 Amara.org 社区提供",
	"請不吝點讚 訂閱 轉發 打賞支持明鏡與點點欄目",
	"请不吝点赞 订阅 转发 打赏支持明镜与点点栏目",
	"請訂閱我的頻道",
	"请订阅我的频道",
	"感謝您的收看",
	"感谢您的收看",
	"多謝觀看",
	"多谢观看",
	"字幕志願者",
	"字幕志愿者",

	// English
	"Thanks for watching",
	"Thank you for watching",
	"Please subscribe to my channel",

	// Japanese
	"ご視聴ありがとうございました",
	"ご視聴ありがとうございます",

	// Korean
	"시청해주셔서 감사합니다",
	"MBC 뉴스",
}

// hallucinationMatchers are compiled once at package init from rawHallucinations.
// The slice is not exported and never mutated after initialization.
var hallucinationMatchers = compileHallucinations(rawHallucinations)

// compileHallucinations builds a matcher per phrase. ASCII-letter-starting
// phrases (English) are compiled with (?i) for case-insensitivity, \b word
// boundaries, and anchored to end-of-utterance — these phrases can plausibly
// occur mid-sentence in real speech, so we only strip them at the tail where
// Whisper actually emits hallucinations. CJK phrases are matched literally
// anywhere because they are specific enough that any occurrence is a
// hallucination; word boundaries don't apply to CJK text.
func compileHallucinations(phrases []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(phrases))
	for _, p := range phrases {
		base := regexp.QuoteMeta(p)
		var pattern string
		if startsWithASCIILetter(p) {
			pattern = `(?i)\b` + base + `\b[!.?…]*\s*$`
		} else {
			pattern = base + `[！。？!.?…]*`
		}
		out = append(out, regexp.MustCompile(pattern))
	}
	return out
}

func startsWithASCIILetter(s string) bool {
	if s == "" {
		return false
	}
	b := s[0]
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// filterHallucinations strips known Whisper hallucination phrases from the
// transcribed text and trims surrounding whitespace. Returns the cleaned text;
// if the original is entirely composed of hallucinations the result is empty.
// Logs a debug entry when filtering actually changed the text.
func filterHallucinations(text string) string {
	cleaned := text
	for _, re := range hallucinationMatchers {
		cleaned = re.ReplaceAllString(cleaned, "")
	}
	cleaned = strings.TrimSpace(cleaned)
	if cleaned != strings.TrimSpace(text) {
		slog.Debug("🎙️ stt: filtered hallucination", "before", text, "after", cleaned)
	}
	return cleaned
}
