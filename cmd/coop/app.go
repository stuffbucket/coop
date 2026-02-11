package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/stuffbucket/coop/internal/backend"
	"github.com/stuffbucket/coop/internal/config"
	"github.com/stuffbucket/coop/internal/logging"
	"github.com/stuffbucket/coop/internal/names"
	"github.com/stuffbucket/coop/internal/sandbox"
	"github.com/stuffbucket/coop/internal/ui"
)

// BuildInfo holds version metadata set via ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// App holds application-wide dependencies for command handlers.
type App struct {
	Config *config.Config
	Build  BuildInfo
}

// NewApp initializes the application: loads config, sets up logging and theme.
func NewApp(build BuildInfo) *App {
	cfg, err := config.Load()
	if err != nil {
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}

	initLogging(cfg)

	theme := ui.ThemeByName(cfg.Settings.UI.Theme)
	ui.SetTheme(theme)

	return &App{
		Config: cfg,
		Build:  build,
	}
}

// Manager creates a sandbox.Manager or exits on failure.
func (a *App) Manager() *sandbox.Manager {
	mgr, err := sandbox.NewManagerWithConfig(a.Config)
	if err != nil {
		log := logging.Get()
		log.Debug("Manager creation failed", "error", err)

		var cancelErr *backend.UserCancelError
		if errors.As(err, &cancelErr) {
			fmt.Fprintln(os.Stderr)
			ui.Muted(cancelErr.Message)
			fmt.Fprintln(os.Stderr)
		} else {
			ui.Errorf("Error: %v", err)
		}
		os.Exit(1)
	}
	return mgr
}

// BackendManager creates a backend.Manager or returns an error.
func (a *App) BackendManager() (*backend.Manager, error) {
	return backend.NewManager(a.Config)
}

// ValidContainerName validates a container name or exits.
func (a *App) ValidContainerName(name string) string {
	if err := names.ValidateContainerName(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
	return name
}

// ValidSnapshotName validates a snapshot name or exits.
func (a *App) ValidSnapshotName(name string) string {
	if err := names.ValidateSnapshotName(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
	return name
}

// ValidMountName validates a mount name or exits.
func (a *App) ValidMountName(name string) string {
	if err := names.ValidateMountName(name); err != nil {
		ui.Errorf("Error: %v", err)
		os.Exit(1)
	}
	return name
}

func initLogging(cfg *config.Config) {
	logCfg := logging.Config{
		Dir:        cfg.Dirs.Logs,
		MaxSizeMB:  cfg.Settings.Log.MaxSizeMB,
		MaxBackups: cfg.Settings.Log.MaxBackups,
		MaxAgeDays: cfg.Settings.Log.MaxAgeDays,
		Compress:   cfg.Settings.Log.Compress,
		Debug:      cfg.Settings.Log.Debug,
	}

	if err := logging.Init(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logging: %v\n", err)
	}
}
