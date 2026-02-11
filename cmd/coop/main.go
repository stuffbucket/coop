// Command coop provides CLI for managing AI agent containers.
package main

import (
	"fmt"
	"os"

	"github.com/stuffbucket/coop/internal/logging"
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

func main() {
	app := NewApp(BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	defer func() { _ = logging.Close() }()

	if len(os.Args) < 2 {
		app.PrintUsage(false)
		os.Exit(1)
	}

	args := os.Args[2:]

	switch os.Args[1] {
	case "init":
		app.InitCmd(args)
	case "create":
		app.CreateCmd(args)
	case "start":
		app.StartCmd(args)
	case "stop":
		app.StopCmd(args)
	case "lock":
		app.LockCmd(args)
	case "unlock":
		app.UnlockCmd(args)
	case "delete", "rm":
		app.DeleteCmd(args)
	case "list", "ls":
		app.ListCmd(args)
	case "status":
		app.StatusCmd(args)
	case "logs":
		app.LogsCmd(args)
	case "shell":
		app.ShellCmd(args)
	case "ssh":
		app.SSHCmd(args)
	case "exec":
		app.ExecCmd(args)
	case "mount":
		app.MountCmd(args)
	case "snapshot":
		app.SnapshotCmd(args)
	case "config":
		app.ConfigCmd(args)
	case "theme":
		app.ThemeCmd(args)
	case "image":
		app.ImageCmd(args)
	case "state":
		app.StateCmd(args)
	case "env":
		app.EnvCmd(args)
	case "vm", "lima":
		app.VMCmd(args)
	case "doctor":
		app.DoctorCmd(args)
	case "version", "-v", "--version":
		app.VersionCmd()
	case "help", "-h", "--help":
		app.HelpCmd(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		app.PrintUsage(false)
		os.Exit(1)
	}
}
