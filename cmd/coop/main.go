// Command coop provides CLI for managing AI agent containers.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/stuffbucket/coop/internal/backend"
	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/doctor"
	"github.com/stuffbucket/coop/internal/logging"
	"github.com/stuffbucket/coop/internal/sandbox"
	"github.com/stuffbucket/coop/internal/state"
	"github.com/stuffbucket/coop/internal/ui"
)

// Build information, set via ldflags:
//
//	-X main.version={{.Version}}
//	-X main.commit={{.Commit}}
//	-X main.date={{.Date}}
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// appConfig holds the application configuration, loaded once at startup.
var appConfig *config.Config

func main() {
	// Load config once for the entire application
	cfg, err := config.Load()
	if err != nil {
		// Use defaults if config doesn't exist
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}
	appConfig = cfg

	// Initialize logging with loaded config
	initLogging(appConfig)
	defer func() { _ = logging.Close() }()

	// Apply UI theme from config
	theme := ui.ThemeByName(appConfig.Settings.UI.Theme)
	ui.SetTheme(theme)

	if len(os.Args) < 2 {
		printUsage(false)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		initCmd(os.Args[2:])
	case "create":
		createCmd(os.Args[2:])
	case "start":
		startCmd(os.Args[2:])
	case "stop":
		stopCmd(os.Args[2:])
	case "lock":
		lockCmd(os.Args[2:])
	case "unlock":
		unlockCmd(os.Args[2:])
	case "delete", "rm":
		deleteCmd(os.Args[2:])
	case "list", "ls":
		listCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "logs":
		logsCmd(os.Args[2:])
	case "shell":
		shellCmd(os.Args[2:])
	case "ssh":
		sshCmd(os.Args[2:])
	case "exec":
		execCmd(os.Args[2:])
	case "mount":
		mountCmd(os.Args[2:])
	case "snapshot":
		snapshotCmd(os.Args[2:])
	case "config":
		configCmd(os.Args[2:])
	case "theme":
		themeCmd(os.Args[2:])
	case "image":
		imageCmd(os.Args[2:])
	case "state":
		stateCmd(os.Args[2:])
	case "env":
		envCmd(os.Args[2:])
	case "vm", "lima": // lima kept as alias for backward compat
		vmCmd(os.Args[2:])
	case "doctor":
		doctorCmd(os.Args[2:])
	case "version", "-v", "--version":
		versionCmd()
	case "help", "-h", "--help":
		helpCmd(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage(false)
		os.Exit(1)
	}
}

func initLogging(cfg *config.Config) {
	logCfg := logging.Config{
		Dir:        cfg.Dirs.Logs,
		MaxSizeMB:  appConfig.Settings.Log.MaxSizeMB,
		MaxBackups: appConfig.Settings.Log.MaxBackups,
		MaxAgeDays: appConfig.Settings.Log.MaxAgeDays,
		Compress:   appConfig.Settings.Log.Compress,
		Debug:      appConfig.Settings.Log.Debug,
	}

	if err := logging.Init(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logging: %v\n", err)
	}
}

func helpCmd(args []string) {
	showAll := false
	for _, arg := range args {
		if arg == "--all" || arg == "-a" {
			showAll = true
		}
	}
	printUsage(showAll)
}

func versionCmd() {
	fmt.Printf("coop %s\n", version)
	if commit != "none" {
		fmt.Printf("  commit: %s\n", commit)
	}
	if date != "unknown" {
		fmt.Printf("  built:  %s\n", date)
	}
	fmt.Printf("  os:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  go:     %s\n", runtime.Version())
}

// detectHelpState checks the current state of coop for contextual help.
func detectHelpState() ui.HelpState {
	dirs := config.GetDirectories()

	state := ui.HelpState{
		IsMacOS: runtime.GOOS == "darwin",
	}

	// Check if initialized (settings file exists)
	if _, err := os.Stat(dirs.SettingsFile); err == nil {
		state.Initialized = true
	}

	// Check VM status (macOS only) - quick check, don't start anything
	if state.IsMacOS {
		// Try to create backend manager and check status without starting
		if vmMgr, err := backend.NewManager(appConfig); err == nil {
			if status, err := vmMgr.Status(); err == nil {
				state.VMRunning = status.State == "Running"
			}
		}
	} else {
		// On Linux, no VM needed
		state.VMRunning = true
	}

	// Check if base image exists - only if we can connect
	if state.Initialized && (state.VMRunning || !state.IsMacOS) {
		if mgr, err := sandbox.NewManagerWithConfig(appConfig); err == nil {
			state.BaseImageOK = mgr.ImageExists(appConfig.Settings.DefaultImage)
			// Check for containers and count them
			if containers, err := mgr.List(); err == nil {
				state.AgentCount = len(containers)
				state.HasContainers = len(containers) > 0
				for _, c := range containers {
					if c.Status == "Running" {
						state.RunningCount++
					}
				}
			}
			// Get storage info
			if avail, total, err := mgr.GetStorageInfo(); err == nil {
				state.StorageAvail = avail
				state.StorageTotal = total
			}
		}
	}

	return state
}

func printUsage(showAll bool) {
	// Detect current state for contextual help
	state := detectHelpState()
	builder := ui.NewHelpBuilder(state)
	dashboard := ui.NewDashboardProvider(state, version, "")

	logo := ui.Logo()
	if logo != "" {
		fmt.Print(logo)
	}
	fmt.Println(ui.Tagline(version))

	if !showAll {
		// Compact view - use narrower width that matches actual content
		compactWidth := builder.CompactLayoutWidth()
		if builder.Width() < compactWidth {
			compactWidth = builder.Width()
		}
		fmt.Println(ui.Separator(compactWidth - 1)) // -1 for indent
		fmt.Println()

		fmt.Println(" " + ui.HelpSection("Usage:"))
		fmt.Println("   coop <command> [options]")
		fmt.Println()

		builder.Add(ui.NewQuickHelpProvider())
		builder.Add(ui.NewQuickStartProvider(state))
		builder.Add(ui.NewHintProvider(false))
		fmt.Println(builder.RenderWithDashboard(dashboard))
		fmt.Println()
	} else {
		// Full view - use max-width constrained header
		headerWidth := builder.EffectiveWidth()
		fmt.Println(ui.Separator(headerWidth - 1)) // -1 for indent
		fmt.Println()

		fmt.Println(" " + ui.HelpSection("Usage:"))
		fmt.Println("   coop <command> [options]")
		fmt.Println()

		builder.Add(ui.NewCommandColumnsProvider())
		builder.Add(ui.NewGettingStartedProvider(state))
		builder.Add(ui.NewExamplesProvider(state))
		builder.Add(ui.NewTerminalInfoProvider(true))
		fmt.Println(builder.RenderWithDashboard(dashboard))
		fmt.Println()
	}
}

// mustManager creates a new sandbox.Manager or exits with an error.
// This centralizes the repeated pattern of creating a manager and handling errors.
func mustManager() *sandbox.Manager {
	mgr, err := sandbox.NewManagerWithConfig(appConfig)
	if err != nil {
		// Log full error context for debugging
		log := logging.Get()
		log.Debug("Manager creation failed", "error", err)

		// Check if this is a user cancellation - show clean message
		var cancelErr *backend.UserCancelError
		if errors.As(err, &cancelErr) {
			// Clean user-facing message
			fmt.Fprintln(os.Stderr)
			ui.Muted(cancelErr.Message)
			fmt.Fprintln(os.Stderr)
		} else {
			// Regular error with full context
			ui.Errorf("Error: %v", err)
		}
		os.Exit(1)
	}
	return mgr
}

func initCmd(args []string) {
	dirs := config.GetDirectories()

	// Detect if already initialized
	_, settingsExists := os.Stat(dirs.SettingsFile)
	_, sshKeyExists := os.Stat(filepath.Join(dirs.SSH, "id_ed25519"))
	alreadyInitialized := settingsExists == nil && sshKeyExists == nil

	fmt.Println()
	if alreadyInitialized {
		ui.Print(ui.Bold("Coop is already initialized"))
		fmt.Println()
		ui.Successf("Config directory: %s", ui.Path(dirs.Config))
		ui.Successf("Data directory:   %s", ui.Path(dirs.Data))
		ui.Successf("Cache directory:  %s", ui.Path(dirs.Cache))
		ui.Successf("Settings file:    %s", ui.Path(dirs.SettingsFile))
		ui.Successf("SSH keys:         %s", ui.Path(dirs.SSH))
		fmt.Println()
		ui.Muted("Run 'coop config' to see full configuration.")
		fmt.Println()
		return
	}

	ui.Print(ui.Bold("Initializing coop..."))

	if err := config.EnsureDirectories(); err != nil {
		ui.Errorf("Error creating directories: %v", err)
		os.Exit(1)
	}

	ui.Successf("Created config directory: %s", ui.Path(dirs.Config))
	ui.Successf("Created data directory:   %s", ui.Path(dirs.Data))
	ui.Successf("Created cache directory:  %s", ui.Path(dirs.Cache))
	ui.Successf("Settings file: %s", ui.Path(dirs.SettingsFile))

	// Generate SSH keys
	pubKey, err := sandbox.EnsureSSHKeys()
	if err != nil {
		ui.Warnf("Could not generate SSH keys: %v", err)
	} else {
		ui.Successf("SSH keys generated in: %s", ui.Path(dirs.SSH))
		_ = pubKey // Used but not printed
	}

	fmt.Println()
	ui.Success("Coop initialized successfully!")
	ui.Muted("Run 'coop config' to see all paths.")
	fmt.Println()
}

func createCmd(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	cpus := fs.Int("cpus", 2, "Number of CPUs")
	memory := fs.Int("memory", 4096, "Memory in MB")
	disk := fs.Int("disk", 20, "Disk size in GB")
	sshKey := fs.String("ssh-key", "", "SSH public key (default: auto-detect)")
	workDir := fs.String("workdir", "", "Host directory to mount as workspace")
	verbose := fs.Bool("verbose", false, "Stream cloud-init logs during setup")

	_ = fs.Parse(args) // ExitOnError mode handles errors

	// Auto-generate name if not provided
	var name string
	if fs.NArg() < 1 {
		name = sandbox.GenerateName()
		ui.Printf("Generated name: %s\n", ui.Name(name))
	} else {
		name = fs.Arg(0)
	}

	mgr := mustManager()

	cfg := sandbox.DefaultContainerConfig(name)
	cfg.CPUs = *cpus
	cfg.MemoryMB = *memory
	cfg.DiskGB = *disk
	cfg.WorkingDir = *workDir
	cfg.Verbose = *verbose

	// Get or generate SSH key from ~/.config/coop/ssh/
	if *sshKey != "" {
		cfg.SSHPubKey = *sshKey
	} else {
		pubKey, err := sandbox.EnsureSSHKeys()
		if err != nil {
			ui.Warnf("Could not setup SSH keys: %v", err)
		} else {
			cfg.SSHPubKey = pubKey
		}
	}

	if err := mgr.Create(cfg); err != nil {
		ui.Errorf("Error creating container: %v", err)
		os.Exit(1)
	}

	// Initialize state tracker for this container
	// Determines base image from config (same logic Manager uses)
	baseImage := appConfig.Settings.DefaultImage
	if baseImage == "" {
		baseImage = sandbox.DefaultImage
	}
	instanceDir := filepath.Join(appConfig.Dirs.Data, "instances")
	if _, err := state.NewTracker(instanceDir, name, baseImage); err != nil {
		ui.Warnf("Container created but state tracking failed: %v", err)
	}

	// Update SSH config for easy access
	if ip, _ := mgr.GetContainerIP(name); ip != "" {
		if err := sandbox.UpdateSSHConfig(name, ip); err != nil {
			ui.Warnf("Could not update SSH config: %v", err)
		}
	}
}

func startCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop start <name>")
		os.Exit(1)
	}

	name := args[0]

	mgr := mustManager()

	ui.Printf("Starting container %s...\n", ui.Name(name))
	if err := mgr.Start(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s started", ui.Name(name))

	// Update SSH config with new IP
	if ip, _ := mgr.GetContainerIP(name); ip != "" {
		if err := sandbox.UpdateSSHConfig(name, ip); err != nil {
			ui.Warnf("Could not update SSH config: %v", err)
		}
	}
}

func stopCmd(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	force := fs.Bool("force", false, "Force stop")
	_ = fs.Parse(args) // ExitOnError mode handles errors

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop stop <name> [--force]")
		os.Exit(1)
	}

	name := fs.Arg(0)

	mgr := mustManager()

	ui.Printf("Stopping container %s...\n", ui.Name(name))
	if err := mgr.Stop(name, *force); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s stopped", ui.Name(name))
}

func lockCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop lock <name>")
		os.Exit(1)
	}

	name := args[0]

	mgr := mustManager()

	ui.Printf("Locking container %s...\n", ui.Name(name))
	if err := mgr.Lock(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s locked (all processes frozen)", ui.Name(name))
}

func unlockCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop unlock <name>")
		os.Exit(1)
	}

	name := args[0]

	mgr := mustManager()

	ui.Printf("Unlocking container %s...\n", ui.Name(name))
	if err := mgr.Unlock(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s unlocked (processes resumed)", ui.Name(name))
}

func logsCmd(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "Follow log output")
	lines := fs.Int("n", 50, "Number of lines to show")
	_ = fs.Parse(args) // ExitOnError mode handles errors

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop logs <name> [-f] [-n lines]")
		os.Exit(1)
	}

	name := fs.Arg(0)

	mgr := mustManager()

	if err := mgr.Logs(name, *follow, *lines); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
}

func deleteCmd(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	force := fs.Bool("force", false, "Force stop running container")
	_ = fs.Parse(args) // ExitOnError mode handles errors

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop delete <name> [--force]")
		os.Exit(1)
	}

	name := fs.Arg(0)

	mgr := mustManager()

	if err := mgr.Delete(name, *force); err != nil {
		ui.Errorf("Error deleting container: %v", err)
		os.Exit(1)
	}
}

func listCmd(args []string) {
	mgr := mustManager()

	containers, err := mgr.List()
	if err != nil {
		ui.Errorf("Error listing containers: %v", err)
		os.Exit(1)
	}

	if len(containers) == 0 {
		ui.Muted("No agent containers found")
		return
	}

	// Use Table for proper alignment with ANSI codes
	table := ui.NewTable(20, 10, 15, 5, 8, 6, 16)
	table.SetHeaders("NAME", "STATUS", "IP", "CPUS", "MEMORY", "DISK", "CREATED")

	for _, c := range containers {
		isRunning := c.Status == "Running"

		ip := c.IP
		if ip == "" {
			ip = "-"
		} else if isRunning {
			ip = ui.IP(ip)
		}
		cpus := c.CPUs
		if cpus == "" {
			cpus = "-"
		}
		mem := c.Memory
		if mem == "" {
			mem = "-"
		}
		disk := c.Disk
		if disk == "" {
			disk = "-"
		}
		created := c.CreatedAt.Format("2006-01-02 15:04")

		if isRunning {
			table.AddRow(
				ui.Name(c.Name),
				ui.Status(c.Status),
				ip,
				cpus,
				mem,
				disk,
				created,
			)
		} else {
			// Dim the entire row for stopped containers
			table.AddRow(
				ui.MutedText(c.Name),
				ui.Status(c.Status),
				ui.MutedText(ip),
				ui.MutedText(cpus),
				ui.MutedText(mem),
				ui.MutedText(disk),
				ui.MutedText(created),
			)
		}
	}

	fmt.Print(table.Render())
}

func statusCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop status <name>")
		os.Exit(1)
	}

	name := args[0]

	mgr := mustManager()

	status, err := mgr.Status(name)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("%s  %s\n", ui.Bold("Name:"), ui.Name(status.Name))
	fmt.Printf("%s  %s\n", ui.Bold("Status:"), ui.Status(status.Status))
	if status.IP != "" {
		fmt.Printf("%s  %s\n", ui.Bold("IP:"), ui.IP(status.IP))
	}
	fmt.Printf("%s  %s\n", ui.Bold("Created:"), status.CreatedAt.Format("2006-01-02 15:04:05"))

	// Print relevant config
	fmt.Println()
	ui.Print(ui.Header("Configuration:"))
	for k, v := range status.Config {
		if strings.HasPrefix(k, "limits.") || k == "security.nesting" {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
}

func sshCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop ssh <name>")
		fmt.Println()
		ui.Muted("Tip: Use 'coop shell <name>' to connect directly")
		os.Exit(1)
	}

	name := args[0]

	mgr := mustManager()

	cmd, err := mgr.SSH(name)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	fmt.Println(cmd)
}

func shellCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop shell <name> [command...]")
		os.Exit(1)
	}

	name := args[0]
	remoteCmd := args[1:] // Extra args become remote command

	mgr := mustManager()

	sshArgs, err := mgr.SSHArgs(name)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	// Append remote command if provided
	if len(remoteCmd) > 0 {
		sshArgs = append(sshArgs, "--")
		sshArgs = append(sshArgs, remoteCmd...)
	}

	// Find ssh binary
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		ui.Error("ssh not found in PATH")
		os.Exit(1)
	}

	// Replace current process with ssh (like colima does)
	// This gives the user full control of the terminal
	if err := syscall.Exec(sshPath, append([]string{"ssh"}, sshArgs...), os.Environ()); err != nil {
		ui.Errorf("failed to exec ssh: %v", err)
		os.Exit(1)
	}
}

func execCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and command required")
		ui.Muted("Usage: coop exec <name> <command> [args...]")
		os.Exit(1)
	}

	name := args[0]
	command := args[1:]

	mgr := mustManager()

	exitCode, err := mgr.Exec(name, command)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func configCmd(args []string) {
	dirs := config.GetDirectories()

	fmt.Println(ui.Bold("Coop Configuration"))
	fmt.Println(strings.Repeat("=", 40))

	fmt.Println("\n" + ui.Header("Directories:"))
	fmt.Printf("  Config:    %s\n", ui.Path(dirs.Config))
	fmt.Printf("  Data:      %s\n", ui.Path(dirs.Data))
	fmt.Printf("  Cache:     %s\n", ui.Path(dirs.Cache))
	fmt.Printf("  SSH:       %s\n", ui.Path(dirs.SSH))
	fmt.Printf("  Settings:  %s\n", ui.Path(dirs.SettingsFile))
	fmt.Printf("  Images:    %s\n", ui.Path(dirs.Images))
	fmt.Printf("  Disks:     %s\n", ui.Path(dirs.Disks))
	fmt.Printf("  Profiles:  %s\n", ui.Path(dirs.Profiles))
	fmt.Printf("  Logs:      %s\n", ui.Path(dirs.Logs))

	fmt.Println("\n" + ui.Header("Defaults:"))
	fmt.Printf("  CPUs:      %d\n", appConfig.Settings.DefaultCPUs)
	fmt.Printf("  Memory:    %d MB\n", appConfig.Settings.DefaultMemoryMB)
	fmt.Printf("  Disk:      %d GB\n", appConfig.Settings.DefaultDiskGB)
	fmt.Printf("  Image:     %s\n", ui.Name(appConfig.Settings.DefaultImage))

	fmt.Println("\n" + ui.Header("Environment Overrides:"))
	printEnvVar("COOP_CONFIG_DIR", dirs.Config)
	printEnvVar("COOP_DATA_DIR", dirs.Data)
	printEnvVar("COOP_CACHE_DIR", dirs.Cache)
	printEnvVar("COOP_DEFAULT_IMAGE", appConfig.Settings.DefaultImage)

	// Check if SSH keys exist
	fmt.Println("\n" + ui.Header("SSH Keys:"))
	if _, err := os.Stat(dirs.SSH + "/id_ed25519"); err == nil {
		fmt.Printf("  Private key: %s %s\n", ui.Path(dirs.SSH+"/id_ed25519"), ui.SuccessText("✓"))
		fmt.Printf("  Public key:  %s %s\n", ui.Path(dirs.SSH+"/id_ed25519.pub"), ui.SuccessText("✓"))
	} else {
		ui.Muted("  Not yet generated (created on first 'create')")
	}

	// Check settings.json
	fmt.Println("\n" + ui.Header("Settings File:"))
	if _, err := os.Stat(dirs.SettingsFile); err == nil {
		fmt.Printf("  %s %s\n", ui.Path(dirs.SettingsFile), ui.SuccessText("✓"))
	} else {
		ui.Muted("  Not yet created (run 'coop init')")
	}

	// Logging info
	fmt.Println("\n" + ui.Header("Logging:"))
	fmt.Printf("  Log file:    %s\n", ui.Path(dirs.Logs+"/coop.log"))
	fmt.Printf("  Max size:    %d MB\n", appConfig.Settings.Log.MaxSizeMB)
	fmt.Printf("  Max backups: %d\n", appConfig.Settings.Log.MaxBackups)
	fmt.Printf("  Max age:     %d days\n", appConfig.Settings.Log.MaxAgeDays)
	fmt.Printf("  Compress:    %t\n", appConfig.Settings.Log.Compress)
	fmt.Printf("  Debug:       %t\n", appConfig.Settings.Log.Debug)

	// VM settings (macOS and non-native Linux)
	if runtime.GOOS == "darwin" {
		fmt.Println("\n" + ui.Header("VM Backend:"))
		fmt.Printf("  Priority:   %v\n", appConfig.Settings.VM.BackendPriority)
		fmt.Printf("  Instance:   %s\n", ui.Name(appConfig.Settings.VM.Instance))
		fmt.Printf("  CPUs:       %d\n", appConfig.Settings.VM.CPUs)
		fmt.Printf("  Memory:     %d GB\n", appConfig.Settings.VM.MemoryGB)
		fmt.Printf("  Disk:       %d GB\n", appConfig.Settings.VM.DiskGB)
		fmt.Printf("  Auto-start: %t\n", appConfig.Settings.VM.AutoStart)
	}
}

func themeCmd(args []string) {
	if len(args) == 0 {
		// Show current theme and available themes
		current := ui.GetTheme()
		fmt.Println(ui.Bold("Coop Themes"))
		fmt.Println(strings.Repeat("=", 40))
		fmt.Println()

		fmt.Printf("%s %s\n\n", ui.Header("Current theme:"), ui.Name(current.Name))

		fmt.Println(ui.Header("Available themes:"))
		for _, themeName := range ui.ListThemes() {
			marker := "  "
			if themeName == current.Name {
				marker = ui.SuccessText("▸ ")
			}
			fmt.Printf("%s%s\n", marker, themeName)
		}
		fmt.Println()
		ui.Muted("Interactive picker: coop theme preview")
		ui.Muted("Preview specific theme: coop theme preview <name>")
		ui.Mutedf("Set in config: %s", ui.Code(`{"ui": {"theme": "dracula"}}`))
		ui.Mutedf("Or set via env: %s", ui.Code("COOP_THEME=dracula coop list"))
		fmt.Println()
		return
	}

	subcommand := args[0]
	switch subcommand {
	case "preview":
		if len(args) < 2 {
			// Interactive mode - no theme specified
			previewTheme("")
		} else {
			// Static preview of specific theme
			previewTheme(args[1])
		}
	case "list":
		for _, name := range ui.ListThemes() {
			fmt.Println(name)
		}
	default:
		ui.Errorf("Unknown theme subcommand: %s", subcommand)
		ui.Muted("Usage: coop theme [preview <name>|list]")
		os.Exit(1)
	}
}

func previewTheme(themeName string) {
	// If specific theme requested, show static preview
	if themeName != "" {
		showStaticThemePreview(themeName)
		return
	}

	// Interactive theme picker
	originalTheme := ui.GetTheme()
	var selectedTheme string
	var confirmed bool

	// Build theme options with color swatches
	themes := []struct {
		name  string
		theme ui.Theme
	}{
		{"default", ui.ThemeDefault},
		{"solarized", ui.ThemeSolarized},
		{"dracula", ui.ThemeDracula},
		{"gruvbox", ui.ThemeGruvbox},
		{"nord", ui.ThemeNord},
	}

	options := make([]huh.Option[string], len(themes))
	for i, t := range themes {
		// Create color swatch display with theme name
		swatch := buildColorSwatch(t.theme)
		label := fmt.Sprintf("%-12s %s", t.name, swatch)
		options[i] = huh.NewOption(label, t.name)
	}

	// Theme selection
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, ui.Bold("Interactive Theme Picker"))
	ui.Muted("Use ↑↓ arrows to navigate, Enter to preview")
	fmt.Fprintln(os.Stderr)

	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a theme to preview").
				Options(options...).
				Value(&selectedTheme),
		),
	).WithShowHelp(false)

	err := selectForm.Run()
	if err != nil {
		// User cancelled - restore original
		ui.SetTheme(originalTheme)
		fmt.Fprintln(os.Stderr)
		ui.Muted("Theme selection cancelled")
		fmt.Fprintln(os.Stderr)
		return
	}

	// Apply selected theme for preview
	selectedThemeObj := ui.ThemeByName(selectedTheme)
	ui.SetTheme(selectedThemeObj)

	// Show live preview with the selected theme
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, strings.Repeat("─", 50))
	showThemePreview(selectedThemeObj)
	fmt.Fprintln(os.Stderr, strings.Repeat("─", 50))
	fmt.Fprintln(os.Stderr)

	// Confirmation dialog
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Apply the '%s' theme?", selectedTheme)).
				Description("This will update your settings.json").
				Affirmative("Apply").
				Negative("Cancel").
				Value(&confirmed),
		),
	)

	err = confirmForm.Run()
	if err != nil || !confirmed {
		// Restore original theme
		ui.SetTheme(originalTheme)
		fmt.Fprintln(os.Stderr)
		ui.Muted("Theme not applied - keeping current theme")
		fmt.Fprintln(os.Stderr)
		return
	}

	// Apply theme permanently
	appConfig.Settings.UI.Theme = selectedTheme
	if err := appConfig.Save(); err != nil {
		ui.Errorf("Failed to save theme: %v", err)
		ui.SetTheme(originalTheme)
		return
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("Theme %s applied!", ui.Name(selectedTheme))
	ui.Mutedf("Saved to %s", ui.Path(appConfig.Dirs.SettingsFile))
	fmt.Fprintln(os.Stderr)
}

func buildColorSwatch(theme ui.Theme) string {
	// Create a compact color swatch showing the theme's palette
	// Each color gets a small "chip" with background for contrast
	swatch := make([]string, 0)

	// Use double-width block for better visibility
	block := "██"

	// Dark background for all swatches to ensure visibility
	bg := lipgloss.Color("235") // dark gray

	// Create styled chips for each color in the theme with backgrounds
	successChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Success)).
		Background(bg).
		Render(block)

	warningChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Warning)).
		Background(bg).
		Render(block)

	errorChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Error)).
		Background(bg).
		Render(block)

	boldChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Bold)).
		Background(bg).
		Render(block)

	pathChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Path)).
		Background(bg).
		Render(block)

	headerChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Header)).
		Background(bg).
		Render(block)

	swatch = append(swatch, successChip, warningChip, errorChip, boldChip, pathChip, headerChip)

	return strings.Join(swatch, " ")
}

func showThemePreview(theme ui.Theme) {
	fmt.Fprintln(os.Stderr, ui.Header("Preview:"))
	fmt.Fprintln(os.Stderr)

	// Show color samples
	fmt.Fprintln(os.Stderr, "  ", ui.SuccessText("✓ Success message"))
	fmt.Fprintln(os.Stderr, "  ", ui.WarningText("⚠ Warning message"))
	fmt.Fprintln(os.Stderr, "  ", ui.ErrorText("✗ Error message"))
	fmt.Fprintln(os.Stderr, "  ", ui.Name("container-name"), ui.MutedText("- subtle info"))
	fmt.Fprintln(os.Stderr, "  ", ui.Path("/path/to/file"), ui.IP("192.168.1.1"))
	fmt.Fprintln(os.Stderr, "  ", "Run command:", ui.Code("coop vm start"))
}

func showStaticThemePreview(themeName string) {
	theme := ui.ThemeByName(themeName)
	ui.SetTheme(theme)

	fmt.Println()
	fmt.Printf("%s %s\n", ui.Header("Theme:"), ui.Name(theme.Name))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	// Show color samples
	fmt.Println(ui.Header("Colors:"))
	fmt.Printf("  %s  %s\n", ui.Bold("Bold/Names:"), ui.Name("example-container"))
	fmt.Printf("  %s  %s\n", ui.Bold("Success:"), ui.SuccessText("✓ Operation successful"))
	fmt.Printf("  %s  %s\n", ui.Bold("Warning:"), ui.WarningText("⚠ Warning message"))
	fmt.Printf("  %s  %s\n", ui.Bold("Error:"), ui.ErrorText("✗ Error message"))
	fmt.Printf("  %s  %s\n", ui.Bold("Muted:"), ui.MutedText("subtle information"))
	fmt.Printf("  %s  %s\n", ui.Bold("Path:"), ui.Path("/path/to/file"))
	fmt.Printf("  %s  %s\n", ui.Bold("IP:"), ui.IP("192.168.1.100"))
	fmt.Printf("  %s  %s\n", ui.Bold("Code:"), ui.Code("coop vm start"))
	fmt.Println()

	// Show a sample table
	fmt.Println(ui.Header("Sample Table:"))
	table := ui.NewTable(20, 10, 15)
	table.SetHeaders("NAME", "STATUS", "IP")
	table.AddRow(ui.Name("agent-01"), ui.Status("Running"), ui.IP("10.0.0.5"))
	table.AddRow(ui.MutedText("agent-02"), ui.Status("Stopped"), ui.MutedText("-"))
	fmt.Print(table.Render())
	fmt.Println()

	ui.Muted("To use this theme permanently:")
	fmt.Printf("  • Edit %s\n", ui.Path(appConfig.Dirs.SettingsFile))
	fmt.Printf("  • Set: %s\n", ui.Code(fmt.Sprintf(`{"ui": {"theme": "%s"}}`, themeName)))
	fmt.Println()
}

func envCmd(args []string) {
	fmt.Println(ui.Bold("Coop Environment Variables"))
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println()

	dirs := config.GetDirectories()

	// Define all COOP environment variables
	envVars := []struct {
		name    string
		desc    string
		current string
	}{
		{"COOP_CONFIG_DIR", "Configuration directory", dirs.Config},
		{"COOP_DATA_DIR", "Data directory (instances, images)", dirs.Data},
		{"COOP_CACHE_DIR", "Cache directory", dirs.Cache},
		{"COOP_DEFAULT_IMAGE", "Default container image", appConfig.Settings.DefaultImage},
		{"COOP_VM_INSTANCE", "VM instance name", appConfig.Settings.VM.Instance},
		{"COOP_VM_BACKEND", "Force VM backend (colima, lima)", ""},
	}

	for _, ev := range envVars {
		val := os.Getenv(ev.name)
		if val != "" {
			// Environment variable is set
			fmt.Printf("  %s\n", ui.Name(ev.name))
			fmt.Printf("    %s\n", ui.MutedText(ev.desc))
			fmt.Printf("    Value: %s %s\n", ui.SuccessText(val), ui.MutedText("(from environment)"))
		} else {
			// Using default/config value
			fmt.Printf("  %s\n", ui.MutedText(ev.name))
			fmt.Printf("    %s\n", ui.MutedText(ev.desc))
			if ev.current != "" {
				fmt.Printf("    Value: %s %s\n", ev.current, ui.MutedText("(default)"))
			} else {
				fmt.Printf("    Value: %s\n", ui.MutedText("(not set)"))
			}
		}
		fmt.Println()
	}

	ui.Muted("Set environment variables to override defaults.")
	ui.Muted("Example: export COOP_DEFAULT_IMAGE=my-custom-image")
}

func snapshotCmd(args []string) {
	if len(args) == 0 {
		printSnapshotUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "create":
		snapshotCreateCmd(args[1:])
	case "restore":
		snapshotRestoreCmd(args[1:])
	case "list", "ls":
		snapshotListCmd(args[1:])
	case "delete", "rm":
		snapshotDeleteCmd(args[1:])
	default:
		ui.Errorf("Unknown snapshot subcommand: %s", args[0])
		printSnapshotUsage()
		os.Exit(1)
	}
}

func snapshotCreateCmd(args []string) {
	fs := flag.NewFlagSet("snapshot create", flag.ExitOnError)
	note := fs.String("note", "", "Optional note about this snapshot")
	_ = fs.Parse(args)

	if fs.NArg() < 2 {
		ui.Error("container name and snapshot name required")
		ui.Muted("Usage: coop snapshot create <container> <snapshot-name> [--note 'reason']")
		os.Exit(1)
	}

	container := fs.Arg(0)
	snapshotName := fs.Arg(1)

	mgr := mustManager()

	ui.Printf("Creating snapshot %s of %s...\n", ui.Name(snapshotName), ui.Name(container))
	if err := mgr.CreateSnapshot(container, snapshotName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	// Record in state tracker (best effort - don't fail if tracker setup fails)
	instanceDir := filepath.Join(appConfig.Dirs.Data, "instances")
	tracker, err := state.NewTracker(instanceDir, container, "")
	if err == nil {
		if _, err := tracker.RecordSnapshot(snapshotName, *note); err != nil {
			ui.Warnf("Snapshot created but state tracking failed: %v", err)
		}
	}

	ui.Successf("Snapshot %s created", ui.Name(snapshotName))
}

func snapshotRestoreCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and snapshot name required")
		ui.Muted("Usage: coop snapshot restore <container> <snapshot-name>")
		os.Exit(1)
	}

	container := args[0]
	snapshotName := args[1]

	mgr := mustManager()

	ui.Printf("Restoring %s to snapshot %s...\n", ui.Name(container), ui.Name(snapshotName))
	if err := mgr.RestoreSnapshot(container, snapshotName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s restored to %s", ui.Name(container), ui.Name(snapshotName))
}

func snapshotListCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop snapshot list <container>")
		os.Exit(1)
	}

	container := args[0]

	mgr := mustManager()

	snapshots, err := mgr.ListSnapshots(container)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	if len(snapshots) == 0 {
		ui.Mutedf("No snapshots found for %s", container)
		return
	}

	table := ui.NewTable(25, 20)
	table.SetHeaders("NAME", "CREATED")

	for _, s := range snapshots {
		table.AddRow(
			ui.Name(s.Name),
			s.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}

	fmt.Print(table.Render())
}

func snapshotDeleteCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and snapshot name required")
		ui.Muted("Usage: coop snapshot delete <container> <snapshot-name>")
		os.Exit(1)
	}

	container := args[0]
	snapshotName := args[1]

	mgr := mustManager()

	ui.Printf("Deleting snapshot %s from %s...\n", ui.Name(snapshotName), ui.Name(container))
	if err := mgr.DeleteSnapshot(container, snapshotName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Snapshot %s deleted", ui.Name(snapshotName))
}

func printSnapshotUsage() {
	fmt.Println("Usage: coop snapshot <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  create <container> <name>   Create a snapshot")
	fmt.Println("  restore <container> <name>  Restore to a snapshot")
	fmt.Println("  list <container>            List snapshots")
	fmt.Println("  delete <container> <name>   Delete a snapshot")
}

func mountCmd(args []string) {
	if len(args) == 0 {
		printMountUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "add":
		mountAddCmd(args[1:])
	case "remove", "rm":
		mountRemoveCmd(args[1:])
	case "list", "ls":
		mountListCmd(args[1:])
	default:
		ui.Errorf("Unknown mount subcommand: %s", args[0])
		printMountUsage()
		os.Exit(1)
	}
}

func mountAddCmd(args []string) {
	fs := flag.NewFlagSet("mount add", flag.ExitOnError)
	name := fs.String("name", "", "Mount name (default: derived from path)")
	path := fs.String("path", "", "Mount path inside container (default: /home/agent/<basename>)")
	readonly := fs.Bool("readonly", false, "Mount read-only (container cannot write)")
	force := fs.Bool("force", false, "Authorize mounting protected directories")
	_ = fs.Parse(args) // ExitOnError mode handles errors

	if fs.NArg() < 2 {
		ui.Error("container name and source path required")
		ui.Muted("Usage: coop mount add [options] <container> <source>")
		os.Exit(1)
	}

	container := fs.Arg(0)
	source := fs.Arg(1)

	// Expand ~ to home directory
	if source == "~" {
		source = os.Getenv("HOME")
	} else if strings.HasPrefix(source, "~/") {
		source = os.Getenv("HOME") + source[1:]
	}

	// Resolve to absolute path
	if !strings.HasPrefix(source, "/") {
		cwd, _ := os.Getwd()
		source = cwd + "/" + source
	}

	// Default mount name from basename
	mountName := *name
	if mountName == "" {
		parts := strings.Split(strings.TrimSuffix(source, "/"), "/")
		mountName = parts[len(parts)-1]
	}

	// Default path inside container
	mountPath := *path
	if mountPath == "" {
		mountPath = "/home/agent/" + mountName
	}

	// Check if path is seatbelted and handle authorization
	forceAuthorized := false
	if seatbelted, reason := sandbox.IsSeatbelted(source); seatbelted {
		if !*force {
			ui.Errorf("Refusing to mount protected path: %s", reason)
			ui.Muted("Use --force to authorize with a one-time code")
			os.Exit(1)
		}

		// Generate code (used for validation, not displayed in notification)
		_, err := sandbox.CurrentAuthCode()
		if err != nil {
			ui.Errorf("Failed to generate authorization code: %v", err)
			os.Exit(1)
		}
		ui.NotifyWithSound("coop", "Protected Path", "Enter the auth code in your terminal", "Purr")

		// Use countdown prompt for auth code
		result := ui.PromptAuthCode(ui.AuthCodePromptConfig{
			Reason:   reason,
			Timeout:  15 * time.Second,
			Attempts: 3,
			Validator: func(code string) (bool, error) {
				return sandbox.ValidateAuthCode(code)
			},
		})

		switch result {
		case ui.AuthCodeSuccess:
			forceAuthorized = true
		case ui.AuthCodeExpired:
			ui.Errorf("Authorization code expired")
			os.Exit(1)
		case ui.AuthCodeFailed:
			ui.Errorf("Authorization failed after 3 attempts")
			os.Exit(1)
		case ui.AuthCodeError:
			ui.Errorf("Cannot read from terminal")
			os.Exit(1)
		}

		ui.Warnf("Mounting protected path: %s", source)
	}

	mgr := mustManager()

	ui.Printf("Mounting %s to %s as %s...\n", ui.Path(source), ui.Path(mountPath), ui.Name(mountName))
	if err := mgr.Mount(container, mountName, source, mountPath, *readonly, forceAuthorized); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	if *readonly {
		ui.Successf("Mount %s added (read-only)", ui.Name(mountName))
	} else {
		ui.Successf("Mount %s added", ui.Name(mountName))
	}
}

func mountRemoveCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and mount name required")
		ui.Muted("Usage: coop mount remove <container> <mount-name>")
		os.Exit(1)
	}

	container := args[0]
	mountName := args[1]

	mgr := mustManager()

	ui.Printf("Removing mount %s from %s...\n", ui.Name(mountName), ui.Name(container))
	if err := mgr.Unmount(container, mountName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Mount %s removed", ui.Name(mountName))
}

func mountListCmd(args []string) {
	mgr := mustManager()

	// If container specified, show just that one
	if len(args) >= 1 {
		container := args[0]
		mounts, err := mgr.ListMounts(container)
		if err != nil {
			ui.Errorf("Error: %v", err)
			os.Exit(1)
		}

		status, _ := mgr.Status(container)
		isRunning := status != nil && status.Status == "Running"

		if len(mounts) == 0 {
			ui.Mutedf("No mounts found for %s", container)
			return
		}

		printContainerMounts(container, isRunning, mounts)
		return
	}

	// List all containers with their mounts
	allMounts, err := mgr.ListAllMounts()
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	if len(allMounts) == 0 {
		ui.Muted("No containers found")
		return
	}

	hasAnyMounts := false
	for _, cm := range allMounts {
		if len(cm.Mounts) > 0 {
			hasAnyMounts = true
			break
		}
	}

	if !hasAnyMounts {
		ui.Muted("No mounts found on any container")
		return
	}

	for i, cm := range allMounts {
		if len(cm.Mounts) == 0 {
			continue
		}
		if i > 0 {
			fmt.Println()
		}
		isRunning := cm.Status == "Running"
		printContainerMounts(cm.Name, isRunning, cm.Mounts)
	}
}

func printContainerMounts(container string, isRunning bool, mounts []sandbox.MountInfo) {
	statusIndicator := ui.Status("Running")
	if !isRunning {
		statusIndicator = ui.Status("Stopped")
	}
	fmt.Printf("%s %s\n\n", ui.Name(container), statusIndicator)

	table := ui.NewTable(15, 40, 7, 30)
	table.SetHeaders("NAME", "SOURCE", "MODE", "VIRTUAL")

	for _, m := range mounts {
		var name, source, arrow, virtual string
		if m.Readonly {
			// ro: source yellow ---> virtual green (read-only)
			arrow = "--->"
			if isRunning {
				name = ui.Name(m.Name)
				source = ui.WarningText(m.Source)
				virtual = ui.SuccessText(m.Path)
			} else {
				name = ui.MutedText(m.Name)
				source = ui.MutedText(m.Source)
				arrow = ui.MutedText(arrow)
				virtual = ui.MutedText(m.Path)
			}
		} else {
			// rw: source red <---> virtual yellow (read-write)
			arrow = "<--->"
			if isRunning {
				name = ui.Name(m.Name)
				source = ui.ErrorText(m.Source)
				virtual = ui.WarningText(m.Path)
			} else {
				name = ui.MutedText(m.Name)
				source = ui.MutedText(m.Source)
				arrow = ui.MutedText(arrow)
				virtual = ui.MutedText(m.Path)
			}
		}
		table.AddRow(name, source, arrow, virtual)
	}

	fmt.Print(table.Render())
}

func printMountUsage() {
	fmt.Println("Usage: coop mount <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  add [options] <container> <source>   Add a host directory mount")
	fmt.Println("  remove <container> <name>            Remove a mount")
	fmt.Println("  list [container]                     List mounts (all containers if omitted)")
	fmt.Println("\nOptions for 'add':")
	fmt.Println("  --name NAME      Mount name (default: directory basename)")
	fmt.Println("  --path PATH      Path inside container (default: /home/agent/<name>)")
	fmt.Println("  --readonly       Mount read-only (container cannot write to host)")
	fmt.Println("  --force          Authorize mounting protected directories")
	fmt.Println("\nProtected directories (~/.ssh, ~/Library, SIP paths) require --force.")
	fmt.Println("A 6-digit code will be sent via macOS notification and you'll be prompted")
	fmt.Println("to enter it interactively (3 attempts, 15 second expiry).")
	fmt.Println("\nOn macOS, source paths must be under /Users (shared with Colima VM).")
}

func printEnvVar(name, currentValue string) {
	if val := os.Getenv(name); val != "" {
		fmt.Printf("  %s=%s %s\n", name, val, ui.SuccessText("(active)"))
	} else {
		fmt.Printf("  %s %s\n", name, ui.MutedText("(not set)"))
	}
}

func vmCmd(args []string) {
	if runtime.GOOS != "darwin" {
		ui.Error("vm command is only needed on macOS (Linux runs Incus natively)")
		os.Exit(1)
	}

	mgr, err := backend.NewManager(appConfig)
	if err != nil {
		ui.Errorf("Error creating backend manager: %v", err)
		os.Exit(1)
	}

	if len(args) == 0 {
		printVMUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "status":
		status, err := mgr.Status()
		if err != nil {
			ui.Errorf("Error getting VM status: %v", err)
			os.Exit(1)
		}
		fmt.Printf("%s  %s\n", ui.Bold("Backend:"), mgr.Backend().Name())
		fmt.Printf("%s  %s\n", ui.Bold("Instance:"), ui.Name(status.Name))
		fmt.Printf("%s  %s\n", ui.Bold("Status:"), ui.Status(string(status.State)))
		if status.CPUs > 0 {
			fmt.Printf("%s  %d\n", ui.Bold("CPUs:"), status.CPUs)
		}
		if status.MemoryGB > 0 {
			fmt.Printf("%s  %d GB\n", ui.Bold("Memory:"), status.MemoryGB)
		}
		if status.DiskGB > 0 {
			fmt.Printf("%s  %d GB\n", ui.Bold("Disk:"), status.DiskGB)
		}
		if status.Arch != "" {
			fmt.Printf("%s  %s\n", ui.Bold("Arch:"), status.Arch)
		}
		if status.Runtime != "" {
			fmt.Printf("%s  %s\n", ui.Bold("Runtime:"), status.Runtime)
		}

	case "start":
		ui.Printf("Starting VM (backend: %s, instance: %s)...\n", mgr.Backend().Name(), ui.Name(appConfig.Settings.VM.Instance))
		if err := mgr.Start(); err != nil {
			ui.Errorf("Error starting VM: %v", err)
			os.Exit(1)
		}
		ui.Success("VM is running")

	case "stop":
		ui.Printf("Stopping VM (backend: %s, instance: %s)...\n", mgr.Backend().Name(), ui.Name(appConfig.Settings.VM.Instance))
		if err := mgr.Stop(); err != nil {
			ui.Errorf("Error stopping VM: %v", err)
			os.Exit(1)
		}
		ui.Success("VM stopped")

	case "shell":
		ui.Printf("Opening shell in VM (backend: %s)...\n", mgr.Backend().Name())
		if err := mgr.Shell(); err != nil {
			ui.Errorf("Error opening shell: %v", err)
			os.Exit(1)
		}

	case "delete":
		ui.Printf("Deleting VM (backend: %s, instance: %s)...\n", mgr.Backend().Name(), ui.Name(appConfig.Settings.VM.Instance))
		if err := mgr.Delete(); err != nil {
			ui.Errorf("Error deleting VM: %v", err)
			os.Exit(1)
		}
		ui.Success("VM deleted")

	case "socket":
		socket, err := mgr.GetIncusSocket()
		if err != nil {
			ui.Errorf("Error getting Incus socket: %v", err)
			os.Exit(1)
		}
		fmt.Println(ui.Path(socket))

	case "backends":
		backends := mgr.ListAvailableBackends()
		fmt.Printf("%s %v\n", ui.Bold("Available backends:"), backends)
		fmt.Printf("%s %s\n", ui.Bold("Selected backend:"), ui.Name(mgr.Backend().Name()))
		fmt.Printf("%s %v\n", ui.Bold("Priority order:"), appConfig.Settings.VM.BackendPriority)

	default:
		ui.Errorf("Unknown vm subcommand: %s", args[0])
		printVMUsage()
		os.Exit(1)
	}
}

func printVMUsage() {
	fmt.Println("Usage: coop vm <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  status     Show VM status")
	fmt.Println("  start      Start VM (creates if needed)")
	fmt.Println("  stop       Stop VM")
	fmt.Println("  shell      Open shell in VM")
	fmt.Println("  delete     Delete VM")
	fmt.Println("  socket     Print Incus socket path")
	fmt.Println("  backends   List available VM backends")
}

func doctorCmd(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fix := fs.Bool("fix", false, "Attempt to fix issues automatically")
	_ = fs.Parse(args)

	fmt.Println()
	ui.Print(ui.Bold("Coop Doctor"))
	fmt.Println()

	report := doctor.Run(appConfig)

	// Print results
	maxNameLen := 0
	for _, r := range report.Results {
		if len(r.Name) > maxNameLen {
			maxNameLen = len(r.Name)
		}
	}

	for _, r := range report.Results {
		var icon, color string
		switch r.Status {
		case doctor.StatusPass:
			icon = "✓"
			color = "green"
		case doctor.StatusWarn:
			icon = "!"
			color = "yellow"
		case doctor.StatusFail:
			icon = "✗"
			color = "red"
		case doctor.StatusSkip:
			icon = "-"
			color = "gray"
		}

		// Format the line
		name := r.Name
		for len(name) < maxNameLen {
			name += " "
		}

		switch color {
		case "green":
			fmt.Printf("  %s  %s  %s\n", ui.SuccessText(icon), name, ui.MutedText(r.Message))
		case "yellow":
			fmt.Printf("  %s  %s  %s\n", ui.WarningText(icon), name, r.Message)
		case "red":
			fmt.Printf("  %s  %s  %s\n", ui.ErrorText(icon), name, ui.ErrorText(r.Message))
		default:
			fmt.Printf("  %s  %s  %s\n", ui.MutedText(icon), ui.MutedText(name), ui.MutedText(r.Message))
		}

		// Show fix suggestion for failures
		if r.Status == doctor.StatusFail && r.Fix != "" {
			if *fix {
				// Attempt auto-fix
				fmt.Printf("      %s %s\n", ui.MutedText("fixing:"), r.Fix)
				// For now, just show the command - actual auto-fix would execute it
			} else {
				fmt.Printf("      %s %s\n", ui.MutedText("fix:"), ui.WarningText(r.Fix))
			}
		}
	}

	// Summary
	pass, warn, fail := report.Summary()
	fmt.Println()
	if fail == 0 && warn == 0 {
		ui.Success("All checks passed!")
	} else if fail == 0 {
		ui.Printf("%d passed, %d warnings\n", pass, warn)
	} else {
		ui.Printf("%d passed, %d warnings, %s\n", pass, warn, ui.ErrorText(fmt.Sprintf("%d failed", fail)))
		fmt.Println()
		ui.Muted("Run suggested fix commands to resolve issues.")
		ui.Muted("For macOS dependencies: brew bundle")
	}
	fmt.Println()

	if report.HasFailures() {
		os.Exit(1)
	}
}

func imageCmd(args []string) {
	if len(args) == 0 {
		printImageUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "build":
		imageBuildCmd(args[1:])
	case "list", "ls":
		imageListCmd(args[1:])
	case "exists":
		imageExistsCmd(args[1:])
	case "publish":
		imagePublishCmd(args[1:])
	case "lineage":
		imageLineageCmd(args[1:])
	default:
		ui.Errorf("Unknown image subcommand: %s", args[0])
		printImageUsage()
		os.Exit(1)
	}
}

func imageBuildCmd(args []string) {
	if err := sandbox.BuildBaseImage(); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
}

func imageListCmd(args []string) {
	mgr := mustManager()

	// Get socket path from backend to pass to incus CLI
	vmMgr, err := backend.NewManager(appConfig)
	if err != nil {
		ui.Errorf("Error initializing backend: %v", err)
		os.Exit(1)
	}

	socketPath, err := vmMgr.GetIncusSocket()
	if err != nil {
		ui.Errorf("Error getting Incus socket: %v", err)
		os.Exit(1)
	}

	// Use incus CLI with JSON output for parsing
	_ = mgr // manager connected, so incus is available

	cmd := exec.Command("incus", "image", "list", "--format", "json")
	cmd.Env = append(os.Environ(), "INCUS_SOCKET="+socketPath)

	output, err := cmd.Output()
	if err != nil {
		ui.Errorf("Failed to list images: %v", err)
		os.Exit(1)
	}

	// Parse JSON output
	var images []struct {
		Aliases []struct {
			Name string `json:"name"`
		} `json:"aliases"`
		Fingerprint  string            `json:"fingerprint"`
		Size         int64             `json:"size"`
		Architecture string            `json:"architecture"`
		UploadedAt   string            `json:"uploaded_at"`
		Properties   map[string]string `json:"properties"`
	}

	if err := json.Unmarshal(output, &images); err != nil {
		ui.Errorf("Failed to parse image list: %v", err)
		os.Exit(1)
	}

	if len(images) == 0 {
		ui.Muted("No images found")
		return
	}

	// Format with table
	table := ui.NewTable(24, 14, 12, 10, 20)
	table.SetHeaders("ALIAS", "FINGERPRINT", "ARCH", "SIZE", "UPLOADED")

	for _, img := range images {
		alias := "-"
		if len(img.Aliases) > 0 {
			alias = img.Aliases[0].Name
		}

		// Format size as human-readable
		sizeMB := float64(img.Size) / (1024 * 1024)
		sizeStr := fmt.Sprintf("%.1f MiB", sizeMB)

		// Parse and format upload time
		uploaded := img.UploadedAt
		if t, err := time.Parse(time.RFC3339, img.UploadedAt); err == nil {
			uploaded = t.Format("2006-01-02 15:04")
		}

		table.AddRow(
			ui.Name(alias),
			img.Fingerprint[:12],
			img.Architecture,
			sizeStr,
			uploaded,
		)
	}

	fmt.Print(table.Render())
}

func imageExistsCmd(args []string) {
	if len(args) < 1 {
		ui.Muted("Usage: coop image exists <alias>")
		os.Exit(1)
	}

	alias := args[0]
	mgr := mustManager()

	if mgr.ImageExists(alias) {
		ui.Successf("Image %s exists", ui.Name(alias))
		os.Exit(0)
	} else {
		ui.Warnf("Image %s not found", alias)
		os.Exit(1)
	}
}

func imagePublishCmd(args []string) {
	if len(args) < 3 {
		ui.Error("container, snapshot, and alias required")
		ui.Muted("Usage: coop image publish <container> <snapshot> <alias>")
		ui.Muted("Example: coop image publish mydev checkpoint1 claude-code-base")
		os.Exit(1)
	}

	container := args[0]
	snapshot := args[1]
	alias := args[2]

	mgr := mustManager()

	ui.Printf("Publishing %s/%s as %s...\n", ui.Name(container), ui.Name(snapshot), ui.Name(alias))
	if err := mgr.PublishSnapshot(container, snapshot, alias); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Image %s published", ui.Name(alias))
	ui.Mutedf("Lineage recorded: %s/%s -> %s", container, snapshot, alias)
}

func imageLineageCmd(args []string) {
	if len(args) < 1 {
		ui.Error("image alias required")
		ui.Muted("Usage: coop image lineage <alias>")
		os.Exit(1)
	}

	alias := args[0]

	registry, err := state.LoadRegistry(appConfig.Dirs.Data)
	if err != nil {
		ui.Errorf("Error loading registry: %v", err)
		os.Exit(1)
	}

	source := registry.GetSource(alias)
	if source == nil {
		ui.Warnf("No lineage recorded for %s", alias)
		ui.Muted("(Image may have been imported or built externally)")
		os.Exit(1)
		return // unreachable, satisfies static analysis
	}

	fmt.Printf("%s  %s\n", ui.Bold("Image:"), ui.Name(alias))
	fmt.Printf("%s  %s\n", ui.Bold("Built from:"), ui.Name(source.Instance))
	fmt.Printf("%s  %s\n", ui.Bold("Snapshot:"), ui.Name(source.Snapshot))

	// Try to show more lineage from the instance's state tracker
	instanceDir := filepath.Join(appConfig.Dirs.Data, "instances")
	tracker, err := state.NewTracker(instanceDir, source.Instance, "")
	if err == nil {
		inst := tracker.Instance()
		if inst.BaseImage != "" {
			fmt.Printf("%s  %s\n", ui.Bold("Base image:"), ui.Name(inst.BaseImage))
		}
	}
}

func stateCmd(args []string) {
	if len(args) == 0 {
		printStateUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "history":
		stateHistoryCmd(args[1:])
	case "show":
		stateShowCmd(args[1:])
	default:
		ui.Errorf("Unknown state subcommand: %s", args[0])
		printStateUsage()
		os.Exit(1)
	}
}

func stateHistoryCmd(args []string) {
	fs := flag.NewFlagSet("state history", flag.ExitOnError)
	limit := fs.Int("n", 20, "Number of entries to show")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop state history <container> [-n limit]")
		os.Exit(1)
	}

	container := fs.Arg(0)

	instanceDir := filepath.Join(appConfig.Dirs.Data, "instances")
	tracker, err := state.NewTracker(instanceDir, container, "")
	if err != nil {
		ui.Errorf("Error loading state: %v", err)
		ui.Muted("(Container may not have state tracking enabled)")
		os.Exit(1)
	}

	history, err := tracker.History(*limit)
	if err != nil {
		ui.Errorf("Error getting history: %v", err)
		os.Exit(1)
	}

	if len(history) == 0 {
		ui.Muted("No state history recorded")
		return
	}

	fmt.Printf("%s state history:\n\n", ui.Name(container))

	for _, entry := range history {
		timeStr := entry.Time.Format("2006-01-02 15:04")
		hash := entry.Hash[:8]
		msg := strings.TrimSpace(entry.Message)
		fmt.Printf("  %s  %s  %s\n", ui.MutedText(hash), ui.MutedText(timeStr), msg)
	}
}

func stateShowCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop state show <container>")
		os.Exit(1)
	}

	container := args[0]

	instanceDir := filepath.Join(appConfig.Dirs.Data, "instances")
	tracker, err := state.NewTracker(instanceDir, container, "")
	if err != nil {
		ui.Errorf("Error loading state: %v", err)
		os.Exit(1)
	}

	inst := tracker.Instance()

	fmt.Printf("%s  %s\n", ui.Bold("Name:"), ui.Name(inst.Name))
	fmt.Printf("%s  %s\n", ui.Bold("Base image:"), inst.BaseImage)
	fmt.Printf("%s  %s\n", ui.Bold("Created:"), inst.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("%s  %s\n", ui.Bold("State dir:"), ui.Path(tracker.Path()))

	if inst.CurrentSnapshot != "" {
		fmt.Printf("%s  %s\n", ui.Bold("Current snapshot:"), ui.Name(inst.CurrentSnapshot))
	}

	// Show packages if any
	hasPackages := len(inst.Packages.Apt) > 0 || len(inst.Packages.Pip) > 0 ||
		len(inst.Packages.Npm) > 0 || len(inst.Packages.Go) > 0 ||
		len(inst.Packages.Cargo) > 0 || len(inst.Packages.Brew) > 0

	if hasPackages {
		fmt.Println()
		fmt.Println(ui.Header("Packages:"))
		if len(inst.Packages.Apt) > 0 {
			fmt.Printf("  apt: %s\n", strings.Join(inst.Packages.Apt, ", "))
		}
		if len(inst.Packages.Pip) > 0 {
			fmt.Printf("  pip: %s\n", strings.Join(inst.Packages.Pip, ", "))
		}
		if len(inst.Packages.Npm) > 0 {
			fmt.Printf("  npm: %s\n", strings.Join(inst.Packages.Npm, ", "))
		}
		if len(inst.Packages.Go) > 0 {
			fmt.Printf("  go: %s\n", strings.Join(inst.Packages.Go, ", "))
		}
		if len(inst.Packages.Cargo) > 0 {
			fmt.Printf("  cargo: %s\n", strings.Join(inst.Packages.Cargo, ", "))
		}
		if len(inst.Packages.Brew) > 0 {
			fmt.Printf("  brew: %s\n", strings.Join(inst.Packages.Brew, ", "))
		}
	}

	if len(inst.Mounts) > 0 {
		fmt.Println()
		fmt.Println(ui.Header("Mounts:"))
		for _, m := range inst.Mounts {
			mode := "rw"
			if m.Readonly {
				mode = "ro"
			}
			fmt.Printf("  %s: %s -> %s (%s)\n", ui.Name(m.Name), m.Source, m.Path, mode)
		}
	}
}

func printStateUsage() {
	fmt.Println("Usage: coop state <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  history <container> [-n limit]   Show state change history")
	fmt.Println("  show <container>                 Show current tracked state")
	fmt.Println("\nState tracking records:")
	fmt.Println("  - Snapshots created with 'coop snapshot create'")
	fmt.Println("  - Base image used to create the container")
	fmt.Println("  - Git-style history with commit messages")
}

func printImageUsage() {
	fmt.Println("Usage: coop image <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  build                              Build the coop-agent-base image (~10 min)")
	fmt.Println("  list                               List local images")
	fmt.Println("  exists <alias>                     Check if an image alias exists")
	fmt.Println("  publish <container> <snap> <alias> Publish snapshot as new image")
	fmt.Println("  lineage <alias>                    Show where an image came from")
	fmt.Println("\nThe base image includes Python 3.13, Go 1.24, Node.js 24,")
	fmt.Println("GitHub CLI, Claude Code CLI, and development tools.")
	fmt.Println("\nWorkflow example:")
	fmt.Println("  coop create mydev                        # Create from base")
	fmt.Println("  coop shell mydev                         # Customize it")
	fmt.Println("  coop snapshot create mydev checkpoint1   # Save state")
	fmt.Println("  coop image publish mydev checkpoint1 my-variant")
}
