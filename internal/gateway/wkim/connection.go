package wkim

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionHandler receives lifecycle and message callbacks from ConnectionManager.
type ConnectionHandler interface {
	HandleConnected()
	HandleDisconnected(err error)
	HandleWKIMMessage(msg *RecvMessage)
}

// ConnectionManager manages a WuKongIM WebSocket connection with automatic
// heartbeat, reconnection, and message decoding.
type ConnectionManager struct {
	cfg     ConnectConfig
	handler ConnectionHandler

	ws            *websocket.Conn
	wsMu          sync.Mutex
	aesKey        []byte
	aesIV         []byte
	connected     bool
	serverVersion int

	pingRetry   int
	heartTicker *time.Ticker
	heartDone   chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(cfg ConnectConfig, handler ConnectionHandler) *ConnectionManager {
	if cfg.PingInterval == 0 {
		cfg.PingInterval = DefaultPingInterval
	}
	if cfg.PingMaxRetry == 0 {
		cfg.PingMaxRetry = DefaultPingMaxRetry
	}
	return &ConnectionManager{
		cfg:     cfg,
		handler: handler,
	}
}

// Connect establishes the WebSocket connection and performs the WuKongIM handshake.
func (cm *ConnectionManager) Connect(ctx context.Context) error {
	cm.ctx, cm.cancel = context.WithCancel(ctx)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(cm.ctx, cm.cfg.WSURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	cm.wsMu.Lock()
	cm.ws = conn
	cm.wsMu.Unlock()

	kp, err := GenerateDHKeyPair()
	if err != nil {
		return err
	}

	connectPkt := EncodeConnectPacket(cm.cfg.UID, cm.cfg.Token, kp.PublicKeyBase64())
	if err := conn.WriteMessage(websocket.BinaryMessage, connectPkt); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read connack: %w", err)
	}

	if err := cm.handleConnack(msg, kp.PrivateKey); err != nil {
		return err
	}

	cm.connected = true

	cm.startHeart()

	cm.wg.Add(1)
	go cm.readLoop()

	if cm.handler != nil {
		cm.handler.HandleConnected()
	}

	return nil
}

// Close shuts down the connection and waits for goroutines to finish.
func (cm *ConnectionManager) Close() error {
	if cm.cancel != nil {
		cm.cancel()
	}
	cm.stopHeart()
	cm.wsMu.Lock()
	if cm.ws != nil {
		cm.ws.Close()
		cm.ws = nil
	}
	cm.wsMu.Unlock()
	cm.wg.Wait()
	cm.connected = false
	return nil
}

// IsConnected reports whether the WebSocket is currently connected.
func (cm *ConnectionManager) IsConnected() bool {
	return cm.connected
}

// Send writes a raw binary message to the WebSocket.
func (cm *ConnectionManager) Send(data []byte) error {
	cm.wsMu.Lock()
	defer cm.wsMu.Unlock()
	if cm.ws == nil {
		return fmt.Errorf("not connected")
	}
	return cm.ws.WriteMessage(websocket.BinaryMessage, data)
}

// AESKey returns the negotiated AES key (available after Connect).
func (cm *ConnectionManager) AESKey() []byte { return cm.aesKey }

// AESIV returns the negotiated AES IV (available after Connect).
func (cm *ConnectionManager) AESIV() []byte { return cm.aesIV }

// ServerVersion returns the server protocol version from CONNACK.
func (cm *ConnectionManager) ServerVersion() int { return cm.serverVersion }

func (cm *ConnectionManager) handleConnack(data []byte, privateKey []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("connack too short")
	}

	header := data[0]
	hasServerVersion := (header & 0x01) > 0

	offset := 1
	_, n := DecodeVariableLength(data[offset:])
	offset += n

	if hasServerVersion {
		if offset >= len(data) {
			return fmt.Errorf("connack: missing server version")
		}
		cm.serverVersion = int(data[offset])
		offset++
	}

	if offset+8 > len(data) {
		return fmt.Errorf("connack: missing time diff")
	}
	offset += 8

	if offset >= len(data) {
		return fmt.Errorf("connack: missing reason code")
	}
	reasonCode := data[offset]
	offset++

	if reasonCode != 1 {
		return fmt.Errorf("connack failed: reason=%d", reasonCode)
	}

	serverKeyStr, n := ReadString(data[offset:])
	offset += n

	salt, _ := ReadString(data[offset:])

	aesKey, aesIV, err := DeriveAESKeys(privateKey, serverKeyStr, salt)
	if err != nil {
		return err
	}

	cm.aesKey = aesKey
	cm.aesIV = aesIV
	return nil
}

func (cm *ConnectionManager) readLoop() {
	defer cm.wg.Done()

	var tempBuf []byte

	for {
		select {
		case <-cm.ctx.Done():
			return
		default:
		}

		cm.wsMu.Lock()
		ws := cm.ws
		cm.wsMu.Unlock()
		if ws == nil {
			return
		}

		_, msg, err := ws.ReadMessage()
		if err != nil {
			if cm.ctx.Err() != nil {
				return
			}
			slog.Debug("wkim ws read error", "error", err)
			cm.scheduleReconnect()
			return
		}

		tempBuf = append(tempBuf, msg...)
		tempBuf = cm.processPackets(tempBuf)
	}
}

func (cm *ConnectionManager) processPackets(data []byte) []byte {
	for len(data) > 0 {
		header := data[0]
		packetType := PacketType(header >> 4)

		if packetType == PktPong {
			cm.pingRetry = 0
			data = data[1:]
			continue
		}
		if packetType == PktPing {
			data = data[1:]
			continue
		}

		if len(data) < 2 {
			break
		}
		remLen, n := DecodeVariableLength(data[1:])
		totalLen := 1 + n + remLen

		if totalLen > len(data) {
			break
		}

		pkt := data[:totalLen]
		cm.handlePacket(pkt)
		data = data[totalLen:]
	}
	return data
}

func (cm *ConnectionManager) handlePacket(data []byte) {
	header := data[0]
	packetType := PacketType(header >> 4)

	offset := 1
	_, n := DecodeVariableLength(data[offset:])
	offset += n

	switch packetType {
	case PktRecv:
		cm.handleRecv(data[offset:])
	case PktDisconnect:
		slog.Warn("wkim disconnected by server")
		if cm.handler != nil {
			cm.handler.HandleDisconnected(fmt.Errorf("disconnected by server"))
		}
	}
}

func (cm *ConnectionManager) handleRecv(body []byte) {
	if len(body) < 10 {
		return
	}

	offset := 0

	// Setting byte.
	offset++

	// msgKey.
	_, n := ReadString(body[offset:])
	offset += n

	// fromUID.
	fromUID, n := ReadString(body[offset:])
	offset += n

	// channelID.
	channelID, n := ReadString(body[offset:])
	offset += n

	// channelType.
	if offset >= len(body) {
		return
	}
	channelType := ChannelType(body[offset])
	offset++

	// expire (v3+).
	if cm.serverVersion >= 3 {
		if offset+4 > len(body) {
			return
		}
		offset += 4
	}

	// clientMsgNo.
	_, n = ReadString(body[offset:])
	offset += n

	// messageID (int64).
	if offset+8 > len(body) {
		return
	}
	messageID := binary.BigEndian.Uint64(body[offset : offset+8])
	offset += 8

	// messageSeq (int32).
	if offset+4 > len(body) {
		return
	}
	messageSeq := int(binary.BigEndian.Uint32(body[offset : offset+4]))
	offset += 4

	// timestamp (int32).
	if offset+4 > len(body) {
		return
	}
	offset += 4

	encryptedPayload := body[offset:]

	ack := EncodeRecvAck(messageID, messageSeq)
	cm.Send(ack)

	decrypted, err := AESDecryptCBC(encryptedPayload, cm.aesKey, cm.aesIV)
	if err != nil {
		slog.Debug("wkim payload decrypt error", "error", err)
		return
	}

	if cm.handler != nil {
		cm.handler.HandleWKIMMessage(&RecvMessage{
			MessageID:   messageID,
			MessageSeq:  messageSeq,
			FromUID:     fromUID,
			ChannelID:   channelID,
			ChannelType: channelType,
			Payload:     decrypted,
		})
	}
}

// --- Heartbeat ---

func (cm *ConnectionManager) startHeart() {
	cm.heartTicker = time.NewTicker(cm.cfg.PingInterval)
	cm.heartDone = make(chan struct{})
	cm.pingRetry = 0

	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		for {
			select {
			case <-cm.heartDone:
				return
			case <-cm.ctx.Done():
				return
			case <-cm.heartTicker.C:
				cm.pingRetry++
				if cm.pingRetry > cm.cfg.PingMaxRetry {
					slog.Debug("wkim ping timeout, reconnecting")
					cm.scheduleReconnect()
					return
				}
				cm.wsMu.Lock()
				if cm.ws != nil {
					cm.ws.WriteMessage(websocket.BinaryMessage, []byte{byte(PktPing << 4)})
				}
				cm.wsMu.Unlock()
			}
		}
	}()
}

func (cm *ConnectionManager) stopHeart() {
	if cm.heartTicker != nil {
		cm.heartTicker.Stop()
	}
	if cm.heartDone != nil {
		select {
		case <-cm.heartDone:
		default:
			close(cm.heartDone)
		}
	}
}

func (cm *ConnectionManager) scheduleReconnect() {
	cm.wsMu.Lock()
	if cm.ws != nil {
		cm.ws.Close()
		cm.ws = nil
	}
	cm.wsMu.Unlock()

	cm.connected = false
	cm.stopHeart()

	time.AfterFunc(3*time.Second, func() {
		if cm.ctx.Err() != nil {
			return
		}
		slog.Info("wkim reconnecting")
		if err := cm.Connect(cm.ctx); err != nil {
			slog.Warn("wkim reconnect failed", "error", err)
			if cm.handler != nil {
				cm.handler.HandleDisconnected(err)
			}
		}
	})
}
