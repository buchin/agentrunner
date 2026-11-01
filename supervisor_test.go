package agentrunner

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestOpenLockAndAcquire(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Acquire first lock
	lock1, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("expected Acquire to succeed, got %v", err)
	}
	defer lock1.Close()

	// Verify active
	active, err := Active(lockPath)
	if err != nil {
		t.Fatalf("expected Active to succeed, got %v", err)
	}
	if !active {
		t.Errorf("expected lock to be active")
	}

	// Second acquire should fail with ErrActive
	lock2, err := Acquire(lockPath)
	if err != ErrActive {
		if lock2 != nil {
			lock2.Close()
		}
		t.Fatalf("expected ErrActive, got %v", err)
	}

	// Close first lock
	lock1.Close()

	// Should not be active anymore
	active, err = Active(lockPath)
	if err != nil {
		t.Fatalf("expected Active to succeed after close, got %v", err)
	}
	if active {
		t.Errorf("expected lock to NOT be active after release")
	}
}

func TestRunInvalidArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := Run([]string{"invalid"}, nil, nil, &stderr)
	if code != 2 {
		t.Errorf("expected code 2 for invalid args, got %d", code)
	}
}

func TestRunParentUnavailable(t *testing.T) {
	var stderr bytes.Buffer
	// Pass an expected parent pid of 999999 which does not match os.Getppid()
	code := Run([]string{"/tmp/test.lock", "999999", "--", "echo", "hi"}, nil, nil, &stderr)
	if code != 74 {
		t.Errorf("expected code 74 for dead parent, got %d", code)
	}
}

func TestRunSupervisorExecution(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "exec.lock")

	ppid := strconv.Itoa(os.Getppid())
	var stdout, stderr bytes.Buffer

	// Supervise an echo command with 5s timeout
	cfg := SupervisorConfig{
		Timeout:     5 * time.Second,
		GracePeriod: 1 * time.Second,
	}

	code := RunConfig(cfg, []string{lockPath, ppid, "--", "echo", "hello-supervised"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}

	if !bytes.Contains(stdout.Bytes(), []byte("hello-supervised")) {
		t.Errorf("expected output to contain 'hello-supervised', got %q", stdout.String())
	}
}
