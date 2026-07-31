package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestObservabilityDeploymentFilesParse(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{"prometheus.yml", "alerts.yml"} {
		data, err := os.ReadFile(filepath.Join(root, "deploy", "prometheus", name))
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := yaml.Unmarshal(data, &document); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "deploy", "grafana", "dashboards", "phase8d-platform.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard["uid"] != "strata-phase8d" {
		t.Fatalf("dashboard uid=%v", dashboard["uid"])
	}
}

func TestEveryAlertHasOwnerSeverityAndExistingRunbookAnchor(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "prometheus", "alerts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var rules struct {
		Groups []struct {
			Rules []struct {
				Alert       string            `yaml:"alert"`
				Labels      map[string]string `yaml:"labels"`
				Annotations map[string]string `yaml:"annotations"`
			} `yaml:"rules"`
		} `yaml:"groups"`
	}
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	runbook, err := os.ReadFile(filepath.Join(root, "docs", "RUNBOOK.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			if rule.Alert == "" || rule.Labels["owner"] == "" || rule.Labels["severity"] == "" {
				t.Errorf("alert lacks identity/owner/severity: %+v", rule)
			}
			link := rule.Annotations["runbook_url"]
			parts := strings.SplitN(link, "#", 2)
			if len(parts) != 2 || !strings.Contains(strings.ToLower(string(runbook)), strings.ToLower(headingFromAnchor(parts[1]))) {
				t.Errorf("%s has missing runbook anchor %q", rule.Alert, link)
			}
		}
	}
}

func headingFromAnchor(anchor string) string {
	words := strings.Split(anchor, "-")
	for i := range words {
		if words[i] == "api" {
			words[i] = "API"
		} else {
			words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
		}
	}
	return "### " + strings.Join(words, " ")
}
