package patterns

import (
	"fmt"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func buildBenchmarkMemories(n int) []*db.Memory {
	memories := make([]*db.Memory, 0, n)
	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		content := fmt.Sprintf("Built a new gRPC service in Go (%d)", i)
		if i%5 == 0 {
			content = fmt.Sprintf("Prefer Terraform for infra (%d)", i)
		}
		if i%7 == 0 {
			content = fmt.Sprintf("Avoid Python for core services (%d)", i)
		}
		if i%9 == 0 {
			content = fmt.Sprintf("We decided to ship the feature (%d)", i)
		}

		memories = append(memories, &db.Memory{
			ID:        fmt.Sprintf("m-%d", i),
			Content:   content,
			Speaker:   "user",
			Area:      []string{"platform", "infra", "security"}[i%3],
			CreatedAt: base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
		})
	}

	return memories
}

func BenchmarkAnalyze(b *testing.B) {
	memories := buildBenchmarkMemories(100)
	analyzer := &Analyzer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = analyzer.Analyze(memories)
	}
}

func BenchmarkAnalyze_Large(b *testing.B) {
	memories := buildBenchmarkMemories(1000)
	analyzer := &Analyzer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = analyzer.Analyze(memories)
	}
}
