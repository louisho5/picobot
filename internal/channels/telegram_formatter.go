package channels

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// markdownToHTML converts markdown text to HTML format compatible with Telegram's HTML parse_mode.
// It escapes standard HTML characters first and then replaces markdown tokens with HTML tags.
// It extracts code blocks and inline code blocks first to ensure no formatting happens inside them.
func markdownToHTML(md string) string {
	escaped := html.EscapeString(md)

	var codeBlocks []string
	placeholderPrefix := "XYZCODEBLOCKXYZ"

	// Match and extract code blocks: ```...```
	codeBlockRegex := regexp.MustCompile("(?s)```[a-zA-Z0-9-]*\\n?(.*?)\\n?```")
	escaped = codeBlockRegex.ReplaceAllStringFunc(escaped, func(match string) string {
		submatch := codeBlockRegex.FindStringSubmatch(match)
		if len(submatch) > 1 {
			codeBlocks = append(codeBlocks, "<pre>"+submatch[1]+"</pre>")
			return fmt.Sprintf("%s%d___", placeholderPrefix, len(codeBlocks)-1)
		}
		return match
	})

	// Match and extract inline code: `...`
	inlineCodeRegex := regexp.MustCompile("`([^`\n]+)`")
	escaped = inlineCodeRegex.ReplaceAllStringFunc(escaped, func(match string) string {
		submatch := inlineCodeRegex.FindStringSubmatch(match)
		if len(submatch) > 1 {
			codeBlocks = append(codeBlocks, "<code>"+submatch[1]+"</code>")
			return fmt.Sprintf("%s%d___", placeholderPrefix, len(codeBlocks)-1)
		}
		return match
	})

	// Extract links: [text](url) -> <a href="url">text</a>
	// Recursively format the link text to support bold/italic inside it.
	linkRegex := regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\n]+)\)`)
	escaped = linkRegex.ReplaceAllStringFunc(escaped, func(match string) string {
		submatch := linkRegex.FindStringSubmatch(match)
		if len(submatch) > 2 {
			linkText := submatch[1]
			linkURL := submatch[2]
			formattedText := markdownToHTML(linkText)
			codeBlocks = append(codeBlocks, fmt.Sprintf(`<a href="%s">%s</a>`, linkURL, formattedText))
			return fmt.Sprintf("%s%d___", placeholderPrefix, len(codeBlocks)-1)
		}
		return match
	})

	// delimiter run holds consecutive identical formatting characters.
	type delRun struct {
		char       rune
		length     int
		origLength int
		start      int
		open       bool
		close      bool
	}

	type runMatch struct {
		openerStart int
		openerEnd   int
		closerStart int
		closerEnd   int
		tag         string
	}

	runes := []rune(escaped)
	n := len(runes)
	var runs []*delRun

	// Scan runes to find all potential formatting delimiter runs
	for i := 0; i < n; {
		r := runes[i]
		if r == '*' || r == '_' || r == '~' || r == '|' {
			start := i
			count := 0
			for i < n && runes[i] == r {
				count++
				i++
			}

			// Determine open / close status based on boundaries of the whole run
			nextIsWhitespace := i >= n || isWhitespace(runes[i])
			prevIsWhitespace := start == 0 || isWhitespace(runes[start-1])

			open := !nextIsWhitespace
			close := !prevIsWhitespace

			// Special word boundary rules for _ to avoid matching inside variable/filenames (e.g. simple_primes.py)
			if r == '_' {
				prevIsWord := start > 0 && isWordChar(runes[start-1])
				nextIsWord := i < n && isWordChar(runes[i])
				if prevIsWord {
					open = false
				}
				if nextIsWord {
					close = false
				}
			}

			runs = append(runs, &delRun{
				char:       r,
				length:     count,
				origLength: count,
				start:      start,
				open:       open,
				close:      close,
			})
		} else {
			i++
		}
	}

	var openRuns []*delRun
	var matches []runMatch

	// Match pairs using a standard markdown run-matching algorithm
	for _, d := range runs {
		if d.close {
			for j := len(openRuns) - 1; j >= 0 && d.length > 0; j-- {
				opener := openRuns[j]
				if opener.char == d.char && opener.length > 0 {
					matchLen := 0
					tag := ""
					if d.char == '*' || d.char == '_' {
						if d.length >= 2 && opener.length >= 2 {
							matchLen = 2
							if d.char == '*' {
								tag = "b"
							} else {
								tag = "u"
							}
						} else if d.length >= 1 && opener.length >= 1 {
							matchLen = 1
							tag = "i"
						}
					} else if d.char == '~' {
						if d.length >= 2 && opener.length >= 2 {
							matchLen = 2
							tag = "s"
						}
					} else if d.char == '|' {
						if d.length >= 2 && opener.length >= 2 {
							matchLen = 2
							tag = "tg-spoiler"
						}
					}

					if matchLen > 0 {
						openerStart := opener.start + opener.length - matchLen
						openerEnd := openerStart + matchLen

						closerStart := d.start + d.origLength - d.length
						closerEnd := closerStart + matchLen

						matches = append(matches, runMatch{
							openerStart: openerStart,
							openerEnd:   openerEnd,
							closerStart: closerStart,
							closerEnd:   closerEnd,
							tag:         tag,
						})

						opener.length -= matchLen
						d.length -= matchLen

						// Prune the stack of delimiters that were pushed after this opener
						if opener.length == 0 {
							openRuns = openRuns[:j]
						} else {
							openRuns = openRuns[:j+1]
						}
					}
				}
			}
		}
		if d.length > 0 && d.open {
			openRuns = append(openRuns, d)
		}
	}

	// Generate tag insertions
	type insertion struct {
		start int
		end   int
		tag   string
	}
	var insertions []insertion

	for _, m := range matches {
		insertions = append(insertions, insertion{
			start: m.openerStart,
			end:   m.openerEnd,
			tag:   "<" + m.tag + ">",
		})
		insertions = append(insertions, insertion{
			start: m.closerStart,
			end:   m.closerEnd,
			tag:   "</" + m.tag + ">",
		})
	}

	// Sort insertions by start index in descending order
	sort.Slice(insertions, func(i, j int) bool {
		return insertions[i].start > insertions[j].start
	})

	// Apply insertions
	resultRunes := runes
	for _, ins := range insertions {
		prefix := resultRunes[:ins.start]
		suffix := resultRunes[ins.end:]
		tagRunes := []rune(ins.tag)

		newResult := make([]rune, len(prefix)+len(tagRunes)+len(suffix))
		copy(newResult, prefix)
		copy(newResult[len(prefix):], tagRunes)
		copy(newResult[len(prefix)+len(tagRunes):], suffix)
		resultRunes = newResult
	}

	escaped = string(resultRunes)

	// Convert headings (e.g. ## Heading) to bold
	headingRegex := regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	escaped = headingRegex.ReplaceAllString(escaped, "<b>$1</b>")

	// Restore all code blocks and link placeholders
	for i, code := range codeBlocks {
		placeholder := fmt.Sprintf("%s%d___", placeholderPrefix, i)
		escaped = strings.Replace(escaped, placeholder, code, 1)
	}

	return escaped
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
