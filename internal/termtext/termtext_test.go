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
		{
			name:  "arabic letter mark",
			input: "path\u061cspoofed",
			want:  `path\u061cspoofed`,
		},
		{
			name:  "left-to-right mark",
			input: "path\u200espoofed",
			want:  `path\u200espoofed`,
		},
		{
			name:  "right-to-left mark",
			input: "path\u200fspoofed",
			want:  `path\u200fspoofed`,
		},
		{
			name:  "left-to-right embedding",
			input: "path\u202aspoofed",
			want:  `path\u202aspoofed`,
		},
		{
			name:  "right-to-left embedding",
			input: "path\u202bspoofed",
			want:  `path\u202bspoofed`,
		},
		{
			name:  "pop directional formatting",
			input: "path\u202cspoofed",
			want:  `path\u202cspoofed`,
		},
		{
			name:  "left-to-right override",
			input: "path\u202dspoofed",
			want:  `path\u202dspoofed`,
		},
		{
			name:  "right-to-left override",
			input: "path\u202espoofed",
			want:  `path\u202espoofed`,
		},
		{
			name:  "left-to-right isolate",
			input: "path\u2066spoofed",
			want:  `path\u2066spoofed`,
		},
		{
			name:  "right-to-left isolate",
			input: "path\u2067spoofed",
			want:  `path\u2067spoofed`,
		},
		{
			name:  "first strong isolate",
			input: "path\u2068spoofed",
			want:  `path\u2068spoofed`,
		},
		{
			name:  "pop directional isolate",
			input: "path\u2069spoofed",
			want:  `path\u2069spoofed`,
		},
		{
			name:  "emoji zwj sequence unchanged",
			input: "\U0001f469\u200d\U0001f4bb",
			want:  "\U0001f469\u200d\U0001f4bb",
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
