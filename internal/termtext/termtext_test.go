package termtext

import "testing"

func TestEscapeControlText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "benign text unchanged",
			input: ".github/dependabot.yml",
			want:  ".github/dependabot.yml",
		},
		{
			name:  "newline",
			input: "safe\ninjected",
			want:  `safe\x0ainjected`,
		},
		{
			name:  "carriage return",
			input: "safe\rspoofed",
			want:  `safe\x0dspoofed`,
		},
		{
			name:  "ansi csi sequence",
			input: "\x1b[31mred\x1b[0m",
			want:  `\x1b[31mred\x1b[0m`,
		},
		{
			name:  "bare escape",
			input: "path\x1b",
			want:  `path\x1b`,
		},
		{
			name:  "delete",
			input: "path\x7f",
			want:  `path\x7f`,
		},
		{
			name:  "c1 control",
			input: "path\u009bspoofed",
			want:  `path\u009bspoofed`,
		},
		{
			name:  "non-control unicode unchanged",
			input: "café",
			want:  "café",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EscapeControlText(tt.input); got != tt.want {
				t.Errorf("EscapeControlText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
