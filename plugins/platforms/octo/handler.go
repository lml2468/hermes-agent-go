package octo

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway/wkim"
)

type octoMessagePayload struct {
	Type    wkim.MessageType `json:"type"`
	Content string           `json:"content,omitempty"`
	Mention *octoMention     `json:"mention,omitempty"`
	Reply   *octoReply       `json:"reply,omitempty"`
}

type octoMention struct {
	UIDs []string `json:"uids,omitempty"`
	All  bool     `json:"all,omitempty"`
}

type octoReply struct {
	FromUID  string              `json:"from_uid,omitempty"`
	FromName string              `json:"from_name,omitempty"`
	Payload  *octoMessagePayload `json:"payload,omitempty"`
}

// handleRecvMessage processes a decoded WuKongIM message for the Octo adapter.
func (o *OctoAdapter) handleRecvMessage(msg *wkim.RecvMessage) {
	var payload octoMessagePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		slog.Debug("octo payload parse error", "error", err)
		return
	}

	if payload.Type != wkim.MsgText || payload.Content == "" {
		return
	}

	if msg.FromUID == o.robotID {
		return
	}

	if msg.ChannelType == wkim.ChannelGroup {
		if !o.shouldRespondInGroup(payload.Mention) {
			return
		}
		payload.Content = o.cleanMentionPrefix(payload.Content)
	}

	chatType := "dm"
	if msg.ChannelType == wkim.ChannelGroup {
		chatType = "group"
	}

	event := &gateway.MessageEvent{
		Text:        payload.Content,
		MessageType: gateway.MessageTypeText,
		Source: gateway.SessionSource{
			Platform: gateway.PlatformOcto,
			ChatID:   msg.ChannelID,
			UserID:   msg.FromUID,
			ChatType: chatType,
			UserName: msg.FromUID,
		},
	}

	o.EmitMessage(event)
}

func (o *OctoAdapter) shouldRespondInGroup(mention *octoMention) bool {
	if mention == nil {
		return false
	}
	if mention.All {
		return true
	}
	for _, uid := range mention.UIDs {
		if uid == o.robotID {
			return true
		}
	}
	return false
}

func (o *OctoAdapter) cleanMentionPrefix(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.Index(text, " "); idx > 0 && strings.HasPrefix(text, "@") {
		text = strings.TrimSpace(text[idx+1:])
	}
	return text
}
