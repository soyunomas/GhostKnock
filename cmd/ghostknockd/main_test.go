package main

import (
	"crypto/ed25519"
	"encoding/base32"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

	"github.com/soyunomas/ghostknock/internal/config"
	"github.com/soyunomas/ghostknock/internal/listener"
	"github.com/soyunomas/ghostknock/internal/protocol"
)

func TestFreshnessAcceptsCurrentAndBoundaryTimestamps(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	window := 5 * time.Second

	tests := []struct {
		name      string
		timestamp time.Time
	}{
		{name: "current", timestamp: now},
		{name: "past boundary", timestamp: now.Add(-window)},
		{name: "future boundary", timestamp: now.Add(window)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePayloadFreshness(now, tt.timestamp.UnixNano(), window, window); err != nil {
				t.Fatalf("expected timestamp %s to be accepted: %v", tt.timestamp, err)
			}
		})
	}
}

func TestFreshnessRejectsOldTimestamp(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	window := 5 * time.Second
	timestamp := now.Add(-window - time.Nanosecond)

	if err := validatePayloadFreshness(now, timestamp.UnixNano(), window, window); err == nil {
		t.Fatal("expected timestamp older than the past window to be rejected")
	}
}

func TestFreshnessRejectsFutureTimestamp(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	window := 5 * time.Second
	timestamp := now.Add(window + time.Nanosecond)

	if err := validatePayloadFreshness(now, timestamp.UnixNano(), window, window); err == nil {
		t.Fatal("expected timestamp newer than the future skew to be rejected")
	}
}

func TestFreshnessRejectsInvalidWindows(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)

	tests := []struct {
		name       string
		pastWindow time.Duration
		futureSkew time.Duration
	}{
		{name: "zero past window", pastWindow: 0, futureSkew: time.Second},
		{name: "negative past window", pastWindow: -time.Second, futureSkew: time.Second},
		{name: "negative future skew", pastWindow: time.Second, futureSkew: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePayloadFreshness(now, now.UnixNano(), tt.pastWindow, tt.futureSkew); err == nil {
				t.Fatal("expected invalid freshness window to be rejected")
			}
		})
	}
}

func TestReplayCacheCoversFutureSkew(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	window := 5 * time.Second
	guard := time.Second
	timestamp := now.Add(window)

	expiration, err := replayCacheExpiration(now, timestamp.UnixNano(), window, guard)
	if err != nil {
		t.Fatalf("unexpected expiration error: %v", err)
	}

	want := now.Add(2*window + guard)
	if !expiration.Equal(want) {
		t.Fatalf("replay expiration = %s, want %s", expiration, want)
	}

	replayAttempt := now.Add(window + time.Second)
	if !expiration.After(replayAttempt) {
		t.Fatalf("signature expires at %s before delayed replay attempt at %s", expiration, replayAttempt)
	}
}

func TestReplayCacheExpirationUsesPayloadTimestamp(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	window := 5 * time.Second
	guard := time.Second

	currentExpiration, err := replayCacheExpiration(now, now.UnixNano(), window, guard)
	if err != nil {
		t.Fatalf("current timestamp expiration: %v", err)
	}
	if want := now.Add(window + guard); !currentExpiration.Equal(want) {
		t.Fatalf("current expiration = %s, want %s", currentExpiration, want)
	}

	oldBoundary := now.Add(-window)
	oldExpiration, err := replayCacheExpiration(now, oldBoundary.UnixNano(), window, guard)
	if err != nil {
		t.Fatalf("old boundary expiration: %v", err)
	}
	if want := now.Add(guard); !oldExpiration.Equal(want) {
		t.Fatalf("old boundary expiration = %s, want %s", oldExpiration, want)
	}
}

func TestReplayWindowDurationRejectsOutOfRangeValues(t *testing.T) {
	tests := []int{-1, 0, config.MaxReplayWindowSeconds + 1}
	for _, seconds := range tests {
		if duration, err := replayWindowDuration(seconds); err == nil {
			t.Fatalf("expected %d seconds to be rejected, got %s", seconds, duration)
		}
	}
}

func TestReplayWindowChangesRequireRestart(t *testing.T) {
	tests := []struct {
		name string
		next int
	}{
		{name: "increase", next: 60},
		{name: "decrease", next: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := &config.Config{Security: config.Security{ReplayWindowSeconds: 5}}
			next := &config.Config{Security: config.Security{ReplayWindowSeconds: tt.next}}

			if !preserveReplayWindowOnReload(current, next) {
				t.Fatal("expected replay window change to require restart")
			}
			if next.Security.ReplayWindowSeconds != current.Security.ReplayWindowSeconds {
				t.Fatalf("hot reload applied replay window %d", next.Security.ReplayWindowSeconds)
			}
		})
	}
}

func TestReplayCheckAndStoreIsAtomic(t *testing.T) {
	server := &Server{signaturesCache: make(map[string]time.Time)}
	signature := []byte("same-signature")
	expiration := time.Now().Add(time.Minute)

	const workers = 64
	var stored int32
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if server.storeSignatureIfNew(signature, expiration) {
				atomic.AddInt32(&stored, 1)
			}
		}()
	}
	wg.Wait()

	if stored != 1 {
		t.Fatalf("signature stored %d times, want exactly 1", stored)
	}
}

func TestReplayCleanerKeepsSignatureThroughAcceptanceWindow(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	window := 5 * time.Second
	timestamp := now.Add(window)
	expiration, err := replayCacheExpiration(now, timestamp.UnixNano(), window, time.Second)
	if err != nil {
		t.Fatalf("calculate expiration: %v", err)
	}

	cache := map[string]time.Time{"signature": expiration}
	replayAttempt := now.Add(window + time.Second)
	if purged := purgeExpiredSignatures(cache, replayAttempt); purged != 0 {
		t.Fatalf("cleaner purged %d signatures while packet was still acceptable", purged)
	}
	if purged := purgeExpiredSignatures(cache, expiration); purged != 1 {
		t.Fatalf("cleaner purged %d signatures at expiration, want 1", purged)
	}
}

func TestProcessKnockRejectsOutOfWindowTimestampBeforeReplayStore(t *testing.T) {
	tests := []struct {
		name      string
		timestamp time.Time
	}{
		{name: "old", timestamp: time.Now().Add(-10 * time.Second)},
		{name: "future", timestamp: time.Now().Add(10 * time.Second)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, packet := newTimestampTestServerAndPacket(t, tt.timestamp)
			marker := filepath.Join(t.TempDir(), "executed")
			enableMarkerAction(t, server, marker)

			server.processKnock(packet)
			server.executionWg.Wait()

			if len(server.signaturesCache) != 0 {
				t.Fatalf("%s packet was stored in replay cache", tt.name)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("%s packet executed the action", tt.name)
			}
		})
	}
}

func TestProcessKnockReplayCacheCoversFutureSkew(t *testing.T) {
	start := time.Now()
	window := 5 * time.Second
	server, packet := newTimestampTestServerAndPacket(t, start.Add(window))

	server.processKnock(packet)

	signature := string(packet.Payload[:ed25519.SignatureSize])
	expiration, exists := server.signaturesCache[signature]
	if !exists {
		t.Fatal("fresh future-dated packet was not stored in replay cache")
	}
	if !expiration.After(start.Add(window + time.Second)) {
		t.Fatalf("signature expires at %s before delayed replay time", expiration)
	}

	server.processKnock(packet)
	if got := server.signaturesCache[signature]; !got.Equal(expiration) {
		t.Fatalf("immediate replay changed cache expiration from %s to %s", expiration, got)
	}
}

func TestConcurrentProcessKnockExecutesOnce(t *testing.T) {
	server, packet := newTimestampTestServerAndPacket(t, time.Now())
	marker := filepath.Join(t.TempDir(), "executions")
	enableMarkerAction(t, server, marker)

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			server.processKnock(packet)
		}()
	}
	wg.Wait()
	server.executionWg.Wait()

	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read execution marker: %v", err)
	}
	if string(content) != "x" {
		t.Fatalf("action output = %q, want exactly one execution", content)
	}
}

func TestProcessKnockRejectsOTPEnvironmentCollision(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("decode TOTP secret: %v", err)
	}
	code := generateTOTP(key, time.Now().Unix()/30)

	tests := []struct {
		name       string
		params     map[string]string
		totpSecret string
	}{
		{
			name:       "verified otp collides with uppercase key",
			params:     map[string]string{"otp": code, "OTP": "attacker"},
			totpSecret: secret,
		},
		{
			name:   "reserved uppercase otp without totp",
			params: map[string]string{"OTP": "attacker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, packet := newTimestampTestServerAndPacketWithParams(t, time.Now(), tt.params)
			server.config.Users[0].TotpSecret = tt.totpSecret
			marker := filepath.Join(t.TempDir(), "executed")
			enableMarkerAction(t, server, marker)

			server.processKnock(packet)
			server.executionWg.Wait()

			if len(server.signaturesCache) != 0 {
				t.Fatal("rejected OTP packet was stored in replay cache")
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatal("rejected OTP packet executed the action")
			}
		})
	}
}

func enableMarkerAction(t *testing.T, server *Server, marker string) {
	t.Helper()

	command := "printf x >> " + marker
	commandTemplate, err := template.New("marker").Parse(command)
	if err != nil {
		t.Fatalf("parse marker command: %v", err)
	}
	server.config.Actions["test-action"] = config.Action{
		Command:        command,
		CommandTmpl:    commandTemplate,
		TimeoutSeconds: 2,
	}
	server.config.Daemon = config.Daemon{
		ShellPath: "/bin/sh",
		ShellFlag: "-c",
	}
}

func newTimestampTestServerAndPacket(t *testing.T, timestamp time.Time) (*Server, listener.PacketInfo) {
	return newTimestampTestServerAndPacketWithParams(t, timestamp, nil)
}

func newTimestampTestServerAndPacketWithParams(t *testing.T, timestamp time.Time, params map[string]string) (*Server, listener.PacketInfo) {
	t.Helper()

	clientPublicKey, clientPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	serverPublicKey, serverPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}

	payload := &protocol.Payload{
		Timestamp: timestamp.UnixNano(),
		ActionID:  "test-action",
		Params:    params,
	}
	packetBytes, err := protocol.EncryptAndSign(payload, clientPrivateKey, serverPublicKey)
	if err != nil {
		t.Fatalf("encrypt and sign payload: %v", err)
	}

	cfg := &config.Config{
		Security: config.Security{
			ReplayWindowSeconds: 5,
			RateLimitPerSecond:  100,
			RateLimitBurst:      100,
		},
		Tuning: config.Tuning{
			MaxTrackedIPs:     100,
			EvictionBatchSize: 10,
		},
		Users: []config.User{
			{
				Name:             "test-user",
				DecodedPublicKey: clientPublicKey,
				AllowedActions:   []string{"test-action"},
			},
		},
		Actions: map[string]config.Action{},
	}

	server := &Server{
		config:           cfg,
		serverPrivateKey: serverPrivateKey,
		actionCooldowns:  make(map[string]time.Time),
		signaturesCache:  make(map[string]time.Time),
		ipLimiters:       make(map[string]*ipLimiter),
		executionSem:     make(chan struct{}, 1),
	}

	return server, listener.PacketInfo{
		Payload:  packetBytes,
		SourceIP: net.ParseIP("192.0.2.10"),
	}
}
