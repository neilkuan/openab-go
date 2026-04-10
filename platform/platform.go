package platform

import "strings"

// Platform is the interface every chat adapter (Discord, Telegram, Teams …) must implement.
type Platform interface {
	Start() error
	Stop() error
}

// SplitMessage splits text into chunks at line boundaries, each <= limit chars.
// Every chat platform has a message-size ceiling, so this lives in the shared package.
func SplitMessage(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder

	for _, line := range strings.Split(text, "\n") {
		// +1 for the newline
		if current.Len() > 0 && current.Len()+len(line)+1 > limit {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		// If a single line exceeds limit, hard-split it
		if len(line) > limit {
			for i := 0; i < len(line); i += limit {
				if current.Len() > 0 {
					chunks = append(chunks, current.String())
					current.Reset()
				}
				end := i + limit
				if end > len(line) {
					end = len(line)
				}
				current.WriteString(line[i:end])
			}
		} else {
			current.WriteString(line)
		}
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
