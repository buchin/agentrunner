package agentrunner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func boundaryScript(t *testing.T, body string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return file
}
func TestCLIEnvironmentIsExplicitAndProviderArgumentsAreExact(t *testing.T) {
	t.Setenv("AMBIENT_PROVIDER_TOKEN", "must-not-leak")
	t.Setenv("HOME", "/private/ambient")
	binary := boundaryScript(t, `printf '%s|%s|%s\n' "${AMBIENT_PROVIDER_TOKEN-unset}" "${HOME-unset}" "$ACCOUNT_ID"; printf '%s\n' "$@"`)
	result, err := NewCLIAdapter("fixture", binary).Execute(context.Background(), ExecutionRequest{Args: []string{"--provider-mode", "message"}, Env: map[string]string{"ACCOUNT_ID": "owned-account"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Stdout, "unset|unset|owned-account\n--provider-mode\nmessage\n") {
		t.Fatalf("environment/args contract violated: %q", result.Stdout)
	}
}
func TestCLIRequestTimeoutAndVersionDeadline(t *testing.T) {
	binary := boundaryScript(t, "sleep 20\n")
	runner := NewCLIAdapter("fixture", binary)
	started := time.Now()
	result, err := runner.Execute(context.Background(), ExecutionRequest{Prompt: "wait", Timeout: 40 * time.Millisecond})
	if err != nil || result.Status != "TIMEOUT" || time.Since(started) > 2*time.Second {
		t.Fatal("request timeout not enforced", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := runner.VersionContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("version probe ignored deadline: %v", err)
	}
}
func TestCLIBoundedOutputDrainsBothStreams(t *testing.T) {
	binary := boundaryScript(t, `i=0; while [ "$i" -lt 500 ]; do printf 'abcdefghij'; printf 'stderr-line' >&2; i=$((i+1)); done`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := NewCLIAdapter("fixture", binary).Execute(ctx, ExecutionRequest{Prompt: "output", MaxOutputBytes: 128})
	if err != nil || result.Status != "SUCCESS" {
		t.Fatal("bounded writer did not drain", err)
	}
	if len(result.Stdout) > 128 || len(result.Stderr) > 128 || !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatal("output capture exceeded its bounds")
	}
}
func TestCLILegacyPermissionBypassRequiresExplicitOptIn(t *testing.T) {
	binary := boundaryScript(t, `printf '%s\n' "$@"`)
	runner := NewCLIAdapter("fixture", binary)
	for _, optIn := range []bool{false, true} {
		result, err := runner.Execute(context.Background(), ExecutionRequest{Prompt: "do work", UnsafeSkipPermissions: optIn})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(result.Stdout, "--dangerously-skip-permissions") != optIn {
			t.Fatal("permission bypass did not match explicit request")
		}
	}
}
func TestTruncatedOutputDoesNotExposePartialKnownSecret(t *testing.T) {
	for _, limit := range []int{1, 4, 8, 12} {
		output := &boundedOutput{limit: limit}
		output.Write([]byte("secret-long-value-after-boundary"))
		safe, truncated := output.sanitized([]string{"secret-long-value"})
		if !truncated || len(safe) > limit || strings.HasPrefix(safe, "s") {
			t.Fatalf("partial secret leaked at limit %d: %q", limit, safe)
		}
	}
	output := &boundedOutput{limit: 64}
	output.Write([]byte("safe secret-long-value"))
	safe, _ := output.sanitized([]string{"secret-long-value"})
	if strings.Contains(safe, "secret-long-value") {
		t.Fatal("full secret not masked")
	}
}
func TestCLIInvalidLimitsAndCanceledContextDoNotStartChild(t *testing.T) {
	binary := boundaryScript(t, "exit 0\n")
	runner := NewCLIAdapter("fixture", binary)
	for _, req := range []ExecutionRequest{{Prompt: "x", MaxOutputBytes: -1}, {Prompt: "x", MaxOutputBytes: MaxOutputBytes + 1}, {Prompt: "x", Timeout: -1}, {Prompt: "x", Env: map[string]string{"BAD=KEY": "x"}}} {
		if _, err := runner.Execute(context.Background(), req); err == nil {
			t.Fatal("invalid execution accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runner.Execute(ctx, ExecutionRequest{Prompt: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request started a child")
	}
}

func TestCLITruncatedCaptureCannotSupplyStructuredEvidence(t *testing.T) {
	binary := boundaryScript(t, `printf '{"status":"SUCCESS","response":"claimed"}'; i=0; while [ "$i" -lt 100 ]; do printf ' '; i=$((i+1)); done`)
	result, err := NewCLIAdapter("fixture", binary).Execute(context.Background(), ExecutionRequest{Prompt: "output", MaxOutputBytes: 64})
	if err != nil || !result.StdoutTruncated || result.StructuredOutput != nil {
		t.Fatal("truncated evidence was treated as complete", err)
	}
}
