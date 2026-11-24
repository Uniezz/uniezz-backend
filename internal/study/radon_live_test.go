package study

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
)

func TestRadonLive(t *testing.T) {
	if os.Getenv("RADON_LIVE") != "1" {
		t.Skip("set RADON_LIVE=1 to call the real POL-on API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
	defer cancel()

	institutions, err := newProfessorRepository(database.New(setupTestDB(t))).radonInstitutions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := newRadonClient(radonEmployeesURL)
	for _, institution := range institutions {
		started := time.Now()
		employees, err := client.employees(ctx, institution.id)
		if err != nil {
			t.Fatalf("%s: %v", institution.university, err)
		}
		people := professorsAt(employees, institution)
		if len(people) == 0 {
			t.Errorf("%s: no professors", institution.university)
		}
		t.Logf("%s: %d employees, %d professors, %s", institution.university, len(employees), len(people), time.Since(started).Round(time.Millisecond))
	}
}
