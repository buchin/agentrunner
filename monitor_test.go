package agentrunner

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestReadRSSAndMonitor(t *testing.T) {
	pid := os.Getpid()
	rss := ReadRSS(pid)
	if rss <= 0 {
		t.Errorf("expected positive RSS for current process, got %d", rss)
	}

	monitor := StartProcessMonitor(pid, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	peak := monitor.Stop()

	if peak <= 0 {
		t.Errorf("expected peak RSS to be positive, got %d", peak)
	}
}

func TestIsProcessAlive(t *testing.T) {
	if !IsProcessAlive(os.Getpid()) {
		t.Errorf("current process should be alive")
	}

	if IsProcessAlive(-1) {
		t.Errorf("negative PID should not be alive")
	}
}

func TestKillProcessTree(t *testing.T) {
	// Start a dummy sleep process
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	pid := cmd.Process.Pid

	if !IsProcessAlive(pid) {
		t.Fatalf("sleep process should be alive")
	}

	if err := KillProcessTree(pid); err != nil {
		t.Fatalf("failed to kill process tree: %v", err)
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("process did not exit within 1s after KillProcessTree")
	}

	if IsProcessAlive(pid) {
		t.Errorf("process %d should be dead after KillProcessTree", pid)
	}
}
