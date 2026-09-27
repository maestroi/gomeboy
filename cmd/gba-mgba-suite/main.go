package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/gomeboy/internal/gba/mgbasuite"
)

func main() {
	configPath := flag.String("config", "", "path to the pinned mGBA-suite category config")
	romPath := flag.String("rom", "", "path to suite.gba")
	outputPath := flag.String("output", "gba-mgba-suite.json", "JSON output path")
	markdownPath := flag.String("markdown", "gba-mgba-suite.md", "Markdown output path")
	commit := flag.String("commit", os.Getenv("GITHUB_SHA"), "GomeBoy commit recorded in results")
	flag.Parse()

	if *configPath == "" || *romPath == "" {
		fmt.Fprintln(os.Stderr, "gba-mgba-suite: -config and -rom are required")
		os.Exit(2)
	}
	cfg, err := mgbasuite.LoadConfig(*configPath)
	if err != nil {
		fatal(err)
	}
	rom, err := os.ReadFile(*romPath)
	if err != nil {
		fatal(fmt.Errorf("read suite ROM: %w", err))
	}
	report := mgbasuite.Run(cfg, rom, *commit)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
		fatal(err)
	}
	markdown := renderMarkdown(report)
	if err := os.WriteFile(*markdownPath, []byte(markdown), 0o644); err != nil {
		fatal(err)
	}

	fmt.Print(markdown)
	if !report.Passed() {
		os.Exit(1)
	}
}

func renderMarkdown(report mgbasuite.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# GBA mGBA-suite accuracy\n\n")
	fmt.Fprintf(&b, "- Suite: %s\n", report.Suite)
	fmt.Fprintf(&b, "- Revision: %s\n", report.SuiteRevision)
	fmt.Fprintf(&b, "- Source commit: %s\n", report.SourceCommit)
	fmt.Fprintf(&b, "- ROM SHA-256: %s\n", report.ROMSHA256)
	fmt.Fprintf(&b, "- GomeBoy commit: %s\n\n", report.GomeBoyCommit)
	b.WriteString("| Category | Status | Passed | Failed | Total | Pass rate |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: |\n")
	for _, category := range report.Categories {
		rate := "-"
		if category.Total > 0 {
			rate = fmt.Sprintf("%.2f%%", category.PassRate)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %s |\n",
			category.Name, category.Status, category.Passed, category.Failed, category.Total, rate)
	}
	b.WriteString("\n")
	for _, category := range report.Categories {
		if category.Detail != "" {
			fmt.Fprintf(&b, "- **%s:** %s\n", category.Name, category.Detail)
		}
		if len(category.Failures) > 0 {
			limit := len(category.Failures)
			if limit > 10 {
				limit = 10
			}
			fmt.Fprintf(&b, "  - first failures:\n")
			for _, failure := range category.Failures[:limit] {
				fmt.Fprintf(&b, "    - %s\n", strings.ReplaceAll(failure, "|", "/"))
			}
			if len(category.Failures) > limit {
				fmt.Fprintf(&b, "    - ... %d more\n", len(category.Failures)-limit)
			}
		}
	}
	return b.String()
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "gba-mgba-suite: %v\n", err)
	os.Exit(2)
}
