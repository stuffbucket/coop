// Package ui provides terminal UI components using charmbracelet libraries.
// All functions gracefully handle non-interactive environments.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"golang.org/x/term"
)

var (
	// Styles for consistent visual language
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))  // green
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // gray
	boldStyle     = lipgloss.NewStyle().Bold(true)
	nameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true) // pink/magenta
	pathStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("cyan"))
	ipStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow
	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))  // green
	statusStopped = lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // gray
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))

	// Logo color palette - gradient from cyan to magenta
	logoStyle1 = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))  // bright cyan
	logoStyle2 = lipgloss.NewStyle().Foreground(lipgloss.Color("87"))  // light cyan
	logoStyle3 = lipgloss.NewStyle().Foreground(lipgloss.Color("123")) // cyan-blue
	logoStyle4 = lipgloss.NewStyle().Foreground(lipgloss.Color("159")) // light blue
	logoStyle5 = lipgloss.NewStyle().Foreground(lipgloss.Color("183")) // lavender

	// Help styles
	cmdStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true) // magenta
	envStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))            // green
	exampleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))            // light gray

	// Logger configured for terminal output
	Logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})

	// Cache whether we're in a terminal
	isTTY = term.IsTerminal(int(os.Stdout.Fd()))
)

// IsInteractive returns true if stdin is a terminal.
// Use this to gate interactive prompts.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// IsTTY returns true if stdout is a terminal.
// Use this to gate colored output.
func IsTTY() bool {
	return isTTY
}

// styled applies a style only if output is a TTY
func styled(style lipgloss.Style, s string) string {
	if !isTTY {
		return s
	}
	return style.Render(s)
}

// Warn prints a warning message with orange styling.
func Warn(msg string) {
	Logger.Warn(msg)
}

// Warnf prints a formatted warning message.
func Warnf(format string, args ...interface{}) {
	Logger.Warnf(format, args...)
}

// Error prints an error message with red styling.
func Error(msg string) {
	Logger.Error(msg)
}

// Errorf prints a formatted error message.
func Errorf(format string, args ...interface{}) {
	Logger.Errorf(format, args...)
}

// Info prints an info message with blue styling.
func Info(msg string) {
	Logger.Info(msg)
}

// Infof prints a formatted info message.
func Infof(format string, args ...interface{}) {
	Logger.Infof(format, args...)
}

// Success prints a success message with green styling.
func Success(msg string) {
	fmt.Println(styled(successStyle, "✓ "+msg))
}

// Successf prints a formatted success message.
func Successf(format string, args ...interface{}) {
	Success(fmt.Sprintf(format, args...))
}

// Muted prints a muted/subtle message.
func Muted(msg string) {
	fmt.Println(styled(mutedStyle, msg))
}

// Mutedf prints a formatted muted message.
func Mutedf(format string, args ...interface{}) {
	Muted(fmt.Sprintf(format, args...))
}

// Print prints a plain message.
func Print(msg string) {
	fmt.Println(msg)
}

// Printf prints a formatted message.
func Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Bold returns bolded text.
func Bold(s string) string {
	return styled(boldStyle, s)
}

// Name returns a styled container/resource name.
func Name(s string) string {
	return styled(nameStyle, s)
}

// Path returns a styled file path.
func Path(s string) string {
	return styled(pathStyle, s)
}

// IP returns a styled IP address.
func IP(s string) string {
	return styled(ipStyle, s)
}

// Status returns a styled status string.
func Status(s string) string {
	switch s {
	case "Running":
		return styled(statusRunning, s)
	case "Stopped", "STOPPED":
		return styled(statusStopped, s)
	default:
		return styled(mutedStyle, s)
	}
}

// Header returns styled header text.
func Header(s string) string {
	return styled(headerStyle, s)
}

// ErrorText returns styled error text (for inline use, not logging).
func ErrorText(s string) string {
	return styled(errorStyle, s)
}

// WarningText returns styled warning text (for inline use).
func WarningText(s string) string {
	return styled(warningStyle, s)
}

// SuccessText returns styled success text (for inline use).
func SuccessText(s string) string {
	return styled(successStyle, s)
}

// MutedText returns styled muted text (for inline use).
func MutedText(s string) string {
	return styled(mutedStyle, s)
}

// Confirm prompts the user for a yes/no confirmation.
// Returns false if not interactive or user declines.
func Confirm(title, description string) bool {
	if !IsInteractive() {
		return false
	}

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	)

	err := form.Run()
	if err != nil {
		return false
	}

	return confirmed
}

// ConfirmWithDefault prompts the user but returns defaultVal in non-interactive mode.
func ConfirmWithDefault(title, description string, defaultVal bool) bool {
	if !IsInteractive() {
		return defaultVal
	}
	return Confirm(title, description)
}

// Select prompts the user to select from options.
// Returns empty string if not interactive or cancelled.
func Select(title string, options []string) string {
	if !IsInteractive() || len(options) == 0 {
		return ""
	}

	var selected string
	opts := make([]huh.Option[string], len(options))
	for i, opt := range options {
		opts[i] = huh.NewOption(opt, opt)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(opts...).
				Value(&selected),
		),
	)

	err := form.Run()
	if err != nil {
		return ""
	}

	return selected
}

// Box renders text in a styled box.
func Box(title, content string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1)

	if title != "" {
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		return titleStyle.Render(title) + "\n" + style.Render(content)
	}
	return style.Render(content)
}

// WarningBox renders a warning box.
func WarningBox(title, content string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1)

	if title != "" {
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
		return titleStyle.Render("⚠ "+title) + "\n" + style.Render(content)
	}
	return style.Render(content)
}

// Logo returns the coop ASCII art logo with gradient coloring.
func Logo() string {
	if !isTTY {
		return ""
	}

	// ASCII art logo - clean tech style
	lines := []string{
		`                         `,
		`  ██████╗ ██████╗  ██████╗ ██████╗  `,
		` ██╔════╝██╔═══██╗██╔═══██╗██╔══██╗ `,
		` ██║     ██║   ██║██║   ██║██████╔╝ `,
		` ██║     ██║   ██║██║   ██║██╔═══╝  `,
		` ╚██████╗╚██████╔╝╚██████╔╝██║      `,
		`  ╚═════╝ ╚═════╝  ╚═════╝ ╚═╝      `,
		`                                     `,
	}

	styles := []lipgloss.Style{logoStyle1, logoStyle1, logoStyle2, logoStyle3, logoStyle4, logoStyle5, logoStyle5, logoStyle5}

	var result strings.Builder
	for i, line := range lines {
		style := styles[i%len(styles)]
		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}

// Tagline returns the styled tagline.
func Tagline() string {
	if !isTTY {
		return "coop - AI Agent Container Manager"
	}
	return styled(mutedStyle, "  AI Agent Container Manager")
}

// HelpSection returns a styled section header for help text.
func HelpSection(title string) string {
	return styled(headerStyle, title)
}

// HelpCommand formats a command entry for help text.
func HelpCommand(cmd, desc string) string {
	return fmt.Sprintf("  %s  %s", styled(cmdStyle, fmt.Sprintf("%-10s", cmd)), styled(mutedStyle, desc))
}

// HelpEnvVar formats an environment variable for help text.
func HelpEnvVar(name, desc string) string {
	return fmt.Sprintf("  %s  %s", styled(envStyle, fmt.Sprintf("%-20s", name)), styled(mutedStyle, desc))
}

// HelpExample formats an example for help text.
func HelpExample(example string) string {
	return styled(exampleStyle, "  "+example)
}

// Table helps render aligned tables with ANSI color support.
type Table struct {
	Headers []string
	Rows    [][]string
	Widths  []int
}

// NewTable creates a new table with the specified column widths.
func NewTable(widths ...int) *Table {
	return &Table{
		Widths: widths,
	}
}

// SetHeaders sets the table headers.
func (t *Table) SetHeaders(headers ...string) {
	t.Headers = headers
}

// AddRow adds a row to the table.
func (t *Table) AddRow(cells ...string) {
	t.Rows = append(t.Rows, cells)
}

// Render returns the formatted table string.
func (t *Table) Render() string {
	var sb strings.Builder

	// Render headers
	if len(t.Headers) > 0 {
		for i, h := range t.Headers {
			width := 10
			if i < len(t.Widths) {
				width = t.Widths[i]
			}
			sb.WriteString(Header(padRight(h, width)))
			if i < len(t.Headers)-1 {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")

		// Separator line
		totalWidth := 0
		for i, w := range t.Widths {
			totalWidth += w
			if i < len(t.Widths)-1 {
				totalWidth += 2 // spacing
			}
		}
		sb.WriteString(strings.Repeat("─", totalWidth))
		sb.WriteString("\n")
	}

	// Render rows
	for _, row := range t.Rows {
		for i, cell := range row {
			width := 10
			if i < len(t.Widths) {
				width = t.Widths[i]
			}
			// Calculate visible width (strip ANSI codes)
			visibleLen := visibleWidth(cell)
			padding := width - visibleLen
			if padding < 0 {
				padding = 0
			}
			sb.WriteString(cell)
			sb.WriteString(strings.Repeat(" ", padding))
			if i < len(row)-1 {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// padRight pads a string to the specified width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// visibleWidth returns the visible width of a string, ignoring ANSI codes.
func visibleWidth(s string) int {
	// Strip ANSI escape sequences
	inEscape := false
	width := 0
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		width++
	}
	return width
}

// Notify sends a macOS notification. Falls back silently on other platforms.
// This bypasses stdout/stderr so forked processes won't capture it.
func Notify(title, subtitle, message string) {
	if runtime.GOOS != "darwin" {
		return
	}

	script := fmt.Sprintf(`display notification %q with title %q subtitle %q`,
		message, title, subtitle)

	// Fire and forget - don't block on notification delivery
	exec.Command("osascript", "-e", script).Start()
}

// NotifyWithSound sends a macOS notification with a sound.
func NotifyWithSound(title, subtitle, message, sound string) {
	if runtime.GOOS != "darwin" {
		return
	}

	script := fmt.Sprintf(`display notification %q with title %q subtitle %q sound name %q`,
		message, title, subtitle, sound)

	exec.Command("osascript", "-e", script).Start()
}

// TTYPrint writes directly to /dev/tty, bypassing stdout/stderr redirection.
// Returns false if no controlling terminal available.
func TTYPrint(format string, args ...interface{}) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	defer tty.Close()
	fmt.Fprintf(tty, format, args...)
	return true
}
