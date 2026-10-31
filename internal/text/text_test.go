package text

import "testing"

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
	} {
		if got := Plain(tc.in, tc.newlines); got != tc.want {
			t.Errorf("Plain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
