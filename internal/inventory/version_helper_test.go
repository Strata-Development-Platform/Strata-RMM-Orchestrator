package inventory

import (
	"strconv"
	"strings"
)

// versionLessThan performs a simple semantic version comparison.
// Returns true if v1 < v2.
func versionLessThan(v1, v2 string) bool {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	max := len(parts1)
	if len(parts2) > max {
		max = len(parts2)
	}
	for i := 0; i < max; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 < n2 {
			return true
		}
		if n1 > n2 {
			return false
		}
	}
	return false
}
