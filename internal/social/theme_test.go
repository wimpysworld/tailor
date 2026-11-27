package social

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

var hex6Regexp = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func assertValidPaletteHex(t *testing.T, label string, p Palette) {
	t.Helper()
	fields := []struct {
		name string
		val  string
	}{
		{"Background", p.Background},
		{"CardBackground", p.CardBackground},
		{"CodeBackground", p.CodeBackground},
		{"Color", p.Color},
		{"MutedColor", p.MutedColor},
		{"MutedBorder", p.MutedBorder},
		{"Primary", p.Primary},
		{"PrimaryBackground", p.PrimaryBackground},
		{"HeadingColor", p.HeadingColor},
	}
	for _, f := range fields {
		if !hex6Regexp.MatchString(f.val) {
			t.Errorf("%s.%s = %q, want valid #rrggbb hex", label, f.name, f.val)
		}
	}
}

func TestExtractTheme(t *testing.T) {
	tests := []struct {
		name      string
		pagesPath string
		css       string
		hasCSS    bool
		cfg       *model.SocialSettings
		want      Theme
	}{
		{
			name:   "stock mucss fallback when style.css absent",
			hasCSS: false,
			want:   DefaultTheme(),
		},
		{
			name:   "stock mucss ignores non-root selectors like brand and terminal",
			hasCSS: true,
			css: `
:root {
  --mu-font-family-sans-serif: "Work Sans", system-ui, sans-serif;
  --mu-border-radius: 0.375rem;
}
.brand {
  --mu-color: #ff0000;
  --mu-primary: #00ff00;
}
:root[data-theme="dark"] .terminal {
  --mu-background-color: #000000;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) .terminal {
    --mu-background-color: #000000;
  }
}
`,
			want: DefaultTheme(),
		},
		{
			name:   "ia-get full dark and light theme overrides",
			hasCSS: true,
			css: `
:root[data-theme="dark"] {
  --mu-background-color: #1A1410;
  --mu-card-background-color: #241C16;
  --mu-code-background-color: #201913;
  --mu-color: #EDE4D8;
  --mu-muted-color: #A89C8E;
  --mu-muted-border-color: #382D24;
  --mu-primary: #F2C96B;
  --mu-primary-background: #D9A742;
  --mu-h1-color: #FFF8EE;
}
:root[data-theme="light"] {
  --mu-background-color: #FBF7F0;
  --mu-card-background-color: #FFFFFF;
  --mu-code-background-color: #F2EBE0;
  --mu-color: #2C241D;
  --mu-muted-color: #6B5D50;
  --mu-muted-border-color: #DFD5C7;
  --mu-primary: #8C5A00;
  --mu-primary-background: #784C00;
  --mu-h1-color: #1A1410;
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        "#1a1410",
					CardBackground:    "#241c16",
					CodeBackground:    "#201913",
					Color:             "#ede4d8",
					MutedColor:        "#a89c8e",
					MutedBorder:       "#382d24",
					Primary:           "#f2c96b",
					PrimaryBackground: "#d9a742",
					HeadingColor:      "#fff8ee",
				},
				Light: Palette{
					Background:        "#fbf7f0",
					CardBackground:    "#ffffff",
					CodeBackground:    "#f2ebe0",
					Color:             "#2c241d",
					MutedColor:        "#6b5d50",
					MutedBorder:       "#dfd5c7",
					Primary:           "#8c5a00",
					PrimaryBackground: "#784c00",
					HeadingColor:      "#1a1410",
				},
			},
		},
		{
			name:   "hue-jackman root comma light selector does not leak into dark",
			hasCSS: true,
			css: `
:root,
:root[data-theme="light"] {
  --mu-background-color: #f6faf6;
  --mu-primary: #227a14;
}
:root[data-theme="dark"] {
  --mu-background-color: #0d120f;
  --mu-primary: #44d62c;
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        "#0d120f",
					CardBackground:    defaultDarkPalette.CardBackground,
					CodeBackground:    defaultDarkPalette.CodeBackground,
					Color:             defaultDarkPalette.Color,
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       defaultDarkPalette.MutedBorder,
					Primary:           "#44d62c",
					PrimaryBackground: "#44d62c",
					HeadingColor:      defaultDarkPalette.HeadingColor,
				},
				Light: Palette{
					Background:        "#f6faf6",
					CardBackground:    "#f6faf6",
					CodeBackground:    defaultLightPalette.CodeBackground,
					Color:             defaultLightPalette.Color,
					MutedColor:        defaultLightPalette.MutedColor,
					MutedBorder:       defaultLightPalette.MutedBorder,
					Primary:           "#227a14",
					PrimaryBackground: "#227a14",
					HeadingColor:      defaultLightPalette.HeadingColor,
				},
			},
		},
		{
			name:   "jive light-dark inside :root:root",
			hasCSS: true,
			css: `
:root:root {
  --mu-primary: light-dark(#855600, #feb81e);
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        defaultDarkPalette.Background,
					CardBackground:    defaultDarkPalette.CardBackground,
					CodeBackground:    defaultDarkPalette.CodeBackground,
					Color:             defaultDarkPalette.Color,
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       defaultDarkPalette.MutedBorder,
					Primary:           "#feb81e",
					PrimaryBackground: "#feb81e",
					HeadingColor:      defaultDarkPalette.HeadingColor,
				},
				Light: Palette{
					Background:        defaultLightPalette.Background,
					CardBackground:    defaultLightPalette.CardBackground,
					CodeBackground:    defaultLightPalette.CodeBackground,
					Color:             defaultLightPalette.Color,
					MutedColor:        defaultLightPalette.MutedColor,
					MutedBorder:       defaultLightPalette.MutedBorder,
					Primary:           "#855600",
					PrimaryBackground: "#855600",
					HeadingColor:      defaultLightPalette.HeadingColor,
				},
			},
		},
		{
			name:   "media prefers-color-scheme dark and light blocks",
			hasCSS: true,
			css: `
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --mu-background-color: #161219;
    --mu-primary: #91c2f2;
    --mu-color: #e4dce8;
  }
}
@media (prefers-color-scheme: light) {
  :root {
    --mu-primary: #3b6ea5;
  }
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        "#161219",
					CardBackground:    defaultDarkPalette.CardBackground,
					CodeBackground:    defaultDarkPalette.CodeBackground,
					Color:             "#e4dce8",
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       defaultDarkPalette.MutedBorder,
					Primary:           "#91c2f2",
					PrimaryBackground: "#91c2f2",
					HeadingColor:      "#e4dce8",
				},
				Light: Palette{
					Background:        defaultLightPalette.Background,
					CardBackground:    defaultLightPalette.CardBackground,
					CodeBackground:    defaultLightPalette.CodeBackground,
					Color:             defaultLightPalette.Color,
					MutedColor:        defaultLightPalette.MutedColor,
					MutedBorder:       defaultLightPalette.MutedBorder,
					Primary:           "#3b6ea5",
					PrimaryBackground: "#3b6ea5",
					HeadingColor:      defaultLightPalette.HeadingColor,
				},
			},
		},
		{
			name:   "decimal rgb, rgba, shorthand hex, and 8-digit hex",
			hasCSS: true,
			css: `
[data-theme="dark"] {
  --mu-background-color: rgb(19, 22.5, 30.5);
  --mu-card-background-color: rgba(24, 28, 37, 0.9);
  --mu-code-background-color: rgb(26 30.5 40.25 / 80%);
  --mu-color: #abc;
  --mu-muted-color: #12345680;
  --mu-muted-border-color: rgb(100%, 50%, 0%);
  --mu-primary: #0f8a;
  --mu-primary-background: #0f8;
  --mu-h1-color: #fff;
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        "#13171f",
					CardBackground:    "#181c25",
					CodeBackground:    "#1a1f28",
					Color:             "#aabbcc",
					MutedColor:        "#123456",
					MutedBorder:       "#ff8000",
					Primary:           "#00ff88",
					PrimaryBackground: "#00ff88",
					HeadingColor:      "#ffffff",
				},
				Light: defaultLightPalette,
			},
		},
		{
			name:   "color-mix and var chains with diamond reuse and cycle protection",
			hasCSS: true,
			css: `
:root {
  --brand: #ff0000;
  --tint-a: var(--brand);
  --tint-b: var(--brand);
  --loop-a: var(--loop-b);
  --loop-b: var(--loop-a);
  --self: var(--self);
}
:root[data-theme="dark"] {
  --mu-primary: color-mix(in srgb, var(--tint-a) 25%, #0000ff 75%);
  --mu-primary-background: color-mix(in srgb, #ffffff 50%, #000000);
  --mu-card-background-color: color-mix(in srgb, var(--tint-b) 20%, #000000);
  --mu-code-background-color: color-mix(in srgb, 40% #00ff00, #000000);
  --mu-muted-border-color: color-mix(in srgb, #ffffff, #000000);
  /* Cyclic tokens must fall back cleanly to defaults. */
  --mu-color: var(--loop-a);
  --mu-muted-color: var(--self);
  /* Missing variable uses fallback value. */
  --mu-h1-color: var(--missing-token, #fedcba);
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        defaultDarkPalette.Background,
					CardBackground:    "#330000",
					CodeBackground:    "#006600",
					Color:             defaultDarkPalette.Color,
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       "#808080",
					Primary:           "#4000bf",
					PrimaryBackground: "#808080",
					HeadingColor:      "#fedcba",
				},
				Light: defaultLightPalette,
			},
		},
		{
			name:   "optional accent and background overrides on dark default mode",
			hasCSS: true,
			css: `
:root[data-theme="dark"] {
  --mu-background-color: #161219;
  --mu-primary: #91c2f2;
}
:root[data-theme="light"] {
  --mu-background-color: #fbf7f0;
  --mu-primary: #8a2b3f;
}
`,
			cfg: &model.SocialSettings{
				Theme:      new("dark"),
				Accent:     new("#44D62C"),
				Background: new("#0D120F"),
			},
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        "#0d120f",
					CardBackground:    defaultDarkPalette.CardBackground,
					CodeBackground:    defaultDarkPalette.CodeBackground,
					Color:             defaultDarkPalette.Color,
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       defaultDarkPalette.MutedBorder,
					Primary:           "#44d62c",
					PrimaryBackground: "#44d62c",
					HeadingColor:      defaultDarkPalette.HeadingColor,
				},
				Light: Palette{
					Background:        "#fbf7f0",
					CardBackground:    "#fbf7f0",
					CodeBackground:    defaultLightPalette.CodeBackground,
					Color:             defaultLightPalette.Color,
					MutedColor:        defaultLightPalette.MutedColor,
					MutedBorder:       defaultLightPalette.MutedBorder,
					Primary:           "#44d62c",
					PrimaryBackground: "#44d62c",
					HeadingColor:      defaultLightPalette.HeadingColor,
				},
			},
		},
		{
			name:   "background override targets light mode when default theme is light",
			hasCSS: false,
			cfg: &model.SocialSettings{
				Theme:      new("light"),
				Accent:     new("#1e66f5"),
				Background: new("#eff1f5"),
			},
			want: Theme{
				DefaultMode: "light",
				Dark: Palette{
					Background:        defaultDarkPalette.Background,
					CardBackground:    defaultDarkPalette.CardBackground,
					CodeBackground:    defaultDarkPalette.CodeBackground,
					Color:             defaultDarkPalette.Color,
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       defaultDarkPalette.MutedBorder,
					Primary:           "#1e66f5",
					PrimaryBackground: "#1e66f5",
					HeadingColor:      defaultDarkPalette.HeadingColor,
				},
				Light: Palette{
					Background:        "#eff1f5",
					CardBackground:    defaultLightPalette.CardBackground,
					CodeBackground:    defaultLightPalette.CodeBackground,
					Color:             defaultLightPalette.Color,
					MutedColor:        defaultLightPalette.MutedColor,
					MutedBorder:       defaultLightPalette.MutedBorder,
					Primary:           "#1e66f5",
					PrimaryBackground: "#1e66f5",
					HeadingColor:      defaultLightPalette.HeadingColor,
				},
			},
		},
		{
			name:      "comments and custom pages path",
			pagesPath: "docs",
			hasCSS:    true,
			css: `
/* :root { --mu-primary: #ff0000; } */
:root[data-theme="dark"] {
  --mu-icon: url("data:image/svg+xml,/*not a comment*/{color:#000};");
  --mu-primary: #8ad45b !important; /* inline comment */
}
`,
			want: Theme{
				DefaultMode: "dark",
				Dark: Palette{
					Background:        defaultDarkPalette.Background,
					CardBackground:    defaultDarkPalette.CardBackground,
					CodeBackground:    defaultDarkPalette.CodeBackground,
					Color:             defaultDarkPalette.Color,
					MutedColor:        defaultDarkPalette.MutedColor,
					MutedBorder:       defaultDarkPalette.MutedBorder,
					Primary:           "#8ad45b",
					PrimaryBackground: "#8ad45b",
					HeadingColor:      defaultDarkPalette.HeadingColor,
				},
				Light: defaultLightPalette,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			pagesDir := tt.pagesPath
			if pagesDir == "" {
				pagesDir = "pages"
			}
			if tt.hasCSS {
				fullDir := filepath.Join(dir, filepath.FromSlash(pagesDir))
				if err := os.MkdirAll(fullDir, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(filepath.Join(fullDir, "style.css"), []byte(tt.css), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatalf("OpenRoot: %v", err)
			}
			defer root.Close()

			got, err := ExtractTheme(root, tt.pagesPath, tt.cfg)
			if err != nil {
				t.Fatalf("ExtractTheme() unexpected error: %v", err)
			}

			assertValidPaletteHex(t, "Dark", got.Dark)
			assertValidPaletteHex(t, "Light", got.Light)

			if got != tt.want {
				t.Fatalf("ExtractTheme() =\n%+v\nwant:\n%+v", got, tt.want)
			}
		})
	}
}

func TestExtractThemeNilRoot(t *testing.T) {
	got, err := ExtractTheme(nil, "pages", nil)
	if err != nil {
		t.Fatalf("ExtractTheme(nil) unexpected error: %v", err)
	}
	if got != DefaultTheme() {
		t.Fatalf("ExtractTheme(nil) = %+v, want %+v", got, DefaultTheme())
	}
}

func TestExtractThemeErrors(t *testing.T) {
	t.Run("rejects symlink style.css", func(t *testing.T) {
		dir := t.TempDir()
		pagesDir := filepath.Join(dir, "pages")
		if err := os.MkdirAll(pagesDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		target := filepath.Join(dir, "real.css")
		if err := os.WriteFile(target, []byte(":root { --mu-primary: #ff0000; }"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(pagesDir, "style.css")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()

		if _, err := ExtractTheme(root, "pages", nil); err == nil {
			t.Fatal("ExtractTheme() expected error for symlink style.css, got nil")
		}
	})

	t.Run("rejects oversized style.css", func(t *testing.T) {
		dir := t.TempDir()
		pagesDir := filepath.Join(dir, "pages")
		if err := os.MkdirAll(pagesDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		big := strings.Repeat(" ", maxStyleCSSSize+1)
		if err := os.WriteFile(filepath.Join(pagesDir, "style.css"), []byte(big), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()

		if _, err := ExtractTheme(root, "pages", nil); err == nil {
			t.Fatal("ExtractTheme() expected error for oversized style.css, got nil")
		}
	})

	t.Run("rejects invalid accent colour", func(t *testing.T) {
		if _, err := ExtractTheme(nil, "pages", &model.SocialSettings{Accent: new("not-a-hex")}); err == nil {
			t.Fatal("ExtractTheme() expected error for invalid accent, got nil")
		}
	})

	t.Run("rejects invalid background colour", func(t *testing.T) {
		if _, err := ExtractTheme(nil, "pages", &model.SocialSettings{Background: new("#12345")}); err == nil {
			t.Fatal("ExtractTheme() expected error for invalid background, got nil")
		}
	})

	t.Run("rejects escaping pagesPath", func(t *testing.T) {
		if _, err := ExtractTheme(nil, "../outside", nil); err == nil {
			t.Fatal("ExtractTheme() expected error for escaping pagesPath, got nil")
		}
	})
}

func TestExtractThemeRepoPagesStyleCSS(t *testing.T) {
	root, err := os.OpenRoot(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer root.Close()

	got, err := ExtractTheme(root, "pages", nil)
	if err != nil {
		t.Fatalf("ExtractTheme(repo pages/style.css) unexpected error: %v", err)
	}
	assertValidPaletteHex(t, "Dark", got.Dark)
	assertValidPaletteHex(t, "Light", got.Light)
	if got != DefaultTheme() {
		t.Fatalf("ExtractTheme(repo pages/style.css) =\n%+v\nwant stock defaults:\n%+v", got, DefaultTheme())
	}
}
