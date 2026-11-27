// Package safehttp fetches URLs that students supply, refusing private,
// loopback and other special-purpose addresses at connect time.
package safehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"time"

	"code.dny.dev/ssrf"
)

const (
	maxRedirects      = 3
	dialTimeout       = 3 * time.Second
	headerTimeout     = 3 * time.Second
	maxResponseHeader = 64 << 10
	defaultPort       = 443
)

var (
	ErrRefused         = errors.New("destination refused")
	ErrTooLarge        = errors.New("response too large")
	ErrUnsupportedType = errors.New("unsupported content type")
	ErrStatus          = errors.New("unexpected status")
)

type Config struct {
	UserAgent string
	Timeout   time.Duration
	Ports     []uint16
	Allowed   []netip.Prefix
	RootCAs   *x509.CertPool
}

type Client struct {
	http      *http.Client
	userAgent string
}

type Limits struct {
	Timeout      time.Duration
	MaxBytes     int64
	ContentTypes []string
}

type Response struct {
	URL         *url.URL
	ContentType string
	Body        []byte
}

func New(cfg Config) Client {
	ports := cfg.Ports
	if len(ports) == 0 {
		ports = []uint16{defaultPort}
	}
	guard := ssrf.New(ssrf.WithPorts(ports...), ssrf.WithAllowedV4Prefixes(cfg.Allowed...))
	dialer := &net.Dialer{Timeout: dialTimeout, Control: guard.Safe}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		TLSClientConfig:        &tls.Config{RootCAs: cfg.RootCAs, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    dialTimeout,
		ResponseHeaderTimeout:  headerTimeout,
		MaxResponseHeaderBytes: maxResponseHeader,
		ForceAttemptHTTP2:      true,
	}
	return Client{
		http: &http.Client{
			Transport:     transport,
			CheckRedirect: checkRedirect,
			Timeout:       cfg.Timeout,
		},
		userAgent: cfg.UserAgent,
	}
}

func (c Client) HTTP() *http.Client {
	return c.http
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return fmt.Errorf("%w: more than %d redirects", ErrRefused, maxRedirects)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("%w: redirect to %s", ErrRefused, req.URL.Scheme)
	}
	return nil
}

func (c Client) Get(ctx context.Context, rawURL string, l Limits) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("%w: only https URLs without credentials", ErrRefused)
	}

	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/*;q=0.8")

	resp, err := c.http.Do(req)
	if err != nil {
		if refused(err) {
			return nil, fmt.Errorf("%w: %w", ErrRefused, err)
		}
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrStatus, resp.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !slices.Contains(l.ContentTypes, contentType) {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, resp.Header.Get("Content-Type"))
	}
	if resp.ContentLength > l.MaxBytes {
		return nil, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, l.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > l.MaxBytes {
		return nil, ErrTooLarge
	}
	return &Response{URL: resp.Request.URL, ContentType: contentType, Body: body}, nil
}

func refused(err error) bool {
	return errors.Is(err, ErrRefused) ||
		errors.Is(err, ssrf.ErrProhibitedIP) ||
		errors.Is(err, ssrf.ErrProhibitedPort) ||
		errors.Is(err, ssrf.ErrProhibitedNetwork) ||
		errors.Is(err, ssrf.ErrInvalidHostPort)
}
