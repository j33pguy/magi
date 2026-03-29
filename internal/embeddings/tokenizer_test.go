package embeddings

import (
	"os"
	"path/filepath"
	"testing"
)

// createTestVocab writes a minimal BERT vocab file for testing.
func createTestVocab(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.txt")
	// Minimal vocab with special tokens and some words
	vocab := "[PAD]\n[UNK]\n[CLS]\n[SEP]\nhello\nworld\n##lo\n##rl\n##d\ntest\n"
	if err := os.WriteFile(path, []byte(vocab), 0644); err != nil {
		t.Fatalf("writing vocab: %v", err)
	}
	return path
}

func TestNewTokenizer(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}
	if tok.maxLen != 16 {
		t.Errorf("maxLen = %d, want 16", tok.maxLen)
	}
	if tok.clsID != 2 {
		t.Errorf("clsID = %d, want 2", tok.clsID)
	}
	if tok.sepID != 3 {
		t.Errorf("sepID = %d, want 3", tok.sepID)
	}
	if tok.padID != 0 {
		t.Errorf("padID = %d, want 0", tok.padID)
	}
	if tok.unkID != 1 {
		t.Errorf("unkID = %d, want 1", tok.unkID)
	}
}

func TestNewTokenizer_BadPath(t *testing.T) {
	_, err := NewTokenizer("/nonexistent/vocab.txt", 16)
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}

func TestTokenize_Basic(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}

	result := tok.Tokenize("hello world")
	if len(result.InputIDs) != 16 {
		t.Fatalf("InputIDs length = %d, want 16", len(result.InputIDs))
	}
	if len(result.AttentionMask) != 16 {
		t.Fatalf("AttentionMask length = %d, want 16", len(result.AttentionMask))
	}
	if len(result.TokenTypeIDs) != 16 {
		t.Fatalf("TokenTypeIDs length = %d, want 16", len(result.TokenTypeIDs))
	}

	// First token should be [CLS]
	if result.InputIDs[0] != int64(tok.clsID) {
		t.Errorf("first token = %d, want CLS=%d", result.InputIDs[0], tok.clsID)
	}
	// Attention mask: first tokens attended, rest padded
	if result.AttentionMask[0] != 1 {
		t.Error("first attention mask should be 1")
	}
	// Token type IDs should all be 0 (single sentence)
	for i, v := range result.TokenTypeIDs {
		if v != 0 {
			t.Errorf("TokenTypeIDs[%d] = %d, want 0", i, v)
		}
	}
}

func TestTokenize_Truncation(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 6)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}

	// "hello world test" has 3+ tokens, but maxLen=6 means max 4 content tokens
	result := tok.Tokenize("hello world test hello world test")
	if len(result.InputIDs) != 6 {
		t.Fatalf("InputIDs length = %d, want 6", len(result.InputIDs))
	}
	// First is CLS
	if result.InputIDs[0] != int64(tok.clsID) {
		t.Error("first should be CLS")
	}
}

func TestTokenize_UnknownWord(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}

	result := tok.Tokenize("xyz")
	// "xyz" is not in vocab, should produce UNK tokens
	// Check that result doesn't panic and has correct structure
	if result.InputIDs[0] != int64(tok.clsID) {
		t.Error("first should be CLS even with unknown words")
	}
}

func TestBasicTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello world", 2},
		{"hello, world!", 4}, // hello, comma, world, exclamation
		{"  spaces  ", 1},
		{"", 0},
		{"a.b", 3}, // a, dot, b
	}
	for _, tt := range tests {
		tokens := basicTokenize(tt.input)
		if len(tokens) != tt.want {
			t.Errorf("basicTokenize(%q) = %d tokens, want %d: %v", tt.input, len(tokens), tt.want, tokens)
		}
	}
}

func TestBasicTokenize_Punctuation(t *testing.T) {
	tokens := basicTokenize("hello, world!")
	if len(tokens) < 3 {
		t.Fatalf("expected >= 3 tokens, got %d", len(tokens))
	}
	// Punctuation should be separate tokens
	found := false
	for _, tok := range tokens {
		if tok == "," {
			found = true
			break
		}
	}
	if !found {
		t.Error("comma should be a separate token")
	}
}

func TestIsChinesePunct(t *testing.T) {
	if !isChinesePunct('中') {
		t.Error("Chinese character should be detected")
	}
	if isChinesePunct('a') {
		t.Error("ASCII should not be detected as Chinese")
	}
}

func TestLoadVocab(t *testing.T) {
	path := createTestVocab(t)
	vocab, err := loadVocab(path)
	if err != nil {
		t.Fatalf("loadVocab: %v", err)
	}
	if _, ok := vocab["[PAD]"]; !ok {
		t.Error("[PAD] not found in vocab")
	}
	if _, ok := vocab["hello"]; !ok {
		t.Error("hello not found in vocab")
	}
}

func TestLoadVocab_BadPath(t *testing.T) {
	_, err := loadVocab("/nonexistent/vocab.txt")
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}

func TestWordPiece(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}

	// "hello" should be found directly
	ids := tok.wordPiece("hello")
	if len(ids) != 1 {
		t.Errorf("wordPiece(hello) = %d tokens, want 1", len(ids))
	}
	if ids[0] != tok.vocab["hello"] {
		t.Errorf("wordPiece(hello) = %v, want [%d]", ids, tok.vocab["hello"])
	}
}

func TestWordPieceTokenize(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}

	// Should lowercase and split
	ids := tok.wordPieceTokenize("Hello World")
	if len(ids) < 2 {
		t.Fatalf("expected >= 2 token IDs, got %d", len(ids))
	}
}

func TestLookupOrPanic(t *testing.T) {
	path := createTestVocab(t)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}

	// Should panic for missing token
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing token")
		}
	}()
	tok.lookupOrPanic("[MISSING]")
}

func TestWordPiece_SubwordSplit(t *testing.T) {
	// Create a vocab that forces subword splitting
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.txt")
	// "un" + "##known" should be split
	content := "[PAD]\n[UNK]\n[CLS]\n[SEP]\nun\n##known\n"
	os.WriteFile(path, []byte(content), 0644)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}
	ids := tok.wordPiece("unknown")
	if len(ids) != 2 {
		t.Errorf("expected 2 subword tokens, got %d: %v", len(ids), ids)
	}
}

func TestWordPiece_CompletelyUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.txt")
	content := "[PAD]\n[UNK]\n[CLS]\n[SEP]\n"
	os.WriteFile(path, []byte(content), 0644)
	tok, err := NewTokenizer(path, 16)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}
	// "abc" — no subwords match, each char becomes UNK
	ids := tok.wordPiece("abc")
	for _, id := range ids {
		if id != tok.unkID {
			t.Errorf("expected UNK id=%d, got %d", tok.unkID, id)
		}
	}
}

func TestLoadVocab_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.txt")
	// Vocab with empty lines — they should increment the index
	content := "[PAD]\n[UNK]\n\n[CLS]\n[SEP]\n"
	os.WriteFile(path, []byte(content), 0644)

	vocab, err := loadVocab(path)
	if err != nil {
		t.Fatalf("loadVocab: %v", err)
	}
	// [PAD]=0, [UNK]=1, (empty)=2, [CLS]=3, [SEP]=4
	if vocab["[CLS]"] != 3 {
		t.Errorf("[CLS] = %d, want 3", vocab["[CLS]"])
	}
}
