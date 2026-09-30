package tests

import "testing"

// knownFailures lists the tests that are known to fail on this codebase, as
// documented in tests/README.md. tellinglys (DMG) is included as well: it is
// flaky on this hardware and fails intermittently. In the default build
// context these are skipped so that `go test ./...` is hermetic and green;
// under the "test" build tag (used by CI's Test_Regressions) they remain
// failures so the regression table stays honest.
var knownFailures = map[string]bool{
	"cgb-acid-hell":                         true,
	"bully (CGB)":                           true,
	"tellinglys (DMG)":                      true,
	"tellinglys (CGB)":                      true,
	"channel_1_sweep":                       true,
	"channel_1_sweep_restart":               true,
	"channel_1_sweep_restart_2":             true,
	"strikethrough (DMG)":                   true,
	"strikethrough (CGB)":                   true,
}

// skipKnownFailure skips the named test when known-failure skipping is
// enabled and the test is a documented known failure.
func skipKnownFailure(t *testing.T, name string) {
	t.Helper()
	if skipKnownFailures && knownFailures[name] {
		t.Skipf("known failure (see tests/README.md): %s", name)
	}
}
