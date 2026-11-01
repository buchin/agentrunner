package agentrunner

import (
	"os"
	"testing"
	"time"
)

func TestCalculateAdaptiveTimeout(t *testing.T) {
	os.Unsetenv("AGENT_TASK_TIMEOUT_OVERRIDE")
	os.Unsetenv("CUAN_TASK_TIMEOUT_OVERRIDE")
	os.Unsetenv("AGENT_TASK_TIMEOUT_BASE")
	os.Unsetenv("CUAN_TASK_TIMEOUT_BASE")
	os.Unsetenv("AGENT_TASK_TIMEOUT_REQUEUE")
	os.Unsetenv("CUAN_TASK_TIMEOUT_REQUEUE")
	os.Unsetenv("AGENT_TASK_TIMEOUT_MAX")
	os.Unsetenv("CUAN_TASK_TIMEOUT_MAX")

	// Attempt 1: 25 minutes
	if got := CalculateAdaptiveTimeout(1); got != 25*time.Minute {
		t.Errorf("Attempt 1 timeout = %v, want 25m", got)
	}

	// Attempt 2: 35 minutes
	if got := CalculateAdaptiveTimeout(2); got != 35*time.Minute {
		t.Errorf("Attempt 2 timeout = %v, want 35m", got)
	}

	// Attempt 3+: 45 minutes
	if got := CalculateAdaptiveTimeout(3); got != 45*time.Minute {
		t.Errorf("Attempt 3 timeout = %v, want 45m", got)
	}
	if got := CalculateAdaptiveTimeout(5); got != 45*time.Minute {
		t.Errorf("Attempt 5 timeout = %v, want 45m", got)
	}

	// Override check
	os.Setenv("AGENT_TASK_TIMEOUT_OVERRIDE", "10m")
	defer os.Unsetenv("AGENT_TASK_TIMEOUT_OVERRIDE")
	if got := CalculateAdaptiveTimeout(1); got != 10*time.Minute {
		t.Errorf("Override timeout = %v, want 10m", got)
	}
}

func TestGetGracePeriod(t *testing.T) {
	os.Unsetenv("AGENT_TASK_GRACE_PERIOD")
	os.Unsetenv("CUAN_TASK_GRACE_PERIOD")

	if got := GetGracePeriod(); got != 30*time.Second {
		t.Errorf("Default grace period = %v, want 30s", got)
	}

	os.Setenv("AGENT_TASK_GRACE_PERIOD", "15s")
	defer os.Unsetenv("AGENT_TASK_GRACE_PERIOD")
	if got := GetGracePeriod(); got != 15*time.Second {
		t.Errorf("Custom grace period = %v, want 15s", got)
	}
}

func TestCanRequeue(t *testing.T) {
	os.Unsetenv("AGENT_TASK_MAX_ATTEMPTS")
	os.Unsetenv("CUAN_TASK_MAX_ATTEMPTS")

	if !CanRequeue(1) {
		t.Errorf("attempt 1 should be requeueable")
	}
	if !CanRequeue(2) {
		t.Errorf("attempt 2 should be requeueable")
	}
	if CanRequeue(3) {
		t.Errorf("attempt 3 should not be requeueable with max 3")
	}
}
