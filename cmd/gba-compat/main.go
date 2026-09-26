package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/maestroi/gomeboy/internal/gba/compatibility"
)

func main() {
	manifestPath := flag.String("manifest", "", "path to a GBA compatibility manifest")
	outputPath := flag.String("output", "-", "JSON result path, or - for stdout")
	markdownPath := flag.String("markdown", "", "optional Markdown report path, or - for stdout")
	commit := flag.String("commit", os.Getenv("GITHUB_SHA"), "GomeBoy commit recorded in results")
	flag.Parse()

	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "gba-compat: -manifest is required")
		os.Exit(2)
	}
	if *outputPath == "-" && *markdownPath == "-" {
		fmt.Fprintln(os.Stderr, "gba-compat: JSON and Markdown cannot both use stdout")
		os.Exit(2)
	}

	manifest, cases, err := compatibility.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-compat: %v\n", err)
		os.Exit(2)
	}
	buildCommit := *commit
	if buildCommit == "" {
		buildCommit = "unknown"
	}

	runner := compatibility.Runner{
		Suite:         manifest.Suite,
		SuiteRevision: manifest.SuiteRevision,
		GomeBoyCommit: buildCommit,
	}
	report := runner.RunAll(cases)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-compat: encode results: %v\n", err)
		os.Exit(2)
	}
	data = append(data, '\n')
	if err := writeOutput(*outputPath, data); err != nil {
		fmt.Fprintf(os.Stderr, "gba-compat: write JSON: %v\n", err)
		os.Exit(2)
	}
	if *markdownPath != "" {
		if err := writeOutput(*markdownPath, []byte(report.Markdown())); err != nil {
			fmt.Fprintf(os.Stderr, "gba-compat: write Markdown: %v\n", err)
			os.Exit(2)
		}
	}

	if !report.Passed() {
		os.Exit(1)
	}
}

func writeOutput(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
