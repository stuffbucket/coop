// Package ui provides terminal UI components using charmbracelet libraries.
// All functions gracefully handle non-interactive environments.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/gen2brain/beeep"
	"golang.org/x/term"
)

// Theme represents a color scheme for the UI.
type Theme struct {
	Name    string
	Warning string // orange/yellow
	Error   string // red
	Success string // green
	Muted   string // gray
	Bold    string // for names/emphasis
	Path    string // cyan/aqua
	IP      string // yellow
	Code    string // for inline code
	Header  string // blue
}

// Predefined themes
var (
	ThemeDefault = Theme{
		Name:    "default",
		Warning: "214", // orange
		Error:   "196", // red
		Success: "82",  // green
		Muted:   "245", // gray
		Bold:    "212", // magenta
		Path:    "cyan",
		IP:      "220", // yellow
		Code:    "212", // magenta
		Header:  "39",  // blue
	}

	ThemeSolarized = Theme{
		Name:    "solarized",
		Warning: "136", // yellow
		Error:   "160", // red
		Success: "64",  // green
		Muted:   "240", // base01
		Bold:    "125", // magenta
		Path:    "37",  // cyan
		IP:      "136", // yellow
		Code:    "33",  // blue
		Header:  "33",  // blue
	}

	ThemeDracula = Theme{
		Name:    "dracula",
		Warning: "228", // yellow
		Error:   "212", // pink
		Success: "84",  // green
		Muted:   "239", // comment gray
		Bold:    "141", // purple
		Path:    "117", // cyan
		IP:      "228", // yellow
		Code:    "141", // purple
		Header:  "141", // purple
	}

	ThemeGruvbox = Theme{
		Name:    "gruvbox",
		Warning: "214", // orange
		Error:   "167", // red
		Success: "142", // green
		Muted:   "243", // gray
		Bold:    "175", // purple
		Path:    "109", // aqua
		IP:      "214", // orange
		Code:    "175", // purple
		Header:  "109", // aqua
	}

	ThemeNord = Theme{
		Name:    "nord",
		Warning: "221", // nord13 - yellow
		Error:   "203", // nord11 - red
		Success: "150", // nord14 - green
		Muted:   "243", // nord3 - gray
		Bold:    "139", // nord15 - purple
		Path:    "116", // nord8 - cyan
		IP:      "221", // nord13 - yellow
		Code:    "109", // nord9 - blue
		Header:  "109", // nord9 - blue
	}

	currentTheme = ThemeDefault
)

var (
	// Styles for consistent visual language
	// These are initialized with default theme and can be changed via SetTheme()
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
	codeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Inline(true) // magenta, inline-friendly

	// Help styles
	cmdStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true) // magenta
	envStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))            // green
	exampleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))            // light gray

	// Logger configured for terminal output
	Logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})
)

// SetTheme applies a theme by reinitializing all style variables.
func SetTheme(theme Theme) {
	currentTheme = theme

	// Reinitialize all styles with the new theme colors
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warning))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Error))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	boldStyle = lipgloss.NewStyle().Bold(true)
	nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Bold)).Bold(true)
	pathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Path))
	ipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.IP))
	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success))
	statusStopped = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Header))
	codeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Code)).Inline(true)

	// Help styles
	cmdStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Bold)).Bold(true)
	envStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success))
	exampleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
}

// GetTheme returns the currently active theme.
func GetTheme() Theme {
	return currentTheme
}

// ThemeByName returns a theme by name, or default if not found.
func ThemeByName(name string) Theme {
	switch name {
	case "solarized":
		return ThemeSolarized
	case "dracula":
		return ThemeDracula
	case "gruvbox":
		return ThemeGruvbox
	case "nord":
		return ThemeNord
	default:
		return ThemeDefault
	}
}

// ListThemes returns all available theme names.
func ListThemes() []string {
	return []string{"default", "solarized", "dracula", "gruvbox", "nord"}
}

// IsInteractive returns true if stdin is a terminal.
// Use this to gate interactive prompts.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// IsTTY returns true if stdout is a terminal.
// Use this to gate colored output.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// TerminalSize returns the terminal width and height.
// Returns (0, 0, err) if not a terminal or size cannot be determined.
func TerminalSize() (width, height int, err error) {
	return term.GetSize(int(os.Stdout.Fd()))
}

// styled applies a style only if output is a TTY
func styled(style lipgloss.Style, s string) string {
	if !IsTTY() {
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

// Code returns styled inline code/command text (like markdown backticks).
// Uses inline-friendly styling that preserves surrounding text styles.
func Code(s string) string {
	if !IsTTY() {
		return "`" + s + "`"
	}
	return codeStyle.Render("`" + s + "`")
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

// InfoDialog shows an informational dialog with details and confirmation.
// Returns true if user confirms, false otherwise.
type InfoDialog struct {
	Title       string   // Main heading (what's happening)
	Description string   // Brief context
	Details     []string // Bullet points of what will happen
	Options     []string // User options/alternatives
	Recommended int      // Index of recommended option (0-based, -1 for none)
	Question    string   // Confirmation question
	Affirmative string   // Yes button text (default: "Yes")
	Negative    string   // No button text (default: "No")
}

// Show displays the info dialog and returns the user's choice.
func (d InfoDialog) Show() bool {
	if !IsInteractive() {
		return false
	}

	// Build the description content with styling
	var content strings.Builder
	if d.Description != "" {
		content.WriteString(d.Description)
		content.WriteString("\n\n")
	}

	if len(d.Details) > 0 {
		// Styled "What happens" section header
		headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
		content.WriteString(headerStyle.Render("What happens:"))
		content.WriteString("\n")
		// Cyan bullets for details
		bulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")) // bright cyan
		for _, detail := range d.Details {
			content.WriteString("  ")
			content.WriteString(bulletStyle.Render("▸"))
			content.WriteString(" ")
			content.WriteString(detail)
			content.WriteString("\n")
		}
	}

	if len(d.Options) > 0 {
		content.WriteString("\n")
		// Styled "Your options" section header
		headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
		content.WriteString(headerStyle.Render("Your options:"))
		content.WriteString("\n")

		for i, opt := range d.Options {
			content.WriteString("  ")
			// Highlight recommended option in green
			if d.Recommended == i {
				recommendedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true) // green
				content.WriteString(recommendedStyle.Render("★"))
			} else {
				// Regular yellow bullet for other options
				bulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow
				content.WriteString(bulletStyle.Render("▸"))
			}
			content.WriteString(" ")
			content.WriteString(opt)
			content.WriteString("\n")
		}
	}

	// Set defaults
	affirmative := d.Affirmative
	if affirmative == "" {
		affirmative = "Yes"
	}
	negative := d.Negative
	if negative == "" {
		negative = "No"
	}

	question := d.Question
	if question == "" {
		question = d.Title
	}

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(question).
				Description(content.String()).
				Affirmative(affirmative).
				Negative(negative).
				Value(&confirmed),
		),
	)

	err := form.Run()
	if err != nil {
		return false
	}

	return confirmed
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

// Tagline returns the styled tagline with optional version.
func Tagline(version string) string {
	text := "AI Agent Container Manager"
	if version != "" && version != "dev" {
		text = fmt.Sprintf("AI Agent Container Manager  v%s", version)
	}
	if !IsTTY() {
		return "coop - " + text
	}
	return styled(mutedStyle, "  "+text)
}

// Separator returns a horizontal line of the specified width.
func Separator(width int) string {
	if !IsTTY() {
		return strings.Repeat("-", width)
	}
	line := strings.Repeat("─", width)
	return styled(mutedStyle, " "+line)
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

// HelpEntry represents a single help item (command, env var, etc.)
type HelpEntry struct {
	Name string
	Desc string
}

// HelpColumn represents a column in the help layout.
type HelpColumn struct {
	Title   string
	Entries []HelpEntry
}

// HelpLayout renders help content responsively based on terminal width.
type HelpLayout struct {
	columns       []HelpColumn
	minColWidth   int
	terminalWidth int
}

// NewHelpLayout creates a new responsive help layout.
func NewHelpLayout() *HelpLayout {
	width, _, err := TerminalSize()
	if err != nil || width < 40 {
		width = 80 // sensible default
	}
	return &HelpLayout{
		columns:       make([]HelpColumn, 0),
		minColWidth:   50, // need room for cmd (12) + desc (30+)
		terminalWidth: width,
	}
}

// SetWidth overrides the terminal width for layout calculations.
func (h *HelpLayout) SetWidth(w int) {
	if w > 0 {
		h.terminalWidth = w
	}
}

// AddColumn adds a section/column to the layout.
func (h *HelpLayout) AddColumn(title string, entries []HelpEntry) {
	h.columns = append(h.columns, HelpColumn{Title: title, Entries: entries})
}

// Render produces the final layout string.
func (h *HelpLayout) Render() string {
	if len(h.columns) == 0 {
		return ""
	}

	// Determine layout: prefer 2 columns, fall back to 1
	numCols := h.terminalWidth / h.minColWidth
	if numCols < 1 {
		numCols = 1
	}
	if numCols > len(h.columns) {
		numCols = len(h.columns)
	}
	// Cap at 2 columns - 3 gets too cramped for descriptions
	if numCols > 2 {
		numCols = 2
	}

	colWidth := (h.terminalWidth / numCols) - 2 // small gap between columns
	cmdWidth := 12
	descWidth := colWidth - cmdWidth - 4 // -4 for indent
	if descWidth < 15 {
		descWidth = 15
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	cmdNameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	// Build column content - each column is a self-contained block
	var renderedCols []string
	for _, col := range h.columns {
		var lines []string
		lines = append(lines, titleStyle.Render(col.Title))
		lines = append(lines, "")
		for _, e := range col.Entries {
			// Pad command name to fixed width
			name := fmt.Sprintf("%-*s", cmdWidth, e.Name)
			// Truncate description only if absolutely necessary
			desc := e.Desc
			if len(desc) > descWidth {
				desc = desc[:descWidth-1] + "…"
			}
			line := "  " + cmdNameStyle.Render(name) + descStyle.Render(desc)
			lines = append(lines, line)
		}
		renderedCols = append(renderedCols, strings.Join(lines, "\n"))
	}

	// Arrange columns into rows
	var rows []string
	for i := 0; i < len(renderedCols); i += numCols {
		end := i + numCols
		if end > len(renderedCols) {
			end = len(renderedCols)
		}
		rowCols := renderedCols[i:end]

		// Find max lines in this row
		maxLines := 0
		for _, col := range rowCols {
			lines := strings.Count(col, "\n") + 1
			if lines > maxLines {
				maxLines = lines
			}
		}

		// Pad columns to equal height, then join horizontally
		var paddedCols []string
		for _, col := range rowCols {
			lines := strings.Split(col, "\n")
			for len(lines) < maxLines {
				lines = append(lines, strings.Repeat(" ", colWidth))
			}
			// Ensure each line is padded to column width
			for j, line := range lines {
				// Calculate visible width (approximate - ANSI codes make this tricky)
				visLen := len(stripANSI(line))
				if visLen < colWidth {
					lines[j] = line + strings.Repeat(" ", colWidth-visLen)
				}
			}
			paddedCols = append(paddedCols, strings.Join(lines, "\n"))
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top, paddedCols...)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n\n")
}

// stripANSI removes ANSI escape codes for length calculation.
// stripANSI removes ANSI escape codes for length calculation.
func stripANSI(s string) string {
	// Simple regex-free approach: skip escape sequences
	var result strings.Builder
	inEscape := false
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
		result.WriteRune(r)
	}
	return result.String()
}

// MaxLineWidth returns the maximum visual width of any line in the string.
// ANSI escape codes are stripped before measuring.
func MaxLineWidth(s string) int {
	maxWidth := 0
	for _, line := range strings.Split(s, "\n") {
		width := len(stripANSI(line))
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// RenderExamples formats examples in a compact style.
func RenderExamples(examples []string, termWidth int) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("248")).
		PaddingLeft(2)

	var lines []string
	for _, ex := range examples {
		lines = append(lines, style.Render(ex))
	}
	return strings.Join(lines, "\n")
}

// RenderEnvVars formats environment variables responsively.
func RenderEnvVars(vars []HelpEntry, termWidth int) string {
	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("114")).
		Width(22)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	var lines []string
	for _, v := range vars {
		line := "  " + nameStyle.Render(v.Name) + descStyle.Render(v.Desc)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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

// Notify sends a user notification (best effort, cross-platform).
// This bypasses stdout/stderr so forked processes won't capture it.
func Notify(title, subtitle, message string) {
	body := message
	if subtitle != "" {
		body = fmt.Sprintf("%s — %s", subtitle, message)
	}

	// Primary: beeep supports macOS, Windows, Linux (notify-send)
	if err := beeep.Notify(title, body, ""); err == nil {
		return
	}

	// Fallback: macOS osascript (subtitle support)
	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf(`display notification %q with title %q subtitle %q`, message, title, subtitle)
		_ = exec.Command("osascript", "-e", script).Start()
	}
}

// NotifyWithSound sends a notification with optional sound.
func NotifyWithSound(title, subtitle, message, sound string) {
	body := message
	if subtitle != "" {
		body = fmt.Sprintf("%s — %s", subtitle, message)
	}

	// Primary: cross-platform popup
	if err := beeep.Notify(title, body, ""); err == nil {
		// Optionally add sound on macOS
		if runtime.GOOS == "darwin" && sound != "" {
			script := fmt.Sprintf(`display notification %q with title %q subtitle %q sound name %q`, message, title, subtitle, sound)
			_ = exec.Command("osascript", "-e", script).Start()
		}
		return
	}

	// Fallback macOS
	if runtime.GOOS == "darwin" {
		if sound != "" {
			script := fmt.Sprintf(`display notification %q with title %q subtitle %q sound name %q`, message, title, subtitle, sound)
			_ = exec.Command("osascript", "-e", script).Start()
		} else {
			script := fmt.Sprintf(`display notification %q with title %q subtitle %q`, message, title, subtitle)
			_ = exec.Command("osascript", "-e", script).Start()
		}
	}
}

// TTYPrint writes directly to /dev/tty, bypassing stdout/stderr redirection.
// Returns false if no controlling terminal available.
func TTYPrint(format string, args ...interface{}) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	defer func() { _ = tty.Close() }()
	_, _ = fmt.Fprintf(tty, format, args...)
	return true
}

// AuthCodeResult represents the outcome of an auth code prompt.
type AuthCodeResult int

const (
	// AuthCodeSuccess indicates the code was validated.
	AuthCodeSuccess AuthCodeResult = iota
	// AuthCodeExpired indicates the timeout was reached.
	AuthCodeExpired
	// AuthCodeFailed indicates all attempts were exhausted.
	AuthCodeFailed
	// AuthCodeError indicates a terminal error.
	AuthCodeError
)

// AuthCodePromptConfig configures the auth code prompt.
type AuthCodePromptConfig struct {
	Reason    string                          // Why authorization is needed
	Timeout   time.Duration                   // Total time allowed
	Attempts  int                             // Max attempts
	Validator func(code string) (bool, error) // Code validation function
}

// PromptAuthCode displays a countdown timer and prompts for an authorization code.
// Uses direct TTY access to prevent stdout/stderr capture by subprocesses.
func PromptAuthCode(cfg AuthCodePromptConfig) AuthCodeResult {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return AuthCodeError
	}
	defer func() { _ = tty.Close() }()

	startTime := time.Now()
	deadline := startTime.Add(cfg.Timeout)

	// Initial display
	_, _ = fmt.Fprintf(tty, "\n⚠️  Protected path: %s\n", cfg.Reason)
	_, _ = fmt.Fprintf(tty, "A 6-digit authorization code is required.\n\n")

	// Style definitions
	barWidth := 20
	var remaining time.Duration

	// Input channel
	inputCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(tty)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			inputCh <- strings.TrimSpace(line)
		}
	}()

	attempt := 0
	for attempt < cfg.Attempts {
		attempt++

		// Update display with countdown
		ticker := time.NewTicker(100 * time.Millisecond)
		lastBar := ""

		// Move cursor to show input area
		_, _ = fmt.Fprintf(tty, "Enter code (%d/%d attempts): ", attempt, cfg.Attempts)

		inputReceived := false
		for !inputReceived {
			select {
			case input := <-inputCh:
				ticker.Stop()
				inputReceived = true

				if ok, _ := cfg.Validator(input); ok {
					// Clear line and show success
					_, _ = fmt.Fprintf(tty, "\r\033[K")
					_, _ = fmt.Fprintf(tty, "%s Authorized\n\n", styled(successStyle, "✓"))
					return AuthCodeSuccess
				}

				// Invalid code
				_, _ = fmt.Fprintf(tty, "\r\033[K")
				if attempt < cfg.Attempts {
					_, _ = fmt.Fprintf(tty, "%s Invalid code\n", styled(errorStyle, "✗"))
				}

			case <-ticker.C:
				remaining = time.Until(deadline)
				if remaining <= 0 {
					ticker.Stop()
					_, _ = fmt.Fprintf(tty, "\r\033[K")
					_, _ = fmt.Fprintf(tty, "\n%s Authorization expired\n", styled(errorStyle, "✗"))
					return AuthCodeExpired
				}

				// Build progress bar
				fraction := float64(remaining) / float64(cfg.Timeout)
				filled := int(fraction * float64(barWidth))
				if filled < 0 {
					filled = 0
				}
				if filled > barWidth {
					filled = barWidth
				}

				secs := int(remaining.Seconds())
				bar := fmt.Sprintf("[%s%s] %2ds",
					strings.Repeat("█", filled),
					strings.Repeat("░", barWidth-filled),
					secs)

				// Only update if changed
				if bar != lastBar {
					// Save cursor, move to end of line, print bar, restore
					_, _ = fmt.Fprintf(tty, "\033[s\033[999C\033[%dD%s\033[u", len(bar), bar)
					lastBar = bar
				}
			}
		}
	}

	_, _ = fmt.Fprintf(tty, "\n%s Authorization failed after %d attempts\n", styled(errorStyle, "✗"), cfg.Attempts)
	return AuthCodeFailed
}
