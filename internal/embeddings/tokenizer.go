package embeddings

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Tokenizer implements BERT WordPiece tokenization for all-MiniLM-L6-v2.
type Tokenizer struct {
	vocab    map[string]int
	idToWord map[int]string
	maxLen   int
	clsID    int
	sepID    int
	padID    int
	unkID    int
}

// NewTokenizer loads a BERT vocab.txt file and creates a WordPiece tokenizer.
func NewTokenizer(vocabPath string, maxLen int) (*Tokenizer, error) {
	vocab, err := loadVocab(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("loading vocab: %w", err)
	}

	idToWord := make(map[int]string, len(vocab))
	for word, id := range vocab {
		idToWord[id] = word
	}

	t := &Tokenizer{
		vocab:    vocab,
		idToWord: idToWord,
		maxLen:   maxLen,
	}

	// Look up special token IDs
	t.clsID = t.lookupOrPanic("[CLS]")
	t.sepID = t.lookupOrPanic("[SEP]")
	t.padID = t.lookupOrPanic("[PAD]")
	t.unkID = t.lookupOrPanic("[UNK]")

	return t, nil
}

// TokenizeResult holds the output of tokenization.
type TokenizeResult struct {
	InputIDs      []int64
	AttentionMask []int64
	TokenTypeIDs  []int64
}

// Tokenize converts text into model-ready token IDs with padding/truncation.
func (t *Tokenizer) Tokenize(text string) *TokenizeResult {
	tokens := t.wordPieceTokenize(text)

	// Truncate to maxLen - 2 (room for [CLS] and [SEP])
	if len(tokens) > t.maxLen-2 {
		tokens = tokens[:t.maxLen-2]
	}

	// Build input_ids: [CLS] + tokens + [SEP] + [PAD]...
	inputIDs := make([]int64, t.maxLen)
	attentionMask := make([]int64, t.maxLen)
	tokenTypeIDs := make([]int64, t.maxLen)

	inputIDs[0] = int64(t.clsID)
	attentionMask[0] = 1
	for i, tok := range tokens {
		inputIDs[i+1] = int64(tok)
		attentionMask[i+1] = 1
	}
	inputIDs[len(tokens)+1] = int64(t.sepID)
	attentionMask[len(tokens)+1] = 1

	// Remaining positions stay 0 (pad)
	for i := len(tokens) + 2; i < t.maxLen; i++ {
		inputIDs[i] = int64(t.padID)
	}

	return &TokenizeResult{
		InputIDs:      inputIDs,
		AttentionMask: attentionMask,
		TokenTypeIDs:  tokenTypeIDs,
	}
}

// wordPieceTokenize performs basic tokenization + WordPiece subword splitting.
func (t *Tokenizer) wordPieceTokenize(text string) []int {
	// Lowercase
	text = strings.ToLower(text)

	// Split into word tokens
	words := basicTokenize(text)

	var ids []int
	for _, word := range words {
		subIDs := t.wordPiece(word)
		ids = append(ids, subIDs...)
	}
	return ids
}

// wordPiece splits a single word into WordPiece subword tokens.
func (t *Tokenizer) wordPiece(word string) []int {
	if _, ok := t.vocab[word]; ok {
		return []int{t.vocab[word]}
	}

	var ids []int
	start := 0
	for start < len(word) {
		end := len(word)
		found := false
		for end > start {
			substr := word[start:end]
			if start > 0 {
				substr = "##" + substr
			}
			if id, ok := t.vocab[substr]; ok {
				ids = append(ids, id)
				found = true
				start = end
				break
			}
			end--
		}
		if !found {
			ids = append(ids, t.unkID)
			start++
		}
	}
	return ids
}

// basicTokenize splits text on whitespace and punctuation, similar to BERT's BasicTokenizer.
func basicTokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		if unicode.IsPunct(r) || isChinesePunct(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func isChinesePunct(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF)
}

func loadVocab(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int)
	scanner := bufio.NewScanner(f)
	idx := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			idx++
			continue
		}
		vocab[line] = idx
		idx++
	}
	return vocab, scanner.Err()
}

func (t *Tokenizer) lookupOrPanic(token string) int {
	id, ok := t.vocab[token]
	if !ok {
		panic(fmt.Sprintf("required token %q not found in vocab", token))
	}
	return id
}
