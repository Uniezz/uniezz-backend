package moderation

import (
	"net/http"
	"sync"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
)

func TestAutoHide(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the third report from different students hides a post", func(t *testing.T) {
		post := e.contentBy(e.user(t))

		for i := range autoHideReports {
			if e.targets.isHidden(post) {
				t.Fatalf("hidden after %d reports", i)
			}
			decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("post", post)), http.StatusCreated)
		}
		if !e.targets.isHidden(post) {
			t.Errorf("not hidden after %d reports", autoHideReports)
		}
	})

	t.Run("comments are hidden the same way", func(t *testing.T) {
		comment := e.contentBy(e.user(t))
		for range autoHideReports {
			decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("comment", comment)), http.StatusCreated)
		}
		if !e.targets.isHidden(comment) {
			t.Error("comment not hidden")
		}
	})

	t.Run("reports on other targets never hide anything", func(t *testing.T) {
		file := e.contentBy(e.user(t))
		for range autoHideReports {
			decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("file", file)), http.StatusCreated)
		}
		if e.targets.isHidden(file) {
			t.Error("a file was hidden")
		}
	})

	t.Run("resolved reports do not count", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		post := e.contentBy(e.user(t))
		first := decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("post", post)), http.StatusCreated)
		decode[Report](t, e.api.Patch("/moderation/reports/"+first.ID.String(), moderator.bearer, map[string]any{"status": "reviewed"}), http.StatusOK)

		for range autoHideReports - 1 {
			decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("post", post)), http.StatusCreated)
		}
		if e.targets.isHidden(post) {
			t.Error("hidden with a resolved report counted")
		}
	})

	t.Run("reports sent at once still hide the post", func(t *testing.T) {
		post := e.contentBy(e.user(t))
		reporters := make([]testUser, autoHideReports)
		for i := range reporters {
			reporters[i] = e.user(t)
		}

		var wg sync.WaitGroup
		codes := make([]int, len(reporters))
		for i, r := range reporters {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = e.api.Post("/reports", r.bearer, report("post", post)).Code
			}()
		}
		wg.Wait()

		for i, c := range codes {
			if c != http.StatusCreated {
				t.Errorf("report %d: status %d", i, c)
			}
		}
		if !e.targets.isHidden(post) {
			t.Error("not hidden after concurrent reports")
		}
	})

	t.Run("dismissing the reports shows the post again", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		post := e.contentBy(e.user(t))
		var last Report
		for range autoHideReports {
			last = decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("post", post)), http.StatusCreated)
		}

		decode[Report](t, e.api.Patch("/moderation/reports/"+last.ID.String(), moderator.bearer, map[string]any{"status": "dismissed"}), http.StatusOK)
		if e.targets.isHidden(post) {
			t.Error("still hidden after dismissal")
		}
	})

	t.Run("marking the reports reviewed keeps the post hidden", func(t *testing.T) {
		moderator := e.staff(t, auth.RoleModerator)
		post := e.contentBy(e.user(t))
		var last Report
		for range autoHideReports {
			last = decode[Report](t, e.api.Post("/reports", e.user(t).bearer, report("post", post)), http.StatusCreated)
		}

		decode[Report](t, e.api.Patch("/moderation/reports/"+last.ID.String(), moderator.bearer, map[string]any{"status": "reviewed"}), http.StatusOK)
		if !e.targets.isHidden(post) {
			t.Error("shown again after review")
		}
	})
}
