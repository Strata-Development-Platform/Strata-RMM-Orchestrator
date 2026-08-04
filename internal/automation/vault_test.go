package automation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// mockLogger is a test logger that collects log messages.
type mockLogger struct {
	messages []string
}

func (l *mockLogger) Info(msg string, keysAndValues ...interface{}) {
	l.messages = append(l.messages, "INFO: "+msg)
}

func (l *mockLogger) Error(msg string, keysAndValues ...interface{}) {
	l.messages = append(l.messages, "ERROR: "+msg)
}

func (l *mockLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.messages = append(l.messages, "WARN: "+msg)
}

// createBareGitRepo creates a bare Git repository in the specified directory
// with an initial commit on the main branch.
func createBareGitRepo(t *testing.T, dir string) string {
	t.Helper()

	// Initialize bare repo
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	// Create a temporary working directory to make an initial commit
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("failed to create work dir: %v", err)
	}

	// Configure git user
	exec.Command("git", "config", "user.email", "test@test.com").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()

	// Create initial file and commit
	testFile := filepath.Join(workDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	exec.Command("git", "-C", workDir, "init").Run()
	exec.Command("git", "-C", workDir, "add", ".").Run()
	exec.Command("git", "-C", workDir, "commit", "-m", "initial commit").Run()

	// Rename to main and push
	cmd = exec.Command("git", "-C", workDir, "branch", "-M", "main")
	cmd.Run()
	cmd = exec.Command("git", "-C", workDir, "remote", "add", "origin", dir)
	cmd.Run()

	// Push all refs (handles master vs main naming)
	cmd = exec.Command("git", "-C", workDir, "push", "-u", "origin", "--all")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to push initial commit: %v", err)
	}

	return dir
}

func TestVaultClone(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")

	err := vault.Clone(ctx, bareRepo, clonePath, nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Verify file was cloned
	data, err := os.ReadFile(filepath.Join(clonePath, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}
	if string(data) != "initial content" {
		t.Errorf("cloned content = %q, want %q", string(data), "initial content")
	}
}

func TestVaultCloneInvalidURL(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	err := vault.Clone(ctx, "not-a-valid-url", t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestVaultCloneEmptyRemote(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	err := vault.Clone(ctx, "", t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error for empty remote URL")
	}
}

func TestVaultCloneEmptyLocal(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	err := vault.Clone(ctx, bareRepo, "", nil)
	if err == nil {
		t.Fatal("expected error for empty local path")
	}
}

func TestVaultCloneAndPull(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	// Use a single temp dir for all paths
	tmpDir := t.TempDir()
	bareRepo := filepath.Join(tmpDir, "bare")
	clonePath := filepath.Join(tmpDir, "clone")
	workDir := filepath.Join(tmpDir, "work")

	// Create bare repo directory
	if err := os.MkdirAll(bareRepo, 0755); err != nil {
		t.Fatalf("failed to create bare repo dir: %v", err)
	}

	// Create bare repo with initial commit
	bareRepo = createBareGitRepo(t, bareRepo)

	// Clone the repository
	if err := vault.Clone(ctx, bareRepo, clonePath, nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Verify initial content
	data, err := os.ReadFile(filepath.Join(clonePath, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "initial content" {
		t.Errorf("content = %q, want %q", string(data), "initial content")
	}

	// Create work dir and add new commits
	cmd := exec.Command("git", "clone", "--no-single-branch", bareRepo, workDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git clone to work dir: %v", err)
	}
	cmd = exec.Command("git", "-C", workDir, "config", "user.email", "test@test.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", workDir, "config", "user.name", "Test")
	cmd.Run()
	// Ensure we're on main branch
	cmd = exec.Command("git", "-C", workDir, "checkout", "main")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git checkout main: %v", err)
	}

	if err := os.WriteFile(filepath.Join(workDir, "new.txt"), []byte("new content"), 0644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}
	cmd = exec.Command("git", "-C", workDir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	cmd = exec.Command("git", "-C", workDir, "commit", "-m", "add new")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Push new commits to bare repo (explicitly to main branch)
	cmd = exec.Command("git", "-C", workDir, "push", "origin", "HEAD:main")
	output, err := cmd.CombinedOutput()
	t.Logf("Push output: %s", string(output))
	if err != nil {
		t.Fatalf("git push: %v (output: %s)", err, string(output))
	}

	// Verify bare repo has new commit
	cmd = exec.Command("git", "-C", bareRepo, "branch", "-v")
	bareOutput, _ := cmd.CombinedOutput()
	t.Logf("Bare repo branches: %s", string(bareOutput))

	// Pull in clone
	if err := vault.Pull(ctx, clonePath, nil); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Verify new file exists
	_, err = os.Stat(filepath.Join(clonePath, "new.txt"))
	if err != nil {
		t.Fatalf("new.txt missing after pull. Files: %v", err)
	}

	// Verify original content unchanged
	data, err = os.ReadFile(filepath.Join(clonePath, "test.txt"))
	if err != nil {
		t.Fatalf("read test.txt: %v", err)
	}
	if string(data) != "initial content" {
		t.Errorf("content after pull = %q, want initial content", string(data))
	}
}

func TestVaultPullNonExistentPath(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	err := vault.Pull(ctx, "/nonexistent/path", nil)
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestVaultPullNotGitRepo(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	dir := t.TempDir()
	err := vault.Pull(ctx, dir, nil)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestVaultPullEmptyPath(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	err := vault.Pull(ctx, "", nil)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestVaultValidate(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")

	// Clone first
	if err := vault.Clone(ctx, bareRepo, clonePath, nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Validate should succeed
	if err := vault.Validate(ctx, clonePath); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestVaultValidateNonExistentPath(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	err := vault.Validate(ctx, "/nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestVaultValidateEmptyPath(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	err := vault.Validate(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestVaultLastCommit(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")

	// Clone first
	if err := vault.Clone(ctx, bareRepo, clonePath, nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Get last commit
	commit, err := vault.LastCommit(ctx, clonePath)
	if err != nil {
		t.Fatalf("LastCommit: %v", err)
	}
	if len(commit) == 0 {
		t.Fatal("commit hash should not be empty")
	}
}

func TestVaultLastCommitEmptyPath(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	_, err := vault.LastCommit(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestVaultLastCommitNonExistentPath(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	_, err := vault.LastCommit(ctx, "/nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestVaultCloneWithAuth(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")

	// Clone with token (token is ignored for local file:// repos, but auth config should not cause error)
	auth := &AuthConfig{Token: "dummy-token"}
	err := vault.Clone(ctx, bareRepo, clonePath, auth)
	if err != nil {
		t.Fatalf("Clone with auth: %v", err)
	}

	// Verify clone worked
	data, err := os.ReadFile(filepath.Join(clonePath, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}
	if string(data) != "initial content" {
		t.Errorf("cloned content = %q, want %q", string(data), "initial content")
	}
}

func TestVaultCloneWithSSHKey(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")

	// Create a fake SSH key for testing
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, []byte("fake ssh key"), 0600); err != nil {
		t.Fatalf("failed to write SSH key: %v", err)
	}

	// Clone with SSH key auth (file:// protocol doesn't need SSH, but auth config should not cause error)
	auth := &AuthConfig{SSHKeyPath: keyPath}
	err := vault.Clone(ctx, bareRepo, clonePath, auth)
	if err != nil {
		t.Fatalf("Clone with SSH key: %v", err)
	}
}

func TestVaultPullAfterClone(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")

	// Clone the repository
	if err := vault.Clone(ctx, bareRepo, clonePath, nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Pull should work on cloned repo
	if err := vault.Pull(ctx, clonePath, nil); err != nil {
		t.Fatalf("Pull after clone: %v", err)
	}
}

func TestVaultCloneContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	logger := &mockLogger{}
	vault := NewVault("main", logger)

	bareRepo := createBareGitRepo(t, t.TempDir())
	err := vault.Clone(ctx, bareRepo, t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestVaultPullContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	logger := &mockLogger{}
	vault := NewVault("main", logger)

	// Create a fake git repo
	bareRepo := createBareGitRepo(t, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")
	vault.Clone(ctx, bareRepo, clonePath, nil)

	err := vault.Pull(ctx, clonePath, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
