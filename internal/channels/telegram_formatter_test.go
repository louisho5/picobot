package channels

import "testing"

func TestMarkdownToHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "HTML escaping",
			input:    "hello & <world>",
			expected: "hello &amp; &lt;world&gt;",
		},
		{
			name:     "Bold and Italic",
			input:    "this is **bold** and *italic*",
			expected: "this is <b>bold</b> and <i>italic</i>",
		},
		{
			name:     "Underscore Italic and Underline",
			input:    "this is _italic_ and __underline__",
			expected: "this is <i>italic</i> and <u>underline</u>",
		},
		{
			name:     "Strikethrough and Spoiler",
			input:    "this is ~~strike~~ and ||spoiler||",
			expected: "this is <s>strike</s> and <tg-spoiler>spoiler</tg-spoiler>",
		},
		{
			name:     "Code block",
			input:    "code:\n```python\nprint('hello')\n```",
			expected: "code:\n<pre>print(&#39;hello&#39;)</pre>",
		},
		{
			name:     "Inline code",
			input:    "use `go test` to run tests",
			expected: "use <code>go test</code> to run tests",
		},
		{
			name:     "Underscores in variable/filenames (no matching)",
			input:    "run python simple_primes.py and prime_numbers.py to test first_25 numbers",
			expected: "run python simple_primes.py and prime_numbers.py to test first_25 numbers",
		},
		{
			name:     "Links",
			input:    "check out [google](https://google.com)",
			expected: "check out <a href=\"https://google.com\">google</a>",
		},
		{
			name:     "Links with formatting in text",
			input:    "check out [google **search**](https://google.com)",
			expected: "check out <a href=\"https://google.com\">google <b>search</b></a>",
		},
		{
			name:     "Nested formatting",
			input:    "**bold *italic*** and *italic **bold*** and __under *italic*__",
			expected: "<b>bold <i>italic</i></b> and <i>italic <b>bold</b></i> and <u>under <i>italic</i></u>",
		},
		{
			name:     "Headings to bold",
			input:    "## Heading 2\n### Heading 3 with *italic*",
			expected: "<b>Heading 2</b>\n<b>Heading 3 with <i>italic</i></b>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := markdownToHTML(tt.input)
			if actual != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, actual)
			}
		})
	}
}
