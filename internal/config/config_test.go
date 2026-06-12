package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateReplayWindowSeconds(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		wantErr bool
	}{
		{name: "negative", seconds: -1, wantErr: true},
		{name: "zero", seconds: 0, wantErr: true},
		{name: "default", seconds: 5},
		{name: "maximum", seconds: MaxReplayWindowSeconds},
		{name: "above maximum", seconds: MaxReplayWindowSeconds + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReplayWindowSeconds(tt.seconds)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestLoadConfigReplayWindowDefaultAndExplicitZero(t *testing.T) {
	tests := []struct {
		name           string
		securityYAML   string
		wantWindow     int
		wantLoadConfig bool
	}{
		{
			name:           "omitted field uses default",
			securityYAML:   "security: {}",
			wantWindow:     5,
			wantLoadConfig: true,
		},
		{
			name:         "explicit zero is rejected",
			securityYAML: "security:\n  replay_window_seconds: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeMinimalConfig(t, tt.securityYAML)
			cfg, err := LoadConfig(configPath)
			if !tt.wantLoadConfig {
				if err == nil {
					t.Fatal("expected LoadConfig to reject replay window")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig failed: %v", err)
			}
			if cfg.Security.ReplayWindowSeconds != tt.wantWindow {
				t.Fatalf("replay window = %d, want %d", cfg.Security.ReplayWindowSeconds, tt.wantWindow)
			}
		})
	}
}

func TestLoadConfigListenerIPPolicy(t *testing.T) {
	tests := []struct {
		name         string
		listenerYAML string
		wantErr      bool
	}{
		{
			name:         "any interface without address",
			listenerYAML: "listener:\n  interface: any\n  port: 3001",
		},
		{
			name:         "IPv4 address",
			listenerYAML: "listener:\n  interface: any\n  port: 3001\n  listen_ip: 203.0.113.10",
		},
		{
			name:         "IPv6 is explicitly unsupported",
			listenerYAML: "listener:\n  interface: any\n  port: 3001\n  listen_ip: 2001:db8::1",
			wantErr:      true,
		},
		{
			name:         "malformed address",
			listenerYAML: "listener:\n  interface: any\n  port: 3001\n  listen_ip: not-an-ip",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeMinimalConfigWithListener(t, tt.listenerYAML, "security: {}")
			_, err := LoadConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func writeMinimalConfig(t *testing.T, securityYAML string) string {
	t.Helper()
	return writeMinimalConfigWithListener(t, "listener:\n  interface: lo\n  port: 3001", securityYAML)
}

func writeMinimalConfigWithListener(t *testing.T, listenerYAML, securityYAML string) string {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tempDir := t.TempDir()
	privateKeyPath := filepath.Join(tempDir, "server_key")
	if err := os.WriteFile(privateKeyPath, privateKey, 0600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	configYAML := fmt.Sprintf(`server_private_key_path: %q
%s
%s
users:
  - name: test
    public_key: %q
    actions: [test]
actions:
  test:
    command: "true"
`, privateKeyPath, listenerYAML, securityYAML, base64.StdEncoding.EncodeToString(publicKey))

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}
