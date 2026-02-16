package agent

import (
	"regexp"
	"testing"
)

func TestSanitizeContent_StripThinkEnabled(t *testing.T) {
	a := &AgentLoop{
		stripThinkTags: true,
		thinkTagRegex:  defaultThinkRE,
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no think tags",
			input:    "Hello, how can I help you?",
			expected: "Hello, how can I help you?",
		},
		{
			name:     "single think tag",
			input:    "<think>I need to think about this</think>Hello!",
			expected: "Hello!",
		},
		{
			name:     "think tag with newlines",
			input:    "<think>\nLet me reason through this...\nStep 1: Analyze\nStep 2: Respond\n</think>Here's my answer.",
			expected: "Here's my answer.",
		},
		{
			name:     "multiple think tags",
			input:    "<think>First thought</think>Hello<think>Second thought</think> World",
			expected: "Hello World",
		},
		{
			name:     "empty think tag",
			input:    "<think></think>Response",
			expected: "Response",
		},
		{
			name:     "only think tag",
			input:    "<think>Thinking...</think>",
			expected: "",
		},
		{
			name:     "think tag with attributes",
			input:    "<think type=\"reasoning\">Thought</think>Response",
			expected: "Response",
		},
		{
			name:     "content before and after",
			input:    "Prefix <think>thinking</think> Suffix",
			expected: "Prefix  Suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.sanitizeContent(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeContent(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeContent_StripThinkDisabled(t *testing.T) {
	a := &AgentLoop{
		stripThinkTags: false,
		thinkTagRegex:  defaultThinkRE,
	}

	input := "<think>Thinking...</think>Hello!"
	expected := "<think>Thinking...</think>Hello!"

	got := a.sanitizeContent(input)
	if got != expected {
		t.Errorf("sanitizeContent disabled: got %q, want %q", got, expected)
	}
}

func TestSanitizeContent_CustomRegex(t *testing.T) {
	// Test with a custom regex that matches <reasoning> tags instead
	customRE := regexp.MustCompile(`(?s)<reasoning>.*?</reasoning>`)
	a := &AgentLoop{
		stripThinkTags: true,
		thinkTagRegex:  customRE,
	}

	input := "<reasoning>Let me think...</reasoning>Hello!"
	expected := "Hello!"

	got := a.sanitizeContent(input)
	if got != expected {
		t.Errorf("sanitizeContent custom regex: got %q, want %q", got, expected)
	}
}
