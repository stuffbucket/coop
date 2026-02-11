package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/stuffbucket/coop/internal/backend"
	"github.com/stuffbucket/coop/internal/sandbox"
	"github.com/stuffbucket/coop/internal/state"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) ImageCmd(args []string) {
	if len(args) == 0 {
		printImageUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "build":
		a.imageBuildCmd(args[1:])
	case "list", "ls":
		a.imageListCmd(args[1:])
	case "exists":
		a.imageExistsCmd(args[1:])
	case "publish":
		a.imagePublishCmd(args[1:])
	case "lineage":
		a.imageLineageCmd(args[1:])
	default:
		ui.Errorf("Unknown image subcommand: %s", args[0])
		printImageUsage()
		os.Exit(1)
	}
}

func (a *App) imageBuildCmd(args []string) {
	// Ensure incus CLI is configured for the active backend
	if vmMgr, err := backend.NewManager(a.Config); err == nil {
		_ = vmMgr.EnsureRunning()
	}

	if err := sandbox.BuildBaseImage(); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
}

func (a *App) imageListCmd(args []string) {
	mgr := a.Manager()

	vmMgr, err := backend.NewManager(a.Config)
	if err != nil {
		ui.Errorf("Error initializing backend: %v", err)
		os.Exit(1)
	}

	socketPath, err := vmMgr.GetIncusSocket()
	if err != nil {
		ui.Errorf("Error getting Incus socket: %v", err)
		os.Exit(1)
	}

	_ = mgr // manager connected, so incus is available

	cmd := exec.Command("incus", "image", "list", "--format", "json")
	cmd.Env = append(os.Environ(), "INCUS_SOCKET="+socketPath)

	output, err := cmd.Output()
	if err != nil {
		ui.Errorf("Failed to list images: %v", err)
		os.Exit(1)
	}

	var images []struct {
		Aliases []struct {
			Name string `json:"name"`
		} `json:"aliases"`
		Fingerprint  string            `json:"fingerprint"`
		Size         int64             `json:"size"`
		Architecture string            `json:"architecture"`
		UploadedAt   string            `json:"uploaded_at"`
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

	table := ui.NewTable(24, 14, 12, 10, 20)
	table.SetHeaders("ALIAS", "FINGERPRINT", "ARCH", "SIZE", "UPLOADED")

	for _, img := range images {
		alias := "-"
		if len(img.Aliases) > 0 {
			alias = img.Aliases[0].Name
		}

		sizeMB := float64(img.Size) / (1024 * 1024)
		sizeStr := fmt.Sprintf("%.1f MiB", sizeMB)

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

func (a *App) imageExistsCmd(args []string) {
	if len(args) < 1 {
		ui.Muted("Usage: coop image exists <alias>")
		os.Exit(1)
	}

	alias := args[0]
	mgr := a.Manager()

	if mgr.ImageExists(alias) {
		ui.Successf("Image %s exists", ui.Name(alias))
		os.Exit(0)
	} else {
		ui.Warnf("Image %s not found", alias)
		os.Exit(1)
	}
}

func (a *App) imagePublishCmd(args []string) {
	if len(args) < 3 {
		ui.Error("container, snapshot, and alias required")
		ui.Muted("Usage: coop image publish <container> <snapshot> <alias>")
		ui.Muted("Example: coop image publish mydev checkpoint1 claude-code-base")
		os.Exit(1)
	}

	container := a.ValidContainerName(args[0])
	snapshot := args[1]
	alias := args[2]

	mgr := a.Manager()

	ui.Printf("Publishing %s/%s as %s...\n", ui.Name(container), ui.Name(snapshot), ui.Name(alias))
	if err := mgr.PublishSnapshot(container, snapshot, alias); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}

	ui.Successf("Image %s published", ui.Name(alias))
	ui.Mutedf("Lineage recorded: %s/%s -> %s", container, snapshot, alias)
}

func (a *App) imageLineageCmd(args []string) {
	if len(args) < 1 {
		ui.Error("image alias required")
		ui.Muted("Usage: coop image lineage <alias>")
		os.Exit(1)
	}

	alias := args[0]

	registry, err := state.LoadRegistry(a.Config.Dirs.Data)
	if err != nil {
		ui.Errorf("Error loading registry: %v", err)
		os.Exit(1)
	}

	source := registry.GetSource(alias)
	if source == nil {
		ui.Warnf("No lineage recorded for %s", alias)
		ui.Muted("(Image may have been imported or built externally)")
		os.Exit(1)
		return
	}

	fmt.Printf("%s  %s\n", ui.Bold("Image:"), ui.Name(alias))
	fmt.Printf("%s  %s\n", ui.Bold("Built from:"), ui.Name(source.Instance))
	fmt.Printf("%s  %s\n", ui.Bold("Snapshot:"), ui.Name(source.Snapshot))

	instanceDir := filepath.Join(a.Config.Dirs.Data, "instances")
	tracker, err := state.NewTracker(instanceDir, source.Instance, "")
	if err == nil {
		inst := tracker.Instance()
		if inst.BaseImage != "" {
			fmt.Printf("%s  %s\n", ui.Bold("Base image:"), ui.Name(inst.BaseImage))
		}
	}
}

func printImageUsage() {
	fmt.Println("Usage: coop image <subcommand>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  build                              Build the coop-agent-base image (~10 min)")
	fmt.Println("  list                               List local images")
	fmt.Println("  exists <alias>                     Check if an image alias exists")
	fmt.Println("  publish <container> <snap> <alias> Publish snapshot as new image")
	fmt.Println("  lineage <alias>                    Show where an image came from")
	fmt.Println("\nThe base image includes Python 3.13, Go 1.24, Node.js 24,")
	fmt.Println("GitHub CLI, Claude Code CLI, and development tools.")
	fmt.Println("\nWorkflow example:")
	fmt.Println("  coop create mydev                        # Create from base")
	fmt.Println("  coop shell mydev                         # Customize it")
	fmt.Println("  coop snapshot create mydev checkpoint1   # Save state")
	fmt.Println("  coop image publish mydev checkpoint1 my-variant")
}
