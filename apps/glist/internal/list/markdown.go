package list

import (
	"regexp"
	"strings"
)

var (
	bulletRe = regexp.MustCompile(`^\s*(?:[-*+]|\d+\.)\s+(.*)$`)
	headerRe = regexp.MustCompile(`^\s*#{1,6}\s+(.+)$`)
)

type ParsedItem struct {
	Text     string
	Category string
	Count    float64
	Unit     string
}

// ParseMarkdown parses a markdown list. Supports bullets (- * + 1.), headers
// (## Produce) as category hints when the header matches a known category, and
// quantity prefixes via ParseQuantity.
func ParseMarkdown(src string) []ParsedItem {
	out := []ParsedItem{}
	currentCat := ""
	anyBullets := false

	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" {
			continue
		}
		if m := headerRe.FindStringSubmatch(line); m != nil {
			h := strings.ToLower(strings.TrimSpace(m[1]))
			h = strings.TrimSuffix(h, ":")
			if IsValidCategory(h) {
				currentCat = h
			} else {
				matched := false
				for _, cat := range Categories {
					if strings.Contains(h, cat) {
						currentCat = cat
						matched = true
						break
					}
				}
				if !matched {
					currentCat = ""
				}
			}
			continue
		}
		m := bulletRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		anyBullets = true
		text := stripMarkdownFormatting(strings.TrimSpace(m[1]))
		count, unit, rest := ParseQuantity(text)
		if rest == "" {
			continue
		}
		out = append(out, ParsedItem{Text: rest, Category: currentCat, Count: count, Unit: unit})
	}
	if !anyBullets {
		for _, raw := range strings.Split(src, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			text := stripMarkdownFormatting(line)
			count, unit, rest := ParseQuantity(text)
			if rest != "" {
				out = append(out, ParsedItem{Text: rest, Count: count, Unit: unit})
			}
		}
	}
	return out
}

func stripMarkdownFormatting(s string) string {
	s = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile("[*_~`]{1,3}").ReplaceAllString(s, "")
	s = regexp.MustCompile(`^\[\s*[xX ]?\s*\]\s*`).ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
