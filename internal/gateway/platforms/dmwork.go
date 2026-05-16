// Package platforms implements messaging platform adapters for the gateway.
//
// DMWork adapter: WuKongIM binary protocol over WebSocket + REST API.
package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway/wkim"
)

// ─── DMWork Types ───────────────────────────────────────────────────────────

type botRegisterResp struct {
	RobotID        string `json:"robot_id"`
	IMToken        string `json:"im_token"`
	WSURL          string `json:"ws_url"`
	APIURL         string `json:"api_url"`
	OwnerUID       string `json:"owner_uid"`
	OwnerChannelID string `json:"owner_channel_id"`
}

type botMessage struct {
	MessageID   string           `json:"message_id"`
	MessageSeq  int              `json:"message_seq"`
	FromUID     string           `json:"from_uid"`
	ChannelID   string           `json:"channel_id"`
	ChannelType wkim.ChannelType `json:"channel_type"`
	Timestamp   int              `json:"timestamp"`
	Payload     messagePayload   `json:"payload"`
}

type messagePayload struct {
	Type    wkim.MessageType       `json:"type"`
	Content string                 `json:"content,omitempty"`
	Mention *mentionPayload        `json:"mention,omitempty"`
	Reply   *replyPayload          `json:"reply,omitempty"`
	Extra   map[string]interface{} `json:"-"`
}

type mentionPayload struct {
	UIDs []string `json:"uids,omitempty"`
	All  bool     `json:"all,omitempty"`
}

type replyPayload struct {
	FromUID  string          `json:"from_uid,omitempty"`
	FromName string          `json:"from_name,omitempty"`
	Payload  *messagePayload `json:"payload,omitempty"`
}

// ─── DMWork Adapter ─────────────────────────────────────────────────────────

// DMWorkAdapter implements the gateway.PlatformAdapter for DMWork.
type DMWorkAdapter struct {
	BasePlatformAdapter

	apiURL   string
	botToken string

	robotID  string
	imToken  string
	wsURL    string
	ownerUID string

	connMgr *wkim.ConnectionManager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDMWorkAdapter creates a new DMWork adapter.
func NewDMWorkAdapter(apiURL, botToken string) *DMWorkAdapter {
	return &DMWorkAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformDMWork),
		apiURL:              strings.TrimRight(apiURL, "/"),
		botToken:            botToken,
	}
}

// Connect registers the bot and establishes the WebSocket connection.
func (d *DMWorkAdapter) Connect(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)

	reg, err := d.registerBot(false)
	if err != nil {
		return fmt.Errorf("dmwork bot registration: %w", err)
	}

	d.robotID = reg.RobotID
	d.imToken = reg.IMToken
	d.wsURL = reg.WSURL
	d.ownerUID = reg.OwnerUID

	if reg.APIURL != "" {
		d.apiURL = strings.TrimRight(reg.APIURL, "/")
	}

	slog.Info("DMWork bot registered", "robot_id", d.robotID, "ws_url", d.wsURL)

	d.connMgr = wkim.NewConnectionManager(wkim.ConnectConfig{
		WSURL:        d.wsURL,
		UID:          d.robotID,
		Token:        d.imToken,
		PingInterval: wkim.DefaultPingInterval,
		PingMaxRetry: wkim.DefaultPingMaxRetry,
	}, d)

	if err := d.connMgr.Connect(d.ctx); err != nil {
		return fmt.Errorf("dmwork websocket: %w", err)
	}

	return nil
}

// Disconnect closes the WebSocket connection.
func (d *DMWorkAdapter) Disconnect() error {
	if d.cancel != nil {
		d.cancel()
	}
	if d.connMgr != nil {
		d.connMgr.Close()
	}
	d.wg.Wait()
	d.BasePlatformAdapter.connected = false
	return nil
}

// HandleConnected implements wkim.ConnectionHandler.
func (d *DMWorkAdapter) HandleConnected() {
	d.BasePlatformAdapter.connected = true
	slog.Info("DMWork WebSocket connected")
}

// HandleDisconnected implements wkim.ConnectionHandler.
func (d *DMWorkAdapter) HandleDisconnected(err error) {
	d.BasePlatformAdapter.connected = false
	slog.Warn("DMWork disconnected", "error", err)
}

// HandleWKIMMessage implements wkim.ConnectionHandler.
func (d *DMWorkAdapter) HandleWKIMMessage(msg *wkim.RecvMessage) {
	var payload messagePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		slog.Debug("dmwork payload parse error", "error", err)
		return
	}

	if payload.Type != wkim.MsgText || payload.Content == "" {
		return
	}

	if msg.FromUID == d.robotID {
		return
	}

	chatType := "dm"
	if msg.ChannelType == wkim.ChannelGroup {
		chatType = "group"
	}

	event := &gateway.MessageEvent{
		Text:        payload.Content,
		MessageType: gateway.MessageTypeText,
		Source: gateway.SessionSource{
			Platform: gateway.PlatformDMWork,
			ChatID:   msg.ChannelID,
			UserID:   msg.FromUID,
			ChatType: chatType,
			UserName: msg.FromUID,
		},
	}

	if d.messageHandler != nil {
		d.messageHandler(event)
	}
}

// Send sends a text message to a DMWork channel.
func (d *DMWorkAdapter) Send(ctx context.Context, chatID, content string, metadata map[string]string) (*gateway.SendResult, error) {
	channelType := wkim.ChannelDM
	if metadata != nil {
		if ct, ok := metadata["channel_type"]; ok && ct == "2" {
			channelType = wkim.ChannelGroup
		}
	}

	parts := splitMessage(content, MaxMessageLength)
	for _, part := range parts {
		if err := d.sendMessage(chatID, channelType, part); err != nil {
			return &gateway.SendResult{Success: false, Error: err.Error()}, err
		}
	}
	return &gateway.SendResult{Success: true}, nil
}

// SendDocument sends a file (not yet implemented).
func (d *DMWorkAdapter) SendDocument(ctx context.Context, chatID, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return &gateway.SendResult{Success: false, Error: "document sending not yet implemented"}, nil
}

// SendImage sends an image (not yet implemented).
func (d *DMWorkAdapter) SendImage(ctx context.Context, chatID, imagePath, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return &gateway.SendResult{Success: false, Error: "image sending not yet implemented"}, nil
}

// SendVoice sends a voice message (not yet implemented).
func (d *DMWorkAdapter) SendVoice(ctx context.Context, chatID, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return &gateway.SendResult{Success: false, Error: "voice sending not yet implemented"}, nil
}

// SendTyping sends a typing indicator.
func (d *DMWorkAdapter) SendTyping(ctx context.Context, chatID string) error {
	channelType := wkim.ChannelDM
	d.sendTyping(chatID, channelType)
	return nil
}

// IsConnected returns the connection status.
func (d *DMWorkAdapter) IsConnected() bool {
	return d.BasePlatformAdapter.connected
}

// ─── REST API ───────────────────────────────────────────────────────────────

func (d *DMWorkAdapter) registerBot(forceRefresh bool) (*botRegisterResp, error) {
	path := "/v1/bot/register"
	if forceRefresh {
		path += "?force_refresh=true"
	}

	body, err := d.postJSON(path, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var resp botRegisterResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse register response: %w", err)
	}
	return &resp, nil
}

func (d *DMWorkAdapter) sendMessage(channelID string, channelType wkim.ChannelType, content string) error {
	_, err := d.postJSON("/v1/bot/sendMessage", map[string]interface{}{
		"channel_id":   channelID,
		"channel_type": int(channelType),
		"payload": map[string]interface{}{
			"type":    int(wkim.MsgText),
			"content": content,
		},
	})
	return err
}

func (d *DMWorkAdapter) sendTyping(channelID string, channelType wkim.ChannelType) {
	d.postJSON("/v1/bot/typing", map[string]interface{}{
		"channel_id":   channelID,
		"channel_type": int(channelType),
	})
}

func (d *DMWorkAdapter) postJSON(path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	url := d.apiURL + path
	req, err := http.NewRequestWithContext(d.ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.botToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dmwork api %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dmwork api %s failed (HTTP %d): %s", path, resp.StatusCode, string(body))
	}

	return body, nil
}

func splitMessage(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}
	var parts []string
	for len(content) > 0 {
		end := maxLen
		if end > len(content) {
			end = len(content)
		}
		if end < len(content) {
			if idx := strings.LastIndex(content[:end], "\n"); idx > end/2 {
				end = idx + 1
			}
		}
		parts = append(parts, content[:end])
		content = content[end:]
	}
	return parts
}
