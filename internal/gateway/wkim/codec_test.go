package wkim

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeVariableLength(t *testing.T) {
	tests := []struct {
		value int
	}{
		{0},
		{1},
		{127},
		{128},
		{255},
		{16383},
		{16384},
		{100000},
	}

	for _, tc := range tests {
		encoded := EncodeVariableLength(tc.value)
		decoded, n := DecodeVariableLength(encoded)
		if decoded != tc.value {
			t.Errorf("EncodeVariableLength(%d): decoded=%d, want %d", tc.value, decoded, tc.value)
		}
		if n != len(encoded) {
			t.Errorf("EncodeVariableLength(%d): consumed %d bytes, encoded %d bytes", tc.value, n, len(encoded))
		}
	}
}

func TestWriteReadString(t *testing.T) {
	tests := []string{
		"",
		"hello",
		"test-uid-12345",
		"中文测试",
	}

	for _, s := range tests {
		var buf bytes.Buffer
		WriteString(&buf, s)
		data := buf.Bytes()
		got, n := ReadString(data)
		if got != s {
			t.Errorf("WriteString/ReadString(%q): got %q", s, got)
		}
		if n != len(data) {
			t.Errorf("WriteString/ReadString(%q): consumed %d, want %d", s, n, len(data))
		}
	}
}

func TestReadStringShort(t *testing.T) {
	s, n := ReadString(nil)
	if s != "" || n != 0 {
		t.Errorf("ReadString(nil): got (%q, %d), want (\"\", 0)", s, n)
	}

	s, n = ReadString([]byte{0})
	if s != "" || n != 0 {
		t.Errorf("ReadString(1 byte): got (%q, %d), want (\"\", 0)", s, n)
	}
}

func TestEncodeConnectPacket(t *testing.T) {
	pkt := EncodeConnectPacket("uid1", "token1", "clientkey1")
	if len(pkt) == 0 {
		t.Fatal("EncodeConnectPacket returned empty")
	}
	header := pkt[0]
	pktType := PacketType(header >> 4)
	if pktType != PktConnect {
		t.Errorf("packet type = %d, want %d (CONNECT)", pktType, PktConnect)
	}
}

func TestEncodeRecvAck(t *testing.T) {
	ack := EncodeRecvAck(12345, 1)
	if len(ack) == 0 {
		t.Fatal("EncodeRecvAck returned empty")
	}
	header := ack[0]
	pktType := PacketType(header >> 4)
	if pktType != PktRecvack {
		t.Errorf("packet type = %d, want %d (RECVACK)", pktType, PktRecvack)
	}
}
