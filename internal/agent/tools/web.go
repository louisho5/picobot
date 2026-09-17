package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// WebTool supports fetch operations.
// Args: {"url": "https://..."}

type WebTool struct{}

func NewWebTool() *WebTool { return &WebTool{} }

func (t *WebTool) Name() string        { return "web" }
func (t *WebTool) Description() string { return "Fetch web content from a URL" }

func (t *WebTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch (must be http or https)",
			},
		},
		"required": []string{"url"},
	}
}

var (
	tagWithAttrsRegex = regexp.MustCompile(`(?i)<([a-z1-6]+)([^>]*)>`)
	hrefRegex         = regexp.MustCompile(`(?i)\bhref=["']([^"']*)["']`)
	srcRegex          = regexp.MustCompile(`(?i)\bsrc=["']([^"']*)["']`)
)

func cleanHTML(html string) string {
	// 1. Remove script blocks
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, "")

	// 2. Remove style blocks
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")

	// 3. Remove SVG blocks
	reSVG := regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	html = reSVG.ReplaceAllString(html, "")

	// 4. Remove head blocks
	reHead := regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	html = reHead.ReplaceAllString(html, "")

	// 5. Remove comments
	reComment := regexp.MustCompile(`(?is)<!--.*?-->`)
	html = reComment.ReplaceAllString(html, "")

	// 6. Clean attributes of all tags (keep only href and src)
	html = tagWithAttrsRegex.ReplaceAllStringFunc(html, func(tag string) string {
		submatches := tagWithAttrsRegex.FindStringSubmatch(tag)
		if len(submatches) < 3 {
			return tag
		}
		tagName := submatches[1]
		attrs := submatches[2]

		var kept []string
		if hrefMatch := hrefRegex.FindStringSubmatch(attrs); len(hrefMatch) == 2 {
			kept = append(kept, fmt.Sprintf(`href="%s"`, hrefMatch[1]))
		}
		if srcMatch := srcRegex.FindStringSubmatch(attrs); len(srcMatch) == 2 {
			kept = append(kept, fmt.Sprintf(`src="%s"`, srcMatch[1]))
		}

		if len(kept) > 0 {
			return fmt.Sprintf("<%s %s>", tagName, strings.Join(kept, " "))
		}
		return fmt.Sprintf("<%s>", tagName)
	})

	// Collapse spaces and excessive newlines
	reSpace := regexp.MustCompile(`[ \t]+`)
	html = reSpace.ReplaceAllString(html, " ")

	reNewlines := regexp.MustCompile(`\n{3,}`)
	html = reNewlines.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}

func (t *WebTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	u, ok := args["url"].(string)
	if !ok || u == "" {
		return "", fmt.Errorf("web: 'url' argument required")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return cleanHTML(string(b)), nil
	}
	return string(b), nil
}
