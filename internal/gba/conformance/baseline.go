package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Baseline is the checked-in expectation for a pinned conformance suite.
type Baseline struct {
	SchemaVersion int                   `json:"schema_version"`
	Suite         string                `json:"suite"`
	SuiteRevision string                `json:"suite_revision"`
	Tests         []BaselineExpectation `json:"tests"`
}

// BaselineExpectation describes the exact currently-known outcome for one ROM.
type BaselineExpectation struct {
	Test           string           `json:"test"`
	Status         Status           `json:"status"`
	DetailContains string           `json:"detail_contains,omitempty"`
	Failure        *BaselineFailure `json:"failure,omitempty"`
	Checks         *BaselineChecks  `json:"checks,omitempty"`
	Context        string           `json:"context,omitempty"`
}

// BaselineFailure pins structured first-failure context when the suite exposes it.
type BaselineFailure struct {
	TestID uint32 `json:"test_id"`
	Group  string `json:"group,omitempty"`
}

// BaselineChecks pins the independently-counted progress reached before failure.
type BaselineChecks struct {
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

// BaselineOutcome classifies one comparison against the checked-in expectation.
type BaselineOutcome string

const (
	BaselinePass       BaselineOutcome = "pass"
	BaselineXFail      BaselineOutcome = "xfail"
	BaselineXPass      BaselineOutcome = "xpass"
	BaselineRegression BaselineOutcome = "regression"
	BaselineChanged    BaselineOutcome = "changed"
)

// BaselineResult is one human- and machine-friendly baseline comparison.
type BaselineResult struct {
	Test    string
	Outcome BaselineOutcome
	Context string
	Detail  string
}

// BaselineComparison contains all per-test comparisons in baseline order.
type BaselineComparison struct {
	Results []BaselineResult
}

// Passed reports whether the current results exactly match the accepted
// baseline. XPASS intentionally fails so improvements are reviewed rather than
// silently changing the published baseline.
func (c BaselineComparison) Passed() bool {
	if len(c.Results) == 0 {
		return false
	}
	for _, result := range c.Results {
		if result.Outcome != BaselinePass && result.Outcome != BaselineXFail {
			return false
		}
	}
	return true
}

// LoadBaseline reads a checked-in baseline JSON document.
func LoadBaseline(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, err
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, fmt.Errorf("decode baseline: %w", err)
	}
	if baseline.SchemaVersion != 1 {
		return Baseline{}, fmt.Errorf("unsupported conformance baseline schema %d", baseline.SchemaVersion)
	}
	if baseline.Suite == "" || baseline.SuiteRevision == "" || len(baseline.Tests) == 0 {
		return Baseline{}, fmt.Errorf("baseline suite, suite_revision, and tests are required")
	}
	return baseline, nil
}

// CompareBaseline compares an actual report with the exact known outcome for
// the same pinned suite revision.
func CompareBaseline(report Report, baseline Baseline) (BaselineComparison, error) {
	if report.Suite != baseline.Suite {
		return BaselineComparison{}, fmt.Errorf("baseline suite %q does not match report suite %q", baseline.Suite, report.Suite)
	}
	if report.SuiteRevision != baseline.SuiteRevision {
		return BaselineComparison{}, fmt.Errorf("baseline revision %q does not match report revision %q", baseline.SuiteRevision, report.SuiteRevision)
	}

	actualByTest := make(map[string]Result, len(report.Results))
	for _, actual := range report.Results {
		if _, exists := actualByTest[actual.Test]; exists {
			return BaselineComparison{}, fmt.Errorf("report contains duplicate test %q", actual.Test)
		}
		actualByTest[actual.Test] = actual
	}

	expected := make(map[string]struct{}, len(baseline.Tests))
	comparison := BaselineComparison{Results: make([]BaselineResult, 0, len(baseline.Tests))}
	for _, want := range baseline.Tests {
		if want.Test == "" {
			return BaselineComparison{}, fmt.Errorf("baseline contains an empty test name")
		}
		if _, exists := expected[want.Test]; exists {
			return BaselineComparison{}, fmt.Errorf("baseline contains duplicate test %q", want.Test)
		}
		expected[want.Test] = struct{}{}

		actual, ok := actualByTest[want.Test]
		if !ok {
			comparison.Results = append(comparison.Results, BaselineResult{
				Test: want.Test, Outcome: BaselineChanged, Context: want.Context, Detail: "test is missing from report",
			})
			continue
		}
		comparison.Results = append(comparison.Results, compareBaselineResult(actual, want))
	}
	for _, actual := range report.Results {
		if _, ok := expected[actual.Test]; !ok {
			comparison.Results = append(comparison.Results, BaselineResult{
				Test: actual.Test, Outcome: BaselineChanged, Detail: "report contains a test not present in baseline",
			})
		}
	}
	return comparison, nil
}

func compareBaselineResult(actual Result, want BaselineExpectation) BaselineResult {
	result := BaselineResult{Test: want.Test, Context: want.Context}
	if actual.Status != want.Status {
		switch {
		case want.Status == StatusFail && actual.Status == StatusPass:
			result.Outcome = BaselineXPass
			result.Detail = "known failure now passes; update baseline intentionally"
		case want.Status == StatusPass:
			result.Outcome = BaselineRegression
			result.Detail = fmt.Sprintf("expected pass, got %s: %s", actual.Status, actual.Detail)
		default:
			result.Outcome = BaselineChanged
			result.Detail = fmt.Sprintf("expected %s, got %s: %s", want.Status, actual.Status, actual.Detail)
		}
		return result
	}

	if want.DetailContains != "" && !strings.Contains(actual.Detail, want.DetailContains) {
		result.Outcome = BaselineChanged
		result.Detail = fmt.Sprintf("failure detail changed: %q does not contain %q", actual.Detail, want.DetailContains)
		return result
	}
	if want.Failure != nil {
		if actual.Failure == nil {
			result.Outcome = BaselineChanged
			result.Detail = "expected structured failure context, but report has none"
			return result
		}
		if actual.Failure.TestID != want.Failure.TestID || actual.Failure.Group != want.Failure.Group {
			result.Outcome = BaselineChanged
			result.Detail = fmt.Sprintf("first failure changed: got test %d group %q, want test %d group %q",
				actual.Failure.TestID, actual.Failure.Group, want.Failure.TestID, want.Failure.Group)
			return result
		}
	}
	if want.Checks != nil {
		if actual.Checks == nil {
			result.Outcome = BaselineChanged
			result.Detail = "expected check progress, but report has none"
			return result
		}
		if actual.Checks.Passed != want.Checks.Passed || actual.Checks.Total != want.Checks.Total {
			result.Outcome = BaselineChanged
			result.Detail = fmt.Sprintf("check progress changed: got %d/%d, want %d/%d",
				actual.Checks.Passed, actual.Checks.Total, want.Checks.Passed, want.Checks.Total)
			return result
		}
	}

	if want.Status == StatusPass {
		result.Outcome = BaselinePass
		result.Detail = "matches expected pass"
	} else {
		result.Outcome = BaselineXFail
		result.Detail = "matches known failure"
	}
	return result
}
