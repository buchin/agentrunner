package agentrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	ErrDefaultBranchProtected = errors.New("cannot push directly to protected default branch (main/master). Changes must be pushed via task branch and reviewed through Pull Request")
	ErrWorkspaceEmpty         = errors.New("workspace directory is empty or invalid")
	ErrNoCommitsToPush        = errors.New("no commits to push on task branch")
)

// TaskWorkspace represents an isolated git checkout dedicated to a specific task.
type TaskWorkspace struct {
	TaskID        string `json:"task_id"`
	ProjectID     string `json:"project_id"`
	Path          string `json:"path"`
	BranchName    string `json:"branch_name"`
	DefaultBranch string `json:"default_branch"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
}

// WorkspaceConfig contains parameters needed to prepare an isolated task workspace.
type WorkspaceConfig struct {
	TaskID            string
	ProjectID         string
	BaseRepoDir       string
	WorkspacesBaseDir string
	BranchName        string
	DefaultBranch     string
	AuthorName        string
	AuthorEmail       string
	RemoteURL         string
	Token             string
}

// PrepareTaskWorkspace ensures an isolated workspace directory exists for the given task.
// If the workspace already exists, it checks out the task branch and resumes.
// If it is new, it clones from BaseRepoDir or RemoteURL (or inits empty repo),
// creates the task branch, configures git author identity, and installs pre-push protection.
func PrepareTaskWorkspace(ctx context.Context, gitBinary string, cfg WorkspaceConfig) (*TaskWorkspace, error) {
	if cfg.TaskID == "" {
		return nil, errors.New("task ID is required to prepare workspace")
	}

	if gitBinary == "" {
		gitBinary = "git"
	}

	branchName := cfg.BranchName
	if branchName == "" {
		branchName = fmt.Sprintf("task/%s", cfg.TaskID)
	}

	defaultBranch := cfg.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	authorName := cfg.AuthorName
	if authorName == "" {
		authorName = "Buchin"
	}

	authorEmail := cfg.AuthorEmail
	if authorEmail == "" {
		authorEmail = "buchin@dropsugar.com"
	}

	workspacesBase := cfg.WorkspacesBaseDir
	if workspacesBase == "" {
		workspacesBase = filepath.Join("data", "workspaces")
	}

	workspacePath := filepath.Join(workspacesBase, cfg.TaskID)

	ws := &TaskWorkspace{
		TaskID:        cfg.TaskID,
		ProjectID:     cfg.ProjectID,
		Path:          workspacePath,
		BranchName:    branchName,
		DefaultBranch: defaultBranch,
		AuthorName:    authorName,
		AuthorEmail:   authorEmail,
	}

	gitDir := filepath.Join(workspacePath, ".git")
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		// Workspace exists -> resume cycle
		checkoutCmd := exec.CommandContext(ctx, gitBinary, "-C", workspacePath, "checkout", "-B", branchName)
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("checkout existing task branch %s: %s", branchName, string(out))
		}

		_ = ConfigureWorkspaceGitIdentity(gitBinary, workspacePath, authorName, authorEmail)
		_ = InstallBranchProtectionHook(workspacePath, defaultBranch)
		LinkWorkspaceDependencies(workspacePath, cfg.BaseRepoDir)

		return ws, nil
	}

	// New workspace -> Create parent directory
	if err := os.MkdirAll(workspacesBase, 0755); err != nil {
		return nil, fmt.Errorf("create workspaces base dir: %w", err)
	}

	// Local clone from BaseRepoDir if available
	baseGitDir := filepath.Join(cfg.BaseRepoDir, ".git")
	if cfg.BaseRepoDir != "" {
		if fi, err := os.Stat(baseGitDir); err == nil && fi.IsDir() {
			cloneCmd := exec.CommandContext(ctx, gitBinary, "clone", "--local", cfg.BaseRepoDir, workspacePath)
			if out, err := cloneCmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("local clone from base repo failed: %s", string(out))
			}
		}
	}

	// If workspace wasn't cloned from BaseRepoDir, clone from RemoteURL or init
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if cfg.RemoteURL != "" {
			cloneCmd := exec.CommandContext(ctx, gitBinary, "clone", "--quiet", cfg.RemoteURL, workspacePath)
			if out, err := cloneCmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("remote clone failed: %s", string(out))
			}
		} else {
			// Fallback: init empty repo with default branch
			_ = os.MkdirAll(workspacePath, 0755)
			initCmd := exec.CommandContext(ctx, gitBinary, "-C", workspacePath, "init", "-b", defaultBranch)
			if out, err := initCmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("git init failed: %s", string(out))
			}
		}
	}

	// Configure remote origin if RemoteURL provided
	if cfg.RemoteURL != "" {
		_ = exec.CommandContext(ctx, gitBinary, "-C", workspacePath, "remote", "set-url", "origin", cfg.RemoteURL).Run()
	}

	// Create and switch to task branch
	checkoutCmd := exec.CommandContext(ctx, gitBinary, "-C", workspacePath, "checkout", "-B", branchName)
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("create task branch %s: %s", branchName, string(out))
	}

	// Configure human author identity
	if err := ConfigureWorkspaceGitIdentity(gitBinary, workspacePath, authorName, authorEmail); err != nil {
		return nil, err
	}

	// Install branch protection pre-push hook
	if err := InstallBranchProtectionHook(workspacePath, defaultBranch); err != nil {
		return nil, err
	}

	LinkWorkspaceDependencies(workspacePath, cfg.BaseRepoDir)

	return ws, nil
}

// ConfigureWorkspaceGitIdentity sets user.name and user.email in local repository config.
func ConfigureWorkspaceGitIdentity(gitBinary, workspacePath, name, email string) error {
	if gitBinary == "" {
		gitBinary = "git"
	}
	cmdName := exec.Command(gitBinary, "-C", workspacePath, "config", "user.name", name)
	if out, err := cmdName.CombinedOutput(); err != nil {
		return fmt.Errorf("set git user.name: %s", string(out))
	}

	cmdEmail := exec.Command(gitBinary, "-C", workspacePath, "config", "user.email", email)
	if out, err := cmdEmail.CombinedOutput(); err != nil {
		return fmt.Errorf("set git user.email: %s", string(out))
	}

	return nil
}

// InstallBranchProtectionHook places an executable pre-push hook in the workspace repository
// that rejects direct pushes to the default branch (main/master).
func InstallBranchProtectionHook(workspacePath, defaultBranch string) error {
	hooksDir := filepath.Join(workspacePath, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	prePushScript := fmt.Sprintf(`#!/bin/sh
# AgentRunner Pre-Push Hook: Branch Protection Guardrail
# Prevents autonomous agents from pushing directly to the protected default branch.

protected_branch="%s"

while read local_ref local_oid remote_ref remote_oid
do
    if [ "$remote_ref" = "refs/heads/$protected_branch" ] || [ "$remote_ref" = "refs/heads/main" ] || [ "$remote_ref" = "refs/heads/master" ]; then
        echo "============================================================" >&2
        echo "ERROR: Direct push to protected default branch '$remote_ref' is PROHIBITED." >&2
        echo "All changes must be committed to a task branch and merged via Pull Request." >&2
        echo "============================================================" >&2
        exit 1
    fi
done

exit 0
`, defaultBranch)

	hookPath := filepath.Join(hooksDir, "pre-push")
	if err := os.WriteFile(hookPath, []byte(prePushScript), 0755); err != nil {
		return fmt.Errorf("write pre-push hook: %w", err)
	}

	return nil
}

// LinkWorkspaceDependencies links node_modules and built assets into the workspace
// from base repository or shared cache to ensure lean fast execution.
func LinkWorkspaceDependencies(workspacePath, baseRepoDir string) {
	webDir := filepath.Join(workspacePath, "web")
	if _, err := os.Stat(webDir); err != nil {
		return
	}

	targetNodeModules := filepath.Join(webDir, "node_modules")
	if _, err := os.Lstat(targetNodeModules); err != nil {
		candidates := []string{
			filepath.Join(baseRepoDir, "web", "node_modules"),
			filepath.Join(baseRepoDir, "node_modules"),
		}
		for _, cand := range candidates {
			if cand != "" {
				if fi, sErr := os.Stat(cand); sErr == nil && fi.IsDir() {
					_ = os.Symlink(cand, targetNodeModules)
					break
				}
			}
		}
	}
}
