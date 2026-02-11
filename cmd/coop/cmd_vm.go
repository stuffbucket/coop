package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/stuffbucket/coop/internal/backend"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) VMCmd(args []string) {
	if runtime.GOOS != "darwin" {
		ui.Error("vm command is only needed on macOS (Linux runs Incus natively)")
		os.Exit(1)
	}

	mgr, err := backend.NewManager(a.Config)
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
		ui.Printf("Starting VM (backend: %s, instance: %s)...\n", mgr.Backend().Name(), ui.Name(a.Config.Settings.VM.Instance))
		if err := mgr.Start(); err != nil {
			ui.Errorf("Error starting VM: %v", err)
			os.Exit(1)
		}
		ui.Success("VM is running")

	case "stop":
		ui.Printf("Stopping VM (backend: %s, instance: %s)...\n", mgr.Backend().Name(), ui.Name(a.Config.Settings.VM.Instance))
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
		ui.Printf("Deleting VM (backend: %s, instance: %s)...\n", mgr.Backend().Name(), ui.Name(a.Config.Settings.VM.Instance))
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
		fmt.Printf("%s %v\n", ui.Bold("Priority order:"), a.Config.Settings.VM.BackendPriority)

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
