package teams

import (
	"testing"
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

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		text    string
		wantCmd string
	}{
		{"sessions", "sessions"},
		{"info", "info"},
		{"reset", "reset"},
		{"resume", "resume"},
		{"hello world", ""},
		{"", ""},
	}

	for _, tt := range tests {
		cmd := extractCommand(tt.text)
		if cmd != tt.wantCmd {
			t.Errorf("extractCommand(%q) = %q, want %q", tt.text, cmd, tt.wantCmd)
		}
	}
}
