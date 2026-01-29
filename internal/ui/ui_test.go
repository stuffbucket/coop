package ui

import (
	"strings"
	"testing"
)

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"plain text", "hello", 5},
		{"empty string", "", 0},
		{"with spaces", "hello world", 11},
		{"ANSI red", "\x1b[31mred\x1b[0m", 3},
		{"ANSI bold", "\x1b[1mbold\x1b[0m", 4},
		{"ANSI complex", "\x1b[38;5;212mcolored\x1b[0m", 7},
		{"multiple ANSI", "\x1b[1m\x1b[31mbold red\x1b[0m", 8},
		{"ANSI at start and end", "\x1b[32mgreen\x1b[0m text", 10},
		{"mixed content", "pre \x1b[33myellow\x1b[0m post", 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibleWidth(tt.input)
			if got != tt.expected {
				t.Errorf("visibleWidth(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello world"}, // wider than width
		{"", 3, "   "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := padRight(tt.input, tt.width)
			if got != tt.expected {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.expected)
			}
		})
	}
}

func TestTableBasic(t *testing.T) {
	tbl := NewTable(10, 15, 8)
	tbl.SetHeaders("NAME", "STATUS", "IP")
	tbl.AddRow("test-1", "Running", "10.0.0.1")
	tbl.AddRow("test-2", "Stopped", "10.0.0.2")

	output := tbl.Render()

	// Should contain headers
	if !strings.Contains(output, "NAME") {
		t.Error("Table should contain NAME header")
	}
	if !strings.Contains(output, "STATUS") {
		t.Error("Table should contain STATUS header")
	}

	// Should contain separator
	if !strings.Contains(output, "─") {
		t.Error("Table should contain separator line")
	}

	// Should contain rows
	if !strings.Contains(output, "test-1") {
		t.Error("Table should contain test-1")
	}
	if !strings.Contains(output, "Running") {
		t.Error("Table should contain Running")
	}
	if !strings.Contains(output, "10.0.0.2") {
		t.Error("Table should contain 10.0.0.2")
	}

	// Check line count: headers + separator + 2 data rows = 4 lines (with trailing newlines)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("Expected 4 lines, got %d: %v", len(lines), lines)
	}
}

func TestTableNoHeaders(t *testing.T) {
	tbl := NewTable(10, 10)
	tbl.AddRow("a", "b")
	tbl.AddRow("c", "d")

	output := tbl.Render()

	// Should not have separator without headers
	lineCount := len(strings.Split(strings.TrimSuffix(output, "\n"), "\n"))
	if lineCount != 2 {
		t.Errorf("Table without headers should have 2 lines, got %d", lineCount)
	}
}

func TestTableEmptyRows(t *testing.T) {
	tbl := NewTable(10)
	tbl.SetHeaders("COL")

	output := tbl.Render()

	// Should still have header and separator
	if !strings.Contains(output, "COL") {
		t.Error("Empty table should still have header")
	}
}

func TestTableWithANSICells(t *testing.T) {
	tbl := NewTable(10, 10)
	tbl.SetHeaders("NAME", "STATUS")

	// Simulate styled cells (ANSI codes)
	styledName := "\x1b[1mtest\x1b[0m"      // bold "test" - visible width 4
	styledStatus := "\x1b[32mOK\x1b[0m"     // green "OK" - visible width 2

	tbl.AddRow(styledName, styledStatus)

	output := tbl.Render()

	// The table should render with proper alignment
	// even though the strings contain ANSI codes
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

	// Data row should exist
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 lines (header, sep, data), got %d", len(lines))
	}

	// Data row should contain our styled content
	dataLine := lines[2]
	if !strings.Contains(dataLine, "test") {
		t.Error("Data line should contain 'test'")
	}
	if !strings.Contains(dataLine, "OK") {
		t.Error("Data line should contain 'OK'")
	}
}

func TestLogoStructure(t *testing.T) {
	// When not a TTY, Logo returns empty
	// We can't easily test the colored version without mocking isTTY
	// but we can verify the Logo function doesn't panic
	_ = Logo()

	// Tagline should exist
	tagline := Tagline()
	if !strings.Contains(tagline, "AI Agent Container Manager") && tagline != "" {
		t.Errorf("Tagline should mention 'AI Agent Container Manager', got %q", tagline)
	}
}

func TestHelpFormatters(t *testing.T) {
	section := HelpSection("Commands")
	if section == "" {
		t.Error("HelpSection should return non-empty string")
	}

	cmd := HelpCommand("list", "List containers")
	if !strings.Contains(cmd, "list") {
		t.Error("HelpCommand should contain command name")
	}
	if !strings.Contains(cmd, "List containers") {
		t.Error("HelpCommand should contain description")
	}

	env := HelpEnvVar("COOP_DEBUG", "Enable debugging")
	if !strings.Contains(env, "COOP_DEBUG") {
		t.Error("HelpEnvVar should contain variable name")
	}

	example := HelpExample("coop create mysandbox")
	if !strings.Contains(example, "coop create") {
		t.Error("HelpExample should contain the example")
	}
}

func TestStatusStyling(t *testing.T) {
	// Status should return the string (potentially styled)
	// We can't test colors without TTY but can verify content
	running := Status("Running")
	if !strings.Contains(running, "Running") {
		t.Error("Status(Running) should contain 'Running'")
	}

	stopped := Status("Stopped")
	if !strings.Contains(stopped, "Stopped") {
		t.Error("Status(Stopped) should contain 'Stopped'")
	}

	unknown := Status("Unknown")
	if !strings.Contains(unknown, "Unknown") {
		t.Error("Status(Unknown) should contain 'Unknown'")
	}
}

func TestStyleFunctions(t *testing.T) {
	// These should at minimum return the input string
	funcs := map[string]func(string) string{
		"Bold":        Bold,
		"Name":        Name,
		"Path":        Path,
		"IP":          IP,
		"Header":      Header,
		"ErrorText":   ErrorText,
		"WarningText": WarningText,
		"SuccessText": SuccessText,
		"MutedText":   MutedText,
	}

	testStr := "test-value"
	for name, fn := range funcs {
		result := fn(testStr)
		if !strings.Contains(result, testStr) {
			t.Errorf("%s(%q) should contain the input, got %q", name, testStr, result)
		}
	}
}

func TestBox(t *testing.T) {
	box := Box("Title", "Content here")
	if !strings.Contains(box, "Title") {
		t.Error("Box should contain title")
	}
	if !strings.Contains(box, "Content") {
		t.Error("Box should contain content")
	}

	// Box without title
	noTitle := Box("", "Just content")
	if !strings.Contains(noTitle, "content") {
		t.Error("Box without title should contain content")
	}
}

func TestWarningBox(t *testing.T) {
	box := WarningBox("Warning", "Something bad")
	if !strings.Contains(box, "Warning") {
		t.Error("WarningBox should contain title")
	}
	if !strings.Contains(box, "Something bad") {
		t.Error("WarningBox should contain content")
	}
}

func BenchmarkVisibleWidth(b *testing.B) {
	input := "\x1b[38;5;212m\x1b[1mstylized-container-name\x1b[0m"
	for i := 0; i < b.N; i++ {
		visibleWidth(input)
	}
}

func BenchmarkTableRender(b *testing.B) {
	tbl := NewTable(20, 12, 15, 10)
	tbl.SetHeaders("NAME", "STATUS", "IP", "UPTIME")
	for i := 0; i < 10; i++ {
		tbl.AddRow("container-name", "Running", "10.0.0.1", "2h30m")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tbl.Render()
	}
}
