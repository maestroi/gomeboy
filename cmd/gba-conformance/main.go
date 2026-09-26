package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/maestroi/gomeboy/internal/gba/conformance"
)

func main() {
	manifestPath := flag.String("manifest", "", "path to a GBA conformance manifest")
	outputPath := flag.String("output", "-", "JSON result path, or - for stdout")
	commit := flag.String("commit", os.Getenv("GITHUB_SHA"), "GomeBoy commit recorded in results")
	flag.Parse()

	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "gba-conformance: -manifest is required")
		os.Exit(2)
	}

	manifest, cases, err := conformance.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance: %v\n", err)
		os.Exit(2)
	}
	buildCommit := *commit
	if buildCommit == "" {
		buildCommit = "unknown"
	}

	runner := conformance.Runner{
		Suite:         manifest.Suite,
		SuiteRevision: manifest.SuiteRevision,
		GomeBoyCommit: buildCommit,
	}
	report := runner.RunAll(cases)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance: encode results: %v\n", err)
		os.Exit(2)
	}
	data = append(data, '\n')

	if *outputPath == "-" {
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "gba-conformance: write stdout: %v\n", err)
			os.Exit(2)
		}
	} else if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance: write results: %v\n", err)
		os.Exit(2)
	}

	if !report.Passed() {
		os.Exit(1)
	}
}
