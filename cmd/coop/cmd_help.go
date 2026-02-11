package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/stuffbucket/coop/internal/backend"
	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/sandbox"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) HelpCmd(args []string) {
	showAll := false
	for _, arg := range args {
		if arg == "--all" || arg == "-a" {
			showAll = true
		}
	}
	a.PrintUsage(showAll)
}

func (a *App) VersionCmd() {
	fmt.Printf("coop %s\n", a.Build.Version)
	if a.Build.Commit != "none" {
		fmt.Printf("  commit: %s\n", a.Build.Commit)
	}
	if a.Build.Date != "unknown" {
		fmt.Printf("  built:  %s\n", a.Build.Date)
	}
	fmt.Printf("  os:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  go:     %s\n", runtime.Version())
}

// detectHelpState checks the current state of coop for contextual help.
func (a *App) detectHelpState() ui.HelpState {
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
		if vmMgr, err := backend.NewManager(a.Config); err == nil {
			state.BackendName = vmMgr.Backend().Name()
			if status, err := vmMgr.Status(); err == nil {
				state.VMRunning = status.State == "Running"
			}
		}
	} else {
		state.VMRunning = true
	}

	// Check if base image exists - only if we can connect
	if state.Initialized && (state.VMRunning || !state.IsMacOS) {
		if mgr, err := sandbox.NewManagerWithConfig(a.Config); err == nil {
			state.BaseImageOK = mgr.ImageExists(a.Config.Settings.DefaultImage)
			if containers, err := mgr.List(); err == nil {
				state.AgentCount = len(containers)
				state.HasContainers = len(containers) > 0
				for _, c := range containers {
					if c.Status == "Running" {
						state.RunningCount++
					}
				}
			}
			if avail, total, err := mgr.GetStorageInfo(); err == nil {
				state.StorageAvail = avail
				state.StorageTotal = total
			}
		}
	}

	return state
}

func (a *App) PrintUsage(showAll bool) {
	state := a.detectHelpState()
	builder := ui.NewHelpBuilder(state)
	dashboard := ui.NewDashboardProvider(state, a.Build.Version, "")

	logo := ui.Banner()
	if logo != "" {
		fmt.Print(logo)
	}
	fmt.Println(ui.Tagline(a.Build.Version))

	if !showAll {
		compactWidth := builder.CompactLayoutWidth()
		if builder.Width() < compactWidth {
			compactWidth = builder.Width()
		}
		fmt.Println(ui.Separator(compactWidth - 1))
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
		headerWidth := builder.EffectiveWidth()
		fmt.Println(ui.Separator(headerWidth - 1))
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
