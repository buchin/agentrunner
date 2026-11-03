package agentrunner

import (
	"context"
	"time"
)

// ExecutionRequest represents the task execution parameters for an agent adapter.
type ExecutionRequest struct {
	// Args, when non-nil, are exact provider-specific arguments. They override
	// the compatibility prompt/model/output-format argument builder.
	Args []string `json:"args,omitempty"`
	// UnsafeSkipPermissions explicitly opts into the legacy provider flag.
	UnsafeSkipPermissions bool `json:"unsafe_skip_permissions,omitempty"`
	// MaxOutputBytes bounds each captured stream; zero uses 1 MiB, maximum 16 MiB.
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
	TaskID         string            `json:"task_id,omitempty"`
	Prompt         string            `json:"prompt"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Secrets        []string          `json:"secrets,omitempty"`
	Timeout        time.Duration     `json:"timeout,omitempty"`
	Model          string            `json:"model,omitempty"`
	OutputFormat   string            `json:"output_format,omitempty"` // "json" or "text"
}

// TokenUsage holds token usage metrics for an agent run.
type TokenUsage struct {
	InputTokens    int64 `json:"input_tokens,omitempty"`
	OutputTokens   int64 `json:"output_tokens,omitempty"`
	ThinkingTokens int64 `json:"thinking_tokens,omitempty"`
	TotalTokens    int64 `json:"total_tokens,omitempty"`
}

// StructuredOutput represents parsed machine-readable output from the agent.
type StructuredOutput struct {
	ConversationID string     `json:"conversation_id,omitempty"`
	Status         string     `json:"status,omitempty"`
	Response       string     `json:"response,omitempty"`
	DurationSec    float64    `json:"duration_seconds,omitempty"`
	NumTurns       int        `json:"num_turns,omitempty"`
	Usage          TokenUsage `json:"usage,omitempty"`
}

// ExecutionResult represents the execution outcome of an agent run.
type ExecutionResult struct {
	StdoutTruncated  bool              `json:"stdout_truncated,omitempty"`
	StderrTruncated  bool              `json:"stderr_truncated,omitempty"`
	Status           string            `json:"status"` // SUCCESS, FAILED, CANCELED, TIMEOUT
	ExitCode         int               `json:"exit_code"`
	Stdout           string            `json:"stdout"`
	Stderr           string            `json:"stderr"`
	StructuredOutput *StructuredOutput `json:"structured_output,omitempty"`
	Duration         time.Duration     `json:"duration"`
	PeakRSSBytes     int64             `json:"peak_rss_bytes"`
	Error            string            `json:"error,omitempty"`
}

// Adapter defines the interface for running coding agents in headless mode.
type Adapter interface {
	Name() string
	Version() (string, error)
	Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error)
}
