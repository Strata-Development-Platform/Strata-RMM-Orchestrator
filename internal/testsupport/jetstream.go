package testsupport

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// EnsureJetStreamURL returns a NATS endpoint with server-side JetStream
// enabled. CI historically supplied a core-NATS-only service even to tests
// whose contract requires retained commands/results. When that happens the
// harness starts an isolated official NATS container with JetStream enabled on
// an ephemeral host port. Tests therefore keep exercising real JetStream
// semantics instead of downgrading to core NATS or silently skipping coverage.
func EnsureJetStreamURL(ctx context.Context, candidate string) (string, func(), error) {
	candidate = strings.TrimSpace(candidate)
	if candidate != "" && jetStreamAvailable(candidate) {
		return candidate, func() {}, nil
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return "", nil, fmt.Errorf("JetStream unavailable at %q and docker is not installed: %w", candidate, err)
	}

	name := fmt.Sprintf("strata-jobintegration-js-%d-%d", os.Getpid(), time.Now().UnixNano())
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-d",
		"-p", "127.0.0.1::4222", "--name", name,
		"nats:2.11-alpine", "-js")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("start isolated JetStream container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", name).Run()
	}

	portDeadline := time.Now().Add(10 * time.Second)
	var url string
	for time.Now().Before(portDeadline) {
		portOutput, portErr := exec.CommandContext(ctx, "docker", "port", name, "4222/tcp").Output()
		if portErr == nil {
			mapping := strings.TrimSpace(string(portOutput))
			if mapping != "" {
				if idx := strings.LastIndex(mapping, ":"); idx >= 0 && idx+1 < len(mapping) {
					url = "nats://127.0.0.1:" + mapping[idx+1:]
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if url == "" {
		cleanup()
		return "", nil, fmt.Errorf("isolated JetStream container did not expose a client port")
	}

	readyDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(readyDeadline) {
		if jetStreamAvailable(url) {
			return url, cleanup, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	cleanup()
	return "", nil, fmt.Errorf("isolated JetStream server at %s did not become ready", url)
}

func jetStreamAvailable(url string) bool {
	nc, err := nats.Connect(url, nats.Timeout(750*time.Millisecond))
	if err != nil {
		return false
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = js.AccountInfo(nats.Context(ctx))
	return err == nil
}
