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
