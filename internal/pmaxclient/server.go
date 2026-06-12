package pmaxclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

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
	APIVersion         string // versioned-namespace prefix, e.g. "100"
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
	return &ServerClient{cfg: cfg, rc: rc}
}

func (c *ServerClient) Name() string { return c.cfg.Name }

// APIVersion returns the configured versioned-namespace prefix (default "100").
func (c *ServerClient) APIVersion() string {
	if c.cfg.APIVersion == "" {
		return "100"
	}
	return c.cfg.APIVersion
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
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode())
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
		return fmt.Errorf("POST %s: status %d", path, resp.StatusCode())
	}
	return nil
}

// Close releases idle connections. Basic auth has no server-side session to end.
func (c *ServerClient) Close() error {
	c.rc.GetClient().CloseIdleConnections()
	return nil
}
