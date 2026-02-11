package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) SSHCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop ssh <name>")
		fmt.Println()
		ui.Muted("Tip: Use 'coop shell <name>' to connect directly")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	mgr := a.Manager()

	cmd, err := mgr.SSH(name)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	fmt.Println(cmd)
}

func (a *App) ShellCmd(args []string) {
	if len(args) < 1 {
		ui.Error("container name required")
		ui.Muted("Usage: coop shell <name> [command...]")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	remoteCmd := args[1:]

	mgr := a.Manager()

	// For backends where container IPs aren't routable from the host
	// (e.g. bladerunner), use the Incus exec API instead of SSH.
	if mgr.UseIncusExec() {
		exitCode, err := mgr.Shell(name, remoteCmd)
		if err != nil {
			ui.Errorf("Error: %v", err)
			os.Exit(1)
		}
		os.Exit(exitCode)
	}

	sshArgs, err := mgr.SSHArgs(name)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	if len(remoteCmd) > 0 {
		sshArgs = append(sshArgs, "--")
		sshArgs = append(sshArgs, remoteCmd...)
	}

	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		ui.Error("ssh not found in PATH")
		os.Exit(1)
	}

	// Replace current process with ssh
	if err := syscall.Exec(sshPath, append([]string{"ssh"}, sshArgs...), os.Environ()); err != nil {
		ui.Errorf("failed to exec ssh: %v", err)
		os.Exit(1)
	}
}

func (a *App) ExecCmd(args []string) {
	if len(args) < 2 {
		ui.Error("container name and command required")
		ui.Muted("Usage: coop exec <name> <command> [args...]")
		os.Exit(1)
	}

	name := a.ValidContainerName(args[0])
	command := args[1:]

	mgr := a.Manager()

	exitCode, err := mgr.Exec(name, command)
	if err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}
