package agentrunner

import (
	"os"
	"strconv"
	"time"
)

const (
	DefaultBaseTimeout    = 25 * time.Minute // Attempt 1 (Base Run)
	DefaultRequeueTimeout = 35 * time.Minute // Attempt 2 (First Requeue / Gap Fixable)
	DefaultMaxCapTimeout  = 45 * time.Minute // Attempt 3+ (Final Requeue / Max Cap)
	DefaultGracePeriod    = 30 * time.Second // Termination tolerance grace period before SIGKILL
	MaxAttempts           = 3                // Maximum attempts allowed per task before review/blocked
)

// CalculateAdaptiveTimeout returns progressive adaptive timeout based on attempt count:
// - Attempt 1 (Base): 25m
// - Attempt 2 (First Requeue): 35m
// - Attempt 3+ (Final Requeue / Hard Cap): 45m
func CalculateAdaptiveTimeout(attemptNumber int) time.Duration {
	for _, envKey := range []string{
		"AGENT_TASK_TIMEOUT",
		"CUAN_TASK_TIMEOUT",
		"AGENT_TASK_TIMEOUT_OVERRIDE",
		"CUAN_TASK_TIMEOUT_OVERRIDE",
	} {
		if v := os.Getenv(envKey); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				return d
			}
		}
	}

	baseTimeout := DefaultBaseTimeout
	if v := os.Getenv("AGENT_TASK_TIMEOUT_BASE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			baseTimeout = d
		}
	} else if v := os.Getenv("CUAN_TASK_TIMEOUT_BASE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			baseTimeout = d
		}
	}

	requeueTimeout := DefaultRequeueTimeout
	if v := os.Getenv("AGENT_TASK_TIMEOUT_REQUEUE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			requeueTimeout = d
		}
	} else if v := os.Getenv("CUAN_TASK_TIMEOUT_REQUEUE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			requeueTimeout = d
		}
	}

	maxCapTimeout := DefaultMaxCapTimeout
	if v := os.Getenv("AGENT_TASK_TIMEOUT_MAX"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			maxCapTimeout = d
		}
	} else if v := os.Getenv("CUAN_TASK_TIMEOUT_MAX"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			maxCapTimeout = d
		}
	}

	switch {
	case attemptNumber <= 1:
		return baseTimeout
	case attemptNumber == 2:
		return requeueTimeout
	default:
		return maxCapTimeout
	}
}

// GetGracePeriod returns the grace period duration for two-stage termination (SIGTERM -> SIGKILL).
func GetGracePeriod() time.Duration {
	if v := os.Getenv("AGENT_TASK_GRACE_PERIOD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	if v := os.Getenv("CUAN_TASK_GRACE_PERIOD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return DefaultGracePeriod
}

// CanRequeue returns true if the attempt count has not exceeded the max attempts threshold (3).
func CanRequeue(attemptNumber int) bool {
	maxAtt := MaxAttempts
	if v := os.Getenv("AGENT_TASK_MAX_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxAtt = n
		}
	} else if v := os.Getenv("CUAN_TASK_MAX_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxAtt = n
		}
	}
	return attemptNumber < maxAtt
}
