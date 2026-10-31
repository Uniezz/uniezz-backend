package text

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestPlain(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		newlines bool
	}{
		{in: "  Hello  ", want: "Hello"},
		{in: "<b>bold</b> and <script>alert(1)</script>text", want: "bold and text"},
		{in: "a < b & c > d", want: "a < b & c > d"},
		{in: "line one\nline two", want: "line one\nline two", newlines: true},
		{in: "line one\nline two", want: "line oneline two"},
		{in: "tab\there\x00", want: "tabhere"},
		{in: "&lt;img src=x onerror=alert(1)&gt;hi", want: "hi"},
		{in: "&lt;b&gt;x&lt;/b&gt;", want: "x"},
		{in: "fish &amp; chips", want: "fish & chips"},
		{in: "&amp;amp;lt;img src=x onerror=alert(1)&amp;amp;gt;hi", want: "hi"},
		{in: "&amp;lt;script&amp;gt;alert(1)&amp;lt;/script&amp;gt;ok", want: "ok"},
		{in: "x <y and y> z", want: "x  z"},
	} {
		if got := Plain(tc.in, tc.newlines); got != tc.want {
			t.Errorf("Plain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlainNeverReturnsMarkup(t *testing.T) {
	payload := "<img src=x onerror=alert(1)>"
	for range 12 {
		payload = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(payload)
		if got := Plain(payload, false); strings.ContainsAny(got, "<>") {
			t.Fatalf("Plain(%q) = %q, still markup", payload, got)
		}
	}
}

func TestTags(t *testing.T) {
	got, err := Tags([]string{" Go ", "go", "<b>Climbing</b>"}, 10, 30)
	if err != nil || !slices.Equal(got, []string{"Go", "Climbing"}) {
		t.Errorf("got %v %v, want [Go Climbing]", got, err)
	}
	for name, in := range map[string][]string{
		"empty tag":    {"go", "  "},
		"tag too long": {strings.Repeat("x", 31)},
		"too many":     {"a", "b", "c"},
	} {
		if _, err := Tags(in, 2, 30); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}
