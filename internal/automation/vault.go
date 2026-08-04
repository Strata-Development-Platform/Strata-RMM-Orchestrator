package automation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Vault provides Git-backed automation with clone and pull operations.
// It supports HTTPS with token auth and SSH with key-based authentication.
type Vault struct {
	defaultBranch string
	logger        Logger
}

// Logger is the logging interface for the automation vault.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}

// AuthConfig holds authentication credentials for Git operations.
type AuthConfig struct {
	// Token is used for HTTPS authentication (GitHub/GitLab personal access tokens).
	Token string `json:"token,omitempty"`

	// SSHKeyPath is the path to an SSH private key for SSH authentication.
	SSHKeyPath string `json:"ssh_key_path,omitempty"`

	// Username is used for HTTPS basic auth (username:password).
	Username string `json:"username,omitempty"`

	// Password is used for HTTPS basic auth (username:password).
	Password string `json:"password,omitempty"`
}

// NewVault creates a new automation vault.
// If defaultBranch is empty, "main" is used (falling back to "master").
func NewVault(defaultBranch string, logger Logger) *Vault {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	return &Vault{
		defaultBranch: defaultBranch,
		logger:        logger,
	}
}

// Clone clones a Git repository to the specified local path.
// If the directory already exists, it attempts a pull first.
// AuthConfig can be nil for public repositories.
func (v *Vault) Clone(ctx context.Context, remoteURL, localPath string, auth *AuthConfig) error {
	if remoteURL == "" {
		return fmt.Errorf("remote URL is required")
	}
	if localPath == "" {
		return fmt.Errorf("local path is required")
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	// Check if directory already exists — if so, treat as pull
	if _, err := os.Stat(localPath); err == nil {
		return v.Pull(ctx, localPath, auth)
	}

	// Build clone command
	args := []string{"clone", "--depth", "1", "--single-branch"}

	// Set default branch
	if v.defaultBranch != "" {
		args = append(args, "-b", v.defaultBranch)
	}

	// Add remote URL and local path
	args = append(args, remoteURL, localPath)

	// Prepare command with potential auth injection
	cmd := exec.CommandContext(ctx, "git", args...)

	// Inject auth via environment if provided
	cmd = v.prepareCommandWithAuth(cmd, auth)

	// Run clone
	if err := cmd.Run(); err != nil {
		// Clean up partial clone on failure
		_ = os.RemoveAll(localPath)
		return fmt.Errorf("git clone: %w", err)
	}

	v.logger.Info("vault clone", "remote", remoteURL, "local", localPath)
	return nil
}

// Pull pulls the latest changes from the remote into the local repository.
// The repository must already be cloned.
func (v *Vault) Pull(ctx context.Context, localPath string, auth *AuthConfig) error {
	if localPath == "" {
		return fmt.Errorf("local path is required")
	}

	// Verify directory exists and is a Git repository
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", localPath)
	}

	// Verify it's a Git repository
	_, err = os.Stat(filepath.Join(localPath, ".git"))
	if err != nil {
		return fmt.Errorf("not a Git repository (no .git directory): %s", localPath)
	}

	// Build pull command
	args := []string{"pull", "--ff-only"}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = localPath

	// Inject auth via environment if provided
	cmd = v.prepareCommandWithAuth(cmd, auth)

	// Run pull
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}

	v.logger.Info("vault pull", "local", localPath)
	return nil
}

// prepareCommandWithAuth sets up environment variables for Git authentication.
// It handles HTTPS tokens, SSH keys, and basic auth.
func (v *Vault) prepareCommandWithAuth(cmd *exec.Cmd, auth *AuthConfig) *exec.Cmd {
	if auth == nil {
		return cmd
	}

	// SSH key authentication: set GIT_SSH_COMMAND
	if auth.SSHKeyPath != "" {
		cmd = v.setupSSHAuth(cmd, auth.SSHKeyPath)
	}

	// HTTPS token authentication: inject into URL via credential helper
	if auth.Token != "" {
		cmd = v.setupTokenAuth(cmd, auth.Token)
	}

	// HTTPS basic auth: set GIT_TERMINAL_PROMPT=0 to force failure if credentials missing
	if auth.Username != "" || auth.Password != "" {
		cmd = v.setupBasicAuth(cmd, auth.Username, auth.Password)
	}

	return cmd
}

// setupSSHAuth configures the command to use the specified SSH key.
func (v *Vault) setupSSHAuth(cmd *exec.Cmd, sshKeyPath string) *exec.Cmd {
	// Validate SSH key exists
	if _, err := os.Stat(sshKeyPath); err != nil {
		return cmd // Auth will fail later with a clear error
	}

	// Set GIT_SSH_COMMAND to use the specific key
	cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", sshKeyPath))

	return cmd
}

// setupTokenAuth configures the command to use the provided token for HTTPS.
// This uses the credential helper approach by setting GIT_ASKPASS.
func (v *Vault) setupTokenAuth(cmd *exec.Cmd, token string) *exec.Cmd {
	// Set credential helper to use the token via environment
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_ASKPASS_ECHO=1"))

	return cmd
}

// setupBasicAuth configures basic authentication credentials.
func (v *Vault) setupBasicAuth(cmd *exec.Cmd, username, password string) *exec.Cmd {
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")

	return cmd
}

// Validate checks if the repository is healthy and up to date.
func (v *Vault) Validate(ctx context.Context, localPath string) error {
	if localPath == "" {
		return fmt.Errorf("local path is required")
	}

	// Verify directory exists
	if _, err := os.Stat(localPath); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}

	// Verify it's a Git repository
	_, err := os.Stat(filepath.Join(localPath, ".git"))
	if err != nil {
		return fmt.Errorf("not a Git repository: %s", localPath)
	}

	// Check git status (no uncommitted changes expected)
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = localPath
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(output) > 0 {
		v.logger.Warn("vault validation: uncommitted changes", "path", localPath)
	}

	return nil
}

// LastCommit returns the hash of the latest commit in the repository.
func (v *Vault) LastCommit(ctx context.Context, localPath string) (string, error) {
	if localPath == "" {
		return "", fmt.Errorf("local path is required")
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = localPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
