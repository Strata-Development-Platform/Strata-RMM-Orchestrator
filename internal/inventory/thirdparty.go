package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

type ThirdPartyApp struct {
	Name           string `json:"name"`
	Vendor         string `json:"vendor"`
	Platform       string `json:"platform"`
	PackageType    string `json:"package_type"`
	DetectCmd      string `json:"detect_command"`
	URLTemplate    string `json:"url_template"`
	VersionURL     string `json:"version_url"`
	VersionRegex   string `json:"version_regex"`
	InstallArgs    string `json:"install_args"`
	CurrentVersion string `json:"current_version,omitempty"`
}

var ThirdPartyApps = []ThirdPartyApp{
	{
		Name: "Google Chrome", Vendor: "Google", Platform: "windows",
		PackageType: "msi", InstallArgs: "/quiet /norestart",
		URLTemplate:  "https://dl.google.com/dl/chrome/install/googlechromestandaloneenterprise64.msi",
		VersionURL:   "https://versionhistory.googleapis.com/v1/chrome/current",
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+\.\d+)"`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Google Chrome' | Select -ExpandProperty DisplayVersion",
	},
	{
		Name: "Mozilla Firefox", Vendor: "Mozilla", Platform: "windows",
		PackageType: "exe", InstallArgs: "-ms -ma",
		URLTemplate:  "https://download.mozilla.org/?product=firefox-msi-latest-ssl&os=win64&lang=en-US",
		VersionURL:   "https://product-details.mozilla.org/1.0/firefox_versions.json",
		VersionRegex: `"LATEST_FIREFOX_VERSION":\s*"(\d+\.\d+(\.\d+)?)"`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\Mozilla\\Mozilla Firefox' | Select -ExpandProperty CurrentVersion",
	},
	{
		Name: "Adobe Acrobat Reader", Vendor: "Adobe", Platform: "windows",
		PackageType: "msi", InstallArgs: "/quiet /norestart",
		URLTemplate:  "https://ardownload2.adobe.com/pub/adobe/reader/win/AcrobatDC/2300820544/AcroRdrDC2300820544_en_US.msi",
		VersionURL:   "https://www.adobe.com/devnet-docs/acrobatetk/tools/ReleaseNotesDC/index.html",
		VersionRegex: `DC.*?(\d{2}\.\d{3}\.\d{4}\.\d+)`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\Adobe\\Acrobat Reader\\DC\\Installation' | Select -ExpandProperty Version",
	},
	{
		Name: "7-Zip", Vendor: "Igor Pavlov", Platform: "windows",
		PackageType: "exe", InstallArgs: "/S",
		URLTemplate:  "https://7-zip.org/a/7z2408-x64.exe",
		VersionURL:   "https://7-zip.org/download.html",
		VersionRegex: `Download 7-Zip (\d+\.\d+)`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\7-Zip' | Select -ExpandProperty Version",
	},
	{
		Name: "VLC Media Player", Vendor: "VideoLAN", Platform: "windows",
		PackageType: "exe", InstallArgs: "/S",
		URLTemplate:  "https://get.videolan.org/vlc/3.0.20/win64/vlc-3.0.20-win64.exe",
		VersionURL:   "https://download.videolan.org/pub/videolan/vlc/last/win64/",
		VersionRegex: `vlc-(\d+\.\d+\.\d+)-win64\.exe`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\VideoLAN\\VLC' | Select -ExpandProperty Version",
	},
	{
		Name: "Microsoft Teams", Vendor: "Microsoft", Platform: "windows",
		PackageType: "exe", InstallArgs: "/quiet",
		URLTemplate:  "https://teams.microsoft.com/downloads/desktopurl?env=production&plat=windows&arch=x64&download=true",
		VersionURL:   "https://config.teams.microsoft.com/config/v1/MicrosoftTeams/1415_1.0?environment=prod",
		VersionRegex: `versionString.*?(\d+\.\d+\.\d+\.\d+)`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\Microsoft\\Teams' | Select -ExpandProperty Version",
	},
	{
		Name: "Zoom", Vendor: "Zoom Video", Platform: "windows",
		PackageType: "msi", InstallArgs: "/quiet /norestart",
		URLTemplate:  "https://zoom.us/client/latest/ZoomInstallerFull.msi",
		VersionURL:   "https://zoom.us/rest/download?os=win64",
		VersionRegex: `"version":\s*"(\d+\.\d+\.\d+)`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\Zoom\\Zoom' | Select -ExpandProperty Version",
	},
	{
		Name: "Notepad++", Vendor: "Notepad++", Platform: "windows",
		PackageType: "exe", InstallArgs: "/S",
		URLTemplate:  "https://github.com/notepad-plus-plus/notepad-plus-plus/releases/download/v8.7.1/npp.8.7.1.Installer.x64.exe",
		VersionURL:   "https://api.github.com/repos/notepad-plus-plus/notepad-plus-plus/releases/latest",
		VersionRegex: `"tag_name":\s*"v(\d+\.\d+(\.\d+)?)"`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\Notepad++' | Select -ExpandProperty Version",
	},
	{
		Name: "LibreOffice", Vendor: "The Document Foundation", Platform: "windows",
		PackageType: "msi", InstallArgs: "/quiet /norestart",
		URLTemplate:  "https://download.documentfoundation.org/libreoffice/stable/24.8.0/win/x86_64/LibreOffice_24.8.0_Win_x86-64.msi",
		VersionURL:   "https://www.libreoffice.org/download/download/",
		VersionRegex: `LibreOffice (\d+\.\d+\.\d+)`,
		DetectCmd:    "Get-ItemProperty 'HKLM:\\Software\\LibreOffice' | Select -ExpandProperty Version",
	},
}

type ThirdPartyEngine struct {
	db         *sql.DB
	logger     *zap.Logger
	httpClient *http.Client
	tenantID   string
}

func NewThirdPartyEngine(db *sql.DB, logger *zap.Logger) *ThirdPartyEngine {
	return &ThirdPartyEngine{
		db:         db,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *ThirdPartyEngine) SyncAll(ctx context.Context) ([]string, error) {
	var created []string
	for _, app := range ThirdPartyApps {
		version, err := e.fetchLatestVersion(ctx, app)
		if err != nil {
			e.logger.Warn("fetch version", zap.String("app", app.Name), zap.Error(err))
			continue
		}
		if version == "" {
			continue
		}

		exists, err := e.packageExists(ctx, app.Name, version)
		if err != nil || exists {
			continue
		}

		url := e.buildDownloadURL(app, version)
		if url == "" {
			continue
		}

		err = e.createPackage(ctx, app, version, url)
		if err != nil {
			e.logger.Warn("create package", zap.String("app", app.Name), zap.Error(err))
			continue
		}

		created = append(created, fmt.Sprintf("%s %s", app.Name, version))
		e.logger.Info("new third-party package created", zap.String("app", app.Name), zap.String("version", version))
	}
	return created, nil
}

func (e *ThirdPartyEngine) fetchLatestVersion(ctx context.Context, app ThirdPartyApp) (string, error) {
	if app.VersionURL == "" {
		return "", nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", app.VersionURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "StrataRMM/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, app.VersionURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if len(body) == 0 {
		return "", fmt.Errorf("empty response body")
	}

	re := regexp.MustCompile(app.VersionRegex)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return "", fmt.Errorf("version not found in response")
	}

	return matches[1], nil
}

func (e *ThirdPartyEngine) buildDownloadURL(app ThirdPartyApp, version string) string {
	url := app.URLTemplate
	hasPlaceholder := strings.Contains(url, "{version}") || strings.Contains(url, "{major}") || strings.Contains(url, "{minor}") || strings.Contains(url, "{patch}")
	if !hasPlaceholder {
		return url
	}
	versionParts := strings.Split(version, ".")
	major := ""
	minor := ""
	patch := ""
	if len(versionParts) > 0 {
		major = versionParts[0]
	}
	if len(versionParts) > 1 {
		minor = versionParts[1]
	}
	if len(versionParts) > 2 {
		patch = versionParts[2]
	}
	url = strings.ReplaceAll(url, "{version}", version)
	url = strings.ReplaceAll(url, "{major}", major)
	url = strings.ReplaceAll(url, "{minor}", minor)
	url = strings.ReplaceAll(url, "{patch}", patch)
	return url
}

func (e *ThirdPartyEngine) packageExists(ctx context.Context, name, version string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM software_packages WHERE name = $1 AND version = $2
	`, name, version).Scan(&count)
	return count > 0, err
}

func (e *ThirdPartyEngine) createPackage(ctx context.Context, app ThirdPartyApp, version, url string) error {
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO software_packages (tenant_id, name, version, description, platform, package_type, source_url, install_args, detect_command, is_third_party)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true)
	`, e.tenantID, app.Name, version,
		fmt.Sprintf("%s %s - %s", app.Vendor, app.Name, version),
		app.Platform, app.PackageType, url, app.InstallArgs, app.DetectCmd)
	return err
}

func (e *ThirdPartyEngine) SetTenantID(tenantID string) {
	e.tenantID = tenantID
}

func (e *ThirdPartyEngine) Start(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	e.logger.Info("third-party patching engine started", zap.Int("apps", len(ThirdPartyApps)))

	created, err := e.SyncAll(ctx)
	if err != nil {
		e.logger.Error("initial third-party sync", zap.Error(err))
	} else if len(created) > 0 {
		e.logger.Info("new packages created", zap.Strings("packages", created))
	}

	for {
		select {
		case <-ticker.C:
			created, err := e.SyncAll(ctx)
			if err != nil {
				e.logger.Error("third-party sync", zap.Error(err))
			} else if len(created) > 0 {
				e.logger.Info("new packages created", zap.Strings("packages", created))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (e *ThirdPartyEngine) ListApps() []ThirdPartyApp {
	return ThirdPartyApps
}

func (e *ThirdPartyEngine) GetPackages(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, name, version, description, platform, package_type, source_url, created_at
		FROM software_packages WHERE is_third_party = true
		ORDER BY name, created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pkgs []map[string]interface{}
	for rows.Next() {
		var id, name, version, desc, platform, pkgType, url string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &version, &desc, &platform, &pkgType, &url, &createdAt); err != nil {
			return nil, fmt.Errorf("scan package row: %w", err)
		}
		pkgs = append(pkgs, map[string]interface{}{
			"id": id, "name": name, "version": version, "description": desc,
			"platform": platform, "package_type": pkgType, "source_url": url, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package rows: %w", err)
	}
	return pkgs, nil
}

func (e *ThirdPartyEngine) SyncApp(ctx context.Context, appName string) (string, error) {
	for _, app := range ThirdPartyApps {
		if strings.EqualFold(app.Name, appName) {
			version, err := e.fetchLatestVersion(ctx, app)
			if err != nil {
				return "", fmt.Errorf("fetch version: %w", err)
			}
			url := e.buildDownloadURL(app, version)
			if err := e.createPackage(ctx, app, version, url); err != nil {
				return "", fmt.Errorf("create package: %w", err)
			}
			return fmt.Sprintf("%s %s", app.Name, version), nil
		}
	}
	return "", fmt.Errorf("app not found: %s", appName)
}

// Vendor represents a third-party software vendor
type Vendor struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	Apps     []string `json:"apps"`
	Platform string   `json:"platform"`
	Active   bool     `json:"active"`
	LastSync string   `json:"last_sync,omitempty"`
}

// DiscoverVendors returns all vendors from the app catalog
func (e *ThirdPartyEngine) DiscoverVendors() []Vendor {
	vendorMap := make(map[string]*Vendor)

	for _, app := range ThirdPartyApps {
		if _, exists := vendorMap[app.Vendor]; !exists {
			vendorMap[app.Vendor] = &Vendor{
				Name:     app.Vendor,
				Platform: app.Platform,
				Active:   true,
			}
		}
		vendorMap[app.Vendor].Apps = append(vendorMap[app.Vendor].Apps, app.Name)
	}

	vendors := make([]Vendor, 0, len(vendorMap))
	for _, v := range vendorMap {
		vendors = append(vendors, *v)
	}

	return vendors
}

// SyncVendor syncs all apps from a specific vendor
func (e *ThirdPartyEngine) SyncVendor(ctx context.Context, vendorName string) ([]string, error) {
	var created []string

	for _, app := range ThirdPartyApps {
		if !strings.EqualFold(app.Vendor, vendorName) {
			continue
		}

		version, err := e.fetchLatestVersion(ctx, app)
		if err != nil {
			e.logger.Warn("fetch vendor app version",
				zap.String("vendor", vendorName),
				zap.String("app", app.Name),
				zap.Error(err))
			continue
		}

		exists, err := e.packageExists(ctx, app.Name, version)
		if err != nil || exists {
			continue
		}

		url := e.buildDownloadURL(app, version)
		if url == "" {
			continue
		}

		if err := e.createPackage(ctx, app, version, url); err != nil {
			e.logger.Warn("create vendor package",
				zap.String("vendor", vendorName),
				zap.String("app", app.Name),
				zap.Error(err))
			continue
		}

		created = append(created, fmt.Sprintf("%s %s", app.Name, version))
	}

	return created, nil
}

// VendorStatus returns the sync status of all vendors
func (e *ThirdPartyEngine) VendorStatus(ctx context.Context) []map[string]interface{} {
	vendors := e.DiscoverVendors()
	status := make([]map[string]interface{}, 0, len(vendors))

	for _, v := range vendors {
		status = append(status, map[string]interface{}{
			"name":      v.Name,
			"platform":  v.Platform,
			"active":    v.Active,
			"apps":      v.Apps,
			"app_count": len(v.Apps),
			"last_sync": v.LastSync,
		})
	}

	return status
}
