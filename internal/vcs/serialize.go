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

// SerializableContext is the JSON representation of redesign-oriented memory metadata.
type SerializableContext struct {
	MemoryID   string `json:"memoryId"`
	Repository struct {
		Host                  string `json:"host,omitempty"`
		Owner                 string `json:"owner,omitempty"`
		Name                  string `json:"name,omitempty"`
		CanonicalName         string `json:"canonicalName,omitempty"`
		DisplayName           string `json:"displayName,omitempty"`
		DefaultBranch         string `json:"defaultBranch,omitempty"`
		IsFork                bool   `json:"isFork,omitempty"`
		UpstreamCanonicalName string `json:"upstreamCanonicalName,omitempty"`
	} `json:"repository,omitempty"`
	Scope struct {
		Owner       string `json:"owner,omitempty"`
		Team        string `json:"team,omitempty"`
		Workspace   string `json:"workspace,omitempty"`
		Machine     string `json:"machine,omitempty"`
		Agent       string `json:"agent,omitempty"`
		Environment string `json:"environment,omitempty"`
	} `json:"scope,omitempty"`
	Provenance struct {
		Transport     string `json:"transport,omitempty"`
		ImportedFrom  string `json:"importedFrom,omitempty"`
		HumanAuthored bool   `json:"humanAuthored,omitempty"`
		DurableAt     string `json:"durableAt,omitempty"`
	} `json:"provenance,omitempty"`
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

func JSONToLinks(fromID string, data []byte) ([]*db.MemoryLink, error) {
	var sl []SerializableLink
	if err := json.Unmarshal(data, &sl); err != nil {
		return nil, fmt.Errorf("unmarshaling links: %w", err)
	}
	links := make([]*db.MemoryLink, len(sl))
	for i, l := range sl {
		links[i] = &db.MemoryLink{ID: l.ID, FromID: fromID, ToID: l.ToID, Relation: l.Relation, Weight: l.Weight, Auto: l.Auto, CreatedAt: l.CreatedAt}
	}
	return links, nil
}

func ContextToJSON(record *db.MemoryContextRecord) ([]byte, error) {
	if record == nil {
		return json.MarshalIndent(nil, "", "  ")
	}
	var sc SerializableContext
	sc.MemoryID = record.MemoryID
	sc.Repository.Host = record.Repository.Host
	sc.Repository.Owner = record.Repository.Owner
	sc.Repository.Name = record.Repository.Name
	sc.Repository.CanonicalName = record.Repository.CanonicalName
	sc.Repository.DisplayName = record.Repository.DisplayName
	sc.Repository.DefaultBranch = record.Repository.DefaultBranch
	sc.Repository.IsFork = record.Repository.IsFork
	sc.Repository.UpstreamCanonicalName = record.Repository.UpstreamCanonicalName
	sc.Scope.Owner = record.ScopeOwner
	sc.Scope.Team = record.ScopeTeam
	sc.Scope.Workspace = record.ScopeWorkspace
	sc.Scope.Machine = record.ScopeMachine
	sc.Scope.Agent = record.ScopeAgent
	sc.Scope.Environment = record.ScopeEnvironment
	sc.Provenance.Transport = record.ProvenanceTransport
	sc.Provenance.ImportedFrom = record.ProvenanceImportedFrom
	sc.Provenance.HumanAuthored = record.ProvenanceHumanAuthored
	sc.Provenance.DurableAt = record.DurableAt
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling context: %w", err)
	}
	return append(data, '\n'), nil
}

func JSONToContext(data []byte) (*db.MemoryContextRecord, error) {
	var sc SerializableContext
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("unmarshaling context: %w", err)
	}
	return &db.MemoryContextRecord{
		MemoryID: sc.MemoryID,
		Repository: db.RepositoryRecord{
			Host:                  sc.Repository.Host,
			Owner:                 sc.Repository.Owner,
			Name:                  sc.Repository.Name,
			CanonicalName:         sc.Repository.CanonicalName,
			DisplayName:           sc.Repository.DisplayName,
			DefaultBranch:         sc.Repository.DefaultBranch,
			IsFork:                sc.Repository.IsFork,
			UpstreamCanonicalName: sc.Repository.UpstreamCanonicalName,
		},
		ScopeOwner:              sc.Scope.Owner,
		ScopeTeam:               sc.Scope.Team,
		ScopeWorkspace:          sc.Scope.Workspace,
		ScopeMachine:            sc.Scope.Machine,
		ScopeAgent:              sc.Scope.Agent,
		ScopeEnvironment:        sc.Scope.Environment,
		ProvenanceTransport:     sc.Provenance.Transport,
		ProvenanceImportedFrom:  sc.Provenance.ImportedFrom,
		ProvenanceHumanAuthored: sc.Provenance.HumanAuthored,
		DurableAt:               sc.Provenance.DurableAt,
	}, nil
}
