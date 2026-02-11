package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stuffbucket/coop/internal/state"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) StateCmd(args []string) {
	if len(args) == 0 {
		printStateUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "history":
		a.stateHistoryCmd(args[1:])
	case "show":
		a.stateShowCmd(args[1:])
	default:
		ui.Errorf("Unknown state subcommand: %s", args[0])
		printStateUsage()
		os.Exit(1)
	}
}

func (a *App) stateHistoryCmd(args []string) {
	fs := flag.NewFlagSet("state history", flag.ExitOnError)
	limit := fs.Int("n", 20, "Number of entries to show")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop state history <container> [-n limit]")
		os.Exit(1)
	}

	container := fs.Arg(0)

	instanceDir := filepath.Join(a.Config.Dirs.Data, "instances")
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

func (a *App) stateShowCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop state show <container>")
		os.Exit(1)
	}

	container := a.ValidContainerName(args[0])

	instanceDir := filepath.Join(a.Config.Dirs.Data, "instances")
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
