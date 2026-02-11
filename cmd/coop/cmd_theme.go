package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) ThemeCmd(args []string) {
	if len(args) == 0 {
		// Show current theme and available themes
		current := ui.GetTheme()
		fmt.Println(ui.Bold("Coop Themes"))
		fmt.Println(strings.Repeat("=", 40))
		fmt.Println()

		fmt.Printf("%s %s\n\n", ui.Header("Current theme:"), ui.Name(current.Name))

		fmt.Println(ui.Header("Available themes:"))
		for _, themeName := range ui.ListThemes() {
			marker := "  "
			if themeName == current.Name {
				marker = ui.SuccessText("▸ ")
			}
			fmt.Printf("%s%s\n", marker, themeName)
		}
		fmt.Println()
		ui.Muted("Interactive picker: coop theme preview")
		ui.Muted("Preview specific theme: coop theme preview <name>")
		ui.Mutedf("Set in config: %s", ui.Code(`{"ui": {"theme": "dracula"}}`))
		ui.Mutedf("Or set via env: %s", ui.Code("COOP_THEME=dracula coop list"))
		fmt.Println()
		return
	}

	subcommand := args[0]
	switch subcommand {
	case "preview":
		if len(args) < 2 {
			a.previewTheme("")
		} else {
			a.previewTheme(args[1])
		}
	case "list":
		for _, name := range ui.ListThemes() {
			fmt.Println(name)
		}
	default:
		ui.Errorf("Unknown theme subcommand: %s", subcommand)
		ui.Muted("Usage: coop theme [preview <name>|list]")
		os.Exit(1)
	}
}

func (a *App) previewTheme(themeName string) {
	if themeName != "" {
		a.showStaticThemePreview(themeName)
		return
	}

	// Interactive theme picker
	originalTheme := ui.GetTheme()
	var selectedTheme string
	var confirmed bool

	themes := []struct {
		name  string
		theme ui.Theme
	}{
		{"default", ui.ThemeDefault},
		{"solarized", ui.ThemeSolarized},
		{"dracula", ui.ThemeDracula},
		{"gruvbox", ui.ThemeGruvbox},
		{"nord", ui.ThemeNord},
	}

	options := make([]huh.Option[string], len(themes))
	for i, t := range themes {
		swatch := buildColorSwatch(t.theme)
		label := fmt.Sprintf("%-12s %s", t.name, swatch)
		options[i] = huh.NewOption(label, t.name)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, ui.Bold("Interactive Theme Picker"))
	ui.Muted("Use ↑↓ arrows to navigate, Enter to preview")
	fmt.Fprintln(os.Stderr)

	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a theme to preview").
				Options(options...).
				Value(&selectedTheme),
		),
	).WithShowHelp(false)

	err := selectForm.Run()
	if err != nil {
		ui.SetTheme(originalTheme)
		fmt.Fprintln(os.Stderr)
		ui.Muted("Theme selection cancelled")
		fmt.Fprintln(os.Stderr)
		return
	}

	selectedThemeObj := ui.ThemeByName(selectedTheme)
	ui.SetTheme(selectedThemeObj)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, strings.Repeat("─", 50))
	showThemePreview(selectedThemeObj)
	fmt.Fprintln(os.Stderr, strings.Repeat("─", 50))
	fmt.Fprintln(os.Stderr)

	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Apply the '%s' theme?", selectedTheme)).
				Description("This will update your settings.json").
				Affirmative("Apply").
				Negative("Cancel").
				Value(&confirmed),
		),
	)

	err = confirmForm.Run()
	if err != nil || !confirmed {
		ui.SetTheme(originalTheme)
		fmt.Fprintln(os.Stderr)
		ui.Muted("Theme not applied - keeping current theme")
		fmt.Fprintln(os.Stderr)
		return
	}

	a.Config.Settings.UI.Theme = selectedTheme
	if err := a.Config.Save(); err != nil {
		ui.Errorf("Failed to save theme: %v", err)
		ui.SetTheme(originalTheme)
		return
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("Theme %s applied!", ui.Name(selectedTheme))
	ui.Mutedf("Saved to %s", ui.Path(a.Config.Dirs.SettingsFile))
	fmt.Fprintln(os.Stderr)
}

func buildColorSwatch(theme ui.Theme) string {
	swatch := make([]string, 0)
	block := "██"
	bg := lipgloss.Color("235")

	successChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Success)).
		Background(bg).
		Render(block)
	warningChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Warning)).
		Background(bg).
		Render(block)
	errorChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Error)).
		Background(bg).
		Render(block)
	boldChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Bold)).
		Background(bg).
		Render(block)
	pathChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Path)).
		Background(bg).
		Render(block)
	headerChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Header)).
		Background(bg).
		Render(block)

	swatch = append(swatch, successChip, warningChip, errorChip, boldChip, pathChip, headerChip)
	return strings.Join(swatch, " ")
}

func showThemePreview(theme ui.Theme) {
	fmt.Fprintln(os.Stderr, ui.Header("Preview:"))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  ", ui.SuccessText("✓ Success message"))
	fmt.Fprintln(os.Stderr, "  ", ui.WarningText("⚠ Warning message"))
	fmt.Fprintln(os.Stderr, "  ", ui.ErrorText("✗ Error message"))
	fmt.Fprintln(os.Stderr, "  ", ui.Name("container-name"), ui.MutedText("- subtle info"))
	fmt.Fprintln(os.Stderr, "  ", ui.Path("/path/to/file"), ui.IP("192.168.1.1"))
	fmt.Fprintln(os.Stderr, "  ", "Run command:", ui.Code("coop vm start"))
}

func (a *App) showStaticThemePreview(themeName string) {
	theme := ui.ThemeByName(themeName)
	ui.SetTheme(theme)

	fmt.Println()
	fmt.Printf("%s %s\n", ui.Header("Theme:"), ui.Name(theme.Name))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	fmt.Println(ui.Header("Colors:"))
	fmt.Printf("  %s  %s\n", ui.Bold("Bold/Names:"), ui.Name("example-container"))
	fmt.Printf("  %s  %s\n", ui.Bold("Success:"), ui.SuccessText("✓ Operation successful"))
	fmt.Printf("  %s  %s\n", ui.Bold("Warning:"), ui.WarningText("⚠ Warning message"))
	fmt.Printf("  %s  %s\n", ui.Bold("Error:"), ui.ErrorText("✗ Error message"))
	fmt.Printf("  %s  %s\n", ui.Bold("Muted:"), ui.MutedText("subtle information"))
	fmt.Printf("  %s  %s\n", ui.Bold("Path:"), ui.Path("/path/to/file"))
	fmt.Printf("  %s  %s\n", ui.Bold("IP:"), ui.IP("192.168.1.100"))
	fmt.Printf("  %s  %s\n", ui.Bold("Code:"), ui.Code("coop vm start"))
	fmt.Println()

	fmt.Println(ui.Header("Sample Table:"))
	table := ui.NewTable(20, 10, 15)
	table.SetHeaders("NAME", "STATUS", "IP")
	table.AddRow(ui.Name("agent-01"), ui.Status("Running"), ui.IP("10.0.0.5"))
	table.AddRow(ui.MutedText("agent-02"), ui.Status("Stopped"), ui.MutedText("-"))
	fmt.Print(table.Render())
	fmt.Println()

	ui.Muted("To use this theme permanently:")
	fmt.Printf("  • Edit %s\n", ui.Path(a.Config.Dirs.SettingsFile))
	fmt.Printf("  • Set: %s\n", ui.Code(fmt.Sprintf(`{"ui": {"theme": "%s"}}`, themeName)))
	fmt.Println()
}
