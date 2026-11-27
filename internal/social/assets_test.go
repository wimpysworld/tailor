package social

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
)

func TestProjectTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single lowercase word", in: "tailor", want: "Tailor"},
		{name: "owner and repo", in: "wimpysworld/tailor", want: "Tailor"},
		{name: "two-char leading segment unchanged", in: "ia-get", want: "ia-get"},
		{name: "owner and two-char segment unchanged", in: "wimpysworld/ia-get", want: "ia-get"},
		{name: "internal capitals preserved", in: "WonKey", want: "WonKey"},
		{name: "owner and internal capitals preserved", in: "wimpysworld/WonKey", want: "WonKey"},
		{name: "hyphenated stream-sprout", in: "stream-sprout", want: "Stream Sprout"},
		{name: "owner and stream-sprout", in: "wimpysworld/stream-sprout", want: "Stream Sprout"},
		{name: "hyphenated hue-jackman", in: "hue-jackman", want: "Hue Jackman"},
		{name: "hyphenated jive-vocals", in: "jive-vocals", want: "Jive Vocals"},
		{name: "single word sidra", in: "sidra", want: "Sidra"},
		{name: "single word nyala", in: "nyala", want: "Nyala"},
		{name: "multi-word internal capitals", in: "myTool-extraFast", want: "MyTool ExtraFast"},
		{name: "trimmed whitespace", in: "  wimpysworld/stream-sprout  ", want: "Stream Sprout"},
		{name: "empty string", in: "", want: ""},
		{name: "whitespace only", in: "   ", want: ""},
		{name: "owner with empty repo", in: "wimpysworld/", want: ""},
		{name: "double hyphen unchanged", in: "stream--sprout", want: "stream--sprout"},
		{name: "leading hyphen unchanged", in: "-tailor", want: "-tailor"},
		{name: "trailing hyphen unchanged", in: "tailor-", want: "tailor-"},
		{name: "two-char single word unchanged", in: "go", want: "go"},
		{name: "trailing two-char segment unchanged", in: "foo-v2", want: "foo-v2"},
		{name: "middle two-char segment unchanged", in: "foo-in-bar", want: "foo-in-bar"},
		{name: "already formatted title idempotent", in: "Stream Sprout", want: "Stream Sprout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectTitle(tt.in); got != tt.want {
				t.Errorf("ProjectTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripTrailingEmoji(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "tailor necktie emoji",
			in:   "Ready-to-wear project templates for GitHub repositories 👔",
			want: "Ready-to-wear project templates for GitHub repositories",
		},
		{
			name: "red apple emoji",
			in:   "Apple-flavoured utility 🍎",
			want: "Apple-flavoured utility",
		},
		{
			name: "wonkey keycap combining mark",
			in:   "One-key launcher 1️⃣",
			want: "One-key launcher",
		},
		{
			name: "down arrow with variation selector",
			in:   "Internet Archive downloader ⬇️",
			want: "Internet Archive downloader",
		},
		{
			name: "nyala deer emoji",
			in:   "Desktop companion 🦌",
			want: "Desktop companion",
		},
		{
			name: "video game controller emoji",
			in:   "Lighting controller 🎮",
			want: "Lighting controller",
		},
		{
			name: "satellite antenna emoji",
			in:   "Stream automation 📡",
			want: "Stream automation",
		},
		{
			name: "speaking head with variation selector",
			in:   "Voice assistant 🗣️",
			want: "Voice assistant",
		},
		{
			name: "compression clamp with variation selector",
			in:   "Compact audio tool 🗜️",
			want: "Compact audio tool",
		},
		{
			name: "sparkles dingbat emoji",
			in:   "Polished workflow ✨",
			want: "Polished workflow",
		},
		{
			name: "plain text without emoji",
			in:   "Plain repository description",
			want: "Plain repository description",
		},
		{
			name: "trailing version number preserved",
			in:   "Fast HTTP/2 CLI v2",
			want: "Fast HTTP/2 CLI v2",
		},
		{
			name: "trailing hash number preserved",
			in:   "Fix issue #1",
			want: "Fix issue #1",
		},
		{
			name: "trailing punctuation preserved",
			in:   "Simple tool for Go.",
			want: "Simple tool for Go.",
		},
		{
			name: "zwj sequence stripped cleanly",
			in:   "Built for developers 👨‍💻",
			want: "Built for developers",
		},
		{
			name: "zwj pirate flag with variation selector stripped",
			in:   "Sailing utilities 🏴‍☠️",
			want: "Sailing utilities",
		},
		{
			name: "regional indicator flag pair stripped",
			in:   "British English linter 🇬🇧",
			want: "British English linter",
		},
		{
			name: "multiple trailing emojis and spaces",
			in:   "All the things ✨ 👔   ",
			want: "All the things",
		},
		{
			name: "text variation selector U+FE0E stripped",
			in:   "Clean text\uFE0E",
			want: "Clean text",
		},
		{
			name: "middle emoji preserved",
			in:   "Tailor 👔 templates for GitHub",
			want: "Tailor 👔 templates for GitHub",
		},
		{
			name: "only emojis returns empty",
			in:   "👔 ✨ 1️⃣",
			want: "",
		},
		{
			name: "empty string unchanged",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripTrailingEmoji(tt.in); got != tt.want {
				t.Errorf("StripTrailingEmoji(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveProjectAssetsIcons(t *testing.T) {
	tests := []struct {
		name      string
		pagesPath string
		files     map[string][]byte
		wantPath  string
		wantKind  string
	}{
		{
			name: "prefers icon.svg over icon.png",
			files: map[string][]byte{
				"pages/icon.svg": []byte("<svg></svg>"),
				"pages/icon.png": []byte("pngdata"),
			},
			wantPath: "pages/icon.svg",
			wantKind: IconKindSVG,
		},
		{
			name: "falls back to icon.png when icon.svg absent",
			files: map[string][]byte{
				"pages/icon.png": []byte("pngdata"),
			},
			wantPath: "pages/icon.png",
			wantKind: IconKindPNG,
		},
		{
			name:     "no icon present",
			files:    nil,
			wantPath: "",
			wantKind: IconKindNone,
		},
		{
			name:      "custom pages path",
			pagesPath: "site",
			files: map[string][]byte{
				"site/icon.svg": []byte("<svg></svg>"),
			},
			wantPath: "site/icon.svg",
			wantKind: IconKindSVG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, content := range tt.files {
				full := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(full, content, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatalf("OpenRoot: %v", err)
			}
			defer root.Close()

			got, err := ResolveProjectAssets(root, tt.pagesPath, nil, "wimpysworld/tailor")
			if err != nil {
				t.Fatalf("ResolveProjectAssets() unexpected error: %v", err)
			}
			if got.IconPath != tt.wantPath || got.IconKind != tt.wantKind {
				t.Errorf("icon = (%q, %q), want (%q, %q)", got.IconPath, got.IconKind, tt.wantPath, tt.wantKind)
			}
		})
	}
}

func TestResolveProjectAssetsIconSymlinkRejection(t *testing.T) {
	t.Run("rejects symlink icon.svg pointing inside root", func(t *testing.T) {
		dir := t.TempDir()
		pagesDir := filepath.Join(dir, "pages")
		if err := os.MkdirAll(pagesDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		target := filepath.Join(dir, "real.svg")
		if err := os.WriteFile(target, []byte("<svg></svg>"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(pagesDir, "icon.svg")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()

		if _, err := ResolveProjectAssets(root, "pages", nil, "tailor"); err == nil {
			t.Fatal("ResolveProjectAssets() expected error for symlink icon.svg, got nil")
		}
	})

	t.Run("rejects symlink icon.png pointing outside root", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		target := filepath.Join(outside, "secret.png")
		if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		pagesDir := filepath.Join(dir, "pages")
		if err := os.MkdirAll(pagesDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(pagesDir, "icon.png")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()

		if _, err := ResolveProjectAssets(root, "pages", nil, "tailor"); err == nil {
			t.Fatal("ResolveProjectAssets() expected error for external symlink icon.png, got nil")
		}
	})

	t.Run("rejects symlinked pages parent directory", func(t *testing.T) {
		dir := t.TempDir()
		realPages := filepath.Join(dir, "real-pages")
		if err := os.MkdirAll(realPages, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(realPages, "icon.svg"), []byte("<svg></svg>"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Symlink(realPages, filepath.Join(dir, "pages")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()

		if _, err := ResolveProjectAssets(root, "pages", nil, "tailor"); err == nil {
			t.Fatal("ResolveProjectAssets() expected error for symlinked pages parent, got nil")
		}
	})
}

func TestResolveProjectAssetsMetadataPreviewAndFooter(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		files    map[string][]byte
		cfg      *config.Config
		want     ProjectAssets
		wantErr  bool
	}{
		{
			name:     "default centred editorial preview none with pages cname",
			repoName: "wimpysworld/tailor",
			files: map[string][]byte{
				"pages/icon.svg":       []byte("<svg></svg>"),
				"pages/demo-still.png": []byte("png"),
			},
			cfg: &config.Config{
				Repository: &model.RepositorySettings{
					Description: new("Ready-to-wear project templates for GitHub repositories 👔"),
					Homepage:    new("https://wimpysworld.github.io/tailor"),
				},
				Pages: &model.PagesSettings{
					CNAME: new("tailor.wimpys.world"),
				},
				Social: &model.SocialSettings{
					Platforms: new("Linux · macOS · Windows"),
					OG: &model.SocialOGSettings{
						IncludePreview: new(false),
						Terminal:       new([]string{"$ tailor fit ."}),
					},
				},
			},
			want: ProjectAssets{
				Title:       "Tailor",
				Tagline:     "Ready-to-wear project templates for GitHub repositories",
				IconPath:    "pages/icon.svg",
				IconKind:    IconKindSVG,
				PreviewMode: PreviewModeNone,
				FooterURL:   "https://tailor.wimpys.world",
				Platforms:   "Linux · macOS · Windows",
			},
		},
		{
			name:     "include_preview prefers explicit og.image over demo-still and terminal",
			repoName: "wimpysworld/nyala",
			files: map[string][]byte{
				"pages/shot.webp":      []byte("webp"),
				"pages/demo-still.png": []byte("png"),
			},
			cfg: &config.Config{
				Repository: &model.RepositorySettings{
					Description: new("Desktop companion 🦌"),
					Homepage:    new("https://nyala.wimpys.world"),
				},
				Social: &model.SocialSettings{
					OG: &model.SocialOGSettings{
						IncludePreview: new(true),
						Image:          new("pages/shot.webp"),
						Terminal:       new([]string{"$ nyala"}),
					},
				},
			},
			want: ProjectAssets{
				Title:        "Nyala",
				Tagline:      "Desktop companion",
				IconKind:     IconKindNone,
				PreviewMode:  PreviewModeImage,
				PreviewImage: "pages/shot.webp",
				FooterURL:    "https://nyala.wimpys.world",
			},
		},
		{
			name:     "include_preview falls back to demo-still.png",
			repoName: "wimpysworld/wonkey",
			files: map[string][]byte{
				"pages/demo-still.png": []byte("png"),
			},
			cfg: &config.Config{
				Repository: &model.RepositorySettings{
					Description: new("One-key launcher 1️⃣"),
				},
				Social: &model.SocialSettings{
					OG: &model.SocialOGSettings{
						IncludePreview: new(true),
						Terminal:       new([]string{"$ wonkey"}),
					},
				},
			},
			want: ProjectAssets{
				Title:        "Wonkey",
				Tagline:      "One-key launcher",
				IconKind:     IconKindNone,
				PreviewMode:  PreviewModeImage,
				PreviewImage: "pages/demo-still.png",
				FooterURL:    "https://github.com/wimpysworld/wonkey",
			},
		},
		{
			name:     "include_preview falls back to terminal lines",
			repoName: "wimpysworld/ia-get",
			cfg: &config.Config{
				Repository: &model.RepositorySettings{
					Description: new("Internet Archive downloader ⬇️"),
				},
				Social: &model.SocialSettings{
					OG: &model.SocialOGSettings{
						IncludePreview: new(true),
						Terminal:       new([]string{"$ ia-get https://archive.org/details/example", "Downloading..."}),
					},
				},
			},
			want: ProjectAssets{
				Title:           "ia-get",
				Tagline:         "Internet Archive downloader",
				IconKind:        IconKindNone,
				PreviewMode:     PreviewModeTerminal,
				PreviewTerminal: []string{"$ ia-get https://archive.org/details/example", "Downloading..."},
				FooterURL:       "https://github.com/wimpysworld/ia-get",
			},
		},
		{
			name:     "include_preview stays none when no image, demo-still, or terminal exists",
			repoName: "bare-repo",
			cfg: &config.Config{
				Social: &model.SocialSettings{
					OG: &model.SocialOGSettings{
						IncludePreview: new(true),
					},
				},
			},
			want: ProjectAssets{
				Title:       "Bare Repo",
				IconKind:    IconKindNone,
				PreviewMode: PreviewModeNone,
				FooterURL:   "",
			},
		},
		{
			name:     "missing explicit og.image returns error",
			repoName: "wimpysworld/tailor",
			cfg: &config.Config{
				Social: &model.SocialSettings{
					OG: &model.SocialOGSettings{
						IncludePreview: new(true),
						Image:          new("pages/missing.png"),
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, content := range tt.files {
				full := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(full, content, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatalf("OpenRoot: %v", err)
			}
			defer root.Close()

			got, err := ResolveProjectAssets(root, "pages", tt.cfg, tt.repoName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveProjectAssets() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResolveProjectAssets() =\n%+v\nwant:\n%+v", got, tt.want)
			}
		})
	}
}

func TestResolveProjectAssetsPreviewSymlinkRejection(t *testing.T) {
	dir := t.TempDir()
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := filepath.Join(dir, "real.png")
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(pagesDir, "demo-still.png")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer root.Close()

	cfg := &config.Config{
		Social: &model.SocialSettings{
			OG: &model.SocialOGSettings{
				IncludePreview: new(true),
			},
		},
	}
	if _, err := ResolveProjectAssets(root, "pages", cfg, "wimpysworld/tailor"); err == nil {
		t.Fatal("ResolveProjectAssets() expected error for symlink demo-still.png, got nil")
	}
}
