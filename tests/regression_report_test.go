//go:build test

package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	regressionBaselinePath = "regression-baseline.json"
	regressionResultsPath  = "regression-results.json"
)

type regressionBaseline struct {
	SchemaVersion int                `json:"schema_version"`
	Tests         []regressionExpect `json:"tests"`
}

type regressionExpect struct {
	ID         string `json:"id"`
	Suite      string `json:"suite"`
	Collection string `json:"collection"`
	Name       string `json:"name"`
	Model      string `json:"model,omitempty"`
	Expected   string `json:"expected"`
	Reason     string `json:"reason,omitempty"`
}

type regressionResult struct {
	ID         string `json:"id"`
	Suite      string `json:"suite"`
	Collection string `json:"collection"`
	Name       string `json:"name"`
	Model      string `json:"model,omitempty"`
	ROM        string `json:"rom,omitempty"`
	Expected   string `json:"expected"`
	Actual     string `json:"actual"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

type regressionSummary struct {
	Total       int `json:"total"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Pass        int `json:"pass"`
	XFail       int `json:"xfail"`
	Regression  int `json:"regression"`
	XPass       int `json:"xpass"`
	New         int `json:"new"`
	Missing     int `json:"missing"`
	ModelChange int `json:"model_change"`
}

type regressionReport struct {
	SchemaVersion int                `json:"schema_version"`
	Baseline      string             `json:"baseline"`
	Summary       regressionSummary  `json:"summary"`
	Tests         []regressionResult `json:"tests"`
}

type regressionMetadata interface {
	regressionModel() string
	regressionROM() string
}

func (b *basicTest) regressionModel() string {
	return b.model.String()
}

func (b *basicTest) regressionROM() string {
	return filepath.ToSlash(b.romPath)
}

func loadRegressionBaseline(path string) (regressionBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return regressionBaseline{}, err
	}
	var baseline regressionBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return regressionBaseline{}, err
	}
	if baseline.SchemaVersion != 1 {
		return regressionBaseline{}, fmt.Errorf("unsupported regression baseline schema %d", baseline.SchemaVersion)
	}
	if len(baseline.Tests) == 0 {
		return regressionBaseline{}, fmt.Errorf("regression baseline contains no tests")
	}
	return baseline, nil
}

func buildRegressionReport(table *TestTable, baseline regressionBaseline) (regressionReport, error) {
	expected := make(map[string]regressionExpect, len(baseline.Tests))
	for _, test := range baseline.Tests {
		if test.ID == "" || test.Suite == "" || test.Collection == "" || test.Name == "" {
			return regressionReport{}, fmt.Errorf("invalid baseline entry: %+v", test)
		}
		if test.Expected != "pass" && test.Expected != "fail" {
			return regressionReport{}, fmt.Errorf("%s has invalid expected status %q", test.ID, test.Expected)
		}
		if _, exists := expected[test.ID]; exists {
			return regressionReport{}, fmt.Errorf("duplicate baseline test id %q", test.ID)
		}
		expected[test.ID] = test
	}

	report := regressionReport{
		SchemaVersion: 1,
		Baseline:      regressionBaselinePath,
		Tests:         make([]regressionResult, 0, len(baseline.Tests)),
	}
	seen := make(map[string]bool, len(baseline.Tests))

	for _, suite := range table.testSuites {
		for _, collection := range suite.AllCollections() {
			for _, test := range collection.tests {
				id := regressionTestID(suite.name, collection.name, test.Name())
				if seen[id] {
					return regressionReport{}, fmt.Errorf("duplicate runtime test id %q", id)
				}
				seen[id] = true

				actual := "fail"
				if test.Passed() {
					actual = "pass"
				}
				result := regressionResult{
					ID:         id,
					Suite:      suite.name,
					Collection: collection.name,
					Name:       test.Name(),
					Expected:   "untracked",
					Actual:     actual,
					Status:     "new",
					Reason:     "test is not present in the checked-in baseline",
				}
				if meta, ok := test.(regressionMetadata); ok {
					result.Model = meta.regressionModel()
					result.ROM = meta.regressionROM()
				}

				if want, ok := expected[id]; ok {
					result.Expected = want.Expected
					switch {
					case want.Model != "" && result.Model != want.Model:
						result.Status = "model-change"
						result.Reason = fmt.Sprintf("model changed from %s to %s; baseline update required", want.Model, result.Model)
					case want.Expected == "pass" && actual == "pass":
						result.Status = "pass"
						result.Reason = ""
					case want.Expected == "pass" && actual == "fail":
						result.Status = "regression"
						result.Reason = "expected pass, observed failure"
					case want.Expected == "fail" && actual == "fail":
						result.Status = "xfail"
						result.Reason = want.Reason
						if result.Reason == "" {
							result.Reason = "known failure matches baseline"
						}
					case want.Expected == "fail" && actual == "pass":
						result.Status = "xpass"
						result.Reason = "expected failure now passes; baseline update required"
					}
				}
				report.Tests = append(report.Tests, result)
			}
		}
	}

	for id, want := range expected {
		if seen[id] {
			continue
		}
		report.Tests = append(report.Tests, regressionResult{
			ID:         want.ID,
			Suite:      want.Suite,
			Collection: want.Collection,
			Name:       want.Name,
			Model:      want.Model,
			Expected:   want.Expected,
			Actual:     "missing",
			Status:     "missing",
			Reason:     "test exists in the checked-in baseline but not in the current suite",
		})
	}

	sort.Slice(report.Tests, func(i, j int) bool {
		return report.Tests[i].ID < report.Tests[j].ID
	})
	report.Summary = summarizeRegressionResults(report.Tests)
	return report, nil
}

func regressionTestID(suite, collection, name string) string {
	return suite + "/" + collection + "/" + name
}

func summarizeRegressionResults(results []regressionResult) regressionSummary {
	var summary regressionSummary
	for _, result := range results {
		summary.Total++
		switch result.Actual {
		case "pass":
			summary.Passed++
		case "fail":
			summary.Failed++
		}
		switch result.Status {
		case "pass":
			summary.Pass++
		case "xfail":
			summary.XFail++
		case "regression":
			summary.Regression++
		case "xpass":
			summary.XPass++
		case "new":
			summary.New++
		case "missing":
			summary.Missing++
		case "model-change":
			summary.ModelChange++
		}
	}
	return summary
}

func (r regressionReport) unexpected() []regressionResult {
	out := make([]regressionResult, 0)
	for _, result := range r.Tests {
		switch result.Status {
		case "regression", "xpass", "new", "missing", "model-change":
			out = append(out, result)
		}
	}
	return out
}

func writeRegressionJSON(path string, report regressionReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (r regressionReport) markdown() string {
	var b strings.Builder
	b.WriteString("# Automated test results\n\n")
	b.WriteString("This report is generated from the structured per-test result set used by CI and evaluated against `tests/regression-baseline.json`. ")
	b.WriteString("PASS and XFAIL match the checked-in baseline; REGRESSION, XPASS, new, missing, or model-change entries require review.\n\n")
	b.WriteString(r.progressBar())
	b.WriteString("\n\n")
	b.WriteString("| Status | Count |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| PASS | %d |\n", r.Summary.Pass)
	fmt.Fprintf(&b, "| XFAIL | %d |\n", r.Summary.XFail)
	fmt.Fprintf(&b, "| REGRESSION | %d |\n", r.Summary.Regression)
	fmt.Fprintf(&b, "| XPASS | %d |\n", r.Summary.XPass)
	fmt.Fprintf(&b, "| New tests | %d |\n", r.Summary.New)
	fmt.Fprintf(&b, "| Missing tests | %d |\n", r.Summary.Missing)
	fmt.Fprintf(&b, "| Model changes | %d |\n\n", r.Summary.ModelChange)
	b.WriteString(readmeBlurb)
	b.WriteString("\n# Test Results\n")
	b.WriteString(r.testResultsTable())
	b.WriteString("\n\n## Per-test results\n")

	lastSuite, lastCollection := "", ""
	for _, result := range r.Tests {
		if result.Suite != lastSuite {
			fmt.Fprintf(&b, "\n### %s\n", result.Suite)
			lastSuite = result.Suite
			lastCollection = ""
		}
		if result.Collection != lastCollection {
			fmt.Fprintf(&b, "\n#### %s\n", result.Collection)
			b.WriteString("| Test | Expected | Actual | Status | Context |\n| --- | --- | --- | --- | --- |\n")
			lastCollection = result.Collection
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			markdownCell(result.Name),
			result.Expected,
			result.Actual,
			result.Status,
			markdownCell(result.Reason),
		)
	}
	return b.String()
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func (r regressionReport) testResultsTable() string {
	type suiteCounts struct {
		passed int
		failed int
	}
	order := make([]string, 0)
	counts := make(map[string]*suiteCounts)
	for _, result := range r.Tests {
		if result.Actual == "missing" {
			continue
		}
		c, ok := counts[result.Suite]
		if !ok {
			c = &suiteCounts{}
			counts[result.Suite] = c
			order = append(order, result.Suite)
		}
		if result.Actual == "pass" {
			c.passed++
		} else {
			c.failed++
		}
	}
	sort.Strings(order)

	var b strings.Builder
	b.WriteString("| Test Suite | Pass Rate | Tests Passed | Tests Failed | Tests Total |\n| --- | --- | --- | --- | --- |")
	for _, suite := range order {
		c := counts[suite]
		total := c.passed + c.failed
		rate := 0
		if total > 0 {
			rate = c.passed * 100 / total
		}
		fmt.Fprintf(&b, "\n| %s | %d%% | %d | %d | %d |", suite, rate, c.passed, c.failed, total)
	}
	return b.String()
}

func (r regressionReport) progressBar() string {
	total := r.Summary.Passed + r.Summary.Failed
	rate := 0
	if total > 0 {
		rate = r.Summary.Passed * 100 / total
	}
	return fmt.Sprintf(
		"![progress](https://progress-bar.xyz/%d/?scale=100&title=passing%%20%d,%%20failing%%20%d&width=500)",
		rate,
		r.Summary.Passed,
		r.Summary.Failed,
	)
}
