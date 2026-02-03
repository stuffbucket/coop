// Package state provides a metadata layer on top of Incus snapshots.
//
// Incus provides the primitives (instances, snapshots, images, publish).
// This package adds:
//   - Why each snapshot was created (git commit messages)
//   - What commands/changes led to each state
//   - Diff between any two states
//   - Branching for parallel experiments
//
// Each instance has a git repo at ~/.local/share/coop/instances/<name>/
// with state.json committed on each state change. Git commits map 1:1
// with Incus snapshots - the commit explains "why", the snapshot is "what".
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Instance represents the tracked state of an Incus instance.
// This is metadata about the instance, not a replacement for Incus.
type Instance struct {
	// Name matches the Incus instance name
	Name string `json:"name"`

	// BaseImage is the image this instance was created from
	BaseImage string `json:"base_image"`

	// CreatedAt is when the instance was created
	CreatedAt time.Time `json:"created_at"`

	// Packages installed via package managers
	Packages Packages `json:"packages,omitempty"`

	// Mounts attached to the instance
	Mounts []Mount `json:"mounts,omitempty"`

	// Env variables set on the instance
	Env map[string]string `json:"env,omitempty"`

	// CurrentSnapshot is the Incus snapshot name for current state (if any)
	CurrentSnapshot string `json:"current_snapshot,omitempty"`

	// UpdatedAt is when state.json was last modified
	UpdatedAt time.Time `json:"updated_at"`
}

// Links holds snapshot-to-commit mappings.
// Stored separately from state.json so it survives git reset.
type Links struct {
	// Snapshots maps Incus snapshot names to git commit hashes.
	Snapshots map[string]string `json:"snapshots"`
}

// linksPath returns the path to links.json for an instance.
func linksPath(stateDir, instance string) string {
	return filepath.Join(stateDir, instance, "links.json")
}

// LoadLinks reads snapshot links from disk.
func LoadLinks(stateDir, instance string) (*Links, error) {
	if err := ValidateInstanceName(instance); err != nil {
		return nil, err
	}

	path := linksPath(stateDir, instance)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Links{Snapshots: make(map[string]string)}, nil
		}
		return nil, err
	}

	var links Links
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, err
	}
	if links.Snapshots == nil {
		links.Snapshots = make(map[string]string)
	}
	return &links, nil
}

// Save writes links to disk.
func (l *Links) Save(stateDir, instance string) error {
	if err := ValidateInstanceName(instance); err != nil {
		return err
	}

	path := linksPath(stateDir, instance)
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0600)
}

// Link associates a snapshot with a commit.
func (l *Links) Link(snapshotName, commitHash string) {
	l.Snapshots[snapshotName] = commitHash
}

// CommitFor returns the commit hash for a snapshot.
func (l *Links) CommitFor(snapshotName string) string {
	return l.Snapshots[snapshotName]
}

// Packages tracks installed packages by manager.
type Packages struct {
	Apt    []string `json:"apt,omitempty"`
	Pip    []string `json:"pip,omitempty"`
	Npm    []string `json:"npm,omitempty"`
	Go     []string `json:"go,omitempty"`
	Cargo  []string `json:"cargo,omitempty"`
	Brew   []string `json:"brew,omitempty"`
	Custom []string `json:"custom,omitempty"`
}

// Mount tracks a mount attached to the instance.
type Mount struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Path     string `json:"path"`
	Readonly bool   `json:"readonly"`
}

// NewInstance creates state for a new Incus instance.
func NewInstance(name, baseImage string) *Instance {
	now := time.Now()
	return &Instance{
		Name:      name,
		BaseImage: baseImage,
		CreatedAt: now,
		UpdatedAt: now,
		Packages:  Packages{},
		Env:       make(map[string]string),
	}
}

// statePath returns the path to state.json for an instance.
func statePath(stateDir, instance string) string {
	return filepath.Join(stateDir, instance, "state.json")
}

// ValidateInstanceName checks that an instance name is safe for use in paths.
func ValidateInstanceName(name string) error {
	if name == "" {
		return fmt.Errorf("instance name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid instance name: %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("instance name contains invalid characters: %q", name)
	}
	return nil
}

// Load reads instance state from disk.
func Load(stateDir, instance string) (*Instance, error) {
	if err := ValidateInstanceName(instance); err != nil {
		return nil, err
	}

	path := statePath(stateDir, instance)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var inst Instance
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

// Save writes instance state to disk (does not commit).
func (inst *Instance) Save(stateDir string) error {
	if err := ValidateInstanceName(inst.Name); err != nil {
		return err
	}

	inst.UpdatedAt = time.Now()

	path := statePath(stateDir, inst.Name)
	// 0700: user-only - state may contain sensitive path info
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return err
	}

	// 0600: user-only read/write
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// AddPackages records installed packages.
func (inst *Instance) AddPackages(manager string, packages []string) {
	switch manager {
	case "apt", "apt-get":
		inst.Packages.Apt = appendUnique(inst.Packages.Apt, packages...)
	case "pip", "pip3":
		inst.Packages.Pip = appendUnique(inst.Packages.Pip, packages...)
	case "npm":
		inst.Packages.Npm = appendUnique(inst.Packages.Npm, packages...)
	case "go":
		inst.Packages.Go = appendUnique(inst.Packages.Go, packages...)
	case "cargo":
		inst.Packages.Cargo = appendUnique(inst.Packages.Cargo, packages...)
	case "brew":
		inst.Packages.Brew = appendUnique(inst.Packages.Brew, packages...)
	default:
		inst.Packages.Custom = appendUnique(inst.Packages.Custom, packages...)
	}
}

// AddMount records a mount.
func (inst *Instance) AddMount(name, source, path string, readonly bool) {
	// Remove existing mount with same name
	inst.RemoveMount(name)
	inst.Mounts = append(inst.Mounts, Mount{
		Name:     name,
		Source:   source,
		Path:     path,
		Readonly: readonly,
	})
}

// RemoveMount removes a mount by name.
func (inst *Instance) RemoveMount(name string) {
	var kept []Mount
	for _, mount := range inst.Mounts {
		if mount.Name != name {
			kept = append(kept, mount)
		}
	}
	inst.Mounts = kept
}

// SetCurrentSnapshot updates the current snapshot name.
func (inst *Instance) SetCurrentSnapshot(snapshotName string) {
	inst.CurrentSnapshot = snapshotName
}

func appendUnique(slice []string, items ...string) []string {
	seen := make(map[string]bool)
	for _, s := range slice {
		seen[s] = true
	}
	for _, item := range items {
		if !seen[item] {
			slice = append(slice, item)
			seen[item] = true
		}
	}
	return slice
}
