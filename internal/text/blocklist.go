package text

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

const (
	blocklistVersionLength = 12
	minRepeat              = 3
)

var (
	ErrNotAllowed   = errors.New("text not allowed")
	errInvalidEntry = errors.New("invalid blocklist entry")
)

type notAllowedError struct {
	field string
}

func (e notAllowedError) Error() string {
	return "text not allowed in " + e.field
}

func (e notAllowedError) Unwrap() error {
	return ErrNotAllowed
}

//go:embed blocklist/*.txt
var blocklistFiles embed.FS

var defaultBlocklist = mustLoadBlocklist()

func Allowed(field, s string) error {
	if defaultBlocklist.blocks(s) {
		return notAllowedError{field: field}
	}
	return nil
}

func ReportNotAllowed(ctx context.Context, userID uuid.UUID, err error) {
	var notAllowed notAllowedError
	if !errors.As(err, &notAllowed) {
		return
	}
	slog.InfoContext(ctx, "text not allowed",
		slog.String("user_id", userID.String()),
		slog.String("field", notAllowed.field),
		slog.String("blocklist_version", BlocklistVersion()),
	)
}

func BlocklistVersion() string {
	return defaultBlocklist.version
}

func mustLoadBlocklist() *blocklist {
	files, err := fs.Sub(blocklistFiles, "blocklist")
	if err != nil {
		panic(err)
	}
	b, err := parseBlocklist(files)
	if err != nil {
		panic(err)
	}
	return b
}

type blocklist struct {
	version       string
	phrases       []string
	roots         []string
	squeezedWords map[string]bool
	squeezedRoots []string
}

func parseBlocklist(files fs.FS) (*blocklist, error) {
	names, err := fs.Glob(files, "*.txt")
	if err != nil {
		return nil, fmt.Errorf("list blocklist files: %w", err)
	}
	b := &blocklist{squeezedWords: map[string]bool{}}
	hash := sha256.New()
	for _, name := range names {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("read blocklist %s: %w", name, err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			hash.Write([]byte(line + "\n"))
			if err := b.add(line); err != nil {
				return nil, fmt.Errorf("parse blocklist %s line %d: %w", name, n+1, err)
			}
		}
	}
	b.version = hex.EncodeToString(hash.Sum(nil))[:blocklistVersionLength]
	return b, nil
}

func (b *blocklist) add(line string) error {
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	root := len(line) > 2 && strings.HasPrefix(line, "*") && strings.HasSuffix(line, "*")
	entry := line
	if root {
		entry = line[1 : len(line)-1]
	}
	if strings.Contains(entry, "*") {
		return fmt.Errorf("%w: %q has a misplaced *", errInvalidEntry, line)
	}
	words := normalize(entry)
	switch {
	case len(words) == 0:
		return fmt.Errorf("%w: %q has no letters", errInvalidEntry, line)
	case root && len(words) > 1:
		return fmt.Errorf("%w: root %q must be one word", errInvalidEntry, line)
	case root:
		b.roots = append(b.roots, words[0])
		b.squeezedRoots = append(b.squeezedRoots, squeeze(words[0]))
	case len(words) == 1:
		b.phrases = append(b.phrases, " "+words[0]+" ")
		b.squeezedWords[squeeze(words[0])] = true
	default:
		b.phrases = append(b.phrases, " "+strings.Join(words, " ")+" ")
	}
	return nil
}

func (b *blocklist) blocks(s string) bool {
	words := normalize(s)
	padded := " " + strings.Join(words, " ") + " "
	for _, phrase := range b.phrases {
		if strings.Contains(padded, phrase) {
			return true
		}
	}
	for _, root := range b.roots {
		if strings.Contains(padded, root) {
			return true
		}
	}
	for _, word := range words {
		if !hasRepeat(word) {
			continue
		}
		squeezed := squeeze(word)
		if b.squeezedWords[squeezed] {
			return true
		}
		for _, root := range b.squeezedRoots {
			if strings.Contains(squeezed, root) {
				return true
			}
		}
	}
	return false
}

func hasRepeat(word string) bool {
	run, last := 0, rune(-1)
	for _, r := range word {
		if r == last {
			run++
		} else {
			run, last = 1, r
		}
		if run >= minRepeat {
			return true
		}
	}
	return false
}

func squeeze(word string) string {
	var b strings.Builder
	last := rune(-1)
	for _, r := range word {
		if r != last {
			b.WriteRune(r)
		}
		last = r
	}
	return b.String()
}
