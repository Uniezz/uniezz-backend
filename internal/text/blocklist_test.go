package text

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
)

func fixtureBlocklist(t *testing.T) *blocklist {
	t.Helper()
	b, err := parseBlocklist(fstest.MapFS{
		"en.txt": {Data: []byte("# English\n*fuck*\nshit\nass\nkill yourself\n")},
		"es.txt": {Data: []byte("puta\nhijo de puta\n")},
		"pl.txt": {Data: []byte("*pierdal*\r\nkurwa\r\n")},
		"ru.txt": {Data: []byte("*хуй*\n*пизд*\n")},
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
	if len(b.phrases) != 1 || len(b.roots) != 0 {
		t.Errorf("got %d phrases and %d roots, want 1 and 0", len(b.phrases), len(b.roots))
	}
}

func TestBlocklistVersion(t *testing.T) {
	version := func(data string) string {
		t.Helper()
		b, err := parseBlocklist(fstest.MapFS{"xx.txt": {Data: []byte(data)}})
		if err != nil {
			t.Fatalf("parseBlocklist: %v", err)
		}
		return b.version
	}

	lf := version("shit\n*fuck*\n")
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(lf) {
		t.Errorf("version = %q, want 12 hex characters", lf)
	}
	if crlf := version("shit\r\n*fuck*\r\n"); crlf != lf {
		t.Errorf("version with CRLF = %q, with LF = %q, want the same", crlf, lf)
	}
	if other := version("shit\n"); other == lf {
		t.Errorf("version did not change when an entry was removed: %q", other)
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
	if v := BlocklistVersion(); len(v) != blocklistVersionLength {
		t.Errorf("BlocklistVersion() = %q, want %d characters", v, blocklistVersionLength)
	}
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func TestReportNotAllowed(t *testing.T) {
	t.Run("a blocked write logs who, where and which list but not the text", func(t *testing.T) {
		buf := captureLog(t)
		userID := uuid.New()
		err := fmt.Errorf("%w: %w", errors.New("module error"), Allowed("title", "kurwa"))

		ReportNotAllowed(context.Background(), userID, err)

		if strings.Contains(buf.String(), "kurwa") {
			t.Errorf("log contains the blocked text: %s", buf)
		}
		var line map[string]any
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatalf("decode log line %q: %v", buf, err)
		}
		want := map[string]string{
			"msg":               "text not allowed",
			"user_id":           userID.String(),
			"field":             "title",
			"blocklist_version": BlocklistVersion(),
		}
		for key, value := range want {
			if line[key] != value {
				t.Errorf("log %s = %v, want %q", key, line[key], value)
			}
		}
	})

	t.Run("any other error logs nothing", func(t *testing.T) {
		buf := captureLog(t)

		ReportNotAllowed(context.Background(), uuid.New(), errors.New("invalid input"))
		ReportNotAllowed(context.Background(), uuid.New(), nil)

		if buf.Len() != 0 {
			t.Errorf("log = %s, want nothing", buf)
		}
	})
}
