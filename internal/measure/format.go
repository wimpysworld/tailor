package measure

import (
	"fmt"
	"strings"
)

// labelWidth is the fixed column width for status labels in formatted output.
// It fits the longest label, "not-configured:", plus one trailing space.
const labelWidth = 16

// AdvisoryMessage is printed when no .tailor.yml is found.
const AdvisoryMessage = "No .tailor.yml found. Run `tailor fit <path>` to initialise, or create `.tailor.yml` manually to enable configuration alignment checks."

// FormatOutput produces the measure command output: health results first,
// then diff results. When hasConfig is false, the advisory message is
// appended after a blank line.
func FormatOutput(health []HealthResult, diff []DiffResult, hasConfig bool) string {
	var b strings.Builder

	for _, r := range health {
		writeResultLine(&b, string(r.Status)+":", r.Path, r.Detail)
	}

	for _, r := range diff {
		writeResultLine(&b, string(r.Category)+":", r.Path, r.Detail)
	}

	if !hasConfig {
		b.WriteString("\n")
		b.WriteString(AdvisoryMessage)
		b.WriteString("\n")
	}

	return b.String()
}

func writeResultLine(b *strings.Builder, label, path, detail string) {
	if detail != "" {
		path += " " + detail
	}
	fmt.Fprintf(b, "%-*s%s\n", labelWidth, label, path)
}
