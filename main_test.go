package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjacquet/pmax_exporter/internal/pmax"
)

func TestLivezReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()

	staticOKHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	staticOKHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthReturns200WhenServerUnhealthy(t *testing.T) {
	store := pmax.NewSnapshotStore()
	store.Store(&pmax.Snapshot{
		BuiltAt: time.Now(),
		Servers: []*pmax.ServerSnapshot{
			{Server: "pmax-01", OK: false, Err: "login POST: status 401"},
		},
	})

	rec := httptest.NewRecorder()
	healthHandler(rec, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Servers []struct {
			Server string `json:"server"`
			OK     bool   `json:"ok"`
			Err    string `json:"err"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Servers) != 1 || body.Servers[0].OK {
		t.Fatalf("servers = %+v, want one server with ok=false", body.Servers)
	}
}

func TestHealthReturns200WhenNoServers(t *testing.T) {
	store := pmax.NewSnapshotStore()

	rec := httptest.NewRecorder()
	healthHandler(rec, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
