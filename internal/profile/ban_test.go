package profile

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestBannedStudentsProfileIsHidden(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.otpUser(t)
	banned := e.newUser(t, "umcs", "banned-"+uuid.NewString()+"@umcs.pl")
	tag := "ban" + uuid.NewString()[:8]
	if _, err := e.pool.Exec(context.Background(), "UPDATE users SET last_name = $2, banned_at = NOW() WHERE id = $1", banned.id, tag); err != nil {
		t.Fatal(err)
	}

	t.Run("the profile is not found directly", func(t *testing.T) {
		expectError(t, e.api.Get("/profiles/"+banned.id.String(), viewer.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("the profile is left out of search", func(t *testing.T) {
		page := decode[struct {
			Items []ProfileSummary `json:"items"`
		}](t, e.api.Get("/profiles?"+url.Values{"q": {tag}}.Encode(), viewer.bearer), http.StatusOK)
		if len(page.Items) != 0 {
			t.Errorf("profiles = %+v", page.Items)
		}
	})
}
