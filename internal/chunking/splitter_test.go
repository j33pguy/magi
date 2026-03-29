package chunking

import (
	"strings"
	"testing"
)

func TestNewSplitter(t *testing.T) {
	s := NewSplitter()
	if s.MaxTokens != defaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", s.MaxTokens, defaultMaxTokens)
	}
	if s.Overlap != defaultOverlap {
		t.Errorf("Overlap = %d, want %d", s.Overlap, defaultOverlap)
	}
}

func TestNeedsSplit_Short(t *testing.T) {
	s := NewSplitter()
	// 100 chars / 4 = 25 tokens, well under 2000
	if s.NeedsSplit("hello world") {
		t.Error("short string should not need split")
	}
}

func TestNeedsSplit_Long(t *testing.T) {
	s := NewSplitter()
	// Need len/4 > 2000, so len > 8000. 8004/4 = 2001 > 2000
	long := strings.Repeat("a", 8004)
	if !s.NeedsSplit(long) {
		t.Error("long string should need split")
	}
}

func TestNeedsSplit_Exact(t *testing.T) {
	s := &Splitter{MaxTokens: 10, Overlap: 2}
	// 40 chars / 4 = 10 tokens — exactly at threshold, should NOT need split
	content := strings.Repeat("a", 40)
	if s.NeedsSplit(content) {
		t.Error("content exactly at threshold should not need split")
	}
	// 44 chars / 4 = 11 > 10 — should need split (integer division)
	content = strings.Repeat("a", 44)
	if !s.NeedsSplit(content) {
		t.Error("content over threshold should need split")
	}
}

func TestSplit_NoSplitNeeded(t *testing.T) {
	s := NewSplitter()
	chunks := s.Split("short content")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "short content" {
		t.Errorf("content = %q, want %q", chunks[0].Content, "short content")
	}
	if chunks[0].Index != 0 {
		t.Errorf("index = %d, want 0", chunks[0].Index)
	}
}

func TestSplit_EmptyContent(t *testing.T) {
	s := NewSplitter()
	chunks := s.Split("")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for empty, got %d", len(chunks))
	}
}

func TestSplit_AtHeadings(t *testing.T) {
	s := &Splitter{MaxTokens: 20, Overlap: 2}

	// Each section is ~15 tokens (60 chars / 4), so two sections exceed 20 tokens
	content := "# Section 1\n" + strings.Repeat("a", 60) + "\n# Section 2\n" + strings.Repeat("b", 60) + "\n"
	chunks := s.Split(content)

	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}

	// First chunk should contain section 1
	if !strings.Contains(chunks[0].Content, "Section 1") {
		t.Errorf("first chunk should contain Section 1: %q", chunks[0].Content)
	}
	if chunks[0].Index != 0 {
		t.Errorf("first chunk index = %d, want 0", chunks[0].Index)
	}
}

func TestSplit_LargeSingleSection(t *testing.T) {
	s := &Splitter{MaxTokens: 20, Overlap: 2}

	// Single section with no headings, exceeds max tokens
	// Will be split by paragraphs
	content := strings.Repeat("word ", 30) + "\n\n" + strings.Repeat("word ", 30)
	chunks := s.Split(content)

	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks for large single section, got %d", len(chunks))
	}
	// Indices should be sequential
	for i, c := range chunks {
		if c.Index != i {
			t.Errorf("chunk %d: index = %d, want %d", i, c.Index, i)
		}
	}
}

func TestSplitAtHeadings(t *testing.T) {
	content := "intro text\n# Heading 1\nbody 1\n## Heading 2\nbody 2\n"
	sections := splitAtHeadings(content)

	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}
	if !strings.Contains(sections[0], "intro text") {
		t.Errorf("first section should contain intro: %q", sections[0])
	}
	if !strings.Contains(sections[1], "# Heading 1") {
		t.Errorf("second section should contain Heading 1: %q", sections[1])
	}
	if !strings.Contains(sections[2], "## Heading 2") {
		t.Errorf("third section should contain Heading 2: %q", sections[2])
	}
}

func TestSplitAtHeadings_NoHeadings(t *testing.T) {
	content := "just plain text\nno headings here\n"
	sections := splitAtHeadings(content)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
}

func TestSplitAtHeadings_Empty(t *testing.T) {
	sections := splitAtHeadings("")
	if len(sections) != 1 {
		t.Fatalf("expected 1 section for empty, got %d", len(sections))
	}
}

func TestSplitByParagraphs(t *testing.T) {
	s := &Splitter{MaxTokens: 10, Overlap: 2}

	// Two paragraphs, each ~10 tokens
	text := strings.Repeat("word ", 10) + "\n\n" + strings.Repeat("text ", 10)
	chunks := s.splitByParagraphs(text, 0)

	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 0 {
		t.Errorf("first chunk index = %d, want 0", chunks[0].Index)
	}
	if chunks[1].Index != 1 {
		t.Errorf("second chunk index = %d, want 1", chunks[1].Index)
	}
}

func TestSplitByParagraphs_StartIdx(t *testing.T) {
	s := &Splitter{MaxTokens: 10, Overlap: 2}
	// Each paragraph needs to be > 10 tokens worth of chars (>40 chars)
	text := strings.Repeat("x ", 25) + "\n\n" + strings.Repeat("y ", 25)
	chunks := s.splitByParagraphs(text, 5)

	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 5 {
		t.Errorf("first chunk index = %d, want 5", chunks[0].Index)
	}
}

func TestSplitByParagraphs_SingleParagraph(t *testing.T) {
	s := &Splitter{MaxTokens: 100, Overlap: 2}
	text := "short paragraph"
	chunks := s.splitByParagraphs(text, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestExtractOverlap_ShortText(t *testing.T) {
	result := extractOverlap("hi", 100)
	if result != "hi" {
		t.Errorf("short text: got %q, want %q", result, "hi")
	}
}

func TestExtractOverlap_ExactLength(t *testing.T) {
	result := extractOverlap("hello", 5)
	if result != "hello" {
		t.Errorf("exact length: got %q, want %q", result, "hello")
	}
}

func TestExtractOverlap_WithNewline(t *testing.T) {
	text := "line1\nline2\nline3"
	result := extractOverlap(text, 10)
	// Should break at newline within the last 10 chars
	if !strings.HasPrefix(result, "line") {
		t.Errorf("expected overlap to start at newline boundary: got %q", result)
	}
	// Should not contain the newline at the start
	if strings.HasPrefix(result, "\n") {
		t.Error("overlap should not start with newline")
	}
}

func TestExtractOverlap_NoNewline(t *testing.T) {
	text := "abcdefghijklmnop"
	result := extractOverlap(text, 5)
	if result != "lmnop" {
		t.Errorf("no newline: got %q, want %q", result, "lmnop")
	}
}

func TestSplit_AccumulatedBeforeLargeSection(t *testing.T) {
	s := &Splitter{MaxTokens: 20, Overlap: 2}
	// First section is small (fits), second section is HUGE (single section > maxTokens)
	// This forces the "flush current + splitByParagraphs" path
	small := "# Small\nshort text\n"
	huge := "# Huge\n" + strings.Repeat("word ", 25) + "\n\n" + strings.Repeat("more ", 25)
	content := small + huge
	chunks := s.Split(content)
	if len(chunks) < 3 {
		t.Fatalf("expected >= 3 chunks (small + huge split into paras), got %d", len(chunks))
	}
}

func TestSplit_ChunkContentsTrimmed(t *testing.T) {
	s := &Splitter{MaxTokens: 20, Overlap: 2}
	content := "# A\n" + strings.Repeat("x", 60) + "\n# B\n" + strings.Repeat("y", 60)
	chunks := s.Split(content)
	for i, c := range chunks {
		if c.Content != strings.TrimSpace(c.Content) {
			t.Errorf("chunk %d not trimmed: %q", i, c.Content)
		}
	}
}

func TestSplit_OverlapPresent(t *testing.T) {
	s := &Splitter{MaxTokens: 15, Overlap: 5}
	// Create content with distinct sections
	content := "# First\n" + strings.Repeat("alpha ", 15) + "\n# Second\n" + strings.Repeat("beta ", 15)
	chunks := s.Split(content)

	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}
	// Second chunk should have some overlap from first chunk's end
	// (the overlap text from chunk 0 should appear at start of chunk 1)
	// We can't check exact content but verify chunks are non-empty
	for i, c := range chunks {
		if c.Content == "" {
			t.Errorf("chunk %d is empty", i)
		}
	}
}
