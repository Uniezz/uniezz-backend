package profile

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSearchProfiles(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.otpUser(t)
	ctx := context.Background()
	marker := "srch" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")

	type student struct {
		university, email, firstName string
		interests                    []string
		faculty, programme           string
		hideFaculty                  bool
	}
	create := func(s student, minutesAgo int) uuid.UUID {
		t.Helper()
		u := e.newUser(t, s.university, s.email)
		privacy := `{}`
		if s.hideFaculty {
			privacy = `{"showFaculty": false}`
		}
		_, err := e.pool.Exec(ctx, `UPDATE users SET first_name = $2, last_name = $3, interests = $4, privacy = $5::jsonb,
			created_at = NOW() - make_interval(mins => $6) WHERE id = $1`,
			u.id, s.firstName, marker, append([]string{}, s.interests...), privacy, minutesAgo)
		if err != nil {
			t.Fatal(err)
		}
		if s.programme != "" {
			_, err := e.pool.Exec(ctx, `INSERT INTO student_programmes (user_id, faculty_name, programme_name) VALUES ($1, $2, $3)`,
				u.id, s.faculty, s.programme)
			if err != nil {
				t.Fatal(err)
			}
		}
		return u.id
	}

	lukasz := create(student{
		university: "umcs", email: "search-" + uuid.NewString() + "@umcs.pl", firstName: "Łukasz",
		interests: []string{"wspinaczka"}, faculty: "Wydział Matematyki, Fizyki i Informatyki", programme: "Informatyka",
	}, 30)
	ivan := create(student{
		university: "kul", email: "search-" + uuid.NewString() + "@student.kul.pl", firstName: "Ivan",
		faculty: "Wydział Tajny", programme: "Prawo", hideFaculty: true,
	}, 20)
	anna := create(student{
		university: "umcs", email: "search-" + uuid.NewString() + "@umcs.pl", firstName: "Anna",
		interests: []string{"Ivanhoe fan club"},
	}, 10)

	type page struct {
		Items      []ProfileSummary `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	search := func(t *testing.T, q string, limit int) []uuid.UUID {
		t.Helper()
		params := url.Values{"q": {q}, "limit": {"50"}}
		if limit > 0 {
			params.Set("limit", strconv.Itoa(limit))
		}
		var got []uuid.UUID
		for range 10 {
			p := decode[page](t, e.api.Get("/profiles?"+params.Encode(), viewer.bearer), http.StatusOK)
			for _, item := range p.Items {
				got = append(got, item.ID)
			}
			if p.NextCursor == nil {
				return got
			}
			params.Set("cursor", *p.NextCursor)
		}
		t.Fatal("too many pages")
		return nil
	}
	sameSet := func(t *testing.T, q string, got, want []uuid.UUID) {
		t.Helper()
		slices.SortFunc(got, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		if !slices.Equal(got, want) {
			t.Errorf("q=%q: got %v, want %v", q, got, want)
		}
	}

	t.Run("matches every source, ignoring case and accents", func(t *testing.T) {
		for _, tc := range []struct {
			q    string
			want []uuid.UUID
		}{
			{"lukasz", []uuid.UUID{lukasz}},
			{"ŁUKASZ", []uuid.UUID{lukasz}},
			{"umcs", []uuid.UUID{lukasz, anna}},
			{"UMCS", []uuid.UUID{lukasz, anna}},
			{"curie-sklodowskiej", []uuid.UUID{lukasz, anna}},
			{"informatyka", []uuid.UUID{lukasz}},
			{"matematyki", []uuid.UUID{lukasz}},
			{"prawo", []uuid.UUID{ivan}},
			{"wspinaczka", []uuid.UUID{lukasz}},
			{"iva", []uuid.UUID{ivan, anna}},
		} {
			q := tc.q + " " + marker
			sameSet(t, q, search(t, q, 0), tc.want)
		}
	})

	t.Run("requires every word to match", func(t *testing.T) {
		q := "lukasz informatyka " + marker
		sameSet(t, q, search(t, q, 0), []uuid.UUID{lukasz})
		q = "ivan informatyka " + marker
		sameSet(t, q, search(t, q, 0), nil)
	})

	t.Run("hidden faculty never matches", func(t *testing.T) {
		q := "tajny " + marker
		sameSet(t, q, search(t, q, 0), nil)
	})

	t.Run("treats LIKE wildcards literally", func(t *testing.T) {
		for _, q := range []string{"%%% " + marker, "___ " + marker, "!!! " + marker} {
			sameSet(t, q, search(t, q, 0), nil)
		}
	})

	t.Run("ranks full name matches first and pages without gaps or repeats", func(t *testing.T) {
		got := search(t, "ivan "+marker, 1)
		if want := []uuid.UUID{ivan, anna}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v (name match before the newer interests match)", got, want)
		}
		got = search(t, marker, 1)
		if want := []uuid.UUID{anna, ivan, lukasz}; !slices.Equal(got, want) {
			t.Errorf("got %v, want newest first %v", got, want)
		}
	})

	t.Run("combines with the university filter", func(t *testing.T) {
		p := decode[page](t, e.api.Get("/profiles?"+url.Values{"q": {marker}, "university": {"kul"}}.Encode(), viewer.bearer), http.StatusOK)
		if len(p.Items) != 1 || p.Items[0].ID != ivan {
			t.Errorf("expected only Ivan, got %+v", p.Items)
		}
	})

	t.Run("rejects short words and long queries", func(t *testing.T) {
		for _, q := range []string{"ab", marker + " ab", "one two three four five six"} {
			expectError(t, e.api.Get("/profiles?"+url.Values{"q": {q}}.Encode(), viewer.bearer), http.StatusBadRequest, "invalid_input")
		}
	})
}
