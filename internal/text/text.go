// Package text cleans user-written text before it is stored: HTML is stripped,
// entities are decoded and control characters are dropped.
package text

import (
	"errors"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

var ErrInvalid = errors.New("invalid text")

const maxDecodeRounds = 8

var stripTags = bluemonday.StrictPolicy()

func Plain(s string, keepNewlines bool) string {
	s = stripMarkup(s)
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

// stripMarkup decodes entities and strips tags until neither changes the text,
// so markup hidden under any number of encodings is removed as well. Anything
// between < and > counts as a tag, encoded or not.
func stripMarkup(s string) string {
	for range maxDecodeRounds {
		next := html.UnescapeString(stripTags.Sanitize(html.UnescapeString(s)))
		if next == s {
			return s
		}
		s = next
	}
	return strings.NewReplacer("<", "", ">", "").Replace(s)
}

// Tags cleans each tag, drops case-insensitive duplicates and returns
// ErrInvalid for an empty or too long tag or too many tags.
func Tags(tags []string, maxTags, maxLength int) ([]string, error) {
	clean := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, raw := range tags {
		tag := Plain(raw, false)
		if tag == "" || utf8.RuneCountInString(tag) > maxLength {
			return nil, ErrInvalid
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, tag)
	}
	if len(clean) > maxTags {
		return nil, ErrInvalid
	}
	return clean, nil
}

// NonNil returns an empty slice for nil, so JSON shows [] instead of null.
func NonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
