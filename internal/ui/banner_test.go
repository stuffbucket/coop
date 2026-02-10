package ui

import (
	"strings"
	"testing"
)

func TestBannerEmbed(t *testing.T) {
	if bannerText == "" {
		t.Fatal("bannerText is empty — go:embed failed")
	}
}

func TestBannerContainsCoop(t *testing.T) {
	// The banner should contain recognisable fragments of the "coop" word art.
	if !strings.Contains(bannerText, "___") {
		t.Error("bannerText does not look like ASCII art")
	}
}

func TestBannerTTYGuard(t *testing.T) {
	// In test (non-TTY) Banner() returns empty string.
	got := Banner()
	if got != "" {
		t.Errorf("Banner() should return empty in non-TTY, got %d bytes", len(got))
	}
}

func TestRenderBanner(t *testing.T) {
	out := renderBanner()
	if out == "" {
		t.Fatal("renderBanner() returned empty")
	}
	// Should end with a trailing newline.
	if !strings.HasSuffix(out, "\n") {
		t.Error("renderBanner() should end with a newline")
	}
	// Should contain ANSI color codes.
	if !strings.Contains(out, "\x1b[38;2;") {
		t.Error("renderBanner() should contain 24-bit ANSI color codes")
	}
	// Should contain reset sequences.
	if !strings.Contains(out, "\x1b[0m") {
		t.Error("renderBanner() should contain ANSI reset sequences")
	}
}

func TestGradientColor(t *testing.T) {
	tests := []struct {
		name string
		t    float64
	}{
		{"start", 0.0},
		{"middle", 0.5},
		{"end", 1.0},
		{"below", -0.1},
		{"above", 1.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b := gradientColor(tt.t)
			if r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
				t.Errorf("gradientColor(%v) = (%d,%d,%d), out of range", tt.t, r, g, b)
			}
		})
	}
}
