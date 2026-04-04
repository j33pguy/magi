package api

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
)

func newBenchmarkServer(tb testing.TB) *Server {
	tb.Helper()
	tmp := tb.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := db.NewSQLiteClient(filepath.Join(tmp, "bench.db"), logger)
	if err != nil {
		tb.Fatalf("NewSQLiteClient: %v", err)
	}
	tb.Cleanup(func() { client.Close() })

	if err := client.Migrate(); err != nil {
		tb.Fatalf("Migrate: %v", err)
	}

	s := &Server{
		db:       client.TursoClient,
		tasks:    client.TursoClient,
		embedder: &mockEmbedder{},
		logger:   logger,
		auth:     &auth.Resolver{},
	}
	if machines, ok := any(client.TursoClient).(MachineRegistryStore); ok {
		s.machines = machines
	}
	if lookup, ok := any(client.TursoClient).(auth.MachineLookup); ok {
		s.auth.SetMachineLookup(lookup)
	}
	return s
}

func seedBenchmarkMemory(tb testing.TB, s *Server, content, project, memType string) {
	tb.Helper()
	emb, _ := s.embedder.Embed(context.Background(), content)
	if _, err := s.db.SaveMemory(&db.Memory{
		Content:    content,
		Embedding:  emb,
		Project:    project,
		Type:       memType,
		Visibility: "internal",
		Speaker:    "bench",
	}); err != nil {
		tb.Fatalf("seed memory: %v", err)
	}
}

func BenchmarkHandleRemember(b *testing.B) {
	s := newBenchmarkServer(b)
	body := `{"content":"deprecate /v2/users","type":"decision","speaker":"agent-a","project":"platform","tags":["api","migration"]}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.handleRemember(w, req)
	}
}

func BenchmarkHandleRecall(b *testing.B) {
	s := newBenchmarkServer(b)
	for i := 0; i < 200; i++ {
		seedBenchmarkMemory(b, s, "api migration details", "platform", "memory")
	}

	body := `{"query":"api migration","project":"platform","top_k":5}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/recall", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.handleRecall(w, req)
	}
}

func BenchmarkHandleSearch(b *testing.B) {
	s := newBenchmarkServer(b)
	for i := 0; i < 200; i++ {
		seedBenchmarkMemory(b, s, "api migration details", "platform", "memory")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/search?q=api+migration&project=platform&top_k=5", nil)
		w := httptest.NewRecorder()
		s.handleSearch(w, req)
	}
}
