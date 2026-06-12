package pmaxclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// Mock is an in-memory Client that serves canned JSON bodies per path. Tests use
// it to drive collectors without a live Unisphere. POST responses are keyed by
// path only — the perf engine sends one POST per object to the same path, so
// tests that need per-object bodies key on path + a request field via SetPostFunc.
type Mock struct {
	name     string
	gets     map[string]string
	posts    map[string]string
	postFunc func(path string, body any) (string, bool)
}

// NewMock returns an empty Mock for the named Unisphere instance.
func NewMock(name string) *Mock {
	return &Mock{name: name, gets: map[string]string{}, posts: map[string]string{}}
}

// SetJSON registers a response body for an exact GET path.
func (m *Mock) SetJSON(path, body string) { m.gets[path] = body }

// SetPostJSON registers a response body for an exact POST path.
func (m *Mock) SetPostJSON(path, body string) { m.posts[path] = body }

// SetPostFunc registers a hook consulted before the static POST map, letting a
// test vary the response by request body (e.g. per directorId).
func (m *Mock) SetPostFunc(f func(path string, body any) (string, bool)) { m.postFunc = f }

func (m *Mock) Name() string       { return m.name }
func (m *Mock) APIVersion() string { return "100" }

func (m *Mock) Get(_ context.Context, path string, out any) error {
	body, ok := m.gets[path]
	if !ok {
		return fmt.Errorf("mock: no GET response registered for %s", path)
	}
	return json.Unmarshal([]byte(body), out)
}

func (m *Mock) Post(_ context.Context, path string, body, out any) error {
	if m.postFunc != nil {
		if resp, ok := m.postFunc(path, body); ok {
			return json.Unmarshal([]byte(resp), out)
		}
	}
	resp, ok := m.posts[path]
	if !ok {
		return fmt.Errorf("mock: no POST response registered for %s", path)
	}
	return json.Unmarshal([]byte(resp), out)
}

func (m *Mock) Close() error { return nil }
