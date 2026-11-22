package pmaxclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

// Config configures a ServerClient. HTTPClient is optional (tests inject the
// httptest TLS client); when nil a client honoring InsecureSkipVerify is built.
type Config struct {
	Name               string
	BaseURL            string // e.g. https://unisphere01.example.com:8443
	Username           string
	Password           string
	APIVersion         string // versioned-namespace prefix, e.g. "105"; empty = auto-detect
	InsecureSkipVerify bool
	HTTPClient         *http.Client
	// Trace logs every API response body (method, URL, status, body) for
	// validating payload shapes against a live Unisphere. Headers are never
	// logged, and Unisphere Basic auth lives only in request headers — no
	// response body carries credentials — so nothing can leak. Verbose —
	// debugging only.
	Trace bool
}

// ServerClient is the live per-instance Unisphere REST client. Auth is HTTP
// Basic on every request; Unisphere has no login/token endpoint.
type ServerClient struct {
	cfg Config
	rc  *resty.Client

	ctx    context.Context // cancelled by Close; parents the version detection
	cancel context.CancelFunc

	mu       sync.Mutex
	inflight chan struct{} // non-nil while one detection runs; closed when it publishes
	detected string        // cached auto-detected APIVersion; empty until a detection succeeds
	retryAt  time.Time
	now      func() time.Time // test seam
}

// NewServerClient builds a client.
func NewServerClient(cfg Config) *ServerClient {
	rc := resty.New().SetBaseURL(cfg.BaseURL)
	if cfg.HTTPClient != nil {
		rc.SetTransport(cfg.HTTPClient.Transport)
	} else {
		rc.SetTLSClientConfig(&tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.InsecureSkipVerify, // operator opt-in for lab/self-signed Unisphere
		})
	}
	rc.SetBasicAuth(cfg.Username, cfg.Password)
	rc.SetHeader("Accept", "application/json")
	// Retry on 5xx only — never retry 4xx (auth/permission failures must not loop).
	rc.SetRetryCount(2).AddRetryCondition(func(r *resty.Response, _ error) bool {
		return r.StatusCode() >= 500
	})
	if cfg.Trace {
		// Deliberately not resty's SetDebug: that dumps request headers, which
		// would leak the Basic Authorization header. This logs only
		// method/URL/status and the response body.
		rc.OnAfterResponse(func(_ *resty.Client, r *resty.Response) error {
			log.WithFields(log.Fields{
				"server": cfg.Name,
				"method": r.Request.Method,
				"url":    r.Request.URL,
				"status": r.StatusCode(),
			}).Infof("API trace:\n%s", r.Body())
			return nil
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ServerClient{cfg: cfg, rc: rc, now: time.Now, ctx: ctx, cancel: cancel}
}

func (c *ServerClient) Name() string { return c.cfg.Name }

// fallbackAPIVersion is used only while auto-detection fails; every
// Unisphere 10.x up to 10.2 serves it (ADR-0015).
const fallbackAPIVersion = "100"

// detectRetryAfter bounds how often a failing detection (up to 30 s) is retried.
const detectRetryAfter = time.Minute

// APIVersion returns the configured versioned-namespace prefix or, when none is
// configured, the server's own api_version from GET /univmax/restapi/version.
// Dell documents N-2 REST version support (10.0–10.2), so a fixed default is
// expected to go stale across upgrades. A detection is cached on success; a failure falls back to "100"
// and is retried after detectRetryAfter, with no network call meanwhile.
func (c *ServerClient) APIVersion() string {
	if c.cfg.APIVersion != "" {
		return c.cfg.APIVersion
	}
	c.mu.Lock()
	if c.detected != "" {
		defer c.mu.Unlock()
		return c.detected
	}
	if c.inflight != nil { // share the running attempt, no second request
		done := c.inflight
		c.mu.Unlock()
		<-done
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.detected != "" {
			return c.detected
		}
		return fallbackAPIVersion
	}
	if c.now().Before(c.retryAt) {
		defer c.mu.Unlock()
		return fallbackAPIVersion
	}
	done := make(chan struct{})
	c.inflight = done
	c.mu.Unlock()

	v, err := c.detectVersion()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight = nil
	close(done)
	if err != nil {
		c.retryAt = c.now().Add(detectRetryAfter)
		log.WithField("server", c.cfg.Name).WithError(err).
			Warnf("apiVersion auto-detect failed; using %s, retrying in %s", fallbackAPIVersion, detectRetryAfter)
		return fallbackAPIVersion
	}
	log.WithFields(log.Fields{"server": c.cfg.Name, "apiVersion": v}).Info("apiVersion auto-detected")
	c.detected = v
	return v
}

// detectVersion does the I/O, outside mu; Close cancels c.ctx and aborts it.
func (c *ServerClient) detectVersion() (string, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	var v struct {
		APIVersion string `json:"api_version"`
	}
	err := c.Get(ctx, "/univmax/restapi/version", &v)
	if err == nil && !isDigits(v.APIVersion) {
		err = fmt.Errorf("unusable api_version %q", v.APIVersion)
	}
	return v.APIVersion, err
}

// isDigits guards the value spliced into request paths.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Get fetches path and decodes the JSON response into out. ForceContentType
// makes resty unmarshal even if a proxy mislabels the response content type.
func (c *ServerClient) Get(ctx context.Context, path string, out any) error {
	resp, err := c.rc.R().SetContext(ctx).
		ForceContentType("application/json").
		SetResult(out).Get(path)
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 300 {
		return &StatusError{Method: "GET", Path: path, Code: resp.StatusCode()}
	}
	return nil
}

// Post sends body as JSON to path and decodes the response into out.
func (c *ServerClient) Post(ctx context.Context, path string, body, out any) error {
	resp, err := c.rc.R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		ForceContentType("application/json").
		SetBody(body).
		SetResult(out).
		Post(path)
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 300 {
		return &StatusError{Method: "POST", Path: path, Code: resp.StatusCode()}
	}
	return nil
}

// Close aborts any in-flight version detection and releases idle connections.
// Basic auth has no server-side session to end.
func (c *ServerClient) Close() error {
	c.cancel()
	c.rc.GetClient().CloseIdleConnections()
	return nil
}

// StatusError is a non-2xx Unisphere response. Its text is the historical
// "GET /p: status 404" form so log lines and assertions stay stable.
type StatusError struct {
	Method, Path string
	Code         int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s %s: status %d", e.Method, e.Path, e.Code)
}

// IsNotFound reports whether err wraps a 404 StatusError.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}
