package text

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const leetSymbols = "@$!|"

var letterFixes = strings.NewReplacer("ß", "ss", "ł", "l")

const maxShortWordLength = 3

var latinLookalikes = map[rune]rune{
	'0': 'o', '1': 'i', 'l': 'i', '!': 'i', '|': 'i',
	'3': 'e', '4': 'a', '@': 'a', '5': 's', '$': 's', '7': 't', '8': 'b',
	'а': 'a', 'в': 'b', 'с': 'c', 'е': 'e', 'н': 'h', 'і': 'i', 'ј': 'j', 'к': 'k',
	'м': 'm', 'о': 'o', 'р': 'p', 'ѕ': 's', 'т': 't', 'х': 'x', 'у': 'y',
}

var cyrillicLookalikes = map[rune]rune{
	'a': 'а', 'b': 'в', 'c': 'с', 'e': 'е', 'h': 'н', 'k': 'к', 'm': 'м',
	'o': 'о', 'p': 'р', 't': 'т', 'x': 'х', 'y': 'у',
	'i': 'и', 'і': 'и', '1': 'и', '!': 'и', '|': 'и',
	'0': 'о', '3': 'з', '4': 'ч', '6': 'б', '@': 'а',
}

func normalize(s string) []string {
	s = strings.ToLower(norm.NFKC.String(s))
	s = letterFixes.Replace(s)
	s, _, _ = transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s)
	s = strings.Map(dropFormat, s)

	tokens := make([]string, 0)
	for _, word := range strings.FieldsFunc(s, isSeparator) {
		if word = strings.TrimRight(word, "!"); word != "" {
			tokens = append(tokens, word)
		}
	}
	tokens = joinLetters(tokens)
	for i, token := range tokens {
		tokens[i] = fold(token)
	}
	return tokens
}

func dropFormat(r rune) rune {
	if unicode.Is(unicode.Cf, r) {
		return -1
	}
	return r
}

func isSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(leetSymbols, r)
}

func joinLetters(tokens []string) []string {
	return joinRuns(tokens, 1)
}

func joinShortWords(tokens []string) []string {
	return joinRuns(tokens, maxShortWordLength)
}

func joinRuns(tokens []string, maxLength int) []string {
	joined := make([]string, 0, len(tokens))
	run := ""
	for _, token := range tokens {
		if utf8.RuneCountInString(token) <= maxLength {
			run += token
			continue
		}
		if run != "" {
			joined = append(joined, run)
			run = ""
		}
		joined = append(joined, token)
	}
	if run != "" {
		joined = append(joined, run)
	}
	return joined
}

func fold(word string) string {
	lookalikes := latinLookalikes
	if mostlyCyrillic(word) {
		lookalikes = cyrillicLookalikes
	}
	return strings.Map(func(r rune) rune {
		if folded, ok := lookalikes[r]; ok {
			return folded
		}
		return r
	}, word)
}

func mostlyCyrillic(word string) bool {
	cyrillic, latin := 0, 0
	for _, r := range word {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	return cyrillic > 0 && cyrillic >= latin
}
