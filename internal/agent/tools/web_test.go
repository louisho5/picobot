package tools

import (
	"strings"
	"testing"
)

func TestCleanHTML(t *testing.T) {
	input := `
<html>
<head>
	<title>Test Page</title>
	<style>body { color: red; }</style>
	<script>console.log("hello");</script>
</head>
<body>
	<!-- this is a comment -->
	<div class="container active" data-id="123" aria-label="main content">
		<h1 id="title">Hello World</h1>
		<p class="description">Welcome to <a href="https://example.com" class="link">my site</a>.</p>
		<img src="https://example.com/avatar.png" alt="Avatar" class="avatar-large" />
		<svg width="100" height="100"><path d="M10 10 H 90 V 90 H 10 Z" /></svg>
	</div>
</body>
</html>
`
	got := cleanHTML(input)

	// Verify scripts, styles, SVG, head, and comments are removed
	if strings.Contains(got, "<script>") || strings.Contains(got, "console.log") {
		t.Errorf("script tag or content not removed: %q", got)
	}
	if strings.Contains(got, "<style>") || strings.Contains(got, "body { color: red; }") {
		t.Errorf("style tag or content not removed: %q", got)
	}
	if strings.Contains(got, "<svg>") || strings.Contains(got, "<path") {
		t.Errorf("svg tag or content not removed: %q", got)
	}
	if strings.Contains(got, "<head>") || strings.Contains(got, "<title>") {
		t.Errorf("head tag or content not removed: %q", got)
	}
	if strings.Contains(got, "<!--") || strings.Contains(got, "this is a comment") {
		t.Errorf("comment not removed: %q", got)
	}

	// Verify attributes are cleaned but href and src are kept
	if strings.Contains(got, "class=") || strings.Contains(got, "data-id=") || strings.Contains(got, "aria-label=") || strings.Contains(got, "id=") {
		t.Errorf("non-essential attributes not removed: %q", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("expected href attribute to be preserved: %q", got)
	}
	if !strings.Contains(got, `src="https://example.com/avatar.png"`) {
		t.Errorf("expected src attribute to be preserved: %q", got)
	}

	// Verify tag names are preserved
	if !strings.Contains(got, "<div>") || !strings.Contains(got, "<h1>") || !strings.Contains(got, "<p>") {
		t.Errorf("expected tag names to be preserved: %q", got)
	}
}
