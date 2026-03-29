// Package chunking provides markdown-aware text splitting for large documents.
package chunking

import (
	"strings"
)

const (
	defaultMaxTokens = 2000
	defaultOverlap   = 200
	charsPerToken    = 4
)

// Splitter splits large documents into overlapping chunks at markdown heading boundaries.
type Splitter struct {
	MaxTokens int
	Overlap   int
}

// Chunk represents a piece of a split document.
type Chunk struct {
	Content string
	Index   int
}

// NewSplitter creates a Splitter with default settings.
func NewSplitter() *Splitter {
	return &Splitter{
		MaxTokens: defaultMaxTokens,
		Overlap:   defaultOverlap,
	}
}

// NeedsSplit returns true if the content exceeds the max token threshold.
func (s *Splitter) NeedsSplit(content string) bool {
	return len(content)/charsPerToken > s.MaxTokens
}

// Split divides content into chunks, preferring markdown heading boundaries.
// Returns a single chunk if the content is small enough.
func (s *Splitter) Split(content string) []Chunk {
	if !s.NeedsSplit(content) {
		return []Chunk{{Content: content, Index: 0}}
	}

	sections := splitAtHeadings(content)

	var chunks []Chunk
	var current strings.Builder
	idx := 0

	for _, section := range sections {
		sectionTokens := len(section) / charsPerToken
		currentTokens := current.Len() / charsPerToken

		// If adding this section would exceed max, flush current chunk
		if currentTokens > 0 && currentTokens+sectionTokens > s.MaxTokens {
			chunks = append(chunks, Chunk{
				Content: strings.TrimSpace(current.String()),
				Index:   idx,
			})
			idx++

			// Start next chunk with overlap from end of previous
			overlap := extractOverlap(current.String(), s.Overlap*charsPerToken)
			current.Reset()
			current.WriteString(overlap)
		}

		// If a single section exceeds max, split it by paragraphs
		if sectionTokens > s.MaxTokens {
			if current.Len() > 0 {
				chunks = append(chunks, Chunk{
					Content: strings.TrimSpace(current.String()),
					Index:   idx,
				})
				idx++
				current.Reset()
			}

			paraChunks := s.splitByParagraphs(section, idx)
			chunks = append(chunks, paraChunks...)
			idx += len(paraChunks)
			continue
		}

		current.WriteString(section)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		chunks = append(chunks, Chunk{
			Content: strings.TrimSpace(current.String()),
			Index:   idx,
		})
	}

	return chunks
}

// splitAtHeadings splits markdown content at heading boundaries (# ## ### etc).
func splitAtHeadings(content string) []string {
	lines := strings.Split(content, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		sections = append(sections, current.String())
	}

	return sections
}

// splitByParagraphs splits text at double-newline paragraph boundaries.
func (s *Splitter) splitByParagraphs(text string, startIdx int) []Chunk {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var current strings.Builder
	idx := startIdx

	for _, para := range paragraphs {
		paraTokens := len(para) / charsPerToken
		currentTokens := current.Len() / charsPerToken

		if currentTokens > 0 && currentTokens+paraTokens > s.MaxTokens {
			chunks = append(chunks, Chunk{
				Content: strings.TrimSpace(current.String()),
				Index:   idx,
			})
			idx++
			overlap := extractOverlap(current.String(), s.Overlap*charsPerToken)
			current.Reset()
			current.WriteString(overlap)
		}

		current.WriteString(para)
		current.WriteString("\n\n")
	}

	if current.Len() > 0 {
		chunks = append(chunks, Chunk{
			Content: strings.TrimSpace(current.String()),
			Index:   idx,
		})
	}

	return chunks
}

// extractOverlap returns the last n characters of text for chunk overlap.
func extractOverlap(text string, n int) string {
	if len(text) <= n {
		return text
	}
	// Try to break at a newline boundary
	overlap := text[len(text)-n:]
	if idx := strings.Index(overlap, "\n"); idx >= 0 {
		return overlap[idx+1:]
	}
	return overlap
}
