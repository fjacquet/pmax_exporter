package pmaxclient

import (
	"context"
	"testing"
)

func TestMockServesGetAndPost(t *testing.T) {
	m := NewMock("uni01")
	m.SetJSON("/v", `{"version":"V10"}`)
	m.SetPostJSON("/p", `{"ok":true}`)

	var v struct {
		Version string `json:"version"`
	}
	if err := m.Get(context.Background(), "/v", &v); err != nil || v.Version != "V10" {
		t.Fatalf("Get = %v, v=%+v", err, v)
	}
	var p struct {
		OK bool `json:"ok"`
	}
	if err := m.Post(context.Background(), "/p", nil, &p); err != nil || !p.OK {
		t.Fatalf("Post = %v, p=%+v", err, p)
	}
	if err := m.Get(context.Background(), "/missing", &v); err == nil {
		t.Fatal("expected error for unregistered GET path")
	}
}

func TestMockPostFuncVariesByBody(t *testing.T) {
	m := NewMock("uni01")
	m.SetPostFunc(func(_ string, body any) (string, bool) {
		b, _ := body.(map[string]any)
		if b["directorId"] == "FA-1D" {
			return `{"hit":1}`, true
		}
		return "", false
	})
	m.SetPostJSON("/p", `{"hit":0}`)

	var out struct {
		Hit int `json:"hit"`
	}
	if err := m.Post(context.Background(), "/p", map[string]any{"directorId": "FA-1D"}, &out); err != nil || out.Hit != 1 {
		t.Fatalf("postFunc path: err=%v out=%+v", err, out)
	}
	if err := m.Post(context.Background(), "/p", map[string]any{"directorId": "FA-2D"}, &out); err != nil || out.Hit != 0 {
		t.Fatalf("static fallback path: err=%v out=%+v", err, out)
	}
}
