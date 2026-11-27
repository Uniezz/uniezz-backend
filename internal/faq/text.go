package faq

import (
	"fmt"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	minQuestionLength = 3
	maxQuestionLength = 200
	minAnswerLength   = 1
	maxAnswerLength   = 2000
)

// languages are the interface languages; a published entry has its text in every one.
var languages = []string{"pl", "en", "uk", "ru", "be", "de", "es"}

func isLanguage(lang string) bool {
	for _, l := range languages {
		if l == lang {
			return true
		}
	}
	return false
}

func cleanText(field, s string, keepNewlines bool, minLength, maxLength int) (string, error) {
	clean := text.Plain(s, keepNewlines)
	n := utf8.RuneCountInString(clean)
	if n < minLength || n > maxLength {
		return "", ErrInvalidInput
	}
	if err := text.Allowed(field, clean); err != nil {
		return "", fmt.Errorf("%w: %w", ErrContentNotAllowed, err)
	}
	return clean, nil
}

func cleanEntryText(t EntryText) (EntryText, error) {
	question, err := cleanText("question", t.Question, false, minQuestionLength, maxQuestionLength)
	if err != nil {
		return EntryText{}, err
	}
	answer, err := cleanText("answer", t.Answer, true, minAnswerLength, maxAnswerLength)
	if err != nil {
		return EntryText{}, err
	}
	return EntryText{Question: question, Answer: answer}, nil
}

// cleanAllTexts requires the text in every language and nothing else.
func cleanAllTexts(in map[string]EntryText) (map[string]EntryText, error) {
	if len(in) != len(languages) {
		return nil, ErrInvalidInput
	}
	out := make(map[string]EntryText, len(in))
	for _, lang := range languages {
		t, ok := in[lang]
		if !ok {
			return nil, ErrInvalidInput
		}
		clean, err := cleanEntryText(t)
		if err != nil {
			return nil, err
		}
		out[lang] = clean
	}
	return out, nil
}
