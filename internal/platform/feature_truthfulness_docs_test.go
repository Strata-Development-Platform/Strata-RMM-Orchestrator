package platform

import (
	"os"
	"strings"
	"testing"
)

func TestFeatureStatusDocumentsRemainTruthful(t *testing.T) {
	files := []string{
		"../../docs/FEATURE_COMPLETENESS_MATRIX.md",
		"../../docs/FEATURE_SPECIFICATION.md",
		"../../docs/IMPLEMENTATION_STATUS.md",
		"../../docs/BETA_GO_NO_GO.md",
	}

	contents := make(map[string]string, len(files))
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		contents[path] = text
		if strings.Contains(text, "Internal-alpha code-complete") {
			t.Fatalf("%s reintroduced an unsupported blanket code-complete claim", path)
		}
	}

	matrix := contents["../../docs/FEATURE_COMPLETENESS_MATRIX.md"]
	for _, required := range []string{
		"Integration Dashboard",
		"mock-backed",
		"LLDP/CDP/STP",
		"Not implemented",
		"Environment pending",
		"#15",
		"#167",
	} {
		if !strings.Contains(matrix, required) {
			t.Fatalf("feature matrix missing truthfulness marker %q", required)
		}
	}

	spec := contents["../../docs/FEATURE_SPECIFICATION.md"]
	if strings.Contains(spec, "LLDP/CDP/STP stubs | ✅ Verified") {
		t.Fatal("feature specification must not classify topology stubs as verified product behavior")
	}

	status := contents["../../docs/IMPLEMENTATION_STATUS.md"]
	if !strings.Contains(status, "Integration Dashboard is mock-backed") {
		t.Fatal("implementation status must preserve the mock Integration Dashboard limitation")
	}

	goNoGo := contents["../../docs/BETA_GO_NO_GO.md"]
	if !strings.Contains(goNoGo, "#15") || !strings.Contains(goNoGo, "NO-GO") {
		t.Fatal("beta go/no-go must keep hosted acceptance separate and explicitly gated")
	}
}
