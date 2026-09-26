package conformance

import "testing"

func TestCompareBaselineAcceptsKnownFailures(t *testing.T) {
	report := Report{
		Suite:         "suite",
		SuiteRevision: "rev",
		Results: []Result{
			{Test: "arm", Status: StatusFail, Detail: "unsupported r15 shift"},
			{
				Test:    "thumb",
				Status:  StatusFail,
				Failure: &FailureInfo{TestID: 211, Group: "memory", Register: "r0"},
				Checks:  &CheckSummary{Passed: 86, Failed: 1, NotRun: 22, Total: 109},
			},
		},
	}
	baseline := Baseline{
		SchemaVersion: 1,
		Suite:         "suite",
		SuiteRevision: "rev",
		Tests: []BaselineExpectation{
			{Test: "arm", Status: StatusFail, DetailContains: "r15 shift"},
			{
				Test:    "thumb",
				Status:  StatusFail,
				Failure: &BaselineFailure{TestID: 211, Group: "memory"},
				Checks:  &BaselineChecks{Passed: 86, Total: 109},
			},
		},
	}

	comparison, err := CompareBaseline(report, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !comparison.Passed() {
		t.Fatalf("comparison = %+v, want accepted XFAILs", comparison.Results)
	}
	if comparison.Results[0].Outcome != BaselineXFail || comparison.Results[1].Outcome != BaselineXFail {
		t.Fatalf("outcomes = %+v, want xfail/xfail", comparison.Results)
	}
}

func TestCompareBaselineSurfacesXPassAndChangedFailure(t *testing.T) {
	baseline := Baseline{
		SchemaVersion: 1,
		Suite:         "suite",
		SuiteRevision: "rev",
		Tests: []BaselineExpectation{{
			Test:    "thumb",
			Status:  StatusFail,
			Failure: &BaselineFailure{TestID: 211, Group: "memory"},
		}},
	}

	for _, tc := range []struct {
		name    string
		actual  Result
		outcome BaselineOutcome
	}{
		{
			name:    "xpass",
			actual:  Result{Test: "thumb", Status: StatusPass},
			outcome: BaselineXPass,
		},
		{
			name:    "first failure moved",
			actual:  Result{Test: "thumb", Status: StatusFail, Failure: &FailureInfo{TestID: 212, Group: "memory"}},
			outcome: BaselineChanged,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := Report{Suite: "suite", SuiteRevision: "rev", Results: []Result{tc.actual}}
			comparison, err := CompareBaseline(report, baseline)
			if err != nil {
				t.Fatal(err)
			}
			if comparison.Passed() || comparison.Results[0].Outcome != tc.outcome {
				t.Fatalf("comparison = %+v, want failing %s", comparison.Results, tc.outcome)
			}
		})
	}
}
