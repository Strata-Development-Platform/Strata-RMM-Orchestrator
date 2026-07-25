package collectors

import (
	"context"
	"runtime"
	"time"
)

type Software struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Publisher   string `json:"publisher"`
	InstallDate string `json:"install_date,omitempty"`
	SizeMB      int64  `json:"size_mb,omitempty"`
	Source      string `json:"source"` // system, choco, winget, brew, flatpak, snap
}

type SoftwareInventory struct {
	DeviceID  string     `json:"device_id"`
	OS        string     `json:"os"`
	OSVersion string     `json:"os_version"`
	Packages  []Software `json:"packages"`
	CollectedAt time.Time `json:"collected_at"`
}

type SoftwareCollector struct{}

func NewSoftwareCollector() *SoftwareCollector {
	return &SoftwareCollector{}
}

func (sc *SoftwareCollector) Collect(ctx context.Context) (*SoftwareInventory, error) {
	inv := &SoftwareInventory{
		OS:          runtime.GOOS,
		OSVersion:   runtime.GOARCH,
		CollectedAt: time.Now().UTC(),
	}

	switch runtime.GOOS {
	case "linux":
		packages, err := sc.collectLinux(ctx)
		if err != nil {
			return nil, err
		}
		inv.Packages = packages
	case "windows":
		packages, err := sc.collectWindows(ctx)
		if err != nil {
			return nil, err
		}
		inv.Packages = packages
	}

	return inv, nil
}

func (sc *SoftwareCollector) collectLinux(ctx context.Context) ([]Software, error) {
	return sc.readDpkg(ctx)
}

func (sc *SoftwareCollector) readDpkg(ctx context.Context) ([]Software, error) {
	return runAndParse(ctx, "dpkg-query", []string{"-W", "-f", "${Package}\t${Version}\t${Installed-Size}\n"}, func(lines []string) []Software {
		var packages []Software
		for _, line := range lines {
			parts := splitN(line, "\t", 3)
			if len(parts) < 2 {
				continue
			}
			pkg := Software{
				Name:    parts[0],
				Version: parts[1],
				Source:  "dpkg",
			}
			if len(parts) == 3 {
				size := parseInt64(parts[2], 0)
				pkg.SizeMB = size / 1024
			}
			packages = append(packages, pkg)
		}
		return packages
	})
}

func (sc *SoftwareCollector) collectWindows(ctx context.Context) ([]Software, error) {
	ps := `Get-WmiObject -Class Win32_Product | Select-Object Name, Version, Vendor, InstallDate, PackageSize | ConvertTo-Json -Compress`
	return runAndParse(ctx, "powershell", []string{"-NoProfile", "-Command", ps}, func(lines []string) []Software {
		if len(lines) == 0 {
			return nil
		}
		return nil
	})
}
