package platforms

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestVerifyFeishuSignature(t *testing.T) {
	// Compute a valid signature for testing.
	computeSig := func(timestamp, nonce, key string, body []byte) string {
		content := timestamp + nonce + key + string(body)
		hash := sha256.Sum256([]byte(content))
		return fmt.Sprintf("%x", hash)
	}

	body := []byte("{}")
	validSig := computeSig("1234567890", "abc", "key", body)

	tests := []struct {
		name       string
		timestamp  string
		nonce      string
		encryptKey string
		signature  string
		body       []byte
		want       bool
	}{
		{"no encrypt key", "", "", "", "", nil, true},
		{"no signature", "", "", "key", "", nil, true},
		{"valid signature", "1234567890", "abc", "key", validSig, body, true},
		{"invalid signature", "1234567890", "abc", "key", "0000000000000000000000000000000000000000000000000000000000000000", body, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyFeishuSignature(tt.timestamp, tt.nonce, tt.encryptKey, tt.signature, tt.body)
			if got != tt.want {
				t.Errorf("verifyFeishuSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildTextCard(t *testing.T) {
	card := BuildTextCard("Test Title", "**bold** text")

	if card.Header == nil {
		t.Fatal("expected header")
	}
	if card.Header.Title.Content != "Test Title" {
		t.Errorf("title = %q, want \"Test Title\"", card.Header.Title.Content)
	}
	if len(card.Elements) != 1 {
		t.Fatalf("elements = %d, want 1", len(card.Elements))
	}
	if card.Elements[0].Text.Content != "**bold** text" {
		t.Errorf("content = %q, want \"**bold** text\"", card.Elements[0].Text.Content)
	}
}

func TestBuildTextCard_NoTitle(t *testing.T) {
	card := BuildTextCard("", "some text")
	if card.Header != nil {
		t.Error("expected nil header for empty title")
	}
}

func TestBuildButtonCard(t *testing.T) {
	buttons := []FeishuCardAction{
		{Tag: "button", Text: &FeishuCardText{Tag: "plain_text", Content: "Click me"}, URL: "https://example.com"},
	}
	card := BuildButtonCard("Actions", "Choose:", buttons)

	if len(card.Elements) != 2 {
		t.Fatalf("elements = %d, want 2 (text + action)", len(card.Elements))
	}
	if card.Elements[1].Tag != "action" {
		t.Errorf("second element tag = %q, want \"action\"", card.Elements[1].Tag)
	}
	if len(card.Elements[1].Actions) != 1 {
		t.Errorf("actions = %d, want 1", len(card.Elements[1].Actions))
	}
}

func TestBuildSimplePost(t *testing.T) {
	tests := []struct {
		name string
		lang string
		zhCN bool
		enUS bool
	}{
		{"chinese", "zh_cn", true, false},
		{"english", "en_us", false, true},
		{"english alt", "en", false, true},
		{"default", "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			post := BuildSimplePost("Title", "Line 1\nLine 2", tt.lang)
			if (post.ZhCN != nil) != tt.zhCN {
				t.Errorf("ZhCN present = %v, want %v", post.ZhCN != nil, tt.zhCN)
			}
			if (post.EnUS != nil) != tt.enUS {
				t.Errorf("EnUS present = %v, want %v", post.EnUS != nil, tt.enUS)
			}
		})
	}
}

func TestFeishuCardToJSON(t *testing.T) {
	card := BuildTextCard("Test", "content")
	j := feishuCardToJSON(card)
	if j == "" || j == "{}" {
		t.Error("expected non-empty JSON")
	}
}
