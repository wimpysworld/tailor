package measure

import (
	"fmt"
	"strings"
)

// labelWidth is the fixed column width for status labels in formatted output.
const labelWidth = 16

// AdvisoryMessage is printed when no .tailor/config.yml is found.
const AdvisoryMessage = "No .tailor/config.yml found. Run `tailor fit <path>` to initialise, or create `.tailor/config.yml` manually to enable configuration alignment checks."

// FormatOutput produces the measure command output. Health results are always
// included. Diff results are included only when a config was loaded. When
// hasConfig is false, the advisory message is appended after a blank line.
func FormatOutput(health []HealthResult, diff []DiffResult, hasConfig bool) string {
	var b strings.Builder

	for _, r := range health {
		fmt.Fprintf(&b, "%-*s%s\n", labelWidth, string(r.Status)+":", r.Destination)
	}

	for _, r := range diff {
		line := r.Destination
		if r.Annotation != "" {
			line += " " + r.Annotation
		}
		fmt.Fprintf(&b, "%-*s%s\n", labelWidth, string(r.Category)+":", line)
	}

	if !hasConfig {
		b.WriteString("\n")
		b.WriteString(AdvisoryMessage)
		b.WriteString("\n")
	}

	return b.String()
}
