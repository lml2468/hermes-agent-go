package octo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway/wkim"
)

// OctoAPIClient wraps the Octo Bot REST API.
type OctoAPIClient struct {
	baseURL    string
	botToken   string
	httpClient *http.Client
}

// BotRegisterResp is the response from /v1/bot/register.
type BotRegisterResp struct {
	RobotID        string `json:"robot_id"`
	IMToken        string `json:"im_token"`
	WSURL          string `json:"ws_url"`
	APIURL         string `json:"api_url"`
	OwnerUID       string `json:"owner_uid"`
	OwnerChannelID string `json:"owner_channel_id"`
}

// NewOctoAPIClient creates a new API client.
func NewOctoAPIClient(baseURL, botToken string) *OctoAPIClient {
	return &OctoAPIClient{
		baseURL:    baseURL,
		botToken:   botToken,
		httpClient: &http.Client{},
	}
}

// RegisterBot calls /v1/bot/register and returns connection credentials.
func (c *OctoAPIClient) RegisterBot(ctx context.Context, forceRefresh bool) (*BotRegisterResp, error) {
	path := "/v1/bot/register"
	if forceRefresh {
		path += "?force_refresh=true"
	}

	body, err := c.postJSON(ctx, path, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var resp BotRegisterResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse register response: %w", err)
	}
	return &resp, nil
}

// SendMessage sends a text message through the Octo Bot API.
func (c *OctoAPIClient) SendMessage(ctx context.Context, channelID string, channelType wkim.ChannelType, content string) error {
	_, err := c.postJSON(ctx, "/v1/bot/sendMessage", map[string]interface{}{
		"channel_id":   channelID,
		"channel_type": int(channelType),
		"payload": map[string]interface{}{
			"type":    int(wkim.MsgText),
			"content": content,
		},
	})
	return err
}

// SendTyping sends a typing indicator through the Octo Bot API.
func (c *OctoAPIClient) SendTyping(ctx context.Context, channelID string, channelType wkim.ChannelType) error {
	_, err := c.postJSON(ctx, "/v1/bot/typing", map[string]interface{}{
		"channel_id":   channelID,
		"channel_type": int(channelType),
	})
	return err
}

func (c *OctoAPIClient) postJSON(ctx context.Context, path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.botToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("octo api %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("octo api %s failed (HTTP %d): %s", path, resp.StatusCode, string(body))
	}

	return body, nil
}
