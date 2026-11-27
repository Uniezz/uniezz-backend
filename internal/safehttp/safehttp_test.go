package safehttp

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

var htmlLimits = Limits{Timeout: 3 * time.Second, MaxBytes: 512 << 10, ContentTypes: []string{"text/html"}}

func TestGetRefusesSpecialAddresses(t *testing.T) {
	c := New(Config{UserAgent: "test"})

	for _, tc := range []struct {
		name string
		url  string
	}{
		{"loopback", "https://127.0.0.1/"},
		{"localhost by name", "https://localhost/"},
		{"private 10/8", "https://10.0.0.1/"},
		{"private 192.168/16", "https://192.168.1.1/"},
		{"cloud metadata", "https://169.254.169.254/latest/meta-data/"},
		{"carrier-grade NAT", "https://100.64.0.1/"},
		{"unspecified", "https://0.0.0.0/"},
		{"IPv6 loopback", "https://[::1]/"},
		{"IPv4-mapped loopback", "https://[::ffff:127.0.0.1]/"},
		{"IPv6 unique local", "https://[fd00::1]/"},
		{"a port other than 443", "https://93.184.215.14:8443/"},
		{"plain http", "http://93.184.215.14/"},
		{"credentials in the URL", "https://user:pass@93.184.215.14/"},
		{"not a URL", "::"},
	} {
		t.Run(tc.name+" is refused", func(t *testing.T) {
			_, err := c.Get(context.Background(), tc.url, htmlLimits)
			if !errors.Is(err, ErrRefused) {
				t.Errorf("err = %v, want ErrRefused", err)
			}
		})
	}
}

func testClient(t *testing.T, srv *httptest.Server) Client {
	t.Helper()
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
	return New(Config{
		UserAgent: "test",
		Ports:     []uint16{443, uint16(port)},
		Allowed:   []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		RootCAs:   roots,
	})
}

func TestGetFromAllowedServer(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			if r.Header.Get("User-Agent") != "test" || r.Header.Get("Cookie") != "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<title>ok</title>"))
		case "/to-page":
			http.Redirect(w, r, "/page", http.StatusFound)
		case "/to-private":
			http.Redirect(w, r, "https://10.0.0.1/", http.StatusFound)
		case "/to-http":
			http.Redirect(w, r, strings.Replace(srv.URL, "https", "http", 1)+"/page", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/big":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(make([]byte, 512<<10+1))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := testClient(t, srv)
	ctx := context.Background()

	t.Run("an HTML page after one redirect is read", func(t *testing.T) {
		resp, err := c.Get(ctx, srv.URL+"/to-page", htmlLimits)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if string(resp.Body) != "<title>ok</title>" || resp.ContentType != "text/html" || resp.URL.Path != "/page" {
			t.Errorf("response = %+v", resp)
		}
	})

	for _, tc := range []struct {
		name string
		path string
		want error
	}{
		{"a redirect to a private address is refused", "/to-private", ErrRefused},
		{"a redirect to plain http is refused", "/to-http", ErrRefused},
		{"more than 3 redirects are refused", "/loop", ErrRefused},
		{"a page over the size limit is refused", "/big", ErrTooLarge},
		{"a non-HTML response is refused", "/json", ErrUnsupportedType},
		{"a missing page is an unexpected status", "/missing", ErrStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.Get(ctx, srv.URL+tc.path, htmlLimits); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
