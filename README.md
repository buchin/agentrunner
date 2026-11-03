# AgentRunner

[![Go Reference](https://pkg.go.dev/badge/github.com/buchin/agentrunner.svg)](https://pkg.go.dev/github.com/buchin/agentrunner)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Robust Agent Process Supervisor, Execution Isolator, and Task Workspace Runner for Autonomous AI Agents in Go.

Extracted and battle-tested from the [Cuan](https://github.com/buchin/cuan) autonomous business agent platform.

---

## Features

- **OS-Backed Process Supervision (`agentrunner.Supervisor`)**:
  - File-based non-blocking exclusive locking (`flock`) to prevent duplicate agent execution.
  - Automatic parent-death detection via Linux `Pdeathsig: syscall.SIGKILL` and periodic heartbeat check.
  - Background CPU priority demotion (`nice 19`) ensuring agent workloads never starve primary web or DB services.
  - Resource guardrails injection (`GOMEMLIMIT`, `GOMAXPROCS`, `GOGC`, `NODE_OPTIONS`).
  - Controlling PTY handoff via `unix.TIOCSPGRP` for interactive terminal sessions.
  - Two-stage timeout termination: graceful `SIGTERM` followed by `SIGKILL` escalation after grace period.

- **CLI Agent Adapter & Process Tree Management**:
  - Process group isolation (`Setpgid: true`) preventing orphaned background child processes.
  - `KillProcessTree`: cleans up process groups and recursively terminates lingering child PIDs via `/proc/<pid>/task/<pid>/children`.
  - Memory usage monitoring (`ProcessMonitor`) reading peak resident set size (RSS) via `/proc/<pid>/statm`.
  - Automatic secret masking (`MaskSecrets`, `SanitizeEnv`) for GitHub PATs, Bearer tokens, DB URIs, and API keys.

- **Task Workspace Isolation (`agentrunner.TaskWorkspace`)**:
  - Fast local clone from base repository with git branch checkout (`task/<task_id>`).
  - Git identity configuration (`user.name`, `user.email`).
  - Pre-push branch protection hook preventing accidental direct pushes to protected default branches (`main`/`master`).
  - Dependency caching and shared asset symlinking.

- **Progressive Adaptive Timeout**:
  - Multi-attempt timeout calculation:
    - Attempt 1 (Base): 25 minutes
    - Attempt 2 (Requeue / Gap Fix): 35 minutes
    - Attempt 3+ (Hard Cap): 45 minutes
  - Requeue threshold evaluation (`CanRequeue`).

---

## Installation

```bash
go get github.com/buchin/agentrunner
```

---

## Usage

### 1. Process Supervision & Execution Ownership

```go
package main

import (
    "fmt"
    "os"
    "github.com/buchin/agentrunner"
)

func main() {
    // Check if invoked as a supervisor re-exec
    if len(os.Args) > 1 && os.Args[1] == agentrunner.Command {
        os.Exit(agentrunner.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
    }

    // Acquire execution lock for a task
    lockFile, err := agentrunner.Acquire("/tmp/task-123.lock")
    if err != nil {
        if err == agentrunner.ErrActive {
            fmt.Println("Another agent process is already executing this task.")
            return
        }
        panic(err)
    }
    defer lockFile.Close()

    fmt.Println("Task lock acquired exclusively.")
}
```

### 2. Task Workspace Preparation & Git Guardrails

```go
package main

import (
    "context"
    "fmt"
    "github.com/buchin/agentrunner"
)

func main() {
    ctx := context.Background()

    ws, err := agentrunner.PrepareTaskWorkspace(ctx, "git", agentrunner.WorkspaceConfig{
        TaskID:            "task-abc-123",
        BaseRepoDir:       "/var/repos/main-repo",
        WorkspacesBaseDir: "/var/workspaces",
        BranchName:        "task/task-abc-123",
        DefaultBranch:     "main",
        AuthorName:        "Autonomous Agent",
        AuthorEmail:       "agent@mycompany.com",
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("Isolated workspace prepared at %s on branch %s\n", ws.Path, ws.BranchName)
}
```

### 3. Progressive Timeout Calculation

```go
package main

import (
    "fmt"
    "github.com/buchin/agentrunner"
)

func main() {
    attemptNumber := 1
    timeout := agentrunner.CalculateAdaptiveTimeout(attemptNumber)
    grace := agentrunner.GetGracePeriod()

    fmt.Printf("Attempt #%d timeout: %v, grace period: %v\n", attemptNumber, timeout, grace)
}
```

### 4. Running a CLI Agent with Resource Monitoring & Secret Redaction

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/buchin/agentrunner"
)

func main() {
    adapter := agentrunner.NewCLIAdapter("agy", "/usr/local/bin/agy")

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()

    result, err := adapter.Execute(ctx, agentrunner.ExecutionRequest{
        Prompt:     "Refactor database connection pool",
        WorkingDir: "/var/workspaces/task-abc-123",
        Secrets:    []string{"super-secret-db-password"},
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("Execution Status: %s (Exit Code: %d, Peak RSS: %d bytes)\n",
        result.Status, result.ExitCode, result.PeakRSSBytes)
}
```

---

## Running Tests

```bash
go test -v -race ./...
```

Targeted test suite completes in under 1 second.

---

## License

MIT License. See [LICENSE](LICENSE) for details.

## Execution boundary changes (next release)

The CLI adapter now inherits only `PATH`, locale, timezone, and terminal settings.
Set the account's `HOME`, credential paths, and any required provider variables
explicitly in `ExecutionRequest.Env`. Ambient application/provider tokens and SSH
agents are not passed through. This affects the CLI adapter; the low-level
supervisor re-exec entrypoint still expects its caller to construct an appropriate
control-plane environment. Do not substitute it for an account-isolation layer.

Each stdout/stderr capture is limited to 1 MiB by default. Set `MaxOutputBytes` to
a positive per-stream limit up to 16 MiB. Writes keep draining after the limit;
`StdoutTruncated` and `StderrTruncated` identify incomplete evidence. Structured
output should not be trusted when capture was truncated. Known secrets are also
masked when capture ends partway through one. File/stream delivery is a separate
consumer concern rather than unlimited in-memory capture.

Use `Args` for an exact provider-specific argument list (an empty, non-nil list is
valid). A nil list retains the prompt/model/output-format compatibility builder,
without automatically bypassing permissions. That provider-specific behavior
requires explicit `UnsafeSkipPermissions: true`; custom `Args` remain entirely
under the caller's control. `Timeout` is enforced in addition to the caller's
context deadline. `Version()` has a five-second deadline; `VersionContext(ctx)`
can impose an earlier limit and caps probe output to 64 KiB.

These are intentional behavior changes for consumers upgrading from v0.1.2.
Cuan must retain its own task authorization, saved execution deadline, account
profile isolation, lease fencing, atomic completion, and evidence validation.
The library's SUCCESS result is a process outcome, not acceptance of a task.
