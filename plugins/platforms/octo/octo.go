// Package octo implements the Octo IM platform adapter as a bundled platform plugin.
// Octo uses the WuKongIM binary protocol (shared via the wkim package).
package octo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway/platforms"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway/wkim"
)

const maxMessageLength = 4096

func init() {
	gateway.GlobalPlatformRegistry().Register(&OctoPlugin{})
}

// OctoPlugin implements gateway.PlatformPlugin.
type OctoPlugin struct{}

func (p *OctoPlugin) Metadata() gateway.PlatformPluginMeta {
	return gateway.PlatformPluginMeta{
		ID:          gateway.PlatformOcto,
		Name:        "octo",
		Version:     "0.1.0",
		Description: "Octo IM platform adapter based on WuKongIM protocol",
	}
}

func (p *OctoPlugin) CreateAdapter(cfg *gateway.PlatformConfig) (gateway.PlatformAdapter, error) {
	apiURL := "https://im.deepminer.com.cn/api"
	if v := os.Getenv("OCTO_API_URL"); v != "" {
		apiURL = v
	}
	if cfg.Settings != nil {
		if v, ok := cfg.Settings["api_url"]; ok && v != "" {
			apiURL = v
		}
	}

	botToken := cfg.Token
	if botToken == "" {
		botToken = os.Getenv("OCTO_BOT_TOKEN")
	}
	if botToken == "" {
		return nil, fmt.Errorf("octo: bot token is required (set OCTO_BOT_TOKEN or config token)")
	}

	mentionMode := "explicit"
	if cfg.Settings != nil {
		if v, ok := cfg.Settings["mention_mode"]; ok && v != "" {
			mentionMode = v
		}
	}

	return NewOctoAdapter(apiURL, botToken, mentionMode), nil
}

// OctoAdapter implements gateway.PlatformAdapter for the Octo IM platform.
type OctoAdapter struct {
	platforms.BasePlatformAdapter

	apiClient *OctoAPIClient
	connMgr   *wkim.ConnectionManager

	robotID  string
	imToken  string
	ownerUID string

	mentionMode string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOctoAdapter creates a new Octo adapter.
func NewOctoAdapter(apiURL, botToken, mentionMode string) *OctoAdapter {
	return &OctoAdapter{
		BasePlatformAdapter: platforms.NewBasePlatformAdapter(gateway.PlatformOcto),
		apiClient:           NewOctoAPIClient(strings.TrimRight(apiURL, "/"), botToken),
		mentionMode:         mentionMode,
	}
}

// Connect registers the bot and establishes the WuKongIM WebSocket connection.
func (o *OctoAdapter) Connect(ctx context.Context) error {
	o.ctx, o.cancel = context.WithCancel(ctx)

	reg, err := o.apiClient.RegisterBot(o.ctx, false)
	if err != nil {
		return fmt.Errorf("octo bot registration: %w", err)
	}

	o.robotID = reg.RobotID
	o.imToken = reg.IMToken
	o.ownerUID = reg.OwnerUID

	if reg.APIURL != "" {
		o.apiClient.baseURL = strings.TrimRight(reg.APIURL, "/")
	}

	slog.Info("Octo bot registered", "robot_id", o.robotID, "ws_url", reg.WSURL)

	o.connMgr = wkim.NewConnectionManager(wkim.ConnectConfig{
		WSURL:        reg.WSURL,
		UID:          o.robotID,
		Token:        o.imToken,
		PingInterval: wkim.DefaultPingInterval,
		PingMaxRetry: wkim.DefaultPingMaxRetry,
	}, o)

	if err := o.connMgr.Connect(o.ctx); err != nil {
		return fmt.Errorf("octo websocket: %w", err)
	}

	return nil
}

// Disconnect closes the connection.
func (o *OctoAdapter) Disconnect() error {
	if o.cancel != nil {
		o.cancel()
	}
	if o.connMgr != nil {
		o.connMgr.Close()
	}
	o.wg.Wait()
	return nil
}

// HandleConnected implements wkim.ConnectionHandler.
func (o *OctoAdapter) HandleConnected() {
	slog.Info("Octo WebSocket connected")
}

// HandleDisconnected implements wkim.ConnectionHandler.
func (o *OctoAdapter) HandleDisconnected(err error) {
	slog.Warn("Octo disconnected", "error", err)
}

// HandleWKIMMessage implements wkim.ConnectionHandler.
func (o *OctoAdapter) HandleWKIMMessage(msg *wkim.RecvMessage) {
	o.handleRecvMessage(msg)
}

// Send sends a text message via the Octo API.
func (o *OctoAdapter) Send(ctx context.Context, chatID, content string, metadata map[string]string) (*gateway.SendResult, error) {
	channelType := wkim.ChannelDM
	if metadata != nil {
		if ct, ok := metadata["channel_type"]; ok && ct == "2" {
			channelType = wkim.ChannelGroup
		}
	}

	parts := platforms.SplitMessage(content, maxMessageLength)
	for _, part := range parts {
		if err := o.apiClient.SendMessage(ctx, chatID, channelType, part); err != nil {
			return &gateway.SendResult{Success: false, Error: err.Error()}, err
		}
	}
	return &gateway.SendResult{Success: true}, nil
}

// SendImage is not yet implemented for Octo.
func (o *OctoAdapter) SendImage(ctx context.Context, chatID, imagePath, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return &gateway.SendResult{Success: false, Error: "image sending not yet implemented"}, nil
}

// SendVoice is not yet implemented for Octo.
func (o *OctoAdapter) SendVoice(ctx context.Context, chatID, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return &gateway.SendResult{Success: false, Error: "voice sending not yet implemented"}, nil
}

// SendDocument is not yet implemented for Octo.
func (o *OctoAdapter) SendDocument(ctx context.Context, chatID, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return &gateway.SendResult{Success: false, Error: "document sending not yet implemented"}, nil
}

// SendTyping sends a typing indicator via the Octo API.
func (o *OctoAdapter) SendTyping(ctx context.Context, chatID string) error {
	return o.apiClient.SendTyping(ctx, chatID, wkim.ChannelDM)
}
