package telegram

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestBuildSessionKey(t *testing.T) {
	tests := []struct {
		name    string
		msg     *tgbotapi.Message
		want    string
	}{
		{
			name: "private chat",
			msg: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 12345, Type: "private"},
			},
			want: "tg:12345",
		},
		{
			name: "group chat",
			msg: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: -100123456789, Type: "supergroup"},
			},
			want: "tg:-100123456789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSessionKey(tt.msg)
			if got != tt.want {
				t.Errorf("buildSessionKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBotMentioned(t *testing.T) {
	tests := []struct {
		name        string
		msg         *tgbotapi.Message
		botUsername string
		want        bool
	}{
		{
			name: "mentioned in text",
			msg: &tgbotapi.Message{
				Text: "@testbot hello",
				Entities: []tgbotapi.MessageEntity{
					{Type: "mention", Offset: 0, Length: 8},
				},
			},
			botUsername: "testbot",
			want:        true,
		},
		{
			name: "mentioned case insensitive",
			msg: &tgbotapi.Message{
				Text: "@TestBot hello",
				Entities: []tgbotapi.MessageEntity{
					{Type: "mention", Offset: 0, Length: 8},
				},
			},
			botUsername: "testbot",
			want:        true,
		},
		{
			name: "not mentioned",
			msg: &tgbotapi.Message{
				Text:     "hello world",
				Entities: []tgbotapi.MessageEntity{},
			},
			botUsername: "testbot",
			want:        false,
		},
		{
			name: "different bot mentioned",
			msg: &tgbotapi.Message{
				Text: "@otherbot hello",
				Entities: []tgbotapi.MessageEntity{
					{Type: "mention", Offset: 0, Length: 9},
				},
			},
			botUsername: "testbot",
			want:        false,
		},
		{
			name: "mentioned in caption",
			msg: &tgbotapi.Message{
				Text: "",
				Caption: "@testbot check this",
				CaptionEntities: []tgbotapi.MessageEntity{
					{Type: "mention", Offset: 0, Length: 8},
				},
			},
			botUsername: "testbot",
			want:        true,
		},
		{
			name: "no entities",
			msg: &tgbotapi.Message{
				Text: "@testbot hello",
			},
			botUsername: "testbot",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBotMentioned(tt.msg, tt.botUsername)
			if got != tt.want {
				t.Errorf("isBotMentioned() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripBotMention(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		botUsername string
		want        string
	}{
		{
			name:        "mention at start",
			text:        "@testbot hello world",
			botUsername:  "testbot",
			want:        "hello world",
		},
		{
			name:        "mention at end",
			text:        "hello @testbot",
			botUsername:  "testbot",
			want:        "hello",
		},
		{
			name:        "mention in middle",
			text:        "hey @testbot how are you",
			botUsername:  "testbot",
			want:        "hey  how are you",
		},
		{
			name:        "case insensitive",
			text:        "@TestBot hello",
			botUsername:  "testbot",
			want:        "hello",
		},
		{
			name:        "no mention",
			text:        "hello world",
			botUsername:  "testbot",
			want:        "hello world",
		},
		{
			name:        "empty text",
			text:        "",
			botUsername:  "testbot",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBotMention(tt.text, tt.botUsername)
			if got != tt.want {
				t.Errorf("stripBotMention() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComposeDisplay_Telegram(t *testing.T) {
	tests := []struct {
		name      string
		toolLines []string
		text      string
		want      string
	}{
		{
			name: "text only",
			text: "Hello world",
			want: "Hello world",
		},
		{
			name: "text with trailing whitespace",
			text: "Hello world  \n\n",
			want: "Hello world",
		},
		{
			name:      "tools and text",
			toolLines: []string{"🔧 `Read`...", "✅ `Write`"},
			text:      "Done!",
			want:      "🔧 `Read`...\n✅ `Write`\n\nDone!",
		},
		{
			name:      "tools only",
			toolLines: []string{"🔧 `Read`..."},
			text:      "",
			want:      "🔧 `Read`...\n\n",
		},
		{
			name: "empty",
			text: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composeDisplay(tt.toolLines, tt.text)
			if got != tt.want {
				t.Errorf("composeDisplay() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPromptContent_Telegram(t *testing.T) {
	tests := []struct {
		name           string
		base           string
		imagePaths     []string
		transcriptions []string
		wantContains   []string
	}{
		{
			name:         "text only",
			base:         "hello",
			wantContains: []string{"hello"},
		},
		{
			name:       "with images",
			base:       "check this",
			imagePaths: []string{"/tmp/photo.jpg"},
			wantContains: []string{
				"check this",
				"<attached_images>",
				"/tmp/photo.jpg",
				"</attached_images>",
			},
		},
		{
			name:           "with transcriptions",
			base:           "",
			transcriptions: []string{"hello from voice"},
			wantContains: []string{
				"<voice_transcription>",
				"hello from voice",
				"</voice_transcription>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPromptContent(tt.base, tt.imagePaths, tt.transcriptions)
			for _, want := range tt.wantContains {
				if !contains(got, want) {
					t.Errorf("buildPromptContent() missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
