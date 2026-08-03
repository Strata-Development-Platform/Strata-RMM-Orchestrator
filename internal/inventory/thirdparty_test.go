package inventory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildDownloadURL(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		version     string
		wantContain string
		wantNotNil  bool
	}{
		{
			name:        "version placeholder in template",
			template:    "https://example.com/download/{version}/package.exe",
			version:     "1.2.3",
			wantContain: "1.2.3",
		},
		{
			name:        "major minor patch placeholders",
			template:    "https://example.com/{major}/{minor}/pkg-{patch}.exe",
			version:     "1.2.3",
			wantContain: "1/2/pkg-3.exe",
		},
		{
			name:        "no placeholders - returns template as-is",
			template:    "https://dl.google.com/chrome/install/googlechromestandaloneenterprise64.msi",
			version:     "120.0.0.0",
			wantContain: "googlechromestandaloneenterprise64.msi",
		},
		{
			name:        "no placeholders - Firefox URL preserved",
			template:    "https://download.mozilla.org/?product=firefox-msi-latest-ssl&os=win64&lang=en-US",
			version:     "120.0",
			wantContain: "product=firefox-msi-latest",
		},
		{
			name:        "only version placeholder",
			template:    "https://example.com/v{version}/installer.exe",
			version:     "3.1.4",
			wantContain: "v3.1.4/installer.exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := ThirdPartyApp{URLTemplate: tt.template}
			url := (&ThirdPartyEngine{}).buildDownloadURL(app, tt.version)
			if url == "" {
				t.Fatal("buildDownloadURL returned empty URL")
			}
			if !strings.Contains(url, tt.wantContain) {
				t.Errorf("buildDownloadURL(%q, %q) = %q, want to contain %q",
					tt.template, tt.version, url, tt.wantContain)
			}
			// Verify no-placeholder templates are unchanged
			if !strings.Contains(tt.template, "{") {
				if url != tt.template {
					t.Errorf("expected unchanged template, got %q", url)
				}
			}
		})
	}
}

func TestFetchLatestVersion_MockHTTP(t *testing.T) {
	tests := []struct {
		name         string
		versionURL   string
		versionRegex string
		responseBody string
		wantVersion  string
		wantErr      bool
	}{
		{
			name:         "Chrome version from Google API",
			versionURL:   "https://example.com/version",
			versionRegex: `"version":\s*"(\d+\.\d+\.\d+\.\d+)"`,
			responseBody: `{"version": "120.0.6099.130"}`,
			wantVersion:  "120.0.6099.130",
		},
		{
			name:         "GitHub release tag",
			versionURL:   "https://api.github.com/repos/test/test/releases/latest",
			versionRegex: `"tag_name":\s*"v(\d+\.\d+(\.\d+)?)"`,
			responseBody: `{"tag_name": "v2.1.0"}`,
			wantVersion:  "2.1.0",
		},
		{
			name:         "version not found - regex mismatch",
			versionURL:   "https://example.com/version",
			versionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
			responseBody: `{"version": "not-a-version"}`,
			wantVersion:  "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			app := ThirdPartyApp{
				VersionURL:   server.URL,
				VersionRegex: tt.versionRegex,
				URLTemplate:  "https://example.com/pkg.exe",
				InstallArgs:  "/quiet",
				DetectCmd:    "detect-cmd",
				Name:         "TestApp",
				Vendor:       "TestVendor",
				Platform:     "windows",
				PackageType:  "exe",
			}

			engine := NewThirdPartyEngine(nil, nil)
			engine.httpClient = &http.Client{Timeout: 5 * time.Second}

			version, err := engine.fetchLatestVersion(context.Background(), app)
			if (err != nil) != tt.wantErr {
				t.Fatalf("fetchLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if version != tt.wantVersion {
				t.Errorf("fetchLatestVersion() = %q, want %q", version, tt.wantVersion)
			}
		})
	}
}

func TestVersionRegexMatching(t *testing.T) {
	tests := []struct {
		name     string
		app      ThirdPartyApp
		response string
		want     string
	}{
		{
			name: "7-Zip HTML parse",
			app: ThirdPartyApp{
				Name:         "7-Zip",
				VersionURL:   "https://7-zip.org/download.html",
				VersionRegex: `Download 7-Zip (\d+\.\d+)`,
			},
			response: `<!DOCTYPE html><html><body><h1>Download 7-Zip 24.09</h1></body></html>`,
			want:     "24.09",
		},
		{
			name: "Zoom REST API",
			app: ThirdPartyApp{
				Name:         "Zoom",
				VersionURL:   "https://zoom.us/rest/download?os=win64",
				VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)"`,
			},
			response: `{"version": "5.16.10", "downloadUrl": "https://zoom.us/client/latest/ZoomInstallerFull.msi"}`,
			want:     "5.16.10",
		},
		{
			name: "LibreOffice HTML parse",
			app: ThirdPartyApp{
				Name:         "LibreOffice",
				VersionURL:   "https://www.libreoffice.org/download/download/",
				VersionRegex: `LibreOffice (\d+\.\d+\.\d+)`,
			},
			response: `Download LibreOffice 24.8.3 for Windows`,
			want:     "24.8.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			app := tt.app
			app.VersionURL = server.URL

			engine := NewThirdPartyEngine(nil, nil)
			engine.httpClient = &http.Client{Timeout: 5 * time.Second}

			version, err := engine.fetchLatestVersion(context.Background(), app)
			if err != nil {
				t.Fatalf("fetchLatestVersion() error = %v", err)
			}
			if version != tt.want {
				t.Errorf("got %q, want %q", version, tt.want)
			}
		})
	}
}
