package vcs

import (
	"encoding/json"
	"fmt"

	"github.com/j33pguy/magi/internal/db"
)

// SerializableMemory is the JSON representation of a memory stored in git.
// Excludes the embedding vector (too large, not human-readable).
type SerializableMemory struct {
	ID         string   `json:"id"`
	Content    string   `json:"content"`
	Summary    string   `json:"summary,omitempty"`
	Project    string   `json:"project"`
	Type       string   `json:"type"`
	Visibility string   `json:"visibility"`
	Source     string   `json:"source,omitempty"`
	SourceFile string   `json:"sourceFile,omitempty"`
	ParentID   string   `json:"parentId,omitempty"`
	ChunkIndex int      `json:"chunkIndex,omitempty"`
	Speaker    string   `json:"speaker,omitempty"`
	Area       string   `json:"area,omitempty"`
	SubArea    string   `json:"subArea,omitempty"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
	ArchivedAt string   `json:"archivedAt,omitempty"`
	TokenCount int      `json:"tokenCount"`
	Tags       []string `json:"tags,omitempty"`
}

// SerializableLink represents outbound links from a memory, stored as JSON in git.
type SerializableLink struct {
	ID        string  `json:"id"`
	ToID      string  `json:"toId"`
	Relation  string  `json:"relation"`
	Weight    float64 `json:"weight"`
	Auto      bool    `json:"auto"`
	CreatedAt string  `json:"createdAt"`
}

// MemoryToJSON serializes a memory to indented JSON bytes.
func MemoryToJSON(m *db.Memory) ([]byte, error) {
	sm := SerializableMemory{
		ID:         m.ID,
		Content:    m.Content,
		Summary:    m.Summary,
		Project:    m.Project,
		Type:       m.Type,
		Visibility: m.Visibility,
		Source:     m.Source,
		SourceFile: m.SourceFile,
		ParentID:   m.ParentID,
		ChunkIndex: m.ChunkIndex,
		Speaker:    m.Speaker,
		Area:       m.Area,
		SubArea:    m.SubArea,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
		ArchivedAt: m.ArchivedAt,
		TokenCount: m.TokenCount,
		Tags:       m.Tags,
	}

	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling memory: %w", err)
	}
	return append(data, '\n'), nil
}

// JSONToMemory deserializes a memory from JSON bytes.
// The returned Memory has no Embedding — caller must regenerate it.
func JSONToMemory(data []byte) (*db.Memory, error) {
	var sm SerializableMemory
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("unmarshaling memory: %w", err)
	}

	return &db.Memory{
		ID:         sm.ID,
		Content:    sm.Content,
		Summary:    sm.Summary,
		Project:    sm.Project,
		Type:       sm.Type,
		Visibility: sm.Visibility,
		Source:     sm.Source,
		SourceFile: sm.SourceFile,
		ParentID:   sm.ParentID,
		ChunkIndex: sm.ChunkIndex,
		Speaker:    sm.Speaker,
		Area:       sm.Area,
		SubArea:    sm.SubArea,
		CreatedAt:  sm.CreatedAt,
		UpdatedAt:  sm.UpdatedAt,
		ArchivedAt: sm.ArchivedAt,
		TokenCount: sm.TokenCount,
		Tags:       sm.Tags,
	}, nil
}

// LinksToJSON serializes a slice of outbound links to indented JSON bytes.
func LinksToJSON(links []*db.MemoryLink) ([]byte, error) {
	sl := make([]SerializableLink, len(links))
	for i, l := range links {
		sl[i] = SerializableLink{
			ID:        l.ID,
			ToID:      l.ToID,
			Relation:  l.Relation,
			Weight:    l.Weight,
			Auto:      l.Auto,
			CreatedAt: l.CreatedAt,
		}
	}

	data, err := json.MarshalIndent(sl, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling links: %w", err)
	}
	return append(data, '\n'), nil
}
