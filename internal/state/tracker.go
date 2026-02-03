package state

import (
	"fmt"
	"os"
	"sync"
)

// Tracker manages state metadata for a single Incus instance.
// It coordinates between state.json (what), git commits (why),
// links.json (snapshot mappings), and Incus snapshots (actual state).
//
// Tracker is safe for concurrent use.
type Tracker struct {
	stateDir string
	instance *Instance
	links    *Links
	repo     *Repo
	mu       sync.Mutex
}

// NewTracker opens or creates a state tracker for an instance.
// The stateDir is typically ~/.local/share/coop/instances/
func NewTracker(stateDir, instanceName, baseImage string) (*Tracker, error) {
	repo, err := OpenRepo(stateDir, instanceName)
	if err != nil {
		return nil, err
	}

	// Load or create links (separate from versioned state)
	links, err := LoadLinks(stateDir, instanceName)
	if err != nil {
		return nil, fmt.Errorf("load links: %w", err)
	}

	// Try to load existing state
	instance, err := Load(stateDir, instanceName)
	if os.IsNotExist(err) {
		// New instance - create initial state
		instance = NewInstance(instanceName, baseImage)
		if err := instance.Save(stateDir); err != nil {
			return nil, fmt.Errorf("save initial state: %w", err)
		}
		if _, err := repo.Commit("initial state"); err != nil {
			return nil, fmt.Errorf("commit initial state: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	return &Tracker{
		stateDir: stateDir,
		instance: instance,
		links:    links,
		repo:     repo,
	}, nil
}

// RecordPackageInstall records packages installed via a package manager.
// Returns the commit hash for linking to an Incus snapshot.
func (t *Tracker) RecordPackageInstall(manager string, packages []string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.instance.AddPackages(manager, packages)
	if err := t.instance.Save(t.stateDir); err != nil {
		return "", err
	}
	return t.repo.Commit(fmt.Sprintf("install %s: %v", manager, packages))
}

// RecordMount records a mount being added.
func (t *Tracker) RecordMount(name, source, path string, readonly bool) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.instance.AddMount(name, source, path, readonly)
	if err := t.instance.Save(t.stateDir); err != nil {
		return "", err
	}
	mode := "rw"
	if readonly {
		mode = "ro"
	}
	return t.repo.Commit(fmt.Sprintf("mount %s: %s -> %s (%s)", name, source, path, mode))
}

// RecordUnmount records a mount being removed.
func (t *Tracker) RecordUnmount(name string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.instance.RemoveMount(name)
	if err := t.instance.Save(t.stateDir); err != nil {
		return "", err
	}
	return t.repo.Commit(fmt.Sprintf("unmount %s", name))
}

// RecordSnapshot records that an Incus snapshot was created.
// Links the snapshot name to the current git commit.
// Returns the commit hash.
func (t *Tracker) RecordSnapshot(snapshotName, note string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	msg := fmt.Sprintf("snapshot: %s", snapshotName)
	if note != "" {
		msg += " - " + note
	}

	// Update current snapshot and commit
	t.instance.SetCurrentSnapshot(snapshotName)
	if err := t.instance.Save(t.stateDir); err != nil {
		return "", err
	}
	hash, err := t.repo.Commit(msg)
	if err != nil {
		return "", err
	}

	// Store link in separate file (not versioned, survives reset)
	t.links.Link(snapshotName, hash)
	if err := t.links.Save(t.stateDir, t.instance.Name); err != nil {
		return "", fmt.Errorf("save links: %w", err)
	}

	return hash, nil
}

// RecordEnv records environment variable changes.
func (t *Tracker) RecordEnv(key, value string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.instance.Env[key] = value
	if err := t.instance.Save(t.stateDir); err != nil {
		return "", err
	}
	return t.repo.Commit(fmt.Sprintf("set env %s", key))
}

// UndoToSnapshot reverts to a named snapshot.
// Returns the commit hash that was reset to.
// The caller should then call `incus restore <instance> <snapshot>`.
func (t *Tracker) UndoToSnapshot(snapshotName string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Look up commit for this snapshot (from links, not state)
	commitHash := t.links.CommitFor(snapshotName)
	if commitHash == "" {
		return "", fmt.Errorf("no commit linked to snapshot %q", snapshotName)
	}

	// Reset to that commit
	if err := t.repo.ResetHard(commitHash); err != nil {
		return "", fmt.Errorf("reset to commit: %w", err)
	}

	// Reload state from the reset state.json
	instance, err := Load(t.stateDir, t.instance.Name)
	if err != nil {
		return "", fmt.Errorf("reload state: %w", err)
	}
	t.instance = instance

	return commitHash, nil
}

// Undo reverts to a previous commit by hash.
// Prefer UndoToSnapshot when possible - it's more robust.
// Returns the Incus snapshot name if one is linked to this commit.
// The caller should then call `incus restore <instance> <snapshot>`.
func (t *Tracker) Undo(commitHash string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Find snapshot name for this commit (reverse lookup in links)
	var snapshotName string
	for name, hash := range t.links.Snapshots {
		if hash == commitHash {
			snapshotName = name
			break
		}
	}

	// Reset to that commit (discards later commits from HEAD)
	if err := t.repo.ResetHard(commitHash); err != nil {
		return "", fmt.Errorf("reset to commit: %w", err)
	}

	// Reload state from the reset state.json
	instance, err := Load(t.stateDir, t.instance.Name)
	if err != nil {
		return "", fmt.Errorf("reload state: %w", err)
	}
	t.instance = instance

	return snapshotName, nil
}

// Instance returns the current instance state.
func (t *Tracker) Instance() *Instance {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.instance
}

// History returns recent state changes with commit info.
func (t *Tracker) History(limit int) ([]CommitInfo, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.repo.History(limit)
}

// Path returns the state directory for this instance.
func (t *Tracker) Path() string {
	return t.repo.Path()
}

// Reload refreshes the instance state from disk.
// Useful after external changes or to sync with Incus state.
func (t *Tracker) Reload() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	instance, err := Load(t.stateDir, t.instance.Name)
	if err != nil {
		return err
	}
	t.instance = instance
	return nil
}
