package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ImageSource records where a published image came from.
type ImageSource struct {
	Instance string `json:"instance"`
	Snapshot string `json:"snapshot"`
}

// ImageRecord tracks a published image's lineage.
type ImageRecord struct {
	Fingerprint string      `json:"fingerprint"`
	Source      ImageSource `json:"source"`
	CreatedAt   time.Time   `json:"created_at"`
}

// Registry tracks published images and their lineage.
type Registry struct {
	Images map[string]ImageRecord `json:"images"` // alias → record
	path   string
	mu     sync.Mutex
}

// LoadRegistry loads or creates the image registry.
func LoadRegistry(stateDir string) (*Registry, error) {
	path := filepath.Join(stateDir, "images.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Registry{
			Images: make(map[string]ImageRecord),
			path:   path,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	reg.path = path
	if reg.Images == nil {
		reg.Images = make(map[string]ImageRecord)
	}
	return &reg, nil
}

// RecordPublish records that a snapshot was published as an image.
func (r *Registry) RecordPublish(alias, fingerprint, instance, snapshot string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Images[alias] = ImageRecord{
		Fingerprint: fingerprint,
		Source: ImageSource{
			Instance: instance,
			Snapshot: snapshot,
		},
		CreatedAt: time.Now(),
	}
	return r.save()
}

// GetSource returns the source of a published image, or nil if not found.
// Returns a copy to prevent concurrent modification.
func (r *Registry) GetSource(alias string) *ImageSource {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rec, ok := r.Images[alias]; ok {
		// Return copy to avoid concurrent access to map data
		copy := rec.Source
		return &copy
	}
	return nil
}

// Remove removes an image record (call when image is deleted).
func (r *Registry) Remove(alias string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.Images, alias)
	return r.save()
}

func (r *Registry) save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, append(data, '\n'), 0600)
}
