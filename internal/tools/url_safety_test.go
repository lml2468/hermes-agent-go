package tools

import (
	"net"
	"testing"
)

func TestIsSafeURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		safe bool
	}{
		// Safe URLs
		{"https public", "https://example.com/path", true},
		{"http public", "http://example.com", true},
		{"https with port", "https://example.com:8080/api", true},

		// Blocked: private IPs
		{"localhost", "http://localhost/secret", false},
		{"127.0.0.1", "http://127.0.0.1:8080/", false},
		{"10.x private", "http://10.0.0.1/admin", false},
		{"172.16 private", "http://172.16.0.1/", false},
		{"192.168 private", "http://192.168.1.1/", false},
		{"169.254 link-local", "http://169.254.169.254/metadata", false},
		{"CGNAT 100.64", "http://100.64.0.1/internal", false},
		{"::1 loopback v6", "http://[::1]:8080/", false},

		// Blocked: dangerous schemes
		{"file scheme", "file:///etc/passwd", false},
		{"ftp scheme", "ftp://evil.com/malware", false},

		// Blocked: internal hostnames
		{"metadata.google.internal", "http://metadata.google.internal/v1/", false},
		{"metadata.goog", "http://metadata.goog/", false},

		// Blocked: edge cases
		{"empty url", "", false},
		{"no hostname", "http:///path", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe, reason := IsSafeURL(tt.url)
			if safe != tt.safe {
				t.Errorf("IsSafeURL(%q) = %v (reason: %s), want %v", tt.url, safe, reason, tt.safe)
			}
		})
	}
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback", "127.0.0.1", true},
		{"private 10", "10.0.0.1", true},
		{"private 172", "172.16.0.1", true},
		{"private 192", "192.168.0.1", true},
		{"link-local", "169.254.1.1", true},
		{"CGNAT", "100.64.0.1", true},
		{"multicast", "224.0.0.1", true},
		{"unspecified", "0.0.0.0", true},
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"ipv6 loopback", "::1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("invalid test IP: %s", tt.ip)
			}
			if got := isBlockedIP(ip); got != tt.blocked {
				t.Errorf("isBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}
