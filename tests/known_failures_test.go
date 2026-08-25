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
	"mgb_oam_dma_halt_sprites":              true,
	"channel_1_extra_length_clocking-cgb0B": true,
	"channel_1_freq_change_timing-A":        true,
	"channel_1_freq_change_timing-cgb0BC":   true,
	"channel_1_freq_change_timing-cgbDE":    true,
	"channel_1_sweep":                       true,
	"channel_1_sweep_restart":               true,
	"channel_1_sweep_restart_2":             true,
	"channel_2_extra_length_clocking-cgb0B": true,
	"channel_3_extra_length_clocking-cgb0":  true,
	"channel_3_extra_length_clocking-cgbB":  true,
	"channel_3_freq_change_delay":           true,
	"channel_3_restart_delay":               true,
	"channel_4_delay":                       true,
	"channel_4_equivalent_frequencies":      true,
	"channel_4_extra_length_clocking-cgb0B": true,
	"channel_4_freq_change":                 true,
	"channel_4_frequency_alignment":         true,
	"command_mlt_req":                       true,
	"command_mlt_req_1_incrementing":        true,
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
