package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// mockEmbedder implements embeddings.Provider for tests.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	if len(text) > 0 {
		emb[0] = float32(len(text)) / 100.0
	}
	return emb, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := m.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int { return 384 }

func newTestMux(t *testing.T) http.Handler {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, client.TursoClient, &mockEmbedder{}, logger)
	return &autoAuthMux{inner: mux}
}

func TestRegisterRoutes(t *testing.T) {
	// Should not panic
	mux := newTestMux(t)
	if mux == nil {
		t.Fatal("expected non-nil mux")
	}
}

func TestWebAPIMemories(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/memories", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/memories status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWebAPIStats(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/stats status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWebAPIGraph(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/graph", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/graph status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWebListPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("expected Content-Type header")
	}
}

func TestWebSearchPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /search status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebStatsPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /stats status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebGraphPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/graph", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /graph status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebPatternsPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/patterns", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /patterns status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebNewPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/new", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /new status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebConversationsPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/conversations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /conversations status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebIngestPage(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/ingest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /ingest status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWebAPISearch(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/search status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWebAPISearchMissingQuery(t *testing.T) {
	mux := newTestMux(t)

	// Without Accept: application/json, empty query returns 200 with partial HTML
	// With Accept: application/json, it returns 200 with "[]"
	req := httptest.NewRequest("GET", "/api/search", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/search (no q, json) status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "[]" {
		t.Errorf("expected [], got %s", body)
	}
}

func TestWebAPIDeleteMemory(t *testing.T) {
	mux := newTestMux(t)

	// DeleteMemory always returns 200 even for nonexistent (no error from SQL DELETE)
	req := httptest.NewRequest("DELETE", "/api/memories/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("DELETE /api/memories/nonexistent status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- Template helpers ----------

func TestTruncate(t *testing.T) {
	// truncate replaces newlines with spaces, then truncates with "..."
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"short", 10, "short"},
		{"first line\nsecond line", 100, "first line second line"},
		{"a very long single line that exceeds the limit", 10, "a very lon..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	// Should not panic on various inputs
	tests := []string{
		"2026-03-29 12:00:00",
		"2026-03-29T12:00:00Z",
		"invalid",
		"",
	}
	for _, input := range tests {
		got := formatDate(input)
		if got == "" && input != "" && input != "invalid" {
			t.Errorf("formatDate(%q) returned empty", input)
		}
	}
}

func TestIsTopicTag(t *testing.T) {
	if !isTopicTag("topic:infrastructure") {
		t.Error("expected true for topic:infrastructure")
	}
	if isTopicTag("channel:discord") {
		t.Error("expected false for channel:discord")
	}
}

func TestReadOnlyModeBlocksWrites(t *testing.T) {
	// When MAGI_API_TOKEN is empty, write methods should be blocked.
	t.Setenv("MAGI_API_TOKEN", "")
	mux := newTestMux(t)

	// GET should work
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/memories", nil))
	if rr.Code == http.StatusForbidden {
		t.Errorf("GET should be allowed in read-only mode, got %d", rr.Code)
	}

	// POST should be blocked
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/api/memories", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("POST should be blocked in read-only mode, got %d", rr.Code)
	}

	// DELETE should be blocked
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("DELETE", "/api/memories/abc", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("DELETE should be blocked in read-only mode, got %d", rr.Code)
	}
}

func TestProxyAuthTrustsAllRoutes(t *testing.T) {
	// When proxy auth is enabled, /api/* routes should also be trusted.
	t.Setenv("MAGI_API_TOKEN", "test-secret")
	t.Setenv("MAGI_TRUSTED_PROXY_AUTH", "true")
	t.Setenv("MAGI_TRUSTED_PROXY_IPS", "127.0.0.1")

	// Use raw mux (no auto-auth) to test actual auth behavior.
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, client.TursoClient, &mockEmbedder{}, logger)

	// Request with proxy header from trusted IP should reach /api/* routes
	req := httptest.NewRequest("GET", "/api/memories", nil)
	req.Header.Set("X-Authentik-Username", "testuser")
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Errorf("proxy-authed request to /api/* should be allowed, got %d", rr.Code)
	}

	// Request without proxy header should require bearer token
	req2 := httptest.NewRequest("GET", "/api/memories", nil)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request should be 401, got %d", rr2.Code)
	}
}

func TestProxyAuthDefaultsCIDRToLoopback(t *testing.T) {
	// When MAGI_TRUSTED_PROXY_IPS is empty, should default to loopback.
	t.Setenv("MAGI_API_TOKEN", "test-secret")
	t.Setenv("MAGI_TRUSTED_PROXY_AUTH", "true")
	t.Setenv("MAGI_TRUSTED_PROXY_IPS", "")

	// Use raw mux (no auto-auth) to test actual auth behavior.
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, client.TursoClient, &mockEmbedder{}, logger)

	// Request from non-loopback IP with proxy header should be rejected
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Authentik-Username", "attacker")
	req.RemoteAddr = "10.5.5.99:12345"
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("non-loopback proxy request should be rejected, got %d", rr.Code)
	}

	// Request from loopback should be trusted
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Authentik-Username", "testuser")
	req2.RemoteAddr = "127.0.0.1:12345"
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code == http.StatusUnauthorized {
		t.Errorf("loopback proxy request should be trusted, got %d", rr2.Code)
	}
}
