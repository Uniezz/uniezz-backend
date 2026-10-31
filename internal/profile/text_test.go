package profile

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCleanBio(t *testing.T) {
	ptr := func(s string) *string { return &s }

	if bio, set, err := cleanBio(nil); bio != nil || set || err != nil {
		t.Errorf("nil bio must leave the bio unchanged, got %v %v %v", bio, set, err)
	}
	if bio, set, err := cleanBio(ptr("  <i></i> ")); bio != nil || !set || err != nil {
		t.Errorf("an empty bio must clear it, got %v %v %v", bio, set, err)
	}
	if bio, _, err := cleanBio(ptr("<p>" + strings.Repeat("ł", 500) + "</p>")); err != nil || len([]rune(*bio)) != 500 {
		t.Errorf("500 characters after stripping HTML must pass, got %v", err)
	}
	if _, _, err := cleanBio(ptr(strings.Repeat("a", 501))); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for 501 characters, got %v", err)
	}
}

func TestCleanInterests(t *testing.T) {
	got, err := cleanInterests(&[]string{" Go ", "go", "<b>Climbing</b>"})
	if err != nil || !slices.Equal(got, []string{"Go", "Climbing"}) {
		t.Errorf("got %v %v, want [Go Climbing]", got, err)
	}

	if got, err := cleanInterests(&[]string{}); err != nil || got == nil || len(got) != 0 {
		t.Errorf("an empty list must clear interests, got %v %v", got, err)
	}

	for name, in := range map[string][]string{
		"empty tag":    {"go", "  "},
		"tag too long": {strings.Repeat("x", 31)},
		"too many":     {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
	} {
		if _, err := cleanInterests(&in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: expected ErrInvalidInput, got %v", name, err)
		}
	}
}
