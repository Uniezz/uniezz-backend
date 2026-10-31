// Package text cleans user-written text before it is stored: HTML is stripped,
// entities are decoded and control characters are dropped.
package text

import (
	"html"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
)

var stripTags = bluemonday.StrictPolicy()

func Plain(s string, keepNewlines bool) string {
	s = html.UnescapeString(stripTags.Sanitize(html.UnescapeString(s)))
	s = strings.Map(func(r rune) rune {
		if r == '\n' && keepNewlines {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
