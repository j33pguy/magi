package cache

import "github.com/j33pguy/magi/internal/db"

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneFloat32s(in []float32) []float32 {
	if in == nil {
		return nil
	}
	out := make([]float32, len(in))
	copy(out, in)
	return out
}

func cloneMemory(m *db.Memory) *db.Memory {
	if m == nil {
		return nil
	}
	cp := *m
	cp.Tags = cloneStrings(m.Tags)
	cp.Embedding = cloneFloat32s(m.Embedding)
	return &cp
}

func cloneMemories(in []*db.Memory) []*db.Memory {
	if in == nil {
		return nil
	}
	out := make([]*db.Memory, len(in))
	for i, m := range in {
		out[i] = cloneMemory(m)
	}
	return out
}

func cloneHybridResults(in []*db.HybridResult) []*db.HybridResult {
	if in == nil {
		return nil
	}
	out := make([]*db.HybridResult, len(in))
	for i, r := range in {
		if r == nil {
			continue
		}
		cp := *r
		cp.Memory = cloneMemory(r.Memory)
		out[i] = &cp
	}
	return out
}

func cloneVectorResults(in []*db.VectorResult) []*db.VectorResult {
	if in == nil {
		return nil
	}
	out := make([]*db.VectorResult, len(in))
	for i, r := range in {
		if r == nil {
			continue
		}
		cp := *r
		cp.Memory = cloneMemory(r.Memory)
		out[i] = &cp
	}
	return out
}
