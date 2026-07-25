package collectors

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

func runAndParse(ctx context.Context, name string, args []string, parser func([]string) []Software) ([]Software, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return parser(lines), nil
}

func splitN(s, sep string, n int) []string {
	parts := strings.SplitN(s, sep, n)
	return parts
}

func parseInt64(s string, defaultVal int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return defaultVal
	}
	return v
}
