package text

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

const exceptionPrefix = "!"

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
	phrases       []string
	roots         []string
	exceptions    []string
	squeezedWords map[string]bool
	squeezedRoots []string
}

func parseBlocklist(files fs.FS) (*blocklist, error) {
	names, err := fs.Glob(files, "*.txt")
	if err != nil {
		return nil, fmt.Errorf("list blocklist files: %w", err)
	}
	b := &blocklist{squeezedWords: map[string]bool{}}
	for _, name := range names {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("read blocklist %s: %w", name, err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			if err := b.add(line); err != nil {
				return nil, fmt.Errorf("parse blocklist %s line %d: %w", name, n+1, err)
			}
		}
	}
	return b, nil
}

func (b *blocklist) add(line string) error {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	if exception, ok := strings.CutPrefix(line, exceptionPrefix); ok {
		return b.addException(line, exception)
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

func (b *blocklist) addException(line, exception string) error {
	if strings.Contains(exception, "*") {
		return fmt.Errorf("%w: exception %q cannot have a *", errInvalidEntry, line)
	}
	words := normalize(exception)
	if len(words) != 1 {
		return fmt.Errorf("%w: exception %q must be one word", errInvalidEntry, line)
	}
	b.exceptions = append(b.exceptions, words[0])
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
	for _, word := range words {
		if squeezed := squeeze(word); squeezed != word && b.squeezedWords[squeezed] {
			return true
		}
	}
	if b.hasRoot(words) {
		return true
	}
	joined := joinShortWords(words)
	return len(joined) < len(words) && b.hasRoot(joined)
}

func (b *blocklist) hasRoot(words []string) bool {
	for _, word := range words {
		for _, exception := range b.exceptions {
			word = strings.ReplaceAll(word, exception, " ")
		}
		for _, root := range b.roots {
			if strings.Contains(word, root) {
				return true
			}
		}
		squeezed := squeeze(word)
		if squeezed == word {
			continue
		}
		for _, root := range b.squeezedRoots {
			if strings.Contains(squeezed, root) {
				return true
			}
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
