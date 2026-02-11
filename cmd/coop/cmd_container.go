package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stuffbucket/coop/internal/backend"
	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/sandbox"
	"github.com/stuffbucket/coop/internal/state"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) InitCmd(args []string) {
	dirs := config.GetDirectories()

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

	pubKey, err := sandbox.EnsureSSHKeys()
	if err != nil {
		ui.Warnf("Could not generate SSH keys: %v", err)
	} else {
		ui.Successf("SSH keys generated in: %s", ui.Path(dirs.SSH))
		_ = pubKey
	}

	fmt.Println()
	ui.Success("Coop initialized successfully!")
	ui.Muted("Run 'coop config' to see all paths.")
	fmt.Println()
}

func (a *App) CreateCmd(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	cpus := fs.Int("cpus", 2, "Number of CPUs")
	memory := fs.Int("memory", 4096, "Memory in MB")
	disk := fs.Int("disk", 20, "Disk size in GB")
	sshKey := fs.String("ssh-key", "", "SSH public key (default: auto-detect)")
	workDir := fs.String("workdir", "", "Host directory to mount as workspace")
	verbose := fs.Bool("verbose", false, "Stream cloud-init logs during setup")

	_ = fs.Parse(args)

	var name string
	if fs.NArg() < 1 {
		name = sandbox.GenerateName()
		ui.Printf("Generated name: %s\n", ui.Name(name))
	} else {
		name = fs.Arg(0)
	}

	mgr := a.Manager()

	cfg := sandbox.DefaultContainerConfig(name)
	cfg.CPUs = *cpus
	cfg.MemoryMB = *memory
	cfg.DiskGB = *disk
	cfg.WorkingDir = *workDir
	cfg.Verbose = *verbose

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

	baseImage := a.Config.Settings.DefaultImage
	if baseImage == "" {
		baseImage = sandbox.DefaultImage
	}
	instanceDir := filepath.Join(a.Config.Dirs.Data, "instances")
	if _, err := state.NewTracker(instanceDir, name, baseImage); err != nil {
		ui.Warnf("Container created but state tracking failed: %v", err)
	}

	if ip, _ := mgr.GetContainerIP(name); ip != "" {
		if err := sandbox.UpdateSSHConfig(name, ip); err != nil {
			ui.Warnf("Could not update SSH config: %v", err)
		}
	}
}

func (a *App) StartCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop start <name>")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	mgr := a.Manager()

	ui.Printf("Starting container %s...\n", ui.Name(name))
	if err := mgr.Start(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s started", ui.Name(name))

	if ip, _ := mgr.GetContainerIP(name); ip != "" {
		if err := sandbox.UpdateSSHConfig(name, ip); err != nil {
			ui.Warnf("Could not update SSH config: %v", err)
		}
	}
}

func (a *App) StopCmd(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	force := fs.Bool("force", false, "Force stop")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop stop <name> [--force]")
		os.Exit(1)
	}

	name := a.ValidContainerName(fs.Arg(0))
	mgr := a.Manager()

	ui.Printf("Stopping container %s...\n", ui.Name(name))
	if err := mgr.Stop(name, *force); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s stopped", ui.Name(name))
}

func (a *App) LockCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop lock <name>")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	mgr := a.Manager()

	ui.Printf("Locking container %s...\n", ui.Name(name))
	if err := mgr.Lock(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s locked (all processes frozen)", ui.Name(name))
}

func (a *App) UnlockCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop unlock <name>")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	mgr := a.Manager()

	ui.Printf("Unlocking container %s...\n", ui.Name(name))
	if err := mgr.Unlock(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s unlocked (processes resumed)", ui.Name(name))
}

func (a *App) LogsCmd(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "Follow log output")
	lines := fs.Int("n", 50, "Number of lines to show")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop logs <name> [-f] [-n lines]")
		os.Exit(1)
	}

	name := a.ValidContainerName(fs.Arg(0))
	mgr := a.Manager()

	if err := mgr.Logs(name, *follow, *lines); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
}

func (a *App) DeleteCmd(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	force := fs.Bool("force", false, "Force stop running container")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop delete <name> [--force]")
		os.Exit(1)
	}

	name := a.ValidContainerName(fs.Arg(0))
	mgr := a.Manager()

	if err := mgr.Delete(name, *force); err != nil {
		ui.Errorf("Error deleting container: %v", err)
		os.Exit(1)
	}
}

func (a *App) ListCmd(args []string) {
	mgr := a.Manager()

	containers, err := mgr.List()
	if err != nil {
		ui.Errorf("Error listing containers: %v", err)
		os.Exit(1)
	}

	if len(containers) == 0 {
		ui.Muted("No agent containers found")
		return
	}

	backendName := "-"
	if vmMgr, err := backend.NewManager(a.Config); err == nil {
		backendName = vmMgr.Backend().Name()
	}

	table := ui.NewTable(20, 10, 15, 5, 8, 6, 14, 16)
	table.SetHeaders("NAME", "STATUS", "IP", "CPUS", "MEMORY", "DISK", "BACKEND", "CREATED")

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
				backendName,
				created,
			)
		} else {
			table.AddRow(
				ui.MutedText(c.Name),
				ui.Status(c.Status),
				ui.MutedText(ip),
				ui.MutedText(cpus),
				ui.MutedText(mem),
				ui.MutedText(disk),
				ui.MutedText(backendName),
				ui.MutedText(created),
			)
		}
	}

	fmt.Print(table.Render())
}

func (a *App) StatusCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop status <name>")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	mgr := a.Manager()

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

	fmt.Println()
	ui.Print(ui.Header("Configuration:"))
	for k, v := range status.Config {
		if strings.HasPrefix(k, "limits.") || k == "security.nesting" {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
}
