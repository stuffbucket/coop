package backend

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stuffbucket/coop/internal/config"
)

func TestBladerunnerBackend_Name(t *testing.T) {
	cfg := &config.Config{}
	b := NewBladerunnerBackend(cfg)
	if b.Name() != "bladerunner" {
		t.Errorf("Name() = %q, want %q", b.Name(), "bladerunner")
	}
}

func TestBladerunnerBackend_StateDir(t *testing.T) {
	cfg := &config.Config{}
	b := NewBladerunnerBackend(cfg)

	t.Run("default", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		dir := b.stateDir()
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".local", "state", "bladerunner")
		if dir != expected {
			t.Errorf("stateDir() = %q, want %q", dir, expected)
		}
	})

	t.Run("xdg_override", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
		dir := b.stateDir()
		expected := "/tmp/xdg-state/bladerunner"
		if dir != expected {
			t.Errorf("stateDir() = %q, want %q", dir, expected)
		}
	})
}

func TestBladerunnerBackend_SocketPath(t *testing.T) {
	cfg := &config.Config{}
	b := NewBladerunnerBackend(cfg)

	t.Setenv("XDG_STATE_HOME", "")
	path := b.socketPath()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "bladerunner", "control.sock")
	if path != expected {
		t.Errorf("socketPath() = %q, want %q", path, expected)
	}
}

func TestBladerunnerBackend_ParseResponse(t *testing.T) {
	cfg := &config.Config{}
	b := NewBladerunnerBackend(cfg)

	tests := []struct {
		name    string
		line    string
		want    string
		wantErr bool
	}{
		{
			name: "success",
			line: "v1 pong",
			want: "pong",
		},
		{
			name: "status_running",
			line: "v1 running",
			want: "running",
		},
		{
			name: "status_stopped",
			line: "v1 stopped",
			want: "stopped",
		},
		{
			name: "ok_response",
			line: "v1 ok",
			want: "ok",
		},
		{
			name:    "error_response",
			line:    "v1 error: unknown command: foo",
			wantErr: true,
		},
		{
			name: "legacy_v0_response",
			line: "pong",
			want: "pong",
		},
		{
			name: "config_value_path",
			line: "v1 /Users/someone/.ssh/id_ed25519",
			want: "/Users/someone/.ssh/id_ed25519",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := b.parseResponse(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Error("parseResponse() should have returned error")
				}
				return
			}
			if err != nil {
				t.Errorf("parseResponse() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("parseResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBladerunnerBackend_GetIncusSocket(t *testing.T) {
	t.Run("explicit_config", func(t *testing.T) {
		cfg := &config.Config{
			Settings: config.Settings{
				IncusSocket: "unix:///custom/path.sock",
			},
		}
		b := NewBladerunnerBackend(cfg)
		got, err := b.GetIncusSocket()
		if err != nil {
			t.Fatalf("GetIncusSocket() error = %v", err)
		}
		if got != "unix:///custom/path.sock" {
			t.Errorf("GetIncusSocket() = %q, want %q", got, "unix:///custom/path.sock")
		}
	})

	t.Run("default_port", func(t *testing.T) {
		cfg := &config.Config{}
		b := NewBladerunnerBackend(cfg)
		got, err := b.GetIncusSocket()
		if err != nil {
			t.Fatalf("GetIncusSocket() error = %v", err)
		}
		if got != "https://127.0.0.1:18443" {
			t.Errorf("GetIncusSocket() = %q, want %q", got, "https://127.0.0.1:18443")
		}
	})
}

func TestBladerunnerBackend_ControlProtocol(t *testing.T) {
	// Use /tmp for shorter socket path (Unix sockets have 108-char limit)
	tmpDir, err := os.MkdirTemp("/tmp", "br-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	socketPath := filepath.Join(tmpDir, "control.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// Mock bladerunner control server
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleMockConn(conn)
		}
	}()

	cfg := &config.Config{}
	b := NewBladerunnerBackend(cfg)

	t.Run("ping", func(t *testing.T) {
		resp, err := sendTestCommand(t, socketPath, "ping")
		if err != nil {
			t.Fatalf("ping error: %v", err)
		}
		got, err := b.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse error: %v", err)
		}
		if got != "pong" {
			t.Errorf("ping = %q, want %q", got, "pong")
		}
	})

	t.Run("status", func(t *testing.T) {
		resp, err := sendTestCommand(t, socketPath, "status")
		if err != nil {
			t.Fatalf("status error: %v", err)
		}
		got, err := b.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse error: %v", err)
		}
		if got != "running" {
			t.Errorf("status = %q, want %q", got, "running")
		}
	})

	t.Run("config_get", func(t *testing.T) {
		resp, err := sendTestCommand(t, socketPath, "config.get cpus")
		if err != nil {
			t.Fatalf("config.get error: %v", err)
		}
		got, err := b.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse error: %v", err)
		}
		if got != "4" {
			t.Errorf("config.get cpus = %q, want %q", got, "4")
		}
	})

	t.Run("unknown_command_returns_error", func(t *testing.T) {
		resp, err := sendTestCommand(t, socketPath, "bogus")
		if err != nil {
			t.Fatalf("send error: %v", err)
		}
		_, err = b.parseResponse(resp)
		if err == nil {
			t.Error("expected error for unknown command")
		}
	})
}

func handleMockConn(c net.Conn) {
	defer func() { _ = c.Close() }()
	scanner := bufio.NewScanner(c)
	if scanner.Scan() {
		cmd := scanner.Text()
		switch cmd {
		case "v1 ping":
			_, _ = fmt.Fprintf(c, "v1 pong\n")
		case "v1 status":
			_, _ = fmt.Fprintf(c, "v1 running\n")
		case "v1 stop":
			_, _ = fmt.Fprintf(c, "v1 ok\n")
		case "v1 config.get local-api-port":
			_, _ = fmt.Fprintf(c, "v1 18443\n")
		case "v1 config.get cpus":
			_, _ = fmt.Fprintf(c, "v1 4\n")
		case "v1 config.get memory-gib":
			_, _ = fmt.Fprintf(c, "v1 8\n")
		case "v1 config.get disk-size-gib":
			_, _ = fmt.Fprintf(c, "v1 60\n")
		case "v1 config.get arch":
			_, _ = fmt.Fprintf(c, "v1 aarch64\n")
		default:
			_, _ = fmt.Fprintf(c, "v1 error: unknown command: %s\n", cmd)
		}
	}
}

func sendTestCommand(t *testing.T, socketPath, command string) (string, error) {
	t.Helper()
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	_, _ = fmt.Fprintf(conn, "v1 %s\n", command)

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return "", fmt.Errorf("no response")
	}
	return scanner.Text(), nil
}
