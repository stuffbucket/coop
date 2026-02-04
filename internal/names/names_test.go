package names

import (
	"regexp"
	"testing"
)

func TestGenerate(t *testing.T) {
	name := Generate()

	// Should match pattern: adjective-noun
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+$`)
	if !pattern.MatchString(name) {
		t.Errorf("Generated name %q doesn't match expected pattern adjective-noun", name)
	}
}

func TestGenerateUniqueness(t *testing.T) {
	// Generate many names and check for reasonable uniqueness
	// With 35 adjectives * 40 nouns = 1400 combinations
	// Generating 100 should rarely have collisions
	names := make(map[string]bool)
	for i := 0; i < 100; i++ {
		name := Generate()
		names[name] = true
	}

	// Allow for some collisions but expect at least 90% unique
	if len(names) < 90 {
		t.Errorf("Expected at least 90 unique names from 100 generations, got %d", len(names))
	}
}

func TestGenerateContainsValidAdjective(t *testing.T) {
	adjSet := make(map[string]bool)
	for _, adj := range adjectives {
		adjSet[adj] = true
	}

	for i := 0; i < 50; i++ {
		name := Generate()
		// Extract adjective (everything before the hyphen)
		for j, c := range name {
			if c == '-' {
				adj := name[:j]
				if !adjSet[adj] {
					t.Errorf("Generated adjective %q not in adjectives list", adj)
				}
				break
			}
		}
	}
}

func TestGenerateContainsValidNoun(t *testing.T) {
	nounSet := make(map[string]bool)
	for _, noun := range nouns {
		nounSet[noun] = true
	}

	for i := 0; i < 50; i++ {
		name := Generate()
		// Extract noun (everything after the hyphen)
		for j, c := range name {
			if c == '-' {
				noun := name[j+1:]
				if !nounSet[noun] {
					t.Errorf("Generated noun %q not in nouns list", noun)
				}
				break
			}
		}
	}
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Generate()
	}
}

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid-name", false},
		{"mycontainer", false},
		{"test123", false},
		{"a", false},
		{"a-b", false},
		{"", true},                       // empty
		{"-invalid", true},               // starts with hyphen
		{"invalid-", true},               // ends with hyphen
		{"UPPERCASE", true},              // uppercase not allowed
		{"has space", true},              // spaces not allowed
		{"has/slash", true},              // slashes not allowed
		{"has\\backslash", true},         // backslashes not allowed
		{"../traversal", true},           // path traversal
		{"..", true},                     // double dot
		{".", true},                      // single dot
		{"with@symbol", true},            // @ not allowed in DNS names
		{string(make([]byte, 64)), true}, // too long
	}

	for _, tc := range tests {
		err := ValidateContainerName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateContainerName(%q) should have failed", tc.name)
		} else if !tc.wantErr && err != nil {
			t.Errorf("ValidateContainerName(%q) failed: %v", tc.name, err)
		}
	}
}

func TestValidateInstanceName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid-name", false},
		{"MyInstance", false}, // uppercase allowed
		{"test_123", false},   // underscores ok
		{"", true},            // empty
		{".", true},           // single dot
		{"..", true},          // double dot
		{"has/slash", true},
		{"has\\backslash", true},
		{string(make([]byte, 64)), true}, // too long
	}

	for _, tc := range tests {
		err := ValidateInstanceName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateInstanceName(%q) should have failed", tc.name)
		} else if !tc.wantErr && err != nil {
			t.Errorf("ValidateInstanceName(%q) failed: %v", tc.name, err)
		}
	}
}

func TestValidateRemoteName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"myremote", false},
		{"remote-server", false},
		{"", true},
		{".", true},
		{"..", true},
		{"../evil", true},
		{"has/slash", true},
	}

	for _, tc := range tests {
		err := ValidateRemoteName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateRemoteName(%q) should have failed", tc.name)
		} else if !tc.wantErr && err != nil {
			t.Errorf("ValidateRemoteName(%q) failed: %v", tc.name, err)
		}
	}
}

func TestValidateSnapshotName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"checkpoint1", false},
		{"my-snapshot", false},
		{"", true},
		{".", true},
		{"..", true},
		{"../evil", true},
	}

	for _, tc := range tests {
		err := ValidateSnapshotName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateSnapshotName(%q) should have failed", tc.name)
		} else if !tc.wantErr && err != nil {
			t.Errorf("ValidateSnapshotName(%q) failed: %v", tc.name, err)
		}
	}
}

func TestValidateMountName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"work", false},
		{"my-mount", false},
		{"", true},
		{".", true},
		{"..", true},
	}

	for _, tc := range tests {
		err := ValidateMountName(tc.name)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateMountName(%q) should have failed", tc.name)
		} else if !tc.wantErr && err != nil {
			t.Errorf("ValidateMountName(%q) failed: %v", tc.name, err)
		}
	}
}
