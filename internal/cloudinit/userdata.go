// Package cloudinit generates cloud-init user-data for agent containers.
package cloudinit

import (
	"bytes"
	"embed"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Config holds the configuration for generating cloud-init user-data.
type Config struct {
	Hostname  string
	SSHPubKey string
	AgentPort int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Hostname:  "agent-sandbox",
		AgentPort: 8888,
	}
}

// Generate produces the cloud-init user-data YAML.
func Generate(cfg Config) (string, error) {
	tmplData, err := templateFS.ReadFile("templates/userdata.yaml.tmpl")
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("userdata").Parse(string(tmplData))
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}
