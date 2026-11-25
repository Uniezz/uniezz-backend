package text

import (
	"slices"
	"testing"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string
	}{
		{name: "spaced letters are joined", in: "F U C K", want: []string{"fuck"}},
		{name: "dotted letters are joined", in: "f.u.c.k", want: []string{"fuck"}},
		{name: "leet symbols fold to letters", in: "$h!t", want: []string{"shit"}},
		{name: "leet digits fold to letters", in: "sh1t", want: []string{"shit"}},
		{name: "zero-width characters are dropped", in: "f\u200Bu\u200Dck", want: []string{"fuck"}},
		{name: "fullwidth letters become ascii", in: "ＦＵＣＫ", want: []string{"fuck"}},
		{name: "a trailing exclamation mark is punctuation", in: "fuck!!!", want: []string{"fuck"}},
		{name: "a trailing dollar stays leetspeak", in: "a$$", want: []string{"ass"}},
		{name: "a latin lookalike in a cyrillic word becomes cyrillic", in: "xуй", want: []string{"хуи"}},
		{name: "spaced mixed-script letters are judged together", in: "x у й", want: []string{"хуи"}},
		{name: "digits in a cyrillic word fold to cyrillic", in: "п1здец", want: []string{"пиздец"}},
		{name: "ukrainian i folds to и", in: "Привіт", want: []string{"привит"}},
		{name: "belarusian short u loses its breve", in: "праўда", want: []string{"прауда"}},
		{name: "russian yo becomes ye", in: "ёжик", want: []string{"ежик"}},
		{name: "german sharp s becomes ss", in: "Größe", want: []string{"grosse"}},
		{name: "spanish tilde is dropped", in: "Mañana", want: []string{"manana"}},
		{name: "polish diacritics are dropped and l folds to i", in: "Łódź", want: []string{"iodz"}},
		{name: "punctuation separates words", in: "Cześć, jak się masz?", want: []string{"czesc", "jak", "sie", "masz"}},
		{name: "single-letter words are joined to each other only", in: "i w domu", want: []string{"iw", "domu"}},
		{name: "empty text has no words", in: "  ?! ", want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalize(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
