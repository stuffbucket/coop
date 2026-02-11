package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stuffbucket/coop/internal/doctor"
	"github.com/stuffbucket/coop/internal/ui"
)

func (a *App) DoctorCmd(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fix := fs.Bool("fix", false, "Attempt to fix issues automatically")
	_ = fs.Parse(args)

	fmt.Println()
	ui.Print(ui.Bold("Coop Doctor"))
	fmt.Println()

	report := doctor.Run(a.Config)

	maxNameLen := 0
	for _, r := range report.Results {
		if len(r.Name) > maxNameLen {
			maxNameLen = len(r.Name)
		}
	}

	for _, r := range report.Results {
		var icon, color string
		switch r.Status {
		case doctor.StatusPass:
			icon = "✓"
			color = "green"
		case doctor.StatusWarn:
			icon = "!"
			color = "yellow"
		case doctor.StatusFail:
			icon = "✗"
			color = "red"
		case doctor.StatusSkip:
			icon = "-"
			color = "gray"
		}

		name := r.Name
		for len(name) < maxNameLen {
			name += " "
		}

		switch color {
		case "green":
			fmt.Printf("  %s  %s  %s\n", ui.SuccessText(icon), name, ui.MutedText(r.Message))
		case "yellow":
			fmt.Printf("  %s  %s  %s\n", ui.WarningText(icon), name, r.Message)
		case "red":
			fmt.Printf("  %s  %s  %s\n", ui.ErrorText(icon), name, ui.ErrorText(r.Message))
		default:
			fmt.Printf("  %s  %s  %s\n", ui.MutedText(icon), ui.MutedText(name), ui.MutedText(r.Message))
		}

		if r.Status == doctor.StatusFail && r.Fix != "" {
			if *fix {
				fmt.Printf("      %s %s\n", ui.MutedText("fixing:"), r.Fix)
			} else {
				fmt.Printf("      %s %s\n", ui.MutedText("fix:"), ui.WarningText(r.Fix))
			}
		}
	}

	pass, warn, fail := report.Summary()
	fmt.Println()
	if fail == 0 && warn == 0 {
		ui.Success("All checks passed!")
	} else if fail == 0 {
		ui.Printf("%d passed, %d warnings\n", pass, warn)
	} else {
		ui.Printf("%d passed, %d warnings, %s\n", pass, warn, ui.ErrorText(fmt.Sprintf("%d failed", fail)))
		fmt.Println()
		ui.Muted("Run suggested fix commands to resolve issues.")
		ui.Muted("For macOS dependencies: brew bundle")
	}
	fmt.Println()

	if report.HasFailures() {
		os.Exit(1)
	}
}
