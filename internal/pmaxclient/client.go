// Package pmaxclient is the per-instance Unisphere for PowerMax REST API client.
package pmaxclient

import "context"

// Client is the per-Unisphere API client abstraction, satisfied by the live
// ServerClient and by Mock (tests). Unisphere authenticates every request with
// HTTP Basic over TLS — there is no session/token endpoint.
type Client interface {
	// Name returns the configured instance name (used as the `server` label).
	Name() string
	// APIVersion returns the Unisphere REST version prefix (e.g. "100") used for
	// versioned namespaces like /system and /sloprovisioning. The /performance
	// namespace is unversioned.
	APIVersion() string
	// Get fetches an absolute API path (e.g. "/univmax/restapi/version") and
	// JSON-decodes the body into out.
	Get(ctx context.Context, path string, out any) error
	// Post sends body as JSON to an absolute API path and JSON-decodes the
	// response into out. The performance namespace is query-by-POST.
	Post(ctx context.Context, path string, body, out any) error
	// Close releases HTTP resources.
	Close() error
}
