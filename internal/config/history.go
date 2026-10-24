package config

import "time"

// DefaultHistoryRetentionDays is how long conversation history is kept on
// disk when historyRetentionDays is missing or non-positive.
const DefaultHistoryRetentionDays = 30

// DefaultHistoryContextChars caps how much conversation history (in
// characters of message content) is sent to the LLM per turn when
// historyContextChars is missing or non-positive. Roughly 25k tokens.
const DefaultHistoryContextChars = 100000

// HistoryRetention converts a configured retention in days to a duration.
// Zero and negative values fall back to DefaultHistoryRetentionDays.
func HistoryRetention(days int) time.Duration {
	if days <= 0 {
		days = DefaultHistoryRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// HistoryContextChars returns the configured history context budget.
// Zero and negative values fall back to DefaultHistoryContextChars.
func HistoryContextChars(chars int) int {
	if chars <= 0 {
		return DefaultHistoryContextChars
	}
	return chars
}
