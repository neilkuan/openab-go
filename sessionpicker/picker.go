// Package sessionpicker lists historical sessions from supported ACP agent
// backends so a user can pick one and resume it.
//
// Pickers read the agent's on-disk session store directly — they do not
// require the agent process to be running. Each backend has its own
// location and format (see individual implementations).
package sessionpicker

import (
	"path/filepath"
	"strings"
	"time"
)

// stripSenderContext removes the `<sender_context>...</sender_context>`
// envelope that quill injects ahead of every user prompt (see the
// `quill.sender.v1` schema documented in CLAUDE.md). Without this,
// agents that naïvely truncate the first prompt to build a session
// title end up saving "<sender_context>" as the title of every quill
// session, which is useless to show in a picker.
//
// The function is conservative: if the envelope is not at the very
// start of the string, the input is returned unchanged. This keeps
// genuine user messages that happen to mention `<sender_context>` in
// the middle of their text untouched.
func stripSenderContext(s string) string {
	const open = "<sender_context>"
	const close = "</sender_context>"
	t := strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(t, open) {
		return s
	}
	end := strings.Index(t, close)
	if end < 0 {
		return s
	}
	rest := t[end+len(close):]
	return strings.TrimLeft(rest, " \t\r\n")
}

// Session is the minimal metadata needed to render a picker row and
// later resume via AcpConnection.SessionLoad.
type Session struct {
	ID           string
	Title        string
	CWD          string
	UpdatedAt    time.Time
	MessageCount int
}

// Picker lists sessions for one agent backend.
type Picker interface {
	// AgentType returns a stable identifier for the backend, matching
	// the agent binary name (e.g. "kiro-cli", "claude-agent-acp").
	AgentType() string

	// List returns sessions newest first. If cwd is non-empty, only
	// sessions matching that working directory are returned. If limit
	// is > 0, at most that many results are returned.
	List(cwd string, limit int) ([]Session, error)
}

// Detect picks a Picker based on the agent binary path or name.
// Returns false when the binary is not recognised or no picker is
// implemented yet for that backend.
func Detect(agentCommand string) (Picker, bool) {
	switch filepath.Base(agentCommand) {
	case "kiro-cli":
		return NewKiroPicker(""), true
	case "claude-agent-acp":
		return NewClaudePicker(""), true
	case "copilot":
		return NewCopilotPicker(""), true
	case "codex-acp", "codex":
		return NewCodexPicker(""), true
	}
	return nil, false
}
