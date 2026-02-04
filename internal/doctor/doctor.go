// Package doctor provides health checks for coop's dependencies and configuration.
package doctor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	incus "github.com/lxc/incus/v6/client"
	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/platform"
)

// CheckStatus represents the result of a health check.
type CheckStatus int

const (
	StatusPass CheckStatus = iota
	StatusWarn
	StatusFail
	StatusSkip
)

func (s CheckStatus) String() string {
	switch s {
	case StatusPass:
		return "✓"
	case StatusWarn:
		return "!"
	case StatusFail:
		return "✗"
	case StatusSkip:
		return "-"
	default:
		return "?"
	}
}

// CheckResult holds the outcome of a single health check.
type CheckResult struct {
	Name    string
	Status  CheckStatus
	Message string
	Fix     string // Suggested fix command or action
}

// Report holds all check results.
type Report struct {
	Results  []CheckResult
	Platform platform.Type
}

// VMBackendChecker provides VM-backend-specific health checks.
type VMBackendChecker interface {
	// Name returns the backend name (e.g., "colima", "lima")
	Name() string
	// Available returns true if this backend is installed
	Available() bool
	// Checks returns the health checks for this backend
	Checks(cfg *config.Config) []CheckResult
	// GetIncusSocketPath returns the Incus socket path for this backend
	GetIncusSocketPath(cfg *config.Config) string
	// GetContainerSubnets returns known container subnets for route checking
	GetContainerSubnets(cfg *config.Config) []string
}

// Run executes all health checks and returns a report.
func Run(cfg *config.Config) *Report {
	report := &Report{
		Platform: platform.Detect(),
	}

	// Select and run VM backend checks (macOS only)
	var vmChecker VMBackendChecker
	if report.Platform == platform.MacOS {
		vmChecker = selectVMBackend(cfg)
		if vmChecker != nil {
			report.Results = append(report.Results, vmChecker.Checks(cfg)...)
		}
	}

	// Common Incus checks (all platforms)
	report.Results = append(report.Results, checkIncusCLI())
	report.Results = append(report.Results, checkIncusSocket(cfg, vmChecker))
	report.Results = append(report.Results, checkIncusConnection(cfg, vmChecker))
	report.Results = append(report.Results, checkIncusStoragePool(cfg, vmChecker))
	report.Results = append(report.Results, checkIncusNetwork(cfg, vmChecker))

	// Network routing (macOS only - containers need route to VM)
	if report.Platform == platform.MacOS && vmChecker != nil {
		report.Results = append(report.Results, checkNetworkRoute(cfg, vmChecker))
	}

	// Coop config checks
	report.Results = append(report.Results, checkCoopDirectories(cfg))
	report.Results = append(report.Results, checkSSHKeys(cfg))
	report.Results = append(report.Results, checkBaseImage(cfg, vmChecker))

	return report
}

// selectVMBackend returns the appropriate VM backend checker
func selectVMBackend(cfg *config.Config) VMBackendChecker {
	// Try backends in priority order
	backends := []VMBackendChecker{
		&ColimaChecker{},
		&LimaChecker{},
	}

	// Check configured priority
	priority := cfg.Settings.VM.BackendPriority
	if len(priority) > 0 {
		for _, name := range priority {
			for _, b := range backends {
				if b.Name() == name && b.Available() {
					return b
				}
			}
		}
	}

	// Fall back to first available
	for _, b := range backends {
		if b.Available() {
			return b
		}
	}

	return nil
}

// Summary returns pass/warn/fail counts.
func (r *Report) Summary() (pass, warn, fail int) {
	for _, res := range r.Results {
		switch res.Status {
		case StatusPass:
			pass++
		case StatusWarn:
			warn++
		case StatusFail:
			fail++
		}
	}
	return
}

// HasFailures returns true if any check failed.
func (r *Report) HasFailures() bool {
	for _, res := range r.Results {
		if res.Status == StatusFail {
			return true
		}
	}
	return false
}

// --- VM Backend Checkers ---

// ColimaChecker provides health checks for the Colima VM backend.
type ColimaChecker struct{}

const (
	// MinColimaVersion is the minimum Colima version required for Incus runtime support.
	MinColimaVersion = "0.7.0"
)

func (c *ColimaChecker) Name() string { return "colima" }

func (c *ColimaChecker) Available() bool {
	_, err := exec.LookPath("colima")
	return err == nil
}

func (c *ColimaChecker) Checks(cfg *config.Config) []CheckResult {
	var results []CheckResult

	// Check Colima installation
	installResult := CheckResult{Name: "Colima installed"}
	if !c.Available() {
		installResult.Status = StatusFail
		installResult.Message = "colima not found in PATH"
		installResult.Fix = "brew install colima"
		results = append(results, installResult)
		return results
	}
	installResult.Status = StatusPass
	installResult.Message = "colima available"
	results = append(results, installResult)

	// Check Colima version (>= 0.7.0 required for Incus runtime)
	versionResult := CheckResult{Name: "Colima version"}
	cmd := exec.Command("colima", "version")
	output, err := cmd.Output()
	if err != nil {
		versionResult.Status = StatusWarn
		versionResult.Message = "cannot determine version"
		results = append(results, versionResult)
	} else {
		// Parse version from output like "colima version 0.9.1"
		versionStr := strings.TrimSpace(string(output))
		if idx := strings.Index(versionStr, "colima version "); idx >= 0 {
			version := strings.Fields(versionStr[idx+15:])[0]
			if compareVersions(version, MinColimaVersion) < 0 {
				versionResult.Status = StatusFail
				versionResult.Message = fmt.Sprintf("%s (minimum: %s for Incus support)", version, MinColimaVersion)
				versionResult.Fix = "brew upgrade colima"
				results = append(results, versionResult)
				return results
			}
			versionResult.Status = StatusPass
			versionResult.Message = version
		} else {
			versionResult.Status = StatusWarn
			versionResult.Message = "cannot parse version"
		}
		results = append(results, versionResult)
	}

	// Check Colima running
	runningResult := CheckResult{Name: "Colima VM running"}
	instance := cfg.Settings.VM.Instance
	if instance == "" {
		instance = "incus"
	}

	// Use colima list to check status
	cmd = exec.Command("colima", "list", "--json")
	output, err = cmd.Output()
	if err != nil {
		runningResult.Status = StatusFail
		runningResult.Message = "cannot get colima status"
		runningResult.Fix = fmt.Sprintf("colima start --profile %s --runtime incus --vm-type vz --network-address", instance)
		results = append(results, runningResult)
		return results
	}

	// Parse JSON output to find our instance
	outputStr := string(output)
	if strings.Contains(outputStr, fmt.Sprintf(`"name":"%s"`, instance)) && strings.Contains(outputStr, `"status":"Running"`) {
		// Extract CPU/memory from output
		runningResult.Status = StatusPass
		runningResult.Message = fmt.Sprintf("%s is running", instance)
	} else if strings.Contains(outputStr, fmt.Sprintf(`"name":"%s"`, instance)) {
		runningResult.Status = StatusFail
		runningResult.Message = fmt.Sprintf("%s is stopped", instance)
		runningResult.Fix = fmt.Sprintf("colima start %s", instance)
	} else {
		runningResult.Status = StatusFail
		runningResult.Message = fmt.Sprintf("%s VM does not exist", instance)
		runningResult.Fix = fmt.Sprintf("colima start --profile %s --runtime incus --vm-type vz --network-address", instance)
	}
	results = append(results, runningResult)

	return results
}

func (c *ColimaChecker) GetIncusSocketPath(cfg *config.Config) string {
	if cfg.Settings.IncusSocket != "" {
		return cfg.Settings.IncusSocket
	}
	home, _ := os.UserHomeDir()
	instance := cfg.Settings.VM.Instance
	if instance == "" {
		instance = "incus"
	}
	return filepath.Join(home, ".colima", instance, "incus.sock")
}

func (c *ColimaChecker) GetContainerSubnets(cfg *config.Config) []string {
	return []string{
		"10.166.11.0/24",  // Colima default
		"192.100.0.0/24",  // README documented
	}
}

// LimaChecker provides health checks for the Lima VM backend.
type LimaChecker struct{}

func (l *LimaChecker) Name() string { return "lima" }

func (l *LimaChecker) Available() bool {
	_, err := exec.LookPath("limactl")
	return err == nil
}

func (l *LimaChecker) Checks(cfg *config.Config) []CheckResult {
	var results []CheckResult

	// Check Lima installation
	installResult := CheckResult{Name: "Lima installed"}
	if !l.Available() {
		installResult.Status = StatusFail
		installResult.Message = "limactl not found in PATH"
		installResult.Fix = "brew install lima"
		results = append(results, installResult)
		return results
	}
	installResult.Status = StatusPass
	installResult.Message = "lima available"
	results = append(results, installResult)

	// Check Lima running
	runningResult := CheckResult{Name: "Lima VM running"}
	instance := cfg.Settings.VM.Instance
	if instance == "" {
		instance = "incus"
	}

	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		runningResult.Status = StatusFail
		runningResult.Message = "cannot get lima status"
		runningResult.Fix = fmt.Sprintf("limactl start %s", instance)
		results = append(results, runningResult)
		return results
	}

	outputStr := string(output)
	if strings.Contains(outputStr, fmt.Sprintf(`"name":"%s"`, instance)) && strings.Contains(outputStr, `"status":"Running"`) {
		runningResult.Status = StatusPass
		runningResult.Message = fmt.Sprintf("%s is running", instance)
	} else if strings.Contains(outputStr, fmt.Sprintf(`"name":"%s"`, instance)) {
		runningResult.Status = StatusFail
		runningResult.Message = fmt.Sprintf("%s is stopped", instance)
		runningResult.Fix = fmt.Sprintf("limactl start %s", instance)
	} else {
		runningResult.Status = StatusFail
		runningResult.Message = fmt.Sprintf("%s VM does not exist", instance)
		runningResult.Fix = fmt.Sprintf("limactl start --name=%s template://incus", instance)
	}
	results = append(results, runningResult)

	return results
}

func (l *LimaChecker) GetIncusSocketPath(cfg *config.Config) string {
	if cfg.Settings.IncusSocket != "" {
		return cfg.Settings.IncusSocket
	}
	home, _ := os.UserHomeDir()
	instance := cfg.Settings.VM.Instance
	if instance == "" {
		instance = "incus"
	}
	return filepath.Join(home, ".lima", instance, "sock", "incus.sock")
}

func (l *LimaChecker) GetContainerSubnets(cfg *config.Config) []string {
	return []string{
		"10.0.100.0/24",   // Lima template default
		"192.100.0.0/24",  // README documented
	}
}

// --- Common Check Functions ---

func checkIncusCLI() CheckResult {
	result := CheckResult{Name: "Incus CLI installed"}

	path, err := exec.LookPath("incus")
	if err != nil {
		result.Status = StatusFail
		result.Message = "incus not found in PATH"
		result.Fix = "brew install incus"
		return result
	}

	// Get version
	cmd := exec.Command(path, "version")
	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusWarn
		result.Message = "incus found but version check failed"
		return result
	}

	version := strings.TrimSpace(string(output))
	result.Status = StatusPass
	result.Message = version
	return result
}

func checkIncusSocket(cfg *config.Config, vmChecker VMBackendChecker) CheckResult {
	result := CheckResult{Name: "Incus socket exists"}

	socketPath := getIncusSocketPath(cfg, vmChecker)
	if socketPath == "" {
		result.Status = StatusSkip
		result.Message = "cannot determine socket path"
		return result
	}

	// Remove unix:// prefix for stat
	cleanPath := strings.TrimPrefix(socketPath, "unix://")

	if _, err := os.Stat(cleanPath); err != nil {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("socket not found: %s", cleanPath)
		if platform.Detect() == platform.MacOS {
			result.Fix = "coop vm start"
		}
		return result
	}

	result.Status = StatusPass
	result.Message = cleanPath
	return result
}

func checkIncusConnection(cfg *config.Config, vmChecker VMBackendChecker) CheckResult {
	result := CheckResult{Name: "Incus API reachable"}

	socketPath := getIncusSocketPath(cfg, vmChecker)
	if socketPath == "" {
		result.Status = StatusSkip
		result.Message = "socket path unknown"
		return result
	}

	cleanPath := strings.TrimPrefix(socketPath, "unix://")
	conn, err := incus.ConnectIncusUnix(cleanPath, nil)
	if err != nil {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("connection failed: %v", err)
		return result
	}

	// Try a simple API call
	_, _, err = conn.GetServer()
	if err != nil {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("API call failed: %v", err)
		return result
	}

	result.Status = StatusPass
	result.Message = "connected successfully"
	return result
}

func checkIncusStoragePool(cfg *config.Config, vmChecker VMBackendChecker) CheckResult {
	result := CheckResult{Name: "Incus storage pool"}

	conn := connectIncus(cfg, vmChecker)
	if conn == nil {
		result.Status = StatusSkip
		result.Message = "not connected"
		return result
	}

	pools, err := conn.GetStoragePoolNames()
	if err != nil {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("failed to list pools: %v", err)
		return result
	}

	if len(pools) == 0 {
		result.Status = StatusFail
		result.Message = "no storage pools configured"
		result.Fix = "incus storage create default dir"
		return result
	}

	// Check for 'default' pool and get its resources
	hasDefault := false
	var spaceInfo string
	for _, p := range pools {
		if p == "default" {
			hasDefault = true
			// Get storage pool resources
			resources, err := conn.GetStoragePoolResources(p)
			if err == nil && resources.Space.Total > 0 {
				used := resources.Space.Used
				total := resources.Space.Total
				avail := total - used
				spaceInfo = fmt.Sprintf(" (%s available / %s total)", formatBytes(avail), formatBytes(total))
			}
			break
		}
	}

	if hasDefault {
		result.Status = StatusPass
		result.Message = fmt.Sprintf("default pool exists%s", spaceInfo)
	} else {
		result.Status = StatusWarn
		result.Message = fmt.Sprintf("no 'default' pool (have: %s)", strings.Join(pools, ", "))
	}

	return result
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func checkIncusNetwork(cfg *config.Config, vmChecker VMBackendChecker) CheckResult {
	result := CheckResult{Name: "Incus network bridge"}

	conn := connectIncus(cfg, vmChecker)
	if conn == nil {
		result.Status = StatusSkip
		result.Message = "not connected"
		return result
	}

	networks, err := conn.GetNetworkNames()
	if err != nil {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("failed to list networks: %v", err)
		return result
	}

	// Look for bridge network (incusbr0 or similar)
	var bridges []string
	for _, n := range networks {
		network, _, err := conn.GetNetwork(n)
		if err != nil {
			continue
		}
		if network.Type == "bridge" {
			bridges = append(bridges, n)
		}
	}

	if len(bridges) == 0 {
		result.Status = StatusFail
		result.Message = "no bridge network found"
		return result
	}

	result.Status = StatusPass
	result.Message = strings.Join(bridges, ", ")
	return result
}

func checkNetworkRoute(cfg *config.Config, vmChecker VMBackendChecker) CheckResult {
	result := CheckResult{Name: "Network route to containers"}

	// Get subnets from VM backend
	var subnets []string
	if vmChecker != nil {
		subnets = vmChecker.GetContainerSubnets(cfg)
	}

	// Try to get the actual subnet from Incus
	conn := connectIncus(cfg, vmChecker)
	if conn != nil {
		networks, _ := conn.GetNetworkNames()
		for _, n := range networks {
			network, _, err := conn.GetNetwork(n)
			if err != nil || network.Type != "bridge" {
				continue
			}
			if addr := network.Config["ipv4.address"]; addr != "" {
				// Convert 10.0.100.1/24 to 10.0.100.0/24
				if ip, ipnet, err := net.ParseCIDR(addr); err == nil {
					ipnet.IP = ip.Mask(ipnet.Mask)
					subnets = append([]string{ipnet.String()}, subnets...)
				}
			}
		}
	}

	// Check if any routes exist to these subnets
	for _, subnet := range subnets {
		if hasRouteToSubnet(subnet) {
			result.Status = StatusPass
			result.Message = fmt.Sprintf("route exists for %s", subnet)
			return result
		}
	}

	result.Status = StatusWarn
	result.Message = "no route to container network (SSH still works via port forward)"
	if vmChecker != nil && vmChecker.Name() == "colima" {
		instance := cfg.Settings.VM.Instance
		if instance == "" {
			instance = "incus"
		}
		result.Fix = fmt.Sprintf("sudo route add -net 10.0.100.0/24 $(colima list -p %s -j | jq -r '.address')", instance)
	}
	return result
}

func checkCoopDirectories(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "Coop directories"}

	dirs := config.GetDirectories()
	missing := []string{}

	checks := []string{dirs.Config, dirs.Data, dirs.Cache, dirs.SSH}
	for _, d := range checks {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			missing = append(missing, d)
		}
	}

	if len(missing) > 0 {
		result.Status = StatusFail
		result.Message = fmt.Sprintf("missing: %s", strings.Join(missing, ", "))
		result.Fix = "coop init"
		return result
	}

	result.Status = StatusPass
	result.Message = "all directories exist"
	return result
}

func checkSSHKeys(cfg *config.Config) CheckResult {
	result := CheckResult{Name: "SSH keys"}

	dirs := config.GetDirectories()
	keyPath := filepath.Join(dirs.SSH, "id_ed25519")

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		result.Status = StatusFail
		result.Message = "SSH keys not generated"
		result.Fix = "coop init"
		return result
	}

	result.Status = StatusPass
	result.Message = keyPath
	return result
}

func checkBaseImage(cfg *config.Config, vmChecker VMBackendChecker) CheckResult {
	result := CheckResult{Name: "Base image"}

	conn := connectIncus(cfg, vmChecker)
	if conn == nil {
		result.Status = StatusSkip
		result.Message = "not connected to Incus"
		return result
	}

	// Use configured image name, not hardcoded
	imageName := cfg.Settings.DefaultImage
	if imageName == "" {
		imageName = "coop-agent-base"
	}

	aliases, err := conn.GetImageAliases()
	if err != nil {
		result.Status = StatusWarn
		result.Message = fmt.Sprintf("failed to list images: %v", err)
		return result
	}

	for _, alias := range aliases {
		if alias.Name == imageName {
			result.Status = StatusPass
			result.Message = fmt.Sprintf("%s exists", imageName)
			return result
		}
	}

	result.Status = StatusWarn
	result.Message = fmt.Sprintf("%s not found (containers will use slow fallback)", imageName)
	result.Fix = "coop image build"
	return result
}

// --- Helper functions ---

func getIncusSocketPath(cfg *config.Config, vmChecker VMBackendChecker) string {
	if cfg.Settings.IncusSocket != "" {
		return cfg.Settings.IncusSocket
	}

	// Use VM backend checker if available
	if vmChecker != nil {
		return vmChecker.GetIncusSocketPath(cfg)
	}

	// Fallback for Linux
	switch platform.Detect() {
	case platform.Linux, platform.WSL2:
		return "/var/lib/incus/unix.socket"
	}
	return ""
}

func connectIncus(cfg *config.Config, vmChecker VMBackendChecker) incus.InstanceServer {
	socketPath := getIncusSocketPath(cfg, vmChecker)
	if socketPath == "" {
		return nil
	}
	cleanPath := strings.TrimPrefix(socketPath, "unix://")
	conn, err := incus.ConnectIncusUnix(cleanPath, nil)
	if err != nil {
		return nil
	}
	return conn
}

func hasRouteToSubnet(subnet string) bool {
	if runtime.GOOS != "darwin" {
		return true // Assume Linux has local Incus
	}

	// Parse subnet to get network address
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return false
	}

	// Try to connect to an IP in the subnet with short timeout
	// This is a heuristic - if we can't route, connection will fail fast
	testIP := make(net.IP, len(ipnet.IP))
	copy(testIP, ipnet.IP)
	testIP[3] = 1 // e.g., 10.0.100.1

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:22", testIP), 100*time.Millisecond)
	if err != nil {
		// Check if it's a "no route" error vs connection refused
		if strings.Contains(err.Error(), "no route") || strings.Contains(err.Error(), "network is unreachable") {
			return false
		}
		// Connection refused or timeout means route exists
		return true
	}
	conn.Close()
	return true
}

// compareVersions compares two semantic version strings.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Compare each part
	for i := 0; i < len(parts1) || i < len(parts2); i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}
