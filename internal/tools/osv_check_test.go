package tools

import (
	"testing"
)

func TestInferEcosystem(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"npx", "npm"},
		{"npx.cmd", "npm"},
		{"uvx", "PyPI"},
		{"pipx", "PyPI"},
		{"node", ""},
		{"python", ""},
		{"/usr/bin/npx", "npm"},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			if got := inferEcosystem(tt.command); got != tt.want {
				t.Errorf("inferEcosystem(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestParseNPMPackage(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		pkg     string
		version string
	}{
		{"simple", "express", "express", ""},
		{"with version", "express@4.18.2", "express", "4.18.2"},
		{"latest", "express@latest", "express", ""},
		{"scoped", "@modelcontextprotocol/server", "@modelcontextprotocol/server", ""},
		{"scoped with version", "@mcp/server@1.0.0", "@mcp/server", "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, ver := parseNPMPackage(tt.token)
			if pkg != tt.pkg || ver != tt.version {
				t.Errorf("parseNPMPackage(%q) = (%q, %q), want (%q, %q)",
					tt.token, pkg, ver, tt.pkg, tt.version)
			}
		})
	}
}

func TestParsePyPIPackage(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		pkg     string
		version string
	}{
		{"simple", "flask", "flask", ""},
		{"with version", "flask==2.3.0", "flask", "2.3.0"},
		{"with extras", "flask[async]==2.3.0", "flask", "2.3.0"},
		{"extras no version", "flask[async]", "flask", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, ver := parsePyPIPackage(tt.token)
			if pkg != tt.pkg || ver != tt.version {
				t.Errorf("parsePyPIPackage(%q) = (%q, %q), want (%q, %q)",
					tt.token, pkg, ver, tt.pkg, tt.version)
			}
		})
	}
}

func TestParsePackageFromArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		ecosystem string
		pkg       string
	}{
		{"npm with flags", []string{"-y", "@mcp/server"}, "npm", "@mcp/server"},
		{"npm no flags", []string{"express"}, "npm", "express"},
		{"empty args", []string{}, "npm", ""},
		{"only flags", []string{"-y", "--yes"}, "npm", ""},
		{"pypi", []string{"flask"}, "PyPI", "flask"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, _ := parsePackageFromArgs(tt.args, tt.ecosystem)
			if pkg != tt.pkg {
				t.Errorf("parsePackageFromArgs(%v, %q) pkg = %q, want %q",
					tt.args, tt.ecosystem, pkg, tt.pkg)
			}
		})
	}
}

func TestCheckPackageForMalware_UnknownCommand(t *testing.T) {
	// Non-npx/uvx commands should be allowed (no check).
	result := CheckPackageForMalware("node", []string{"server.js"})
	if result != "" {
		t.Errorf("expected empty for unknown command, got %q", result)
	}
}

func TestCheckPackageForMalware_EmptyArgs(t *testing.T) {
	result := CheckPackageForMalware("npx", nil)
	if result != "" {
		t.Errorf("expected empty for nil args, got %q", result)
	}
}
