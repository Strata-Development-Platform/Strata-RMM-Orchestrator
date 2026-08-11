package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModuleVersionRoutesAreExplicitPostOnly(t *testing.T) {
	for _, path := range []string{
		"/api/v2/platform/modules/com.example.backup/upgrade",
		"/api/v2/platform/modules/com.example.backup/rollback",
	} {
		if !isModuleLifecycleRequest(http.MethodPost, path) {
			t.Fatalf("POST route not classified: %s", path)
		}
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if isModuleLifecycleRequest(method, path) {
				t.Fatalf("unexpected %s route classification for %s", method, path)
			}
		}
	}
}

func TestRequireEmptyModuleVersionBodyRejectsManifestInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "empty", body: "", want: true},
		{name: "manifest", body: `{"manifest":{"id":"com.example.backup","version":"0.0.1"}}`, want: false},
		{name: "whitespace is still input", body: " ", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v2/platform/modules/com.example.backup/rollback", strings.NewReader(test.body))
			w := httptest.NewRecorder()
			if got := requireEmptyModuleVersionBody(w, r); got != test.want {
				t.Fatalf("got=%v want=%v body=%q response=%s", got, test.want, test.body, w.Body.String())
			}
			if !test.want && w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=%d", w.Code, http.StatusBadRequest)
			}
		})
	}
}
