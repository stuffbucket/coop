// Package names generates and validates container names.
package names

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

var adjectives = []string{
	"cosmic", "quantum", "stellar", "velvet", "crystal",
	"ember", "frost", "thunder", "nimble", "swift",
	"clever", "brave", "keen", "lucid", "vivid",
	"serene", "radiant", "gleaming", "dapper", "plucky",
	"sprightly", "zesty", "snappy", "mellow", "groovy",
	"funky", "jazzy", "breezy", "astral", "lunar",
	"solar", "orbital", "twilight", "pixel", "vector",
}

var nouns = []string{
	"otter", "penguin", "owl", "fox", "badger",
	"raven", "falcon", "dolphin", "octopus", "mantis",
	"gecko", "panda", "koala", "lemur", "quokka",
	"axolotl", "capybara", "pangolin", "narwhal", "tardigrade",
	"comet", "pulsar", "quasar", "nova", "meteor",
	"cascade", "tempest", "zephyr", "vortex", "monsoon",
	"lantern", "compass", "beacon", "catalyst", "cipher",
	"nexus", "vertex", "zenith", "flux", "spark",
}

// Generate returns a random whimsical name like "cosmic-otter"
func Generate() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	return adj + "-" + noun
}

// nameRegex validates DNS-compatible names (RFC 1123 subdomain).
// Must start/end with alphanumeric, can contain hyphens, max 63 chars.
var nameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateContainerName checks that a container name is safe and valid.
// Container names must be DNS-compatible (lowercase alphanumeric + hyphens).
func ValidateContainerName(name string) error {
	if name == "" {
		return fmt.Errorf("container name cannot be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("container name too long (max 63 chars): %q", name)
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("invalid container name %q: must be lowercase alphanumeric with hyphens, start/end with letter or number", name)
	}
	return nil
}

// ValidateInstanceName checks that a VM instance name is safe for use in paths.
// Slightly more permissive than container names (allows uppercase).
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
	if len(name) > 63 {
		return fmt.Errorf("instance name too long (max 63 chars): %q", name)
	}
	return nil
}

// ValidateRemoteName checks that a remote name is safe for use in paths.
func ValidateRemoteName(name string) error {
	if name == "" {
		return fmt.Errorf("remote name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid remote name: %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("remote name contains path separators: %q", name)
	}
	if len(name) > 63 {
		return fmt.Errorf("remote name too long (max 63 chars): %q", name)
	}
	return nil
}

// ValidateSnapshotName checks that a snapshot name is valid.
func ValidateSnapshotName(name string) error {
	if name == "" {
		return fmt.Errorf("snapshot name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid snapshot name: %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("snapshot name contains invalid characters: %q", name)
	}
	if len(name) > 63 {
		return fmt.Errorf("snapshot name too long (max 63 chars): %q", name)
	}
	return nil
}

// ValidateMountName checks that a mount name is valid.
func ValidateMountName(name string) error {
	if name == "" {
		return fmt.Errorf("mount name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid mount name: %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("mount name contains invalid characters: %q", name)
	}
	if len(name) > 63 {
		return fmt.Errorf("mount name too long (max 63 chars): %q", name)
	}
	return nil
}
