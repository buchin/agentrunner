package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIAdapterExecution(t *testing.T) {
	// Create a mock script that outputs structured JSON
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-agent.sh")
	mockScript := `#!/bin/sh
echo '{"status": "SUCCESS", "response": "Task completed successfully", "duration_seconds": 1.5, "usage": {"total_tokens": 120}}'
exit 0
`
	if err := os.WriteFile(mockBin, []byte(mockScript), 0755); err != nil {
		t.Fatalf("failed to create mock agent: %v", err)
	}

	adapter := NewCLIAdapter("mock", mockBin)
	if adapter.Name() != "mock" {
		t.Errorf("expected adapter name 'mock', got %s", adapter.Name())
	}

	req := ExecutionRequest{
		Prompt:       "Fix user authentication bug",
		OutputFormat: "json",
	}

	res, err := adapter.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if res.Status != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", res.Status)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if res.StructuredOutput == nil {
		t.Fatalf("expected structured output")
	}
	if res.StructuredOutput.Response != "Task completed successfully" {
		t.Errorf("unexpected response: %s", res.StructuredOutput.Response)
	}
	if res.StructuredOutput.Usage.TotalTokens != 120 {
		t.Errorf("unexpected total tokens: %d", res.StructuredOutput.Usage.TotalTokens)
	}
}

func TestCLIAdapterTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-sleep.sh")
	mockScript := `#!/bin/sh
sleep 10
exit 0
`
	if err := os.WriteFile(mockBin, []byte(mockScript), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	adapter := NewCLIAdapter("mock-sleep", mockBin)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := ExecutionRequest{
		Prompt: "Run slow command",
	}

	res, err := adapter.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "TIMEOUT" {
		t.Errorf("expected TIMEOUT status, got %s", res.Status)
	}
}
