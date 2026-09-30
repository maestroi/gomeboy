//go:build test

package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// skipKnownFailures is disabled under the "test" build tag so Test_All records
// the real outcome of every test. Expected failures are classified against the
// checked-in per-test baseline instead of being skipped.
var skipKnownFailures = false

func Test_All(t *testing.T) {
	testTable := testAllTable()

	for _, top := range testTable.testSuites {
		suite := top
		t.Run(suite.name, func(t *testing.T) {
			t.Parallel()
			for _, collection := range suite.collections {
				col := collection
				t.Run(col.name, func(t *testing.T) {
					t.Parallel()
					col.Run(t)
				})
			}
		})
	}

	t.Cleanup(func() {
		baseline, err := loadRegressionBaseline(regressionBaselinePath)
		if err != nil {
			panic(err)
		}
		report, err := buildRegressionReport(testTable, baseline)
		if err != nil {
			panic(err)
		}
		if err := writeRegressionJSON(regressionResultsPath, report); err != nil {
			panic(err)
		}
		if err := os.WriteFile("README.md", []byte(report.markdown()), 0o644); err != nil {
			panic(err)
		}

		// Keep the compact summary in the repository root README in sync with
		// the same structured result set used by the regression gate.
		rootREADME, err := os.ReadFile("../README.md")
		if err != nil {
			panic(err)
		}
		rootREADME = findTableRE.ReplaceAll(rootREADME, []byte(report.testResultsTable()))
		rootREADME = progressRE.ReplaceAll(rootREADME, []byte(report.progressBar()))
		if err := os.WriteFile("../README.md", rootREADME, 0o644); err != nil {
			panic(err)
		}
	})
}

func Test_Regressions(t *testing.T) {
	if err := os.Chdir(basePath); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(regressionResultsPath)

	cmd := exec.Command("go", "test", "-tags", "test", "-v", "-run", "^Test_All$")
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() > 1 {
			t.Fatalf("Test_All could not complete: %v\n%s", err, output.String())
		}
	}

	data, err := os.ReadFile(regressionResultsPath)
	if err != nil {
		t.Fatalf("structured regression results were not produced: %v\n%s", err, output.String())
	}
	var report regressionReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid structured regression results: %v", err)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("unexpected regression result schema %d", report.SchemaVersion)
	}

	unexpected := report.unexpected()
	for _, result := range unexpected {
		result := result
		t.Run(result.ID, func(t *testing.T) {
			t.Errorf("%s: expected=%s actual=%s status=%s: %s",
				result.ID,
				result.Expected,
				result.Actual,
				result.Status,
				result.Reason,
			)
		})
	}
	if len(unexpected) > 0 {
		fmt.Println(output.String())
	}
}
