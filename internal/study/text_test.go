package study

import (
	"strings"
	"testing"
)

func TestCleanProfessorName(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
		want  string
		ok    bool
	}{
		{"joins the parts that are set", []string{"Anna", "", "Nowak"}, "Anna Nowak", true},
		{"strips markup", []string{"<img src=x onerror=alert(1)>Jan", "Kowalski"}, "Jan Kowalski", true},
		{"rejects a name that is empty after cleaning", []string{"<b></b>", " "}, "", false},
		{"rejects a name over 200 characters", []string{strings.Repeat("a", 201)}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cleanProfessorName(tc.parts...)
			if got != tc.want || ok != tc.ok {
				t.Errorf("got %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCleanDegree(t *testing.T) {
	if got := cleanDegree(" dr hab. "); got == nil || *got != "dr hab." {
		t.Errorf("got %v", got)
	}
	if got := cleanDegree(""); got != nil {
		t.Errorf("empty degree = %q, want nil", *got)
	}
	if got := cleanDegree(strings.Repeat("x", 101)); got != nil {
		t.Errorf("long degree kept")
	}
}
