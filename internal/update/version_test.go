package update

import "testing"

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		name       string
		left       string
		right      string
		comparison int
	}{
		{name: "minor is numeric", left: "1.10.0", right: "1.9.0", comparison: 1},
		{name: "patch is numeric", left: "v2.0.10", right: "2.0.9", comparison: 1},
		{name: "same version", left: "1.2.3", right: "v1.2.3", comparison: 0},
		{name: "prerelease before release", left: "1.2.3-rc.1", right: "1.2.3", comparison: -1},
		{name: "prerelease numeric order", left: "1.2.3-rc.10", right: "1.2.3-rc.2", comparison: 1},
		{name: "build metadata ignored", left: "1.2.3+build.2", right: "1.2.3+build.1", comparison: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := compareSemanticVersions(test.left, test.right)
			if err != nil {
				t.Fatalf("compareSemanticVersions() error = %v", err)
			}
			if got != test.comparison {
				t.Fatalf("compareSemanticVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.comparison)
			}
		})
	}
}

func TestCompareSemanticVersionsRejectsMalformedInput(t *testing.T) {
	for _, version := range []string{"", "latest", "1.2", "1.02.3", "1.2.3-", "1.2.3-rc..1", "1.2.3-01"} {
		t.Run(version, func(t *testing.T) {
			if _, err := compareSemanticVersions(version, "1.0.0"); err == nil {
				t.Fatalf("compareSemanticVersions(%q) accepted malformed version", version)
			}
		})
	}
}
