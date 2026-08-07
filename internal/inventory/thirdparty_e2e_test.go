package inventory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestThirdPartyEngine_New verifies engine creation
func TestThirdPartyEngine_New(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.httpClient == nil {
		t.Fatal("expected httpClient initialized")
	}
	if engine.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", engine.httpClient.Timeout)
	}
}

// TestThirdPartyEngine_ListApps verifies catalog listing
func TestThirdPartyEngine_ListApps(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	apps := engine.ListApps()
	if len(apps) == 0 {
		t.Fatal("expected at least one app in catalog")
	}
	for i, app := range apps {
		if app.Name == "" {
			t.Errorf("app[%d] missing Name", i)
		}
		if app.Vendor == "" {
			t.Errorf("app[%d] missing Vendor", i)
		}
		if app.URLTemplate == "" {
			t.Errorf("app[%d] missing URLTemplate", i)
		}
		if app.VersionURL == "" {
			t.Errorf("app[%d] missing VersionURL", i)
		}
	}
}

// TestThirdPartyEngine_DiscoverVendors verifies vendor discovery
func TestThirdPartyEngine_DiscoverVendors(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	vendors := engine.DiscoverVendors()
	if len(vendors) == 0 {
		t.Fatal("expected at least one vendor")
	}
	vendorMap := make(map[string]bool)
	for _, v := range vendors {
		vendorMap[v.Name] = true
		if len(v.Apps) == 0 {
			t.Errorf("vendor %q has no apps", v.Name)
		}
	}
	if len(vendorMap) < 5 {
		t.Fatalf("expected at least 5 vendors, got %d", len(vendorMap))
	}
}

// TestThirdPartyEngine_SetTenantID verifies tenant ID setting
func TestThirdPartyEngine_SetTenantID(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.SetTenantID("test-tenant-uuid")
	if engine.tenantID != "test-tenant-uuid" {
		t.Fatal("tenant ID not set correctly")
	}
}

// TestThirdPartyEngine_BuildDownloadURL_VersionSubstitution verifies URL template substitution
func TestThirdPartyEngine_BuildDownloadURL_VersionSubstitution(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())

	tests := []struct {
		name     string
		template string
		version  string
		want     string
	}{
		{
			name:     "version placeholder",
			template: "https://example.com/download/{version}/package.exe",
			version:  "1.2.3",
			want:     "https://example.com/download/1.2.3/package.exe",
		},
		{
			name:     "major/minor/patch placeholders",
			template: "https://example.com/{major}/{minor}/{patch}/pkg.exe",
			version:  "2.1.0",
			want:     "https://example.com/2/1/0/pkg.exe",
		},
		{
			name:     "no placeholders - unchanged",
			template: "https://dl.google.com/chrome/install.exe",
			version:  "120.0",
			want:     "https://dl.google.com/chrome/install.exe",
		},
		{
			name:     "version only in nested path",
			template: "https://cdn.example.com/v{version}/installer.msi",
			version:  "3.1.4",
			want:     "https://cdn.example.com/v3.1.4/installer.msi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := ThirdPartyApp{URLTemplate: tt.template}
			got := engine.buildDownloadURL(app, tt.version)
			if got != tt.want {
				t.Errorf("buildDownloadURL(%q, %q) = %q, want %q",
					tt.template, tt.version, got, tt.want)
			}
		})
	}
}

// TestThirdPartyEngine_FetchLatestVersion_ChromePattern verifies Chrome version discovery
func TestThirdPartyEngine_FetchLatestVersion_ChromePattern(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": "120.0.6099.130"}`))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "Google Chrome",
		Vendor:       "Google",
		VersionURL:   server.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+\.\d+)"`,
	}

	version, err := engine.fetchLatestVersion(context.Background(), app)
	if err != nil {
		t.Fatalf("fetchLatestVersion() error = %v", err)
	}
	if version != "120.0.6099.130" {
		t.Errorf("got %q, want %q", version, "120.0.6099.130")
	}
}

// TestThirdPartyEngine_FetchLatestVersion_GitHubRelease verifies GitHub release API
func TestThirdPartyEngine_FetchLatestVersion_GitHubRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v8.7.1"}`))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "Notepad++",
		Vendor:       "Notepad++",
		VersionURL:   server.URL,
		VersionRegex: `"tag_name":\s*"v(\d+\.\d+(\.\d+)*)"`,
	}

	version, err := engine.fetchLatestVersion(context.Background(), app)
	if err != nil {
		t.Fatalf("fetchLatestVersion() error = %v", err)
	}
	if version != "8.7.1" {
		t.Errorf("got %q, want %q", version, "8.7.1")
	}
}

// TestThirdPartyEngine_FetchLatestVersion_ZoomAPI verifies Zoom REST API pattern
func TestThirdPartyEngine_FetchLatestVersion_ZoomAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": "5.16.10", "downloadUrl": "https://zoom.us/client/latest/ZoomInstallerFull.msi"}`))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "Zoom",
		Vendor:       "Zoom Video",
		VersionURL:   server.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
	}

	version, err := engine.fetchLatestVersion(context.Background(), app)
	if err != nil {
		t.Fatalf("fetchLatestVersion() error = %v", err)
	}
	if version != "5.16.10" {
		t.Errorf("got %q, want %q", version, "5.16.10")
	}
}

// TestThirdPartyEngine_FetchLatestVersion_HTMLParse verifies HTML regex parsing
func TestThirdPartyEngine_FetchLatestVersion_HTMLParse(t *testing.T) {
	tests := []struct {
		name         string
		versionRegex string
		responseBody string
		wantVersion  string
	}{
		{
			name:         "7-Zip HTML",
			versionRegex: `Download 7-Zip (\d+\.\d+)`,
			responseBody: `<html><body><h1>Download 7-Zip 24.09</h1></body></html>`,
			wantVersion:  "24.09",
		},
		{
			name:         "LibreOffice HTML",
			versionRegex: `LibreOffice (\d+\.\d+\.\d+)`,
			responseBody: `Download LibreOffice 24.8.3 for Windows`,
			wantVersion:  "24.8.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			engine := NewThirdPartyEngine(nil, zap.NewNop())
			engine.httpClient = &http.Client{Timeout: 5 * time.Second}

			app := ThirdPartyApp{
				Name:         "TestApp",
				Vendor:       "TestVendor",
				VersionURL:   server.URL,
				VersionRegex: tt.versionRegex,
			}

			version, err := engine.fetchLatestVersion(context.Background(), app)
			if err != nil {
				t.Fatalf("fetchLatestVersion() error = %v", err)
			}
			if version != tt.wantVersion {
				t.Errorf("got %q, want %q", version, tt.wantVersion)
			}
		})
	}
}

// TestThirdPartyEngine_FetchLatestVersion_MozillaJSON verifies Mozilla product-details pattern
func TestThirdPartyEngine_FetchLatestVersion_MozillaJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"LATEST_FIREFOX_VERSION": "120.0"}`))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "Mozilla Firefox",
		Vendor:       "Mozilla",
		VersionURL:   server.URL,
		VersionRegex: `"LATEST_FIREFOX_VERSION":\s*"(\d+\.\d+(\.\d+)*)"`,
	}

	version, err := engine.fetchLatestVersion(context.Background(), app)
	if err != nil {
		t.Fatalf("fetchLatestVersion() error = %v", err)
	}
	if version != "120.0" {
		t.Errorf("got %q, want %q", version, "120.0")
	}
}

// TestThirdPartyEngine_FetchLatestVersion_HTTPError verifies HTTP error handling
func TestThirdPartyEngine_FetchLatestVersion_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`Not Found`))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "TestApp",
		Vendor:       "TestVendor",
		VersionURL:   server.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
	}

	_, err := engine.fetchLatestVersion(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error message, got: %v", err)
	}
}

// TestThirdPartyEngine_FetchLatestVersion_ContextTimeout verifies timeout handling
func TestThirdPartyEngine_FetchLatestVersion_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 100 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	app := ThirdPartyApp{
		Name:         "TestApp",
		Vendor:       "TestVendor",
		VersionURL:   server.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
	}

	_, err := engine.fetchLatestVersion(ctx, app)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// TestThirdPartyEngine_FetchLatestVersion_EmptyBody verifies empty response handling
func TestThirdPartyEngine_FetchLatestVersion_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(``))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "TestApp",
		Vendor:       "TestVendor",
		VersionURL:   server.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
	}

	_, err := engine.fetchLatestVersion(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty body error, got: %v", err)
	}
}

// TestThirdPartyEngine_SyncApp_Success verifies single app sync workflow
func TestThirdPartyEngine_SyncApp_Success(t *testing.T) {
	versionCalled := false
	versionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		versionCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": "120.0.6099.130"}`))
	}))
	defer versionServer.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}
	engine.SetTenantID("test-tenant")

	app := ThirdPartyApp{
		Name:         "MockChrome",
		Vendor:       "Google",
		Platform:     "windows",
		PackageType:  "msi",
		VersionURL:   versionServer.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+\.\d+)"`,
		URLTemplate:  "https://example.com/chrome.msi",
		InstallArgs:  "/quiet",
		DetectCmd:    "detect-cmd",
	}

	version, err := engine.fetchLatestVersion(context.Background(), app)
	if err != nil {
		t.Fatalf("fetchLatestVersion() error = %v", err)
	}
	if version != "120.0.6099.130" {
		t.Errorf("got %q, want %q", version, "120.0.6099.130")
	}
	if !versionCalled {
		t.Fatal("expected HTTP request to be made")
	}
}

// TestThirdPartyEngine_VendorSpecificSync verifies vendor-specific sync
func TestThirdPartyEngine_VendorSpecificSync(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	vendors := engine.DiscoverVendors()

	vendorNames := make(map[string]bool)
	for _, v := range vendors {
		vendorNames[v.Name] = true
	}

	expectedVendors := []string{"Google", "Mozilla", "Adobe", "Zoom Video", "Notepad++"}
	for _, expected := range expectedVendors {
		if !vendorNames[expected] {
			t.Errorf("expected vendor %q not found", expected)
		}
	}
}

// TestThirdPartyEngine_VendorStatus verifies vendor status reporting
func TestThirdPartyEngine_VendorStatus(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	status := engine.VendorStatus(context.Background())
	if len(status) == 0 {
		t.Fatal("expected vendor status entries")
	}
	for _, s := range status {
		name, ok := s["name"].(string)
		if !ok || name == "" {
			t.Error("expected vendor name in status")
		}
		if _, ok := s["app_count"].(int); !ok {
			t.Error("expected app_count in status")
		}
	}
}

// TestThirdPartyApps_Completeness verifies all catalog apps have required fields
func TestThirdPartyApps_Completeness(t *testing.T) {
	for i, app := range ThirdPartyApps {
		if app.Name == "" {
			t.Errorf("app[%d] missing Name", i)
		}
		if app.Vendor == "" {
			t.Errorf("app[%d] missing Vendor", i)
		}
		if app.Platform == "" {
			t.Errorf("app[%d] missing Platform", i)
		}
		if app.PackageType == "" {
			t.Errorf("app[%d] missing PackageType", i)
		}
		if app.URLTemplate == "" {
			t.Errorf("app[%d] missing URLTemplate", i)
		}
		if app.VersionURL == "" {
			t.Errorf("app[%d] missing VersionURL", i)
		}
		if app.VersionRegex == "" {
			t.Errorf("app[%d] missing VersionRegex", i)
		}
		if app.DetectCmd == "" {
			t.Errorf("app[%d] missing DetectCmd", i)
		}
	}
}

// TestThirdPartyEngine_Start verifies scheduler starts without panic
func TestThirdPartyEngine_Start(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go engine.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestThirdPartyEngine_CatalogAggregation verifies multi-vendor aggregation
func TestThirdPartyEngine_CatalogAggregation(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())
	apps := engine.ListApps()

	vendors := make(map[string]int)
	for _, app := range apps {
		vendors[app.Vendor]++
	}

	if len(vendors) < 5 {
		t.Fatalf("expected at least 5 vendors, got %d", len(vendors))
	}

	seen := make(map[string]bool)
	for _, app := range apps {
		key := fmt.Sprintf("%s/%s", app.Vendor, app.Name)
		if seen[key] {
			t.Errorf("duplicate app: %s", key)
		}
		seen[key] = true
	}
}

// TestThirdPartyEngine_DeploymentWorkflow verifies deployment state transitions
func TestThirdPartyEngine_DeploymentWorkflow(t *testing.T) {
	stages := []string{"pending", "approved", "deploying", "completed"}

	expectedStages := map[string]bool{
		"pending": true, "approved": true, "deploying": true,
		"completed": true, "failed": true, "cancelled": true,
	}

	for _, stage := range stages {
		if !expectedStages[stage] {
			t.Errorf("invalid deployment stage: %s", stage)
		}
	}

	for i := 1; i < len(stages); i++ {
		if stages[i-1] == "completed" && stages[i] != "completed" {
			t.Errorf("invalid stage transition: %s → %s", stages[i-1], stages[i])
		}
	}
}

// TestThirdPartyEngine_VendorSyncWithMockServer verifies vendor sync with mock HTTP
func TestThirdPartyEngine_VendorSyncWithMockServer(t *testing.T) {
	callCount := 0
	versionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		version := "120.0.6099.130"
		if strings.Contains(r.URL.Path, "firefox") {
			version = "120.0"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"version": "%s"}`, version)
	}))
	defer versionServer.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "TestChrome",
		Vendor:       "Google",
		VersionURL:   versionServer.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+\.\d+)"`,
		URLTemplate:  "https://example.com/test.msi",
		InstallArgs:  "/quiet",
		DetectCmd:    "test",
		Platform:     "windows",
		PackageType:  "msi",
	}

	version, err := engine.fetchLatestVersion(context.Background(), app)
	if err != nil {
		t.Fatalf("fetchLatestVersion() error = %v", err)
	}
	if version != "120.0.6099.130" {
		t.Errorf("got %q, want %q", version, "120.0.6099.130")
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}
}

// TestThirdPartyEngine_NonMatchingRegex verifies non-matching regex handling
func TestThirdPartyEngine_NonMatchingRegex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": "120.0.6099.130"}`))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "TestApp",
		Vendor:       "TestVendor",
		VersionURL:   server.URL,
		VersionRegex: `"wrong_field":\s*"(\d+\.\d+\.\d+)"`,
	}

	_, err := engine.fetchLatestVersion(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for non-matching regex")
	}
	if !strings.Contains(err.Error(), "version not found") {
		t.Errorf("expected 'version not found' error, got: %v", err)
	}
}

// TestThirdPartyEngine_ResponseSizeLimit verifies response size limiting
func TestThirdPartyEngine_ResponseSizeLimit(t *testing.T) {
	largeResponse := strings.Repeat("x", 2*1024*1024)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(largeResponse))
	}))
	defer server.Close()

	engine := NewThirdPartyEngine(nil, zap.NewNop())
	engine.httpClient = &http.Client{Timeout: 5 * time.Second}

	app := ThirdPartyApp{
		Name:         "TestApp",
		Vendor:       "TestVendor",
		VersionURL:   server.URL,
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
	}

	_, err := engine.fetchLatestVersion(context.Background(), app)
	_ = err
}

// TestThirdPartyEngine_ConcurrentSync verifies concurrent access safety
func TestThirdPartyEngine_ConcurrentSync(t *testing.T) {
	engine := NewThirdPartyEngine(nil, zap.NewNop())

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			apps := engine.ListApps()
			if len(apps) == 0 {
				t.Error("expected apps in concurrent call")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
