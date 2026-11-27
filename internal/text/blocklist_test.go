package text

import (
	"errors"
	"fmt"
	"testing"
	"testing/fstest"
)

func fixtureBlocklist(t *testing.T) *blocklist {
	t.Helper()
	b, err := parseBlocklist(fstest.MapFS{
		"en.txt": {Data: []byte("# English\n*fuck*\nshit\nass\nkill yourself\n")},
		"es.txt": {Data: []byte("puta\nhijo de puta\n")},
		"pl.txt": {Data: []byte("*pierdal*\r\nkurwa\r\n")},
		"ru.txt": {Data: []byte("*хуй*\n!страху\n*пизд*\n")},
	})
	if err != nil {
		t.Fatalf("parseBlocklist: %v", err)
	}
	return b
}

func TestBlocklistBlocks(t *testing.T) {
	b := fixtureBlocklist(t)
	for _, tc := range []struct {
		name string
		in   string
	}{
		{name: "a plain word", in: "This is shit"},
		{name: "a word in capitals", in: "KURWA"},
		{name: "letters spaced out", in: "f u c k this"},
		{name: "letters split by dots", in: "f.u.c.k"},
		{name: "leetspeak digits", in: "sh1t"},
		{name: "leetspeak symbols", in: "$h!t"},
		{name: "a leet word ending in symbols", in: "a$$"},
		{name: "a zero-width space inside a word", in: "f" + string(rune(0x200B)) + "uck"},
		{name: "fullwidth letters", in: "ＳＨＩＴ"},
		{name: "a stretched word", in: "fuuuuck"},
		{name: "a stretched short word", in: "asss"},
		{name: "a root inside a longer word", in: "motherfucker"},
		{name: "a polish root with a prefix", in: "spierdalaj"},
		{name: "a phrase with extra spaces and capitals", in: "Hijo  de  PUTA"},
		{name: "a phrase followed by punctuation", in: "kill   yourself!"},
		{name: "a cyrillic root with a latin lookalike", in: "xуйня"},
		{name: "a cyrillic root with a digit", in: "п1здец"},
		{name: "cyrillic letters spaced out", in: "п и з д а"},
		{name: "a stretched cyrillic root", in: "хууууй"},
		{name: "a cyrillic lookalike in a latin root", in: "fuсk"},
		{name: "a cyrillic lookalike in a latin word", in: "kurwа"},
		{name: "a cyrillic first letter in a latin word", in: "кurwa"},
		{name: "a ukrainian i in a latin word", in: "shіt"},
		{name: "one letter doubled", in: "shiit"},
		{name: "one letter of a root doubled", in: "fuuck"},
		{name: "a root split into short chunks", in: "fu ck off"},
		{name: "a root split by a hyphen", in: "pi-er-da-la-j"},
		{name: "a cyrillic root split into short chunks", in: "ху й"},
		{name: "a blocked root next to an exception", in: "страх хуй"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !b.blocks(tc.in) {
				t.Errorf("blocks(%q) = false, want true", tc.in)
			}
		})
	}
}

func TestBlocklistAllows(t *testing.T) {
	b := fixtureBlocklist(t)
	for _, tc := range []struct {
		name string
		in   string
	}{
		{name: "english student text", in: "Anyone up for a study group in the library tonight?"},
		{name: "a blocked word inside an innocent one", in: "Our class assistant has the slides"},
		{name: "a word that collapses to a blocked one", in: "as well as the exam"},
		{name: "a word that starts like a blocked one", in: "Shiitake soup in the canteen"},
		{name: "spanish student text", in: "Mi computadora no funciona, ¿alguien me ayuda?"},
		{name: "spanish word containing a blocked one", in: "Una disputa sobre el examen"},
		{name: "polish student text", in: "Cześć, ktoś ma notatki z analizy matematycznej?"},
		{name: "german student text", in: "Hat jemand die Schlüssel für den Seminarraum?"},
		{name: "russian student text", in: "Привет, кто идёт на пару по матанализу?"},
		{name: "ukrainian student text", in: "Привіт, хто має конспект з історії України?"},
		{name: "belarusian student text", in: "Добры дзень, хто ідзе на лекцыю па праву?"},
		{name: "russian words for insuring", in: "Страхуйте квартиру, я застрахую машину"},
		{name: "ukrainian word for insuring", in: "Підстрахуй мене на іспиті"},
		{name: "doubled letters in ordinary words", in: "Pass the class, see you at noon"},
		{name: "short words next to each other", in: "I am a fan of it, so be it"},
		{name: "short polish words next to each other", in: "Dzięki ci, pa! Ku temu nie ma racji"},
		{name: "empty text", in: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if b.blocks(tc.in) {
				t.Errorf("blocks(%q) = true, want false", tc.in)
			}
		})
	}
}

func TestParseBlocklistRejectsInvalidEntries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry string
	}{
		{name: "a star only at the end", entry: "pierdol*"},
		{name: "a star only at the start", entry: "*pierdol"},
		{name: "a star in the middle", entry: "pier*dol"},
		{name: "a lone star", entry: "*"},
		{name: "two stars", entry: "**"},
		{name: "a root of several words", entry: "*hijo de*"},
		{name: "no letters", entry: "?!"},
		{name: "an exception with a star", entry: "!*страху*"},
		{name: "an exception of several words", entry: "!hijo de"},
		{name: "an empty exception", entry: "!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseBlocklist(fstest.MapFS{"xx.txt": {Data: []byte("shit\n" + tc.entry + "\n")}})
			if !errors.Is(err, errInvalidEntry) {
				t.Errorf("parseBlocklist(%q) error = %v, want errInvalidEntry", tc.entry, err)
			}
		})
	}
}

func TestParseBlocklistSkipsCommentsAndBlankLines(t *testing.T) {
	b, err := parseBlocklist(fstest.MapFS{"xx.txt": {Data: []byte("# a comment\n\n   \nshit\n")}})
	if err != nil {
		t.Fatalf("parseBlocklist: %v", err)
	}
	if len(b.phrases) != 1 || len(b.roots) != 0 || len(b.exceptions) != 0 {
		t.Errorf("got %d phrases, %d roots and %d exceptions, want 1, 0 and 0", len(b.phrases), len(b.roots), len(b.exceptions))
	}
}

func TestEmbeddedBlocklist(t *testing.T) {
	err := Allowed("bio", "kurwa")
	if !errors.Is(fmt.Errorf("%w: %w", errors.New("module error"), err), ErrNotAllowed) {
		t.Errorf("Allowed(kurwa) = %v, want ErrNotAllowed through a wrapper", err)
	}
	if err := Allowed("bio", "Cześć, ktoś ma notatki z analizy?"); err != nil {
		t.Errorf("Allowed(student text) = %v, want nil", err)
	}
	for _, in := range []string{"страхуйте квартиру", "застрахуй машину", "укроп и петрушка", "Nalej ukropu do garnka"} {
		if err := Allowed("bio", in); err != nil {
			t.Errorf("Allowed(%q) = %v, want nil", in, err)
		}
	}
	for _, in := range []string{"fuсk", "кurwa", "shіt", "kurrwa", "ku-rwa"} {
		if err := Allowed("bio", in); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("Allowed(%q) = %v, want ErrNotAllowed", in, err)
		}
	}
}
