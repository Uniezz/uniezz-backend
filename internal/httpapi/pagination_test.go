package httpapi

import (
	"errors"
	"testing"
	"time"
)

func TestCursor(t *testing.T) {
	type position struct {
		At time.Time `json:"a"`
		ID string    `json:"i"`
	}
	want := position{At: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), ID: "x"}

	cursor, err := EncodeCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	var got position
	if err := DecodeCursor(cursor, &got); err != nil || !got.At.Equal(want.At) || got.ID != want.ID {
		t.Errorf("round trip = %+v, %v; want %+v", got, err, want)
	}

	for _, bad := range []string{"%%%", "bm90IGpzb24"} {
		if err := DecodeCursor(bad, &got); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("DecodeCursor(%q) = %v, want ErrInvalidCursor", bad, err)
		}
	}
}

func TestPage(t *testing.T) {
	position := func(last int) any { return map[string]int{"n": last} }

	rows, next, err := Page([]int{1, 2}, 2, position)
	if err != nil || len(rows) != 2 || next != nil {
		t.Fatalf("last page = %v, %v, %v", rows, next, err)
	}

	rows, next, err = Page([]int{1, 2, 3}, 2, position)
	if err != nil || len(rows) != 2 || next == nil {
		t.Fatalf("full page = %v, %v, %v", rows, next, err)
	}
	var got map[string]int
	if err := DecodeCursor(*next, &got); err != nil || got["n"] != 2 {
		t.Errorf("cursor points at %v, want the last returned row", got)
	}
}
