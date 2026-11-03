package agentrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// CLIAdapter executes CLI-based AI agents with process group isolation,
// timeout handling, secret masking, and RSS monitoring.
type CLIAdapter struct {
	name       string
	binaryPath string
}

// NewCLIAdapter creates an adapter for the specified agent CLI binary.
func NewCLIAdapter(name, binaryPath string) *CLIAdapter {
	if name == "" {
		name = "agent-cli"
	}
	return &CLIAdapter{
		name:       name,
		binaryPath: binaryPath,
	}
}

func (a *CLIAdapter) Name() string {
	return a.name
}

// Version bounds the compatibility probe to five seconds.
func (a *CLIAdapter) Version() (string, error) { return a.VersionContext(context.Background()) }
func (a *CLIAdapter) VersionContext(ctx context.Context) (string, error) {
	if a.binaryPath == "" {
		return "", errors.New("binary path not specified")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.binaryPath, "--version")
	cmd.Env = executionEnvironment(nil)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return KillProcessTree(cmd.Process.Pid)
		}
		return nil
	}
	cmd.WaitDelay = time.Second
	output := &boundedOutput{limit: 64 << 10}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrWaitDelay) && cmd.Process != nil {
			_ = KillProcessTree(cmd.Process.Pid)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("execute --version: %w", err)
	}
	value, _ := output.sanitized(nil)
	return strings.TrimSpace(value), nil
}

// Execute runs the CLI agent with the provided execution parameters.
func (a *CLIAdapter) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if a.binaryPath == "" {
		return nil, errors.New("agent binary path is empty")
	}
	if req.Prompt == "" && req.Args == nil {
		return nil, errors.New("prompt is required")
	}

	if !validEnvironment(req.Env) {
		return nil, errors.New("invalid execution environment")
	}
	if req.MaxOutputBytes < 0 || req.MaxOutputBytes > MaxOutputBytes {
		return nil, errors.New("max output bytes must be between 0 and 16 MiB")
	}
	if req.Timeout < 0 {
		return nil, errors.New("timeout must not be negative")
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.MaxOutputBytes
	if limit == 0 {
		limit = DefaultMaxOutputBytes
	}
	outputFormat := req.OutputFormat
	if outputFormat == "" {
		outputFormat = "json"
	}

	args := []string{
		"-p", req.Prompt,
		"--output-format", outputFormat,
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	if req.Args != nil {
		args = append([]string{}, req.Args...)
	} else if req.UnsafeSkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	cmdCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, a.binaryPath, args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	cmd.Env = executionEnvironment(req.Env)

	// Setup process group for clean cancellation
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second

	stdoutBuf, stderrBuf := boundedOutput{limit: limit}, boundedOutput{limit: limit}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent process: %w", err)
	}

	pid := cmd.Process.Pid
	monitor := StartProcessMonitor(pid, 20*time.Millisecond)

	doneChan := make(chan error, 1)
	go func() {
		doneChan <- cmd.Wait()
	}()

	var waitErr error
	var canceled bool
	var timedOut bool

	select {
	case <-ctx.Done():
		canceled = true
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			timedOut = true
		}
		if pid > 0 {
			_ = KillProcessTree(pid)
		}
		waitErr = <-doneChan

	case waitErr = <-doneChan:
		if errors.Is(waitErr, exec.ErrWaitDelay) {
			_ = KillProcessTree(pid)
		}
	}

	duration := time.Since(startTime)
	peakRSS := monitor.Stop()

	safeStdout, stdoutTruncated := stdoutBuf.sanitized(req.Secrets)
	safeStderr, stderrTruncated := stderrBuf.sanitized(req.Secrets)

	result := &ExecutionResult{
		StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated,
		Duration:     duration,
		PeakRSSBytes: peakRSS,
		Stdout:       safeStdout,
		Stderr:       safeStderr,
	}

	if canceled {
		if timedOut {
			result.Status = "TIMEOUT"
			result.Error = "execution timed out"
		} else {
			result.Status = "CANCELED"
			result.Error = "execution canceled by context"
		}
		result.ExitCode = -1
		return result, nil
	}

	if waitErr != nil {
		result.Status = "FAILED"
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
		result.Error = waitErr.Error()
	} else {
		result.Status = "SUCCESS"
		result.ExitCode = 0
	}

	// Parse structured JSON output if present
	if outputFormat == "json" && !stdoutTruncated && len(safeStdout) > 0 {
		var structured StructuredOutput
		trimmed := strings.TrimSpace(safeStdout)
		if idx := strings.Index(trimmed, "{"); idx != -1 {
			trimmed = trimmed[idx:]
		}
		if err := json.Unmarshal([]byte(trimmed), &structured); err == nil {
			result.StructuredOutput = &structured
			if structured.Status != "" && result.Status == "SUCCESS" {
				result.Status = structured.Status
			}
		}
	}

	return result, nil
}
