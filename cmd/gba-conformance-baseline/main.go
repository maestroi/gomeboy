package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/gomeboy/internal/gba/conformance"
)

func main() {
	reportPath := flag.String("report", "", "path to a GBA conformance report")
	baselinePath := flag.String("baseline", "", "path to the checked-in expected baseline")
	flag.Parse()

	if *reportPath == "" || *baselinePath == "" {
		fmt.Fprintln(os.Stderr, "gba-conformance-baseline: -report and -baseline are required")
		os.Exit(2)
	}

	data, err := os.ReadFile(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance-baseline: read report: %v\n", err)
		os.Exit(2)
	}
	var report conformance.Report
	if err := json.Unmarshal(data, &report); err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance-baseline: decode report: %v\n", err)
		os.Exit(2)
	}
	baseline, err := conformance.LoadBaseline(*baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance-baseline: %v\n", err)
		os.Exit(2)
	}
	comparison, err := conformance.CompareBaseline(report, baseline)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gba-conformance-baseline: %v\n", err)
		os.Exit(2)
	}

	for _, result := range comparison.Results {
		context := ""
		if result.Context != "" {
			context = " [" + result.Context + "]"
		}
		fmt.Printf("%s %s%s: %s\n", strings.ToUpper(string(result.Outcome)), result.Test, context, result.Detail)
	}
	if !comparison.Passed() {
		os.Exit(1)
	}
}
