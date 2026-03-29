package ingest

import (
	"testing"
)

func TestDetectGrokFormat(t *testing.T) {
	data := []byte(`[{"conversation_id":"abc123","title":"Test","create_time":"2024-01-01","turns":[{"author":{"role":"user"},"content":{"content_type":"text","parts":["hello"]}}]}]`)
	got := Detect(data)
	if got != FormatGrok {
		t.Errorf("Detect() = %q, want %q", got, FormatGrok)
	}
}

func TestDetectChatGPTFormat(t *testing.T) {
	data := []byte(`[{"id":"abc","title":"Test","mapping":{"node1":{"id":"node1","parent":"","children":["node2"],"message":{"id":"msg1","author":{"role":"user"},"content":{"content_type":"text","parts":["hello"]}}},"node2":{"id":"node2","parent":"node1","children":[],"message":{"id":"msg2","author":{"role":"assistant"},"content":{"content_type":"text","parts":["hi there"]}}}}}]`)
	got := Detect(data)
	if got != FormatChatGPT {
		t.Errorf("Detect() = %q, want %q", got, FormatChatGPT)
	}
}

func TestDetectPlainText(t *testing.T) {
	data := []byte("Human: How are you?\nAssistant: I'm good, thanks!")
	got := Detect(data)
	if got != FormatPlainText {
		t.Errorf("Detect() = %q, want %q", got, FormatPlainText)
	}
}

func TestDetectPlainTextUser(t *testing.T) {
	data := []byte("User: How are you?\nAssistant: I'm good, thanks!")
	got := Detect(data)
	if got != FormatPlainText {
		t.Errorf("Detect() = %q, want %q", got, FormatPlainText)
	}
}

func TestDetectMarkdown(t *testing.T) {
	data := []byte("# My Notes\n\nSome content here")
	got := Detect(data)
	if got != FormatMarkdown {
		t.Errorf("Detect() = %q, want %q", got, FormatMarkdown)
	}
}

func TestDetectUnknown(t *testing.T) {
	data := []byte("just some random text without markers")
	got := Detect(data)
	if got != FormatUnknown {
		t.Errorf("Detect() = %q, want %q", got, FormatUnknown)
	}
}

func TestDetectEmpty(t *testing.T) {
	got := Detect([]byte(""))
	if got != FormatUnknown {
		t.Errorf("Detect() = %q, want %q", got, FormatUnknown)
	}
}

func TestParseGrok(t *testing.T) {
	data := []byte(`[{
		"conversation_id": "abc123",
		"title": "Test Convo",
		"create_time": "2024-01-01",
		"turns": [
			{"author": {"role": "user"}, "content": {"content_type": "text", "parts": ["we decided to use gRPC"]}},
			{"author": {"role": "model"}, "content": {"content_type": "text", "parts": ["Good choice!"]}}
		]
	}]`)

	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if conv.Format != FormatGrok {
		t.Errorf("Format = %q, want %q", conv.Format, FormatGrok)
	}
	if conv.Source != "grok" {
		t.Errorf("Source = %q, want %q", conv.Source, "grok")
	}
	if conv.Title != "Test Convo" {
		t.Errorf("Title = %q, want %q", conv.Title, "Test Convo")
	}
	if len(conv.Turns) != 2 {
		t.Fatalf("len(Turns) = %d, want 2", len(conv.Turns))
	}
	if conv.Turns[0].Role != "user" {
		t.Errorf("Turn[0].Role = %q, want %q", conv.Turns[0].Role, "user")
	}
	if conv.Turns[0].Content != "we decided to use gRPC" {
		t.Errorf("Turn[0].Content = %q, want %q", conv.Turns[0].Content, "we decided to use gRPC")
	}
	if conv.Turns[1].Role != "assistant" {
		t.Errorf("Turn[1].Role = %q, want %q", conv.Turns[1].Role, "assistant")
	}
}

func TestParseChatGPT(t *testing.T) {
	data := []byte(`[{
		"id": "conv1",
		"title": "ChatGPT Test",
		"mapping": {
			"root": {
				"id": "root",
				"parent": "",
				"children": ["msg1"],
				"message": null
			},
			"msg1": {
				"id": "msg1",
				"parent": "root",
				"children": ["msg2"],
				"message": {
					"id": "m1",
					"author": {"role": "user"},
					"content": {"content_type": "text", "parts": ["turns out the port was wrong"]}
				}
			},
			"msg2": {
				"id": "msg2",
				"parent": "msg1",
				"children": [],
				"message": {
					"id": "m2",
					"author": {"role": "assistant"},
					"content": {"content_type": "text", "parts": ["I see, let me help fix that"]}
				}
			}
		}
	}]`)

	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if conv.Format != FormatChatGPT {
		t.Errorf("Format = %q, want %q", conv.Format, FormatChatGPT)
	}
	if conv.Title != "ChatGPT Test" {
		t.Errorf("Title = %q, want %q", conv.Title, "ChatGPT Test")
	}
	if len(conv.Turns) != 2 {
		t.Fatalf("len(Turns) = %d, want 2", len(conv.Turns))
	}
	if conv.Turns[0].Content != "turns out the port was wrong" {
		t.Errorf("Turn[0].Content = %q", conv.Turns[0].Content)
	}
}

func TestParsePlainText(t *testing.T) {
	data := []byte("Human: How are you?\nAssistant: I'm doing well.\nHuman: Great!")
	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if conv.Format != FormatPlainText {
		t.Errorf("Format = %q, want %q", conv.Format, FormatPlainText)
	}
	if len(conv.Turns) != 3 {
		t.Fatalf("len(Turns) = %d, want 3", len(conv.Turns))
	}
	if conv.Turns[0].Role != "user" {
		t.Errorf("Turn[0].Role = %q, want %q", conv.Turns[0].Role, "user")
	}
	if conv.Turns[1].Role != "assistant" {
		t.Errorf("Turn[1].Role = %q, want %q", conv.Turns[1].Role, "assistant")
	}
}

func TestExtractMemoriesDecision(t *testing.T) {
	conv := &ParsedConversation{
		Format: FormatGrok,
		Source: "grok",
		Turns: []Turn{
			{Role: "user", Content: "we decided to use gRPC for the API"},
		},
	}

	memories := ExtractMemories(conv)
	if len(memories) == 0 {
		t.Fatal("expected at least one extracted memory")
	}

	found := false
	for _, m := range memories {
		if m.Type == "decision" {
			found = true
			if m.Speaker != "user" {
				t.Errorf("Speaker = %q, want %q", m.Speaker, "user")
			}
			if m.Source != "grok" {
				t.Errorf("Source = %q, want %q", m.Source, "grok")
			}
			break
		}
	}
	if !found {
		t.Error("expected a 'decision' type memory for 'we decided to use gRPC'")
	}
}

func TestExtractMemoriesLesson(t *testing.T) {
	conv := &ParsedConversation{
		Format: FormatChatGPT,
		Source: "chatgpt",
		Turns: []Turn{
			{Role: "user", Content: "context"},
			{Role: "assistant", Content: "context"},
			{Role: "user", Content: "context"},
			{Role: "assistant", Content: "turns out the port was wrong and that caused the connection failures"},
		},
	}

	memories := ExtractMemories(conv)
	found := false
	for _, m := range memories {
		if m.Type == "lesson" {
			found = true
			if m.Speaker != "chatgpt" {
				t.Errorf("Speaker = %q, want %q", m.Speaker, "chatgpt")
			}
			break
		}
	}
	if !found {
		t.Error("expected a 'lesson' type memory for 'turns out the port was wrong'")
	}
}

func TestExtractMemoriesPreference(t *testing.T) {
	conv := &ParsedConversation{
		Format: FormatPlainText,
		Source: "plaintext",
		Turns: []Turn{
			{Role: "user", Content: "context"},
			{Role: "user", Content: "context"},
			{Role: "user", Content: "context"},
			{Role: "user", Content: "I prefer using Go over Python for CLI tools"},
		},
	}

	memories := ExtractMemories(conv)
	found := false
	for _, m := range memories {
		if m.Type == "preference" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a 'preference' type memory for 'I prefer using Go'")
	}
}

func TestExtractMemoriesProjectContext(t *testing.T) {
	conv := &ParsedConversation{
		Format: FormatPlainText,
		Source: "plaintext",
		Turns: []Turn{
			{Role: "user", Content: "Help me set up a new project"},
			{Role: "assistant", Content: "Sure, what kind of project?"},
		},
	}

	memories := ExtractMemories(conv)
	contextCount := 0
	for _, m := range memories {
		if m.Type == "project_context" {
			contextCount++
		}
	}
	if contextCount != 2 {
		t.Errorf("expected 2 project_context memories for first 2 turns, got %d", contextCount)
	}
}

func TestExtractMemoriesEmpty(t *testing.T) {
	memories := ExtractMemories(nil)
	if memories != nil {
		t.Errorf("expected nil for nil conversation, got %d memories", len(memories))
	}

	memories = ExtractMemories(&ParsedConversation{})
	if memories != nil {
		t.Errorf("expected nil for empty conversation, got %d memories", len(memories))
	}
}

func TestParseMalformedJSON(t *testing.T) {
	_, err := Parse([]byte(`{bad json`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseInputTooLarge(t *testing.T) {
	data := make([]byte, MaxInputSize+1)
	for i := range data {
		data[i] = 'a'
	}
	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for oversized input")
	}
}
