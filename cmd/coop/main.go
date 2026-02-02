// Command coop provides CLI for managing AI agent containers.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/bsmi021/coop/internal/config"
	"github.com/bsmi021/coop/internal/logging"
	"github.com/bsmi021/coop/internal/sandbox"
	"github.com/bsmi021/coop/internal/ui"
	"github.com/bsmi021/coop/internal/vm"
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

	if len(os.Args) < 2 {
		printUsage()
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
	case "image":
		imageCmd(os.Args[2:])
	case "vm", "lima": // lima kept as alias for backward compat
		vmCmd(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
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

func printUsage() {
	logo := ui.Logo()
	if logo != "" {
		fmt.Print(logo)
	}
	fmt.Println(ui.Tagline())
	fmt.Println()

	fmt.Println(ui.HelpSection("Usage:"))
	fmt.Println("  coop <command> [options]")
	fmt.Println()

	fmt.Println(ui.HelpSection("Commands:"))
	fmt.Println(ui.HelpCommand("init", "Initialize coop directories and settings"))
	fmt.Println(ui.HelpCommand("create", "Create a new agent container"))
	fmt.Println(ui.HelpCommand("start", "Start a stopped container"))
	fmt.Println(ui.HelpCommand("stop", "Stop a running container"))
	fmt.Println(ui.HelpCommand("lock", "Freeze container (pause all processes)"))
	fmt.Println(ui.HelpCommand("unlock", "Unfreeze container (resume processes)"))
	fmt.Println(ui.HelpCommand("delete", "Delete an agent container"))
	fmt.Println(ui.HelpCommand("list", "List all agent containers"))
	fmt.Println(ui.HelpCommand("status", "Show container status"))
	fmt.Println(ui.HelpCommand("logs", "View container logs (journalctl)"))
	fmt.Println(ui.HelpCommand("shell", "Open interactive shell in container"))
	fmt.Println(ui.HelpCommand("ssh", "Print SSH command for container"))
	fmt.Println(ui.HelpCommand("exec", "Execute command in container"))
	fmt.Println(ui.HelpCommand("mount", "Manage container mounts"))
	fmt.Println(ui.HelpCommand("snapshot", "Manage container snapshots"))
	fmt.Println(ui.HelpCommand("config", "Show configuration and paths"))
	fmt.Println(ui.HelpCommand("image", "Manage base images (build, list)"))
	fmt.Println(ui.HelpCommand("vm", "Manage VM backend (Colima/Lima)"))
	fmt.Println(ui.HelpCommand("help", "Show this help"))
	fmt.Println()

	fmt.Println(ui.HelpSection("Environment Variables:"))
	fmt.Println(ui.HelpEnvVar("COOP_CONFIG_DIR", "Config directory"))
	fmt.Println(ui.HelpEnvVar("COOP_DATA_DIR", "Data directory"))
	fmt.Println(ui.HelpEnvVar("COOP_CACHE_DIR", "Cache directory"))
	fmt.Println(ui.HelpEnvVar("COOP_DEFAULT_IMAGE", "Default container image"))
	fmt.Println(ui.HelpEnvVar("COOP_VM_INSTANCE", "VM instance name"))
	fmt.Println(ui.HelpEnvVar("COOP_VM_BACKEND", "Force backend (colima, lima)"))
	fmt.Println()

	fmt.Println(ui.HelpSection("Examples:"))
	fmt.Println(ui.HelpExample("coop init"))
	fmt.Println(ui.HelpExample("coop create myagent"))
	fmt.Println(ui.HelpExample("coop create myagent --cpus 4 --memory 8192"))
	fmt.Println(ui.HelpExample("coop list"))
	fmt.Println(ui.HelpExample("coop shell myagent"))
	fmt.Println(ui.HelpExample("coop vm status"))
	fmt.Println(ui.HelpExample("coop image build"))
}

// mustManager creates a new sandbox.Manager or exits with an error.
// This centralizes the repeated pattern of creating a manager and handling errors.
func mustManager() *sandbox.Manager {
	mgr, err := sandbox.NewManagerWithConfig(appConfig)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
	return mgr
}

func initCmd(args []string) {
	ui.Print(ui.Bold("Initializing coop..."))

	if err := config.EnsureDirectories(); err != nil {
		ui.Errorf("Error creating directories: %v", err)
		os.Exit(1)
	}

	dirs := config.GetDirectories()
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
	if len(args) < 2 {
		ui.Error("container name and snapshot name required")
		ui.Muted("Usage: coop snapshot create <container> <snapshot-name>")
		os.Exit(1)
	}

	container := args[0]
	snapshotName := args[1]

	mgr := mustManager()

	ui.Printf("Creating snapshot %s of %s...\n", ui.Name(snapshotName), ui.Name(container))
	if err := mgr.CreateSnapshot(container, snapshotName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
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

		// Generate code and send notification
		code := sandbox.CurrentAuthCode()
		codeGenTime := time.Now()
		ui.NotifyWithSound("coop", "Protected Path", fmt.Sprintf("Code: %s", code), "Purr")

		// Prompt via TTY (not stdout) - invisible to process capture
		ui.TTYPrint("\n⚠️  Protected path: %s\n", reason)
		ui.TTYPrint("A 6-digit authorization code has been sent via notification.\n")
		ui.TTYPrint("Enter code (3 attempts, expires in 15s): ")

		// Open TTY for reading
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			ui.Errorf("Cannot read from terminal: %v", err)
			os.Exit(1)
		}
		defer func() { _ = tty.Close() }()

		// 3 attempts
		for attempt := 1; attempt <= 3; attempt++ {
			// Check if code has expired
			if time.Since(codeGenTime) > 15*time.Second {
				ui.TTYPrint("\n")
				ui.Errorf("Authorization code expired")
				os.Exit(1)
			}

			var input string
			_, _ = fmt.Fscanln(tty, &input)
			input = strings.TrimSpace(input)

			if sandbox.ValidateAuthCode(input) {
				forceAuthorized = true
				ui.TTYPrint("✓ Authorized\n\n")
				break
			}

			remaining := 3 - attempt
			if remaining > 0 {
				ui.TTYPrint("✗ Invalid code. %d attempt(s) remaining: ", remaining)
			} else {
				ui.TTYPrint("\n")
				ui.Errorf("Authorization failed after 3 attempts")
				os.Exit(1)
			}
		}

		if !forceAuthorized {
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

	mgr, err := vm.NewManager(appConfig)
	if err != nil {
		ui.Errorf("Error creating VM manager: %v", err)
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

	// Get socket path from VM backend to pass to incus CLI
	vmMgr, err := vm.NewManager(appConfig)
	if err != nil {
		ui.Errorf("Error initializing VM manager: %v", err)
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
		Fingerprint  string `json:"fingerprint"`
		Size         int64  `json:"size"`
		Architecture string `json:"architecture"`
		UploadedAt   string `json:"uploaded_at"`
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

func printImageUsage() {
	fmt.Println("Usage: coop image <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  build      Build the coop-agent-base image (~10 min)")
	fmt.Println("  list       List local images")
	fmt.Println("  exists     Check if an image alias exists")
	fmt.Println("\nThe base image includes Python 3.13, Go 1.24, Node.js 24,")
	fmt.Println("GitHub CLI, Claude Code CLI, and development tools.")
}
