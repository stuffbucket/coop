package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/stuffbucket/coop/internal/sandbox"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) MountCmd(args []string) {
	if len(args) == 0 {
		printMountUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "add":
		a.mountAddCmd(args[1:])
	case "remove", "rm":
		a.mountRemoveCmd(args[1:])
	case "list", "ls":
		a.mountListCmd(args[1:])
	default:
		ui.Errorf("Unknown mount subcommand: %s", args[0])
		printMountUsage()
		os.Exit(1)
	}
}

func (a *App) mountAddCmd(args []string) {
	fs := flag.NewFlagSet("mount add", flag.ExitOnError)
	name := fs.String("name", "", "Mount name (default: derived from path)")
	path := fs.String("path", "", "Mount path inside container (default: /home/agent/<basename>)")
	readonly := fs.Bool("readonly", false, "Mount read-only (container cannot write)")
	force := fs.Bool("force", false, "Authorize mounting protected directories")
	_ = fs.Parse(args)

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

		_, err := sandbox.CurrentAuthCode()
		if err != nil {
			ui.Errorf("Failed to generate authorization code: %v", err)
			os.Exit(1)
		}
		ui.NotifyWithSound("coop", "Protected Path", "Enter the auth code in your terminal", "Purr")

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

	mgr := a.Manager()

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

func (a *App) mountRemoveCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and mount name required")
		ui.Muted("Usage: coop mount remove <container> <mount-name>")
		os.Exit(1)
	}

	container := a.ValidContainerName(args[0])
	mountName := a.ValidMountName(args[1])

	mgr := a.Manager()

	ui.Printf("Removing mount %s from %s...\n", ui.Name(mountName), ui.Name(container))
	if err := mgr.Unmount(container, mountName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Mount %s removed", ui.Name(mountName))
}

func (a *App) mountListCmd(args []string) {
	mgr := a.Manager()

	if len(args) >= 1 {
		container := a.ValidContainerName(args[0])
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
