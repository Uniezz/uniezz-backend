package push

import (
	"regexp"
	"unicode/utf8"
)

const (
	maxPreviewLength = 150
	defaultLanguage  = "pl"
	photoPreview     = "📷"
	fallbackTitle    = "Uniezz"
)

var tokenPattern = regexp.MustCompile(`^Expo(nent)?PushToken\[[A-Za-z0-9_-]{1,190}\]$`)

var matchBodies = map[string]string{
	"pl": "Macie dopasowanie! Przywitaj się.",
	"en": "It's a match! Say hi.",
	"uk": "У вас метч! Привітайся.",
	"ru": "У вас мэтч! Поздоровайся.",
	"be": "У вас метч! Прывітайся.",
	"de": "Es ist ein Match! Sag Hallo.",
	"es": "¡Es un match! Saluda.",
}

func cleanToken(token string) (string, error) {
	if !tokenPattern.MatchString(token) {
		return "", ErrInvalidInput
	}
	return token, nil
}

func shortPreview(text string) string {
	if utf8.RuneCountInString(text) <= maxPreviewLength {
		return text
	}
	return string([]rune(text)[:maxPreviewLength-1]) + "…"
}

func matchBody(language string) string {
	if body, ok := matchBodies[language]; ok {
		return body
	}
	return matchBodies[defaultLanguage]
}
