package cache

import (
	"context"
	"testing"
)

type fakeProvider struct {
	embedCalls      int
	embedBatchCalls int
}

func (f *fakeProvider) Embed(_ context.Context, text string) ([]float32, error) {
	f.embedCalls++
	return []float32{float32(len(text)), 1}, nil
}

func (f *fakeProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	f.embedBatchCalls++
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = []float32{float32(len(text)), 2}
	}
	return out, nil
}

func (f *fakeProvider) Dimensions() int {
	return 2
}

func TestProviderCachesEmbedCalls(t *testing.T) {
	delegate := &fakeProvider{}
	provider := NewProvider(delegate, 10)

	first, err := provider.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	first[0] = 999

	second, err := provider.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed second: %v", err)
	}

	if delegate.embedCalls != 1 {
		t.Fatalf("expected 1 delegate call, got %d", delegate.embedCalls)
	}
	if second[0] != 5 {
		t.Fatalf("expected cached embedding to remain unchanged, got %v", second)
	}
}

func TestProviderCachesEmbedBatchCalls(t *testing.T) {
	delegate := &fakeProvider{}
	provider := NewProvider(delegate, 10)

	if _, err := provider.Embed(context.Background(), "alpha"); err != nil {
		t.Fatalf("warm single embed: %v", err)
	}

	out, err := provider.EmbedBatch(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}

	if delegate.embedCalls != 1 {
		t.Fatalf("expected single embed call count 1, got %d", delegate.embedCalls)
	}
	if delegate.embedBatchCalls != 1 {
		t.Fatalf("expected 1 batch delegate call, got %d", delegate.embedBatchCalls)
	}
	if len(out) != 2 || out[0][0] != 5 || out[1][0] != 4 {
		t.Fatalf("unexpected batch output: %#v", out)
	}
}
