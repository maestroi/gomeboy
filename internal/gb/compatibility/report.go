package compatibility

import (
	"fmt"
	"strings"
)

// Markdown renders a human-readable compatibility matrix. Compatibility stages
// deliberately remain separate from the conformance pass percentage.
func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# GB/GBC compatibility smoke report\n\n")
	fmt.Fprintf(&b, "- Suite: %s\n", markdownText(r.Suite))
	fmt.Fprintf(&b, "- Suite revision: %s\n", markdownText(r.SuiteRevision))
	fmt.Fprintf(&b, "- GomeBoy commit: %s\n", markdownText(r.GomeBoyCommit))
	fmt.Fprintf(&b, "- Results: %d passed, %d failed, %d timed out, %d total\n\n",
		r.Summary.Passed, r.Summary.Failed, r.Summary.TimedOut, r.Summary.Total)
	b.WriteString("Compatibility stages describe end-to-end software progress; they are not hardware-accuracy percentages.\n\n")
	b.WriteString("| ROM/test | Model | Source | Revision | ROM SHA-256 | Status | Highest stage | Target stage | Frames | Cycles | Detail |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | --- |\n")
	for _, result := range r.Results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %d | %d | %s |\n",
			markdownText(result.Test),
			markdownText(string(result.Model)),
			markdownText(result.Source),
			markdownText(result.SourceRevision),
			markdownText(result.ROMSHA256),
			result.Status,
			result.HighestStage,
			result.TargetStage,
			result.Frames,
			result.Cycles,
			markdownText(result.Detail),
		)
	}
	return b.String()
}

func markdownText(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
