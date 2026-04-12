package acp

import (
	"os"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("OPENAB_TEST_VAR", "expanded_value")

	tests := []struct {
		input    string
		expected string
	}{
		{"${OPENAB_TEST_VAR}", "expanded_value"},
		{"plain_value", "plain_value"},
		{"${OPENAB_NONEXISTENT_VAR_12345}", ""},
		{"partial${VAR}", "partial${VAR}"},
		{"", ""},
	}

	for _, tt := range tests {
		result := expandEnv(tt.input)
		if result != tt.expected {
			t.Errorf("expandEnv(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestSpawnConnection_InvalidCommand(t *testing.T) {
	_, err := SpawnConnection("/nonexistent/binary", nil, os.TempDir(), nil, "test")
	if err == nil {
		t.Fatal("expected error for invalid command")
	}
}
