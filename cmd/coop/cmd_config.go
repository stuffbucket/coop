package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) ConfigCmd(args []string) {
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
	fmt.Printf("  CPUs:      %d\n", a.Config.Settings.DefaultCPUs)
	fmt.Printf("  Memory:    %d MB\n", a.Config.Settings.DefaultMemoryMB)
	fmt.Printf("  Disk:      %d GB\n", a.Config.Settings.DefaultDiskGB)
	fmt.Printf("  Image:     %s\n", ui.Name(a.Config.Settings.DefaultImage))

	fmt.Println("\n" + ui.Header("Environment Overrides:"))
	printEnvVar("COOP_CONFIG_DIR", dirs.Config)
	printEnvVar("COOP_DATA_DIR", dirs.Data)
	printEnvVar("COOP_CACHE_DIR", dirs.Cache)
	printEnvVar("COOP_DEFAULT_IMAGE", a.Config.Settings.DefaultImage)

	fmt.Println("\n" + ui.Header("SSH Keys:"))
	if _, err := os.Stat(dirs.SSH + "/id_ed25519"); err == nil {
		fmt.Printf("  Private key: %s %s\n", ui.Path(dirs.SSH+"/id_ed25519"), ui.SuccessText("✓"))
		fmt.Printf("  Public key:  %s %s\n", ui.Path(dirs.SSH+"/id_ed25519.pub"), ui.SuccessText("✓"))
	} else {
		ui.Muted("  Not yet generated (created on first 'create')")
	}

	fmt.Println("\n" + ui.Header("Settings File:"))
	if _, err := os.Stat(dirs.SettingsFile); err == nil {
		fmt.Printf("  %s %s\n", ui.Path(dirs.SettingsFile), ui.SuccessText("✓"))
	} else {
		ui.Muted("  Not yet created (run 'coop init')")
	}

	fmt.Println("\n" + ui.Header("Logging:"))
	fmt.Printf("  Log file:    %s\n", ui.Path(dirs.Logs+"/coop.log"))
	fmt.Printf("  Max size:    %d MB\n", a.Config.Settings.Log.MaxSizeMB)
	fmt.Printf("  Max backups: %d\n", a.Config.Settings.Log.MaxBackups)
	fmt.Printf("  Max age:     %d days\n", a.Config.Settings.Log.MaxAgeDays)
	fmt.Printf("  Compress:    %t\n", a.Config.Settings.Log.Compress)
	fmt.Printf("  Debug:       %t\n", a.Config.Settings.Log.Debug)

	if runtime.GOOS == "darwin" {
		fmt.Println("\n" + ui.Header("VM Backend:"))
		fmt.Printf("  Priority:   %v\n", a.Config.Settings.VM.BackendPriority)
		fmt.Printf("  Instance:   %s\n", ui.Name(a.Config.Settings.VM.Instance))
		fmt.Printf("  CPUs:       %d\n", a.Config.Settings.VM.CPUs)
		fmt.Printf("  Memory:     %d GB\n", a.Config.Settings.VM.MemoryGB)
		fmt.Printf("  Disk:       %d GB\n", a.Config.Settings.VM.DiskGB)
		fmt.Printf("  Auto-start: %t\n", a.Config.Settings.VM.AutoStart)
	}
}

func (a *App) EnvCmd(args []string) {
	fmt.Println(ui.Bold("Coop Environment Variables"))
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println()

	dirs := config.GetDirectories()

	envVars := []struct {
		name    string
		desc    string
		current string
	}{
		{"COOP_CONFIG_DIR", "Configuration directory", dirs.Config},
		{"COOP_DATA_DIR", "Data directory (instances, images)", dirs.Data},
		{"COOP_CACHE_DIR", "Cache directory", dirs.Cache},
		{"COOP_DEFAULT_IMAGE", "Default container image", a.Config.Settings.DefaultImage},
		{"COOP_VM_INSTANCE", "VM instance name", a.Config.Settings.VM.Instance},
		{"COOP_VM_BACKEND", "Force VM backend (bladerunner, colima, lima)", ""},
	}

	for _, ev := range envVars {
		val := os.Getenv(ev.name)
		if val != "" {
			fmt.Printf("  %s\n", ui.Name(ev.name))
			fmt.Printf("    %s\n", ui.MutedText(ev.desc))
			fmt.Printf("    Value: %s %s\n", ui.SuccessText(val), ui.MutedText("(from environment)"))
		} else {
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

func printEnvVar(name, currentValue string) {
	if val := os.Getenv(name); val != "" {
		fmt.Printf("  %s=%s %s\n", name, val, ui.SuccessText("(active)"))
	} else {
		fmt.Printf("  %s %s\n", name, ui.MutedText("(not set)"))
	}
}
