package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestShellStarterBytes(t *testing.T) {
	for name, want := range map[string]string{
		".shellcheckrc": "# Keep the default checks and infer the dialect from each script.\n# Supply sourced files explicitly, or use -x in a project-specific hook.\n\n# Add only after checking that runtime source paths are script-relative:\n# source-path=SCRIPTDIR\n\n# Enable only after reviewing intentional conditional function calls:\n# enable=check-set-e-suppressed\n",
		".editorconfig": "[*.{sh,bash}]\nindent_style = space\nindent_size = 4\nswitch_case_indent = true\nsimplify = false\nminify = false\n",
	} {
		t.Run(name, func(t *testing.T) {
			body, err := swatch.Content(name)
			if err != nil || string(body) != want {
				t.Fatalf("Content(%s) = %q, %v", name, body, err)
			}
		})
	}
}
