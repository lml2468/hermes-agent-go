package platforms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// FeishuCardElement represents an element in a Feishu interactive card.
type FeishuCardElement struct {
	Tag     string             `json:"tag"`
	Content string             `json:"content,omitempty"`
	Text    *FeishuCardText    `json:"text,omitempty"`
	Actions []FeishuCardAction `json:"actions,omitempty"`
	URL     string             `json:"url,omitempty"`
}

// FeishuCardText is a text element within a card.
type FeishuCardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

// FeishuCardAction is a button or action in a card.
type FeishuCardAction struct {
	Tag   string          `json:"tag"`
	Text  *FeishuCardText `json:"text,omitempty"`
	URL   string          `json:"url,omitempty"`
	Type  string          `json:"type,omitempty"`
	Value map[string]any  `json:"value,omitempty"`
}

// FeishuCard represents a complete interactive card message.
type FeishuCard struct {
	Header   *FeishuCardHeader   `json:"header,omitempty"`
	Elements []FeishuCardElement `json:"elements"`
}

// FeishuCardHeader is the card title/header.
type FeishuCardHeader struct {
	Title    *FeishuCardText `json:"title"`
	Template string          `json:"template,omitempty"` // color: blue, green, red, etc.
}

// BuildTextCard builds a simple card with markdown content.
func BuildTextCard(title, markdown string) FeishuCard {
	card := FeishuCard{
		Elements: []FeishuCardElement{
			{
				Tag:  "div",
				Text: &FeishuCardText{Tag: "lark_md", Content: markdown},
			},
		},
	}
	if title != "" {
		card.Header = &FeishuCardHeader{
			Title:    &FeishuCardText{Tag: "plain_text", Content: title},
			Template: "blue",
		}
	}
	return card
}

// BuildButtonCard builds a card with text and action buttons.
func BuildButtonCard(title, text string, buttons []FeishuCardAction) FeishuCard {
	card := FeishuCard{
		Elements: []FeishuCardElement{
			{
				Tag:  "div",
				Text: &FeishuCardText{Tag: "lark_md", Content: text},
			},
			{
				Tag:     "action",
				Actions: buttons,
			},
		},
	}
	if title != "" {
		card.Header = &FeishuCardHeader{
			Title:    &FeishuCardText{Tag: "plain_text", Content: title},
			Template: "blue",
		}
	}
	return card
}

// FeishuPostContent represents a rich text "post" message.
type FeishuPostContent struct {
	ZhCN *FeishuPostBody `json:"zh_cn,omitempty"`
	EnUS *FeishuPostBody `json:"en_us,omitempty"`
}

// FeishuPostBody is the body of a post message.
type FeishuPostBody struct {
	Title   string             `json:"title"`
	Content [][]FeishuPostNode `json:"content"`
}

// FeishuPostNode is a single node in a post content line.
type FeishuPostNode struct {
	Tag      string `json:"tag"`
	Text     string `json:"text,omitempty"`
	Href     string `json:"href,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	ImageKey string `json:"image_key,omitempty"`
}

// BuildSimplePost builds a post message from markdown-like text.
// Each paragraph becomes a line with text nodes.
func BuildSimplePost(title, text, lang string) FeishuPostContent {
	lines := strings.Split(text, "\n")
	var content [][]FeishuPostNode
	for _, line := range lines {
		if line == "" {
			continue
		}
		content = append(content, []FeishuPostNode{
			{Tag: "text", Text: line},
		})
	}

	body := &FeishuPostBody{Title: title, Content: content}
	post := FeishuPostContent{}
	switch lang {
	case "en_us", "en":
		post.EnUS = body
	default:
		post.ZhCN = body
	}
	return post
}

// verifyFeishuSignature verifies the event callback signature.
// Feishu signs callbacks with: SHA256(timestamp + nonce + encryptKey + body).
func verifyFeishuSignature(timestamp, nonce, encryptKey, signature string, body []byte) bool {
	if encryptKey == "" || signature == "" {
		// No encryption configured — skip verification.
		return true
	}

	content := timestamp + nonce + encryptKey + string(body)
	hash := sha256.Sum256([]byte(content))
	expected := fmt.Sprintf("%x", hash)

	return expected == signature
}

// decryptFeishuEvent decrypts an AES-256-CBC encrypted event body.
// Feishu encrypts the event payload when encrypt_key is configured.
func decryptFeishuEvent(encrypted, encryptKey string) ([]byte, error) {
	if encryptKey == "" {
		return nil, fmt.Errorf("encrypt key is empty")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	// Remove PKCS7 padding.
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("empty plaintext after decryption")
	}
	padLen := int(ciphertext[len(ciphertext)-1])
	if padLen > aes.BlockSize || padLen > len(ciphertext) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	plaintext := ciphertext[:len(ciphertext)-padLen]

	return plaintext, nil
}

// feishuCardToJSON marshals a card for the API content field.
func feishuCardToJSON(card FeishuCard) string {
	b, err := json.Marshal(card)
	if err != nil {
		return "{}"
	}
	return string(b)
}
