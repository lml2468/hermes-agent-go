// Package wkim implements the WuKongIM binary protocol shared between platform adapters.
package wkim

import "time"

// Protocol version for WuKongIM.
const ProtoVersion = 4

// Default connection parameters.
const (
	DefaultPingInterval = 60 * time.Second
	DefaultPingMaxRetry = 3
)

// PacketType identifies WuKongIM protocol packet types.
type PacketType byte

const (
	PktConnect    PacketType = 1
	PktConnack    PacketType = 2
	PktSend       PacketType = 3
	PktSendack    PacketType = 4
	PktRecv       PacketType = 5
	PktRecvack    PacketType = 6
	PktPing       PacketType = 7
	PktPong       PacketType = 8
	PktDisconnect PacketType = 9
)

// ChannelType represents WuKongIM channel types.
type ChannelType int

const (
	ChannelDM    ChannelType = 1
	ChannelGroup ChannelType = 2
)

// MessageType represents WuKongIM payload message types.
type MessageType int

const (
	MsgText  MessageType = 1
	MsgImage MessageType = 2
	MsgFile  MessageType = 8
)

// RecvMessage is a decoded received message from WuKongIM.
type RecvMessage struct {
	MessageID  uint64
	MessageSeq int
	FromUID    string
	ChannelID  string
	ChannelType ChannelType
	Payload    []byte
}

// ConnectConfig holds parameters for establishing a WuKongIM WebSocket connection.
type ConnectConfig struct {
	WSURL        string
	UID          string
	Token        string
	PingInterval time.Duration
	PingMaxRetry int
}
