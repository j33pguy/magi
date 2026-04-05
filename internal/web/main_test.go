package web

import (
	"net/http"
	"os"
	"testing"
)

const testToken = "test-token-for-web-tests"

func TestMain(m *testing.M) {
	// Set a known token so auth middleware is active (not read-only mode).
	// Test muxes inject the auth header automatically via autoAuthMux.
	os.Setenv("MAGI_API_TOKEN", testToken)
	os.Exit(m.Run())
}

// autoAuthMux wraps a ServeMux to auto-inject the test auth header on every
// request that doesn't already have an Authorization header set.
// This lets existing tests work without modification.
type autoAuthMux struct {
	inner *http.ServeMux
}

func (a *autoAuthMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		r.Header.Set("Authorization", "Bearer "+testToken)
	}
	a.inner.ServeHTTP(w, r)
}
