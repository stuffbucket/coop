package state

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Repo manages the git repository for an instance's state metadata.
// Git commits map 1:1 with Incus snapshots.
type Repo struct {
	instance string
	path     string
	repo     *git.Repository
}

// ErrLimitReached is returned by History when the limit is reached.
var errLimitReached = fmt.Errorf("limit reached")

// OpenRepo opens or creates a state repo for an instance.
func OpenRepo(stateDir, instance string) (*Repo, error) {
	if err := ValidateInstanceName(instance); err != nil {
		return nil, err
	}

	path := filepath.Join(stateDir, instance)

	// Try to open existing repo
	repo, err := git.PlainOpen(path)
	if err == git.ErrRepositoryNotExists {
		// Create new repo
		// 0700: user-only access
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, fmt.Errorf("create state dir: %w", err)
		}

		repo, err = git.PlainInit(path, false)
		if err != nil {
			return nil, fmt.Errorf("init git repo: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("open git repo: %w", err)
	}

	return &Repo{
		instance: instance,
		path:     path,
		repo:     repo,
	}, nil
}

// Commit stages and commits state.json with a message.
// Returns the commit hash for linking to Incus snapshots.
func (r *Repo) Commit(message string) (string, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("get worktree: %w", err)
	}

	// Stage state.json
	if _, err := w.Add("state.json"); err != nil {
		return "", fmt.Errorf("stage state.json: %w", err)
	}

	// Check if there are changes to commit
	status, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("get status: %w", err)
	}

	if status.IsClean() {
		// Return current HEAD hash if nothing to commit
		head, err := r.repo.Head()
		if err != nil {
			return "", nil // No commits yet
		}
		return head.Hash().String(), nil
	}

	hash, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "coop",
			Email: "coop@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	return hash.String(), nil
}

// Head returns the current HEAD commit hash.
func (r *Repo) Head() (string, error) {
	ref, err := r.repo.Head()
	if err != nil {
		return "", err
	}
	return ref.Hash().String(), nil
}

// ResetHard resets HEAD to a specific commit, discarding changes after it.
// This is used for undo - the discarded commits remain in reflog for recovery.
func (r *Repo) ResetHard(commitHash string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	hash := plumbing.NewHash(commitHash)

	// Reset working tree and index to the target commit
	err = w.Reset(&git.ResetOptions{
		Commit: hash,
		Mode:   git.HardReset,
	})
	if err != nil {
		return fmt.Errorf("reset: %w", err)
	}

	return nil
}

// History returns recent commits with their messages and hashes.
func (r *Repo) History(limit int) ([]CommitInfo, error) {
	ref, err := r.repo.Head()
	if err != nil {
		// No commits yet
		return nil, nil
	}

	iter, err := r.repo.Log(&git.LogOptions{
		From: ref.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("get log: %w", err)
	}

	var commits []CommitInfo
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if limit > 0 && count >= limit {
			return errLimitReached
		}
		commits = append(commits, CommitInfo{
			Hash:    c.Hash.String(),
			Message: c.Message,
			Time:    c.Author.When,
		})
		count++
		return nil
	})

	if err != nil && err != errLimitReached {
		return nil, err
	}

	return commits, nil
}

// CommitInfo holds information about a git commit.
type CommitInfo struct {
	Hash    string
	Message string
	Time    time.Time
}

// Path returns the repository path.
func (r *Repo) Path() string {
	return r.path
}
