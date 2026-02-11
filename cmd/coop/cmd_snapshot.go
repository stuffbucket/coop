package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stuffbucket/coop/internal/state"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) SnapshotCmd(args []string) {
	if len(args) == 0 {
		printSnapshotUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "create":
		a.snapshotCreateCmd(args[1:])
	case "restore":
		a.snapshotRestoreCmd(args[1:])
	case "list", "ls":
		a.snapshotListCmd(args[1:])
	case "delete", "rm":
		a.snapshotDeleteCmd(args[1:])
	default:
		ui.Errorf("Unknown snapshot subcommand: %s", args[0])
		printSnapshotUsage()
		os.Exit(1)
	}
}

func (a *App) snapshotCreateCmd(args []string) {
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

	mgr := a.Manager()

	ui.Printf("Creating snapshot %s of %s...\n", ui.Name(snapshotName), ui.Name(container))
	if err := mgr.CreateSnapshot(container, snapshotName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	instanceDir := filepath.Join(a.Config.Dirs.Data, "instances")
	tracker, err := state.NewTracker(instanceDir, container, "")
	if err == nil {
		if _, err := tracker.RecordSnapshot(snapshotName, *note); err != nil {
			ui.Warnf("Snapshot created but state tracking failed: %v", err)
		}
	}

	ui.Successf("Snapshot %s created", ui.Name(snapshotName))
}

func (a *App) snapshotRestoreCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and snapshot name required")
		ui.Muted("Usage: coop snapshot restore <container> <snapshot-name>")
		os.Exit(1)
	}

	container := a.ValidContainerName(args[0])
	snapshotName := a.ValidSnapshotName(args[1])

	mgr := a.Manager()

	ui.Printf("Restoring %s to snapshot %s...\n", ui.Name(container), ui.Name(snapshotName))
	if err := mgr.RestoreSnapshot(container, snapshotName); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Container %s restored to %s", ui.Name(container), ui.Name(snapshotName))
}

func (a *App) snapshotListCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop snapshot list <container>")
		os.Exit(1)
	}

	container := a.ValidContainerName(args[0])
	mgr := a.Manager()

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

func (a *App) snapshotDeleteCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and snapshot name required")
		ui.Muted("Usage: coop snapshot delete <container> <snapshot-name>")
		os.Exit(1)
	}

	container := a.ValidContainerName(args[0])
	snapshotName := a.ValidSnapshotName(args[1])

	mgr := a.Manager()

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
