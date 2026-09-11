package tui

import "strings"

func compactionSkipMessage(reason string) string {
	switch reason {
	case "too_short":
		return "No summary needed — the conversation is still short"
	case "cancelled", "canceled":
		return "Summary cancelled"
	case "", "unknown":
		return "Conversation kept as is"
	default:
		return "Conversation kept as is: " + strings.ReplaceAll(reason, "_", " ")
	}
}
