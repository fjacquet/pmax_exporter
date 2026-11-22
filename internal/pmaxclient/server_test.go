package pmaxclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	log "github.com/sirupsen/logrus"
)

func newTestClient(t *testing.T, handler http.Handler, trace bool) *ServerClient {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	return NewServerClient(Config{
		Name: "uni-test", BaseURL: ts.URL, Username: "mon", Password: "s3cret",
		HTTPClient: ts.Client(), Trace: trace,
	})
}

func TestGetSendsBasicAuth(t *testing.T) {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("mon:s3cret"))
	var got string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"version":"V10.2.0.1"}`))
	}), false)
	defer func() { _ = c.Close() }()

	var out struct {
		Version string `json:"version"`
	}
	if err := c.Get(context.Background(), "/univmax/restapi/version", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if out.Version != "V10.2.0.1" {
		t.Fatalf("version = %q", out.Version)
	}
}

func TestRetriesOn5xxButNotOn4xx(t *testing.T) {
	var calls5xx atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flaky":
			if calls5xx.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		case "/forbidden":
			w.WriteHeader(http.StatusUnauthorized)
		}
	}), false)
	defer func() { _ = c.Close() }()

	var out map[string]any
	if err := c.Get(context.Background(), "/flaky", &out); err != nil {
		t.Fatalf("expected 5xx retry to succeed: %v", err)
	}
	if calls5xx.Load() != 2 {
		t.Fatalf("flaky calls = %d, want 2 (one retry)", calls5xx.Load())
	}

	if err := c.Get(context.Background(), "/forbidden", &out); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestPostSendsBodyAndDecodes(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		if !strings.Contains(buf.String(), `"symmetrixId":"000297900046"`) {
			t.Errorf("body = %s, want symmetrixId", buf.String())
		}
		_, _ = w.Write([]byte(`{"resultList":{"result":[{"HostIOs":42.0,"timestamp":1700000000000}]}}`))
	}), false)
	defer func() { _ = c.Close() }()

	var out struct {
		ResultList struct {
			Result []map[string]any `json:"result"`
		} `json:"resultList"`
	}
	err := c.Post(context.Background(), "/univmax/restapi/performance/Array/metrics",
		map[string]any{"symmetrixId": "000297900046"}, &out)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(out.ResultList.Result) != 1 || out.ResultList.Result[0]["HostIOs"] != 42.0 {
		t.Fatalf("decoded = %+v", out)
	}
}

func TestTraceNeverLogsCredentials(t *testing.T) {
	var buf bytes.Buffer
	orig := log.StandardLogger().Out
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"V10.2.0.1"}`))
	}), true)
	defer func() { _ = c.Close() }()

	var out map[string]any
	if err := c.Get(context.Background(), "/univmax/restapi/version", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	logged := buf.String()
	if !strings.Contains(logged, "V10.2.0.1") {
		t.Fatalf("trace should log the response body; got: %s", logged)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("mon:s3cret"))
	for _, secret := range []string{"s3cret", b64, "Authorization"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("trace leaked %q in: %s", secret, logged)
		}
	}
}

func TestAPIVersionAutoDetectsAndCaches(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/univmax/restapi/version" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(`{"version":"T10.5.0.1","api_version":"105","supported_api_versions":["105","104","103"]}`))
	}), false)
	defer func() { _ = c.Close() }()

	for range 3 {
		if got := c.APIVersion(); got != "105" {
			t.Fatalf("APIVersion = %q, want 105", got)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("version endpoint hit %d times, want 1 (cached)", n)
	}
}

func TestAPIVersionConfiguredSkipsDetection(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(ts.Close)
	c := NewServerClient(Config{BaseURL: ts.URL, APIVersion: "103", HTTPClient: ts.Client()})
	if got := c.APIVersion(); got != "103" {
		t.Fatalf("APIVersion = %q, want 103", got)
	}
	if hits.Load() != 0 {
		t.Fatal("configured apiVersion must not call the version endpoint")
	}
}

func TestAPIVersionDetectionFailureFallsBackAndRetries(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"api_version":"104"}`))
	}), false)
	defer func() { _ = c.Close() }()

	if got := c.APIVersion(); got != "100" {
		t.Fatalf("APIVersion on failure = %q, want fallback 100", got)
	}
	fail.Store(false)
	if got := c.APIVersion(); got != "104" {
		t.Fatalf("APIVersion after recovery = %q, want 104 (failure not cached)", got)
	}
}
