package rewards

import (
	"regexp"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	defaultLanguage      = "pl"
	maxChallengeLangs    = 10
	maxChallengeTitle    = 80
	maxChallengeDescribe = 300
)

var languageCode = regexp.MustCompile(`^[a-z]{2}$`)

func cleanTexts(in map[string]ChallengeText) (map[string]ChallengeText, error) {
	if _, ok := in[defaultLanguage]; !ok || len(in) > maxChallengeLangs {
		return nil, ErrInvalidInput
	}
	out := make(map[string]ChallengeText, len(in))
	for lang, t := range in {
		if !languageCode.MatchString(lang) {
			return nil, ErrInvalidInput
		}
		title := text.Plain(t.Title, false)
		if title == "" || utf8.RuneCountInString(title) > maxChallengeTitle {
			return nil, ErrInvalidInput
		}
		cleaned := ChallengeText{Title: title}
		if t.Description != nil {
			description := text.Plain(*t.Description, true)
			if utf8.RuneCountInString(description) > maxChallengeDescribe {
				return nil, ErrInvalidInput
			}
			if description != "" {
				cleaned.Description = &description
			}
		}
		out[lang] = cleaned
	}
	return out, nil
}
