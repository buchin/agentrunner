package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareTaskWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	workspacesDir := filepath.Join(tmpDir, "workspaces")

	cfg := WorkspaceConfig{
		TaskID:            "task-unit-test-1",
		ProjectID:         "proj-test-1",
		WorkspacesBaseDir: workspacesDir,
		BranchName:        "task/test-branch-1",
		DefaultBranch:     "main",
		AuthorName:        "Test Runner",
		AuthorEmail:       "runner@test.local",
	}

	ws, err := PrepareTaskWorkspace(context.Background(), "git", cfg)
	if err != nil {
		t.Fatalf("PrepareTaskWorkspace failed: %v", err)
	}

	if ws.Path != filepath.Join(workspacesDir, "task-unit-test-1") {
		t.Errorf("unexpected workspace path: %s", ws.Path)
	}

	// Verify .git exists
	if _, err := os.Stat(filepath.Join(ws.Path, ".git")); os.IsNotExist(err) {
		t.Errorf("expected .git directory to exist")
	}

	// Verify pre-push hook exists and is executable
	hookPath := filepath.Join(ws.Path, ".git", "hooks", "pre-push")
	fi, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("expected pre-push hook to exist: %v", err)
	}
	if fi.Mode()&0111 == 0 {
		t.Errorf("expected pre-push hook to be executable")
	}

	// Test resumption on existing workspace
	ws2, err := PrepareTaskWorkspace(context.Background(), "git", cfg)
	if err != nil {
		t.Fatalf("resuming PrepareTaskWorkspace failed: %v", err)
	}
	if ws2.Path != ws.Path {
		t.Errorf("resumed workspace path mismatch")
	}
}
