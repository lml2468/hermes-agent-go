package wkim

import (
	"bytes"
	"encoding/binary"
	"time"
)

// EncodeConnectPacket builds a CONNECT packet for the WuKongIM protocol.
func EncodeConnectPacket(uid, token, clientKey string) []byte {
	var body bytes.Buffer

	body.WriteByte(ProtoVersion)
	body.WriteByte(0) // deviceFlag (0 = app)
	WriteString(&body, GenerateDeviceID()+"W")
	WriteString(&body, uid)
	WriteString(&body, token)

	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(time.Now().UnixMilli()))
	body.Write(ts[:])

	WriteString(&body, clientKey)

	bodyBytes := body.Bytes()

	var frame bytes.Buffer
	frame.WriteByte(byte(PktConnect<<4) | 0)
	frame.Write(EncodeVariableLength(len(bodyBytes)))
	frame.Write(bodyBytes)

	return frame.Bytes()
}

// EncodeRecvAck builds a RECVACK packet acknowledging a received message.
func EncodeRecvAck(messageID uint64, messageSeq int) []byte {
	var buf bytes.Buffer

	var idBuf [8]byte
	binary.BigEndian.PutUint64(idBuf[:], messageID)
	buf.Write(idBuf[:])

	var seqBuf [4]byte
	binary.BigEndian.PutUint32(seqBuf[:], uint32(messageSeq))
	buf.Write(seqBuf[:])

	bodyBytes := buf.Bytes()

	var frame bytes.Buffer
	frame.WriteByte(byte(PktRecvack<<4) | 0)
	frame.Write(EncodeVariableLength(len(bodyBytes)))
	frame.Write(bodyBytes)

	return frame.Bytes()
}

// EncodeVariableLength encodes an integer as a WuKongIM variable-length field.
func EncodeVariableLength(length int) []byte {
	var buf []byte
	for length > 0 {
		digit := byte(length % 0x80)
		length /= 0x80
		if length > 0 {
			digit |= 0x80
		}
		buf = append(buf, digit)
	}
	if len(buf) == 0 {
		buf = append(buf, 0)
	}
	return buf
}

// DecodeVariableLength reads a variable-length integer from data.
// Returns the decoded value and the number of bytes consumed.
func DecodeVariableLength(data []byte) (int, int) {
	multiplier := 0
	rLength := 0
	bytesRead := 0
	for multiplier < 27 && bytesRead < len(data) {
		b := data[bytesRead]
		bytesRead++
		rLength |= int(b&127) << multiplier
		if b&128 == 0 {
			break
		}
		multiplier += 7
	}
	return rLength, bytesRead
}

// WriteString writes a length-prefixed string to the buffer (big-endian uint16 length).
func WriteString(buf *bytes.Buffer, s string) {
	data := []byte(s)
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(data)))
	buf.Write(lenBuf[:])
	buf.Write(data)
}

// ReadString reads a length-prefixed string from data.
// Returns the string and total bytes consumed (including 2-byte length prefix).
func ReadString(data []byte) (string, int) {
	if len(data) < 2 {
		return "", 0
	}
	l := int(binary.BigEndian.Uint16(data[:2]))
	if l <= 0 {
		return "", 2
	}
	if 2+l > len(data) {
		return "", 2
	}
	return string(data[2 : 2+l]), 2 + l
}
