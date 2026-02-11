package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpSectionProvider generates content for a help section.
// Providers can check system state to provide contextual help.
type HelpSectionProvider interface {
	// ShouldRender returns true if this section should be included
	ShouldRender() bool
	// Render returns the section content
	Render(width int) string
	// Priority determines ordering (lower = earlier)
	Priority() int
}

// HelpState represents the current state of coop for contextual help.
type HelpState struct {
	Initialized   bool   // coop init has been run
	VMRunning     bool   // VM is running (macOS)
	BaseImageOK   bool   // base image exists
	HasContainers bool   // at least one container exists
	IsMacOS       bool   // running on macOS
	AgentCount    int    // total number of agents
	RunningCount  int    // number of running agents
	StorageAvail  uint64 // available storage bytes
	StorageTotal  uint64 // total storage bytes
	BackendName   string // active backend (lima, colima, bladerunner)
}

// IsFresh returns true if coop appears to be in initial state.
func (s HelpState) IsFresh() bool {
	return !s.Initialized || (s.IsMacOS && !s.VMRunning) || !s.BaseImageOK
}

// IsReady returns true if coop is ready for normal use.
func (s HelpState) IsReady() bool {
	if s.IsMacOS {
		return s.Initialized && s.VMRunning && s.BaseImageOK
	}
	return s.Initialized && s.BaseImageOK
}

// HelpBuilder constructs help output from providers.
type HelpBuilder struct {
	providers []HelpSectionProvider
	state     HelpState
	width     int
}

// NewHelpBuilder creates a new help builder with state detection.
func NewHelpBuilder(state HelpState) *HelpBuilder {
	width, _, err := TerminalSize()
	if err != nil || width < 40 {
		width = 80
	}
	return &HelpBuilder{
		providers: make([]HelpSectionProvider, 0),
		state:     state,
		width:     width,
	}
}

// Add registers a section provider.
func (b *HelpBuilder) Add(p HelpSectionProvider) {
	b.providers = append(b.providers, p)
}

// Render produces the complete help output.
func (b *HelpBuilder) Render() string {
	// Sort by priority (simple insertion sort is fine for small lists)
	for i := 1; i < len(b.providers); i++ {
		for j := i; j > 0 && b.providers[j].Priority() < b.providers[j-1].Priority(); j-- {
			b.providers[j], b.providers[j-1] = b.providers[j-1], b.providers[j]
		}
	}

	var sections []string
	for _, p := range b.providers {
		if !p.ShouldRender() {
			continue
		}
		content := p.Render(b.width)
		if content == "" {
			continue
		}
		sections = append(sections, content)
	}

	return strings.Join(sections, "\n\n")
}

// MaxContentWidth returns the maximum width for content (accommodates 2 command columns + dashboard).
const MaxContentWidth = 140

// RenderWithDashboard produces help output with dashboard at bottom right.
// Footer content (HintProvider) is rendered full-width below the dashboard.
func (b *HelpBuilder) RenderWithDashboard(dashboard HelpSectionProvider) string {
	// Sort by priority
	for i := 1; i < len(b.providers); i++ {
		for j := i; j > 0 && b.providers[j].Priority() < b.providers[j-1].Priority(); j-- {
			b.providers[j], b.providers[j-1] = b.providers[j-1], b.providers[j]
		}
	}

	// Apply max-width constraint
	effectiveWidth := b.width
	if effectiveWidth > MaxContentWidth {
		effectiveWidth = MaxContentWidth
	}

	// Dashboard dimensions
	dashMaxWidth := 26

	// Calculate main content width (leave room for dashboard)
	mainWidth := effectiveWidth
	hasDashboard := dashboard != nil && dashboard.ShouldRender() && effectiveWidth >= 100
	if hasDashboard {
		mainWidth = effectiveWidth - dashMaxWidth - 4
	}

	// Separate footer (HintProvider) from other content
	var sections []string
	var footerContent string
	for _, p := range b.providers {
		if !p.ShouldRender() {
			continue
		}
		// HintProvider renders as footer, separate from main content
		if _, isHint := p.(*HintProvider); isHint {
			footerContent = p.Render(effectiveWidth)
			continue
		}
		content := p.Render(mainWidth)
		if content == "" {
			continue
		}
		sections = append(sections, content)
	}
	mainContent := strings.Join(sections, "\n\n")

	// No dashboard - just return main content + footer
	if !hasDashboard {
		contentStyle := lipgloss.NewStyle().PaddingLeft(1)
		footerStyle := lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingTop(1).
			PaddingBottom(1)
		result := contentStyle.Render(mainContent)
		if dashboard != nil && dashboard.ShouldRender() {
			// Narrow terminal - put dashboard below
			dashContent := dashboard.Render(dashMaxWidth)
			dashStyle := lipgloss.NewStyle().
				PaddingLeft(1).
				PaddingRight(1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240"))
			result = result + "\n\n" + contentStyle.Render(dashStyle.Render(dashContent))
		}
		if footerContent != "" {
			result = result + "\n\n" + footerStyle.Render(footerContent)
		}
		return result
	}

	// Render dashboard
	dashContent := dashboard.Render(dashMaxWidth)
	dashStyle := lipgloss.NewStyle().
		Width(dashMaxWidth).
		MarginLeft(2).
		PaddingLeft(1).
		PaddingRight(0).
		PaddingBottom(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
	styledDash := dashStyle.Render(dashContent)

	// Join horizontally with bottom alignment so dashboard grows upward
	contentWithDash := lipgloss.JoinHorizontal(lipgloss.Bottom, mainContent, styledDash)

	// Add 1ch left padding to content
	contentStyle := lipgloss.NewStyle().PaddingLeft(1)
	footerStyle := lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingTop(1).
		PaddingBottom(1)
	paddedContent := contentStyle.Render(contentWithDash)

	// Add footer below (full width, with 1ch left padding and 1ch top/bottom padding)
	if footerContent != "" {
		paddedFooter := footerStyle.Render(footerContent)
		return paddedContent + "\n\n" + paddedFooter
	}
	return paddedContent
}

// EffectiveWidth returns the width constrained by MaxContentWidth.
func (b *HelpBuilder) EffectiveWidth() int {
	if b.width > MaxContentWidth {
		return MaxContentWidth
	}
	return b.width
}

// CompactLayoutWidth returns the width actually used for compact (non-columnar) layouts.
// This is narrower since compact content doesn't need the full max-width.
func (b *HelpBuilder) CompactLayoutWidth() int {
	// Dashboard dimensions: 26 content + 2 marginLeft + 2 padding + 2 border = 32
	dashTotalWidth := 32
	// Main content for compact view (longest line ~43 chars)
	compactContentWidth := 43
	// Add 1 for left padding
	return compactContentWidth + dashTotalWidth + 1
}

// State returns the current help state.
func (b *HelpBuilder) State() HelpState {
	return b.state
}

// Width returns the terminal width.
func (b *HelpBuilder) Width() int {
	return b.width
}

// --- Built-in Providers ---

// CommandColumnsProvider renders commands in responsive columns.
type CommandColumnsProvider struct {
	columns []HelpColumn
}

func NewCommandColumnsProvider() *CommandColumnsProvider {
	return &CommandColumnsProvider{
		columns: []HelpColumn{
			{Title: "Commands", Entries: []HelpEntry{
				{"create", "Create agent"},
				{"list", "List agents"},
				{"delete", "Delete agent"},
			}},
			{Title: "Lifecycle", Entries: []HelpEntry{
				{"init", "Initialize coop"},
				{"start", "Start agent"},
				{"stop", "Stop agent"},
				{"lock", "Freeze processes"},
				{"unlock", "Resume processes"},
				{"status", "Show details"},
				{"logs", "View logs"},
			}},
			{Title: "Access", Entries: []HelpEntry{
				{"shell", "Interactive shell"},
				{"ssh", "Print SSH command"},
				{"exec", "Run command"},
				{"mount", "Manage mounts"},
			}},
			{Title: "State & Images", Entries: []HelpEntry{
				{"snapshot", "Manage snapshots"},
				{"state", "View history"},
				{"image", "Manage images"},
			}},
			{Title: "Infrastructure", Entries: []HelpEntry{
				{"doctor", "Check setup health"},
				{"vm", "VM backend (macOS)"},
				{"config", "Show config"},
				{"env", "Show environment"},
				{"version", "Show version"},
			}},
		},
	}
}

func (p *CommandColumnsProvider) ShouldRender() bool { return true }
func (p *CommandColumnsProvider) Priority() int      { return 10 }

func (p *CommandColumnsProvider) Render(width int) string {
	layout := NewHelpLayout()
	layout.SetWidth(width)
	for _, col := range p.columns {
		layout.AddColumn(col.Title, col.Entries)
	}
	return layout.Render()
}

// GettingStartedProvider shows setup steps for fresh installs.
type GettingStartedProvider struct {
	state HelpState
}

func NewGettingStartedProvider(state HelpState) *GettingStartedProvider {
	return &GettingStartedProvider{state: state}
}

func (p *GettingStartedProvider) ShouldRender() bool { return !p.state.IsReady() }
func (p *GettingStartedProvider) Priority() int      { return 20 }

func (p *GettingStartedProvider) Render(width int) string {

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	stepNumStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212")).
		Bold(true)

	stepDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	cmdStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("114"))

	doneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Strikethrough(true)

	checkStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))

	var lines []string
	lines = append(lines, titleStyle.Render("Getting Started:"))
	lines = append(lines, "")

	stepNum := 1

	// Step 1: Initialize
	if !p.state.Initialized {
		lines = append(lines, fmt.Sprintf("  %s %s",
			stepNumStyle.Render(fmt.Sprintf("%d.", stepNum)),
			stepDescStyle.Render("Initialize coop directories:")))
		lines = append(lines, fmt.Sprintf("     %s", cmdStyle.Render("coop init")))
		stepNum++
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s",
			checkStyle.Render("✓"),
			doneStyle.Render("Directories initialized")))
	}

	// Step 2: VM (macOS only)
	if p.state.IsMacOS {
		if !p.state.VMRunning {
			lines = append(lines, fmt.Sprintf("  %s %s",
				stepNumStyle.Render(fmt.Sprintf("%d.", stepNum)),
				stepDescStyle.Render("Start the VM (first run takes a few minutes):")))
			lines = append(lines, fmt.Sprintf("     %s", cmdStyle.Render("coop vm start")))
			stepNum++
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s",
				checkStyle.Render("✓"),
				doneStyle.Render("VM running")))
		}
	}

	// Step 3: Base image
	if !p.state.BaseImageOK {
		lines = append(lines, fmt.Sprintf("  %s %s",
			stepNumStyle.Render(fmt.Sprintf("%d.", stepNum)),
			stepDescStyle.Render("Build the base image (~10 min, one-time):")))
		lines = append(lines, fmt.Sprintf("     %s", cmdStyle.Render("coop image build")))
		lines = append(lines, "")
		lines = append(lines, stepDescStyle.Render("     Or skip this step - coop will use a slower fallback."))
		stepNum++
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s",
			checkStyle.Render("✓"),
			doneStyle.Render("Base image ready")))
	}

	// Final step: Create container
	lines = append(lines, fmt.Sprintf("  %s %s",
		stepNumStyle.Render(fmt.Sprintf("%d.", stepNum)),
		stepDescStyle.Render("Create your first container:")))
	lines = append(lines, fmt.Sprintf("     %s", cmdStyle.Render("coop create myagent")))
	lines = append(lines, fmt.Sprintf("     %s", cmdStyle.Render("coop shell myagent")))

	return strings.Join(lines, "\n")
}

// ExamplesProvider shows usage examples for ready systems.
type ExamplesProvider struct {
	state    HelpState
	examples []string
}

func NewExamplesProvider(state HelpState) *ExamplesProvider {
	return &ExamplesProvider{
		state: state,
		examples: []string{
			"coop create myagent --cpus 4 --memory 8192",
			"coop mount add myagent ~/projects --readonly",
			"coop snapshot create myagent checkpoint1 --note 'before upgrade'",
			"coop image publish myagent checkpoint1 my-custom-image",
			"coop state history myagent",
		},
	}
}

func (p *ExamplesProvider) ShouldRender() bool { return p.state.IsReady() }
func (p *ExamplesProvider) Priority() int      { return 20 }

func (p *ExamplesProvider) Render(width int) string {

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	var lines []string
	lines = append(lines, titleStyle.Render("Examples:"))
	lines = append(lines, RenderExamples(p.examples, width))

	return strings.Join(lines, "\n")
}

// TerminalInfoProvider shows terminal dimensions (for debugging).
type TerminalInfoProvider struct {
	showAlways bool
}

func NewTerminalInfoProvider(showAlways bool) *TerminalInfoProvider {
	return &TerminalInfoProvider{showAlways: showAlways}
}

func (p *TerminalInfoProvider) ShouldRender() bool { return p.showAlways }
func (p *TerminalInfoProvider) Priority() int      { return 100 }

func (p *TerminalInfoProvider) Render(width int) string {
	w, h, err := TerminalSize()
	if err == nil {
		return MutedText(fmt.Sprintf("Terminal: %dx%d", w, h))
	}
	return MutedText("Terminal: not a tty")
}

// QuickHelpProvider shows minimal help for compact view.
type QuickHelpProvider struct{}

func NewQuickHelpProvider() *QuickHelpProvider {
	return &QuickHelpProvider{}
}

func (p *QuickHelpProvider) ShouldRender() bool { return true }
func (p *QuickHelpProvider) Priority() int      { return 10 }

func (p *QuickHelpProvider) Render(width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	var lines []string
	lines = append(lines, titleStyle.Render("Commands:"))
	lines = append(lines, HelpCommand("create", "Create a new agent container"))
	lines = append(lines, HelpCommand("list", "List all agent containers"))
	lines = append(lines, HelpCommand("shell", "Open shell in agent container"))
	lines = append(lines, HelpCommand("stop", "Stop an agent container"))
	lines = append(lines, HelpCommand("delete", "Delete an agent container"))
	lines = append(lines, HelpCommand("snapshot", "Create/restore snapshots"))

	return strings.Join(lines, "\n")
}

// QuickStartProvider shows quick start for compact view.
type QuickStartProvider struct {
	state HelpState
}

func NewQuickStartProvider(state HelpState) *QuickStartProvider {
	return &QuickStartProvider{state: state}
}

func (p *QuickStartProvider) ShouldRender() bool { return true }
func (p *QuickStartProvider) Priority() int      { return 20 }

func (p *QuickStartProvider) Render(width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	var lines []string
	lines = append(lines, titleStyle.Render("Quick Start:"))

	if p.state.IsFresh() {
		// Fresh install - show setup steps
		if !p.state.Initialized {
			lines = append(lines, HelpExample("coop init"))
		}
		if p.state.IsMacOS && !p.state.VMRunning {
			lines = append(lines, HelpExample("coop vm start"))
		}
	}

	lines = append(lines, HelpExample("coop create myagent"))
	lines = append(lines, HelpExample("coop shell myagent"))
	lines = append(lines, HelpExample("coop snapshot create myagent checkpoint1"))

	return strings.Join(lines, "\n")
}

// HintProvider shows contextual hints.
type HintProvider struct {
	showAll bool
}

func NewHintProvider(showAll bool) *HintProvider {
	return &HintProvider{showAll: showAll}
}

func (p *HintProvider) ShouldRender() bool { return !p.showAll }
func (p *HintProvider) Priority() int      { return 90 }

func (p *HintProvider) Render(width int) string {
	return MutedText("Run 'coop help --all' for complete command reference")
}

// DashboardProvider shows mini system status.
type DashboardProvider struct {
	state         HelpState
	version       string
	latestVersion string
}

func NewDashboardProvider(state HelpState, version, latestVersion string) *DashboardProvider {
	return &DashboardProvider{state: state, version: version, latestVersion: latestVersion}
}

func (p *DashboardProvider) ShouldRender() bool { return true }
func (p *DashboardProvider) Priority() int      { return 5 } // Before commands

func (p *DashboardProvider) Render(width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	okStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))

	warnStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	upgradeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213"))

	var lines []string
	lines = append(lines, titleStyle.Render("Status"))
	lines = append(lines, "")

	// Version
	versionStr := p.version
	if versionStr == "" || versionStr == "dev" {
		versionStr = "dev"
	}
	lines = append(lines, fmt.Sprintf("  %s %s",
		dimStyle.Render("version"),
		versionStr))

	// Upgrade available
	if p.latestVersion != "" && p.latestVersion != p.version && p.version != "dev" {
		lines = append(lines, fmt.Sprintf("  %s",
			upgradeStyle.Render("↑ "+p.latestVersion+" available")))
	}

	lines = append(lines, "")

	// Agents count
	if p.state.AgentCount > 0 {
		agentText := fmt.Sprintf("%d/%d agents", p.state.RunningCount, p.state.AgentCount)
		if p.state.RunningCount > 0 {
			lines = append(lines, fmt.Sprintf("  %s %s",
				okStyle.Render("●"),
				agentText))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s",
				dimStyle.Render("○"),
				dimStyle.Render(agentText)))
		}
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s",
			dimStyle.Render("○"),
			dimStyle.Render("No agents")))
	}

	// VM (macOS)
	if p.state.IsMacOS {
		providerLabel := "VM"
		if p.state.BackendName != "" {
			providerLabel = p.state.BackendName
		}
		if p.state.VMRunning {
			lines = append(lines, fmt.Sprintf("  %s %s",
				okStyle.Render("●"),
				fmt.Sprintf("%s running", providerLabel)))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s",
				warnStyle.Render("○"),
				dimStyle.Render(fmt.Sprintf("%s stopped", providerLabel))))
		}
	}

	// Base image
	if p.state.BaseImageOK {
		lines = append(lines, fmt.Sprintf("  %s %s",
			okStyle.Render("●"),
			"Base image"))
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s",
			warnStyle.Render("○"),
			dimStyle.Render("No base image")))
	}

	// Storage
	if p.state.StorageTotal > 0 {
		availGB := float64(p.state.StorageAvail) / (1024 * 1024 * 1024)
		pctFree := 100.0 * float64(p.state.StorageAvail) / float64(p.state.StorageTotal)
		storageText := fmt.Sprintf("%.0f%% Free (%.0fG)", pctFree, availGB)

		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

		if pctFree < 5 {
			lines = append(lines, fmt.Sprintf("  %s %s",
				errStyle.Render("●"),
				errStyle.Render(storageText)))
		} else if pctFree < 10 {
			lines = append(lines, fmt.Sprintf("  %s %s",
				warnStyle.Render("●"),
				storageText))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s",
				okStyle.Render("●"),
				storageText))
		}
	}

	return strings.Join(lines, "\n")
}
