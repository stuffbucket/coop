// Package names generates whimsical container names.
package names

import (
	"math/rand"
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
