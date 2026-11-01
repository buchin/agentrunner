package agentrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

func (a *CLIAdapter) Version() (string, error) {
	if a.binaryPath == "" {
		return "", errors.New("binary path not specified")
	}
	cmd := exec.Command(a.binaryPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("execute --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Execute runs the CLI agent with the provided execution parameters.
func (a *CLIAdapter) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if a.binaryPath == "" {
		return nil, errors.New("agent binary path is empty")
	}
	if req.Prompt == "" {
		return nil, errors.New("prompt is required")
	}

	outputFormat := req.OutputFormat
	if outputFormat == "" {
		outputFormat = "json"
	}

	args := []string{
		"-p", req.Prompt,
		"--output-format", outputFormat,
		"--dangerously-skip-permissions",
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	cmdCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, a.binaryPath, args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	env := os.Environ()
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Setup process group for clean cancellation
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdoutBuf, stderrBuf bytes.Buffer
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
	}

	duration := time.Since(startTime)
	peakRSS := monitor.Stop()

	safeStdout := MaskSecrets(stdoutBuf.String(), req.Secrets)
	safeStderr := MaskSecrets(stderrBuf.String(), req.Secrets)

	result := &ExecutionResult{
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
	if outputFormat == "json" && len(safeStdout) > 0 {
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
