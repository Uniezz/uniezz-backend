package database_test

import (
	"slices"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/database"
)

func TestArgs(t *testing.T) {
	args := database.Args{"viewer"}
	if got := args.Add("a"); got != "$2" {
		t.Errorf("first placeholder = %s, want $2", got)
	}
	if got := args.Add(3); got != "$3" {
		t.Errorf("second placeholder = %s, want $3", got)
	}
	if !slices.Equal(args, database.Args{"viewer", "a", 3}) {
		t.Errorf("args = %v", args)
	}
}
