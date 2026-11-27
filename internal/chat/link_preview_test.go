package chat

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/safehttp"
	"github.com/google/uuid"
)

const previewPage = `<!doctype html><html><head>
<title>Fallback title</title>
<meta property="og:title" content="Lab schedule &amp; rooms">
<meta property="og:description" content="Everything about <b>labs</b>">
<meta property="og:site_name" content="UMCS">
<meta property="og:image" content="/cover.png">
</head><body><meta property="og:title" content="ignored"></body></html>`

func newPageServer(t *testing.T, e *testEnv) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(previewPage))
		case "/plain":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<p>no metadata</p>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	e.module.pages = safehttp.New(safehttp.Config{
		UserAgent: "test",
		Ports:     []uint16{443, uint16(port)},
		Allowed:   []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		RootCAs:   roots,
	})
	return srv
}

func (e *testEnv) cleanPreview(t *testing.T, link string) {
	t.Helper()
	hash := sha256.Sum256([]byte(link))
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM chat_link_previews WHERE url_hash = $1", hash[:])
	})
}

func (e *testEnv) queuedPreviews(t *testing.T, messageID uuid.UUID) int {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM jobs WHERE kind = $1 AND payload->>'messageId' = $2", FetchLinkPreviewArgs{}.Kind(), messageID.String())
	})
	var n int
	err := e.pool.QueryRow(ctx, "SELECT COUNT(*) FROM jobs WHERE kind = $1 AND payload->>'messageId' = $2", FetchLinkPreviewArgs{}.Kind(), messageID.String()).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *testEnv) runPreview(t *testing.T, messageID uuid.UUID) {
	t.Helper()
	if err := e.module.fetchLinkPreview(context.Background(), FetchLinkPreviewArgs{MessageID: messageID}); err != nil {
		t.Fatalf("fetchLinkPreview: %v", err)
	}
}

func (e *testEnv) message(t *testing.T, u testUser, id uuid.UUID) Message {
	t.Helper()
	return decode[Message](t, e.api.Get("/chat/messages/"+id.String(), u.bearer), http.StatusOK)
}

func TestLinkPreview(t *testing.T) {
	e := newTestEnv(t)
	srv := newPageServer(t, e)

	t.Run("a link is sent without a preview, then the job fills it in and bumps changeSeq", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		link := srv.URL + "/page?t=" + uuid.NewString()
		e.cleanPreview(t, link)

		sent := e.send(t, a, conversation.ID, "Look: "+link+"#rooms.")
		if sent.LinkPreview != nil {
			t.Errorf("preview on send = %+v, want null while pending", sent.LinkPreview)
		}
		if n := e.queuedPreviews(t, sent.ID); n != 1 {
			t.Fatalf("queued jobs = %d, want 1", n)
		}

		e.runPreview(t, sent.ID)

		got := e.message(t, b, sent.ID)
		p := got.LinkPreview
		if p == nil || p.URL != link || p.Title == nil || *p.Title != "Lab schedule & rooms" ||
			p.Description == nil || *p.Description != "Everything about labs" || p.SiteName == nil || *p.SiteName != "UMCS" || p.ImageURL == nil {
			t.Fatalf("preview = %+v", p)
		}
		if got.ChangeSeq <= sent.ChangeSeq {
			t.Errorf("changeSeq = %d, want more than %d", got.ChangeSeq, sent.ChangeSeq)
		}
		e.media.mu.Lock()
		imported := slices.Clone(e.media.imported)
		e.media.mu.Unlock()
		if want := srv.URL + "/cover.png"; !slices.Contains(imported, want) {
			t.Errorf("imported = %v, want %s", imported, want)
		}
		page := e.history(t, b, conversation.ID, "changedAfter="+strconv.FormatInt(sent.ChangeSeq, 10))
		if len(page.Items) != 1 || page.Items[0].LinkPreview == nil {
			t.Errorf("catch-up = %+v, want the message with its preview", page.Items)
		}
	})

	t.Run("a link already previewed is shown at once and queues nothing", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		link := srv.URL + "/page?t=" + uuid.NewString()
		e.cleanPreview(t, link)
		first := e.send(t, a, conversation.ID, link)
		e.queuedPreviews(t, first.ID)
		e.runPreview(t, first.ID)

		again := e.send(t, b, conversation.ID, "same "+link)
		if again.LinkPreview == nil || again.LinkPreview.Title == nil {
			t.Errorf("preview = %+v, want the cached one", again.LinkPreview)
		}
		if n := e.queuedPreviews(t, again.ID); n != 0 {
			t.Errorf("queued jobs = %d, want 0", n)
		}
	})

	t.Run("noPreview stores no link", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := decode[Message](t, e.rawSend(a, conversation.ID, uuid.New(), map[string]any{"body": srv.URL + "/page", "noPreview": true}), http.StatusCreated)
		if n := e.queuedPreviews(t, sent.ID); n != 0 {
			t.Errorf("queued jobs = %d, want 0", n)
		}
	})

	t.Run("plain http links are not previewed", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := e.send(t, a, conversation.ID, "http://example.com/page")
		if n := e.queuedPreviews(t, sent.ID); n != 0 {
			t.Errorf("queued jobs = %d, want 0", n)
		}
	})

	for _, tc := range []struct {
		name string
		path string
	}{
		{"a missing page", "/missing"},
		{"a page without title or description", "/plain"},
	} {
		t.Run(tc.name+" leaves the message without a preview and unchanged", func(t *testing.T) {
			a, b := e.friends(t)
			conversation := e.direct(t, a, b)
			link := srv.URL + tc.path + "?t=" + uuid.NewString()
			e.cleanPreview(t, link)
			sent := e.send(t, a, conversation.ID, link)
			e.queuedPreviews(t, sent.ID)

			e.runPreview(t, sent.ID)

			got := e.message(t, a, sent.ID)
			if got.LinkPreview != nil || got.ChangeSeq != sent.ChangeSeq {
				t.Errorf("message = %+v", got)
			}
		})
	}

	t.Run("a link to a private address is never fetched", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		link := "https://10.0.0.1/admin?t=" + uuid.NewString()
		e.cleanPreview(t, link)
		sent := e.send(t, a, conversation.ID, link)
		e.queuedPreviews(t, sent.ID)

		e.runPreview(t, sent.ID)

		var status string
		hash := sha256.Sum256([]byte(link))
		if err := e.pool.QueryRow(context.Background(), "SELECT status FROM chat_link_previews WHERE url_hash = $1", hash[:]).Scan(&status); err != nil || status != "failed" {
			t.Errorf("status = %q (%v), want failed", status, err)
		}
	})

	t.Run("deleting the message drops its link, and the job then does nothing", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		link := srv.URL + "/page?t=" + uuid.NewString()
		e.cleanPreview(t, link)
		sent := e.send(t, a, conversation.ID, link)
		e.queuedPreviews(t, sent.ID)
		if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}

		e.runPreview(t, sent.ID)

		var n int
		hash := sha256.Sum256([]byte(link))
		if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM chat_link_previews WHERE url_hash = $1", hash[:]).Scan(&n); err != nil || n != 0 {
			t.Errorf("previews = %d (%v), want 0", n, err)
		}
	})
}

func TestPruneLinkPreviews(t *testing.T) {
	e := newTestEnv(t)
	srv := newPageServer(t, e)
	ctx := context.Background()

	a, b := e.friends(t)
	conversation := e.direct(t, a, b)
	_ = b
	used := srv.URL + "/page?t=" + uuid.NewString()
	unused := srv.URL + "/page?t=" + uuid.NewString()
	failed := srv.URL + "/missing?t=" + uuid.NewString()
	for _, link := range []string{used, unused, failed} {
		e.cleanPreview(t, link)
		sent := e.send(t, a, conversation.ID, link)
		e.queuedPreviews(t, sent.ID)
		e.runPreview(t, sent.ID)
		if link == unused {
			if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("delete: %d", rec.Code)
			}
		}
	}
	hashes := map[string][]byte{}
	for _, link := range []string{used, unused, failed} {
		h := sha256.Sum256([]byte(link))
		hashes[link] = h[:]
	}
	var unusedMedia uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT media_id FROM chat_link_previews WHERE url_hash = $1", hashes[unused]).Scan(&unusedMedia); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE chat_link_previews SET fetched_at = NOW() - INTERVAL '31 days' WHERE url_hash = ANY($1)",
		[][]byte{hashes[used], hashes[unused], hashes[failed]}); err != nil {
		t.Fatal(err)
	}

	if err := e.module.pruneLinkPreviews(ctx, PruneLinkPreviewsArgs{}); err != nil {
		t.Fatalf("pruneLinkPreviews: %v", err)
	}

	for link, want := range map[string]int{used: 1, unused: 0, failed: 0} {
		var n int
		if err := e.pool.QueryRow(ctx, "SELECT COUNT(*) FROM chat_link_previews WHERE url_hash = $1", hashes[link]).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("rows for %s = %d, want %d", link, n, want)
		}
	}
	if !e.media.wasDeleted(unusedMedia) {
		t.Errorf("image of the pruned preview was not deleted")
	}
}
