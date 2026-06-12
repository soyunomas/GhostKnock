package executor

import (
	"bytes"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/soyunomas/ghostknock/internal/config"
)

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
	}{
		{name: "nil", params: nil},
		{name: "empty", params: map[string]string{}},
		{name: "valid", params: map[string]string{"target_1": "abc-123.example"}},
		{name: "invalid command substitution", params: map[string]string{"target": "$(touch_tmp_pwn)"}, wantErr: true},
		{name: "invalid space", params: map[string]string{"target": "abc def"}, wantErr: true},
		{name: "invalid leading hyphen", params: map[string]string{"target": "--help"}, wantErr: true},
		{name: "invalid parent directory", params: map[string]string{"target": ".."}, wantErr: true},
		{name: "invalid key hyphen", params: map[string]string{"bad-key": "value"}, wantErr: true},
		{name: "invalid key equals", params: map[string]string{"x=evil": "value"}, wantErr: true},
		{name: "invalid key first digit", params: map[string]string{"1bad": "value"}, wantErr: true},
		{name: "case insensitive collision", params: map[string]string{"target": "one", "TARGET": "two"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParams(tt.params)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestExecuteRejectsInvalidSensitiveParamNames(t *testing.T) {
	tests := [][]string{
		{"bad-key"},
		{"token", "TOKEN"},
	}
	for _, sensitive := range tests {
		action := testAction(t, "true")
		action.SensitiveParams = sensitive
		err := Execute(action, "tester", net.ParseIP("192.0.2.1"), nil, config.Hooks{}, testDaemon())
		if err == nil {
			t.Fatalf("expected sensitive_params %v to be rejected", sensitive)
		}
	}
}

func TestExecuteRejectsInvalidParamsBeforeHooks(t *testing.T) {
	invalidParams := []map[string]string{
		{"target": "$(touch /tmp/pwn)"},
		{"target": "abc def"},
		{"target": "--help"},
		{"bad-key": "value"},
		{"x=evil": "value"},
	}

	for _, hookType := range []string{"global", "action"} {
		t.Run(hookType, func(t *testing.T) {
			for i, params := range invalidParams {
				t.Run(string(rune('a'+i)), func(t *testing.T) {
					tempDir := t.TempDir()
					hookMarker := filepath.Join(tempDir, "hook-ran")
					hook := writeHook(t, tempDir, "hook.sh", "printf reached > "+hookMarker+"\n")

					action := testAction(t, "true")
					var globalHooks config.Hooks
					if hookType == "global" {
						globalHooks.PreExecute = hook
					} else {
						action.PreHook = hook
					}

					err := Execute(action, "tester", net.ParseIP("192.0.2.1"), params, globalHooks, testDaemon())
					if err == nil {
						t.Fatal("expected invalid params to cancel execution")
					}
					if _, statErr := os.Stat(hookMarker); !os.IsNotExist(statErr) {
						t.Fatal("hook executed with invalid params")
					}
				})
			}
		})
	}
}

func TestExecuteRejectsMissingRequiredParamBeforeHooks(t *testing.T) {
	tempDir := t.TempDir()
	hookMarker := filepath.Join(tempDir, "hook-ran")
	hook := writeHook(t, tempDir, "hook.sh", "printf reached > "+hookMarker+"\n")

	action := testAction(t, "printf '%s' {{ .Params.required }}")
	action.PreHook = hook
	err := Execute(action, "tester", net.ParseIP("192.0.2.1"), nil, config.Hooks{}, testDaemon())
	if err == nil {
		t.Fatal("expected missing required param to cancel execution")
	}
	if _, statErr := os.Stat(hookMarker); !os.IsNotExist(statErr) {
		t.Fatal("hook executed before required params were validated")
	}
}

func TestExecuteRejectsDynamicParamAccessBeforeHooks(t *testing.T) {
	tempDir := t.TempDir()
	hookMarker := filepath.Join(tempDir, "hook-ran")
	hook := writeHook(t, tempDir, "hook.sh", "printf reached > "+hookMarker+"\n")

	action := testAction(t, `printf '%s' {{index .Params "required"}}`)
	action.PreHook = hook
	err := Execute(
		action,
		"tester",
		net.ParseIP("192.0.2.1"),
		map[string]string{"required": "value"},
		config.Hooks{},
		testDaemon(),
	)
	if err == nil {
		t.Fatal("expected dynamic Params access to be rejected")
	}
	if _, statErr := os.Stat(hookMarker); !os.IsNotExist(statErr) {
		t.Fatal("hook executed before dynamic Params access was rejected")
	}
}

func TestExecuteValidParamsReachHook(t *testing.T) {
	tempDir := t.TempDir()
	hookMarker := filepath.Join(tempDir, "hook-value")
	hook := writeHook(t, tempDir, "hook.sh", "printf '%s' \"$GK_PARAM_TARGET\" > "+hookMarker+"\n")

	action := testAction(t, "true")
	action.PreHook = hook
	if err := Execute(
		action,
		"tester",
		net.ParseIP("192.0.2.1"),
		map[string]string{"target": "safe-value"},
		config.Hooks{},
		testDaemon(),
	); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	content, err := os.ReadFile(hookMarker)
	if err != nil {
		t.Fatalf("read hook marker: %v", err)
	}
	if string(content) != "safe-value" {
		t.Fatalf("hook received %q, want safe-value", content)
	}
}

func TestHookFailureCancelsAction(t *testing.T) {
	tempDir := t.TempDir()
	commandMarker := filepath.Join(tempDir, "command-ran")
	hook := writeHook(t, tempDir, "hook.sh", "exit 1\n")

	action := testAction(t, "printf reached > "+commandMarker)
	action.PreHook = hook
	err := Execute(action, "tester", net.ParseIP("192.0.2.1"), nil, config.Hooks{}, testDaemon())
	if err == nil {
		t.Fatal("expected failing hook to cancel action")
	}
	if _, statErr := os.Stat(commandMarker); !os.IsNotExist(statErr) {
		t.Fatal("command executed after hook failure")
	}
}

func TestRunHookRejectsInvalidParamsDirectly(t *testing.T) {
	tempDir := t.TempDir()
	hookMarker := filepath.Join(tempDir, "hook-ran")
	hook := writeHook(t, tempDir, "hook.sh", "printf reached > "+hookMarker+"\n")

	err := RunHook(hook, HookContext{Params: map[string]string{"bad-key": "value"}})
	if err == nil {
		t.Fatal("expected RunHook to reject invalid params")
	}
	if _, statErr := os.Stat(hookMarker); !os.IsNotExist(statErr) {
		t.Fatal("direct RunHook call executed with invalid params")
	}
}

func TestHookOutputRedactsSensitiveParams(t *testing.T) {
	for _, exitLine := range []string{"exit 0", "exit 1"} {
		t.Run(exitLine, func(t *testing.T) {
			tempDir := t.TempDir()
			hook := writeHook(t, tempDir, "hook.sh", "printf '%s' \"$GK_PARAM_TOKEN\"\n"+exitLine+"\n")

			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() {
				slog.SetDefault(previousLogger)
			})

			action := testAction(t, "true")
			action.PreHook = hook
			action.SensitiveParams = []string{"token"}
			_ = Execute(
				action,
				"tester",
				net.ParseIP("192.0.2.1"),
				map[string]string{"Token": "supersecret"},
				config.Hooks{},
				testDaemon(),
			)

			if strings.Contains(logs.String(), "supersecret") {
				t.Fatalf("sensitive hook output leaked in logs: %s", logs.String())
			}
			if !strings.Contains(logs.String(), "*****") {
				t.Fatalf("redaction marker missing from hook logs: %s", logs.String())
			}
		})
	}
}

func TestCommandOutputRedactsSensitiveParams(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	action := testAction(t, "printf '%s' {{.Params.Token}}; printf '%s' {{.Params.Token}} >&2; exit 1")
	action.SensitiveParams = []string{"token"}
	err := Execute(
		action,
		"tester",
		net.ParseIP("192.0.2.1"),
		map[string]string{"Token": "supersecret"},
		config.Hooks{},
		testDaemon(),
	)
	if err == nil {
		t.Fatal("expected command failure")
	}
	if strings.Contains(logs.String(), "supersecret") || strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("sensitive command output leaked: logs=%s error=%v", logs.String(), err)
	}
	if !strings.Contains(logs.String(), "*****") || !strings.Contains(err.Error(), "*****") {
		t.Fatalf("redaction marker missing: logs=%s error=%v", logs.String(), err)
	}
}

func TestPostHookUsesValidatedParamSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	hookMarker := filepath.Join(tempDir, "post-value")
	hook := writeHook(t, tempDir, "hook.sh", "sleep 0.05\nprintf '%s' \"$GK_PARAM_TARGET\" > "+hookMarker+"\n")

	params := map[string]string{"target": "validated"}
	action := testAction(t, "true")
	action.PostHook = hook
	if err := Execute(action, "tester", net.ParseIP("192.0.2.1"), params, config.Hooks{}, testDaemon()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	params["target"] = "mutated"

	deadline := time.Now().Add(2 * time.Second)
	for {
		content, err := os.ReadFile(hookMarker)
		if err == nil {
			if string(content) != "validated" {
				t.Fatalf("post-hook received %q, want validated snapshot", content)
			}
			return
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read post-hook marker: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("post-hook did not produce marker")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRevertCommandAndHookUseParamSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	commandMarker := filepath.Join(tempDir, "revert-command")
	hookMarker := filepath.Join(tempDir, "revert-hook")
	hook := writeHook(t, tempDir, "hook.sh", "printf '%s' \"$GK_PARAM_TARGET\" > "+hookMarker+"\n")

	revertCommand := "printf '%s' {{.Params.target}} > " + commandMarker
	revertTemplate, err := template.New("revert").Parse(revertCommand)
	if err != nil {
		t.Fatalf("parse revert template: %v", err)
	}
	action := config.Action{
		RevertCommand:     revertCommand,
		RevertCommandTmpl: revertTemplate,
		RevertHook:        hook,
		TimeoutSeconds:    2,
	}

	params := map[string]string{"target": "validated"}
	snapshot := cloneParams(params)
	params["target"] = "mutated"
	scheduleRevert(
		action,
		"tester",
		net.ParseIP("192.0.2.1"),
		snapshot,
		config.Hooks{},
		testDaemon(),
	)

	for _, marker := range []string{commandMarker, hookMarker} {
		content, err := os.ReadFile(marker)
		if err != nil {
			t.Fatalf("read marker %s: %v", marker, err)
		}
		if string(content) != "validated" {
			t.Fatalf("marker %s contains %q, want validated", marker, content)
		}
	}
}

func testAction(t *testing.T, command string) config.Action {
	t.Helper()
	tmpl, err := template.New("test-command").Parse(command)
	if err != nil {
		t.Fatalf("parse command template: %v", err)
	}
	return config.Action{
		Command:        command,
		CommandTmpl:    tmpl,
		TimeoutSeconds: 2,
	}
}

func testDaemon() config.Daemon {
	return config.Daemon{ShellPath: "/bin/sh", ShellFlag: "-c"}
}

func writeHook(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\nset -eu\n" + body
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatalf("write hook: %v", err)
	}
	return path
}
