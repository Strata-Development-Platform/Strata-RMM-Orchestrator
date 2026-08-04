package core

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// Role represents a detected server role.
type Role string

const (
	RoleADDomainController Role = "ad_dc"
	RoleSQLServer          Role = "sql_server"
	RoleHyperV             Role = "hyper_v"
	RoleLinuxDNS           Role = "linux_dns"
	RoleLinuxWebServer     Role = "linux_web_server"
	RoleLinuxDatabase      Role = "linux_database"
)

// roleID uniquely identifies a roleChecker function.
type roleID int

const (
	idADDomainController roleID = iota
	idSQLServer
	idHyperV
	idLinuxDNS
	idLinuxWebServer
	idLinuxDatabase
)

// roleChecker detects a single role and returns true if detected.
type roleChecker struct {
	id   roleID
	name string
	fn   func(ctx context.Context) bool
}

// RoleScanner detects server roles on the local machine.
// On Windows, it uses WMI/CIM to detect AD DC, SQL Server, Hyper-V.
// On Linux, it uses systemd to detect DNS, web server, database services.
type RoleScanner struct {
	checkers []roleChecker
}

// NewRoleScanner creates a new RoleScanner with platform-appropriate checkers.
func NewRoleScanner() *RoleScanner {
	scanner := &RoleScanner{}

	switch runtime.GOOS {
	case "windows":
		scanner.checkers = []roleChecker{
			{idADDomainController, string(RoleADDomainController), checkADDomainController},
			{idSQLServer, string(RoleSQLServer), checkSQLServer},
			{idHyperV, string(RoleHyperV), checkHyperV},
		}
	case "linux":
		scanner.checkers = []roleChecker{
			{idLinuxDNS, string(RoleLinuxDNS), checkLinuxDNS},
			{idLinuxWebServer, string(RoleLinuxWebServer), checkLinuxWebServer},
			{idLinuxDatabase, string(RoleLinuxDatabase), checkLinuxDatabase},
		}
	default:
		scanner.checkers = []roleChecker{}
	}

	return scanner
}

// Scan returns the list of roles detected on the local machine.
func (rs *RoleScanner) Scan(ctx context.Context) []string {
	roles := make([]string, 0, len(rs.checkers))

	for _, c := range rs.checkers {
		select {
		case <-ctx.Done():
			return roles
		default:
			if c.fn(ctx) {
				roles = append(roles, c.name)
			}
		}
	}

	return roles
}

// HasRole checks if the given role was detected.
func (rs *RoleScanner) HasRole(ctx context.Context, role Role) bool {
	detected := rs.Scan(ctx)
	for _, r := range detected {
		if r == string(role) {
			return true
		}
	}
	return false
}

// --- Windows role checkers ---

// checkADDomainController detects if the machine is an Active Directory Domain Controller.
func checkADDomainController(ctx context.Context) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	// Check via WMIC for Domain role
	cmd := exec.CommandContext(ctx, "wmic", "computersystem", "get", "DomainRole")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// DomainRole 5 = Domain Controller
	// DomainRole 4 = Standalone Server (not DC)
	// DomainRole 3 = Member Server
	// DomainRole 2 = Workstation
	// DomainRole 1 = Standalone Workstation
	// DomainRole 0 = Unknown
	outputLower := strings.ToLower(strings.TrimSpace(string(output)))
	return strings.Contains(outputLower, "5") || strings.Contains(outputLower, "domain controller")
}

// checkSQLServer detects if SQL Server is installed and running.
func checkSQLServer(ctx context.Context) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	// Check for SQL Server service
	cmd := exec.CommandContext(ctx, "sc", "query", "MSSQLSERVER")
	err := cmd.Run()
	if err == nil {
		return true
	}

	// Also check for named instances
	cmd = exec.CommandContext(ctx, "sc", "query", "SQLSERVERAGENT")
	return cmd.Run() == nil
}

// checkHyperV detects if Hyper-V is installed.
func checkHyperV(ctx context.Context) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	// Check via DISM for Hyper-V feature
	cmd := exec.CommandContext(ctx, "dism", "/online", "/Get-FeatureName", "/FeatureName:Microsoft-Hyper-V-All")
	output, err := cmd.Output()
	if err != nil {
		// Try alternative method via PowerShell
		psCmd := exec.CommandContext(ctx, "powershell", "-Command", "Get-WindowsFeature -Name Hyper-V")
		psOutput, psErr := psCmd.Output()
		if psErr == nil {
			return strings.Contains(strings.ToLower(string(psOutput)), "installed")
		}
		return false
	}

	return strings.Contains(strings.ToLower(string(output)), "state : enabled")
}

// --- Linux role checkers ---

// checkLinuxDNS detects if DNS service (BIND, Knot, Unbound) is running.
func checkLinuxDNS(ctx context.Context) bool {
	checkers := []string{"named", "knot", "unbound", "bind9"}
	for _, svc := range checkers {
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", svc)
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "active" {
			return true
		}
	}
	return false
}

// checkLinuxWebServer detects if a web server (Apache, Nginx, Caddy) is running.
func checkLinuxWebServer(ctx context.Context) bool {
	checkers := []string{"nginx", "apache2", "httpd", "caddy"}
	for _, svc := range checkers {
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", svc)
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "active" {
			return true
		}
	}
	return false
}

// checkLinuxDatabase detects if a database server (PostgreSQL, MySQL, MariaDB) is running.
func checkLinuxDatabase(ctx context.Context) bool {
	checkers := []string{"postgresql", "mysql", "mariadb", "mongod"}
	for _, svc := range checkers {
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", svc)
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "active" {
			return true
		}
	}
	return false
}
