package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/social"
	"github.com/wimpysworld/tailor/internal/testutil"
)

type stubSocialFetcher struct {
	stats             social.StatsData
	stars             []social.StarPoint
	sponsors          social.SponsorsData
	contributors      []social.Contributor
	contributorsTotal int
}

func (s *stubSocialFetcher) FetchRepoStats(_ context.Context, _, _ string) (social.StatsData, error) {
	return s.stats, nil
}

func (s *stubSocialFetcher) FetchStarHistory(_ context.Context, _, _ string) ([]social.StarPoint, error) {
	return s.stars, nil
}

func (s *stubSocialFetcher) FetchSponsors(_ context.Context, _ string) (social.SponsorsData, error) {
	return s.sponsors, nil
}

func (s *stubSocialFetcher) FetchContributors(_ context.Context, _, _ string) ([]social.Contributor, int, error) {
	return s.contributors, s.contributorsTotal, nil
}

func (s *stubSocialFetcher) FetchDiscussionsCount(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

func (s *stubSocialFetcher) FetchDiscordInvite(_ context.Context, _ string) (int, int, bool) {
	return 0, 0, false
}

func (s *stubSocialFetcher) FetchDiscourseStats(_ context.Context, _ string) (int, bool) {
	return 0, false
}

func writeSocialFixture(t *testing.T, dir string) {
	t.Helper()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
repository:
  description: "Bespoke project templates"
pages:
  enabled: true
  generator: static
  path: pages
  links:
    website: https://tailor.wimpys.world
    discord: https://discord.gg/tailor123
social:
  enabled: true
  readme:
    header: true
    community: auto
    packages:
      - apt
    stats: true
    star_history:
      enabled: true
      min_stars: 25
      min_days: 30
    sponsors: auto
swatches: []
`)
	testutil.WriteFile(t, dir, "README.md", `# Tailor

<!-- tailor:social-header:start -->
<!-- tailor:social-header:end -->

<!-- tailor:social-stats:start -->
<!-- tailor:social-stats:end -->

<!-- tailor:social-stars:start -->
<!-- tailor:social-stars:end -->
`)
}

func TestSocialStaticOfflineRequiresNoAuth(t *testing.T) {
	// No GitHub token and no git repository: static generation must succeed offline.
	ghfake.FakeAuth(t, "")
	ghfake.FakeNoRepo(t)

	dir := t.TempDir()
	writeSocialFixture(t, dir)
	t.Chdir(dir)

	var stdout, stderr strings.Builder
	code := run([]string{"social", "--format=plain", "--update-readme"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(social) exit code = %d, stderr:\n%s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"created pages/social/hero-dark.svg",
		"created pages/social/hero-light.svg",
		"created pages/social/link-website-dark.svg",
		"created pages/social/pkg-apt-dark.svg",
		"updated README.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// Second run reports unchanged.
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"social", "--format=plain", "--update-readme"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second run(social) exit code = %d, stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "unchanged pages/social/hero-dark.svg") ||
		!strings.Contains(stdout.String(), "unchanged README.md") {
		t.Errorf("second run expected unchanged status:\n%s", stdout.String())
	}
}

func TestSocialDryRunReportsWouldActionsWithoutWriting(t *testing.T) {
	ghfake.FakeAuth(t, "")
	ghfake.FakeNoRepo(t)

	dir := t.TempDir()
	writeSocialFixture(t, dir)
	t.Chdir(dir)

	var stdout, stderr strings.Builder
	code := run([]string{"social", "--dry-run", "--update-readme", "--format=plain"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(social --dry-run) exit code = %d, stderr:\n%s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "would create pages/social/hero-dark.svg") ||
		!strings.Contains(out, "would update README.md") {
		t.Errorf("dry-run output missing expected would-lines:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages", "social")); !os.IsNotExist(err) {
		t.Error("dry-run must not create pages/social")
	}
}

func TestSocialLiveRequiresGitHubAuth(t *testing.T) {
	ghfake.FakeAuth(t, "")
	ghfake.FakeRepo(t, "wimpysworld", "tailor")

	dir := t.TempDir()
	writeSocialFixture(t, dir)
	t.Chdir(dir)

	var stdout, stderr strings.Builder
	code := run([]string{"social", "--live"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code when --live runs without GitHub auth")
	}
	if !strings.Contains(stderr.String(), "GitHub authentication") {
		t.Errorf("stderr = %q, want GitHub authentication error", stderr.String())
	}
}

func TestSocialLiveCustomOutputDirAndAssetsSHA(t *testing.T) {
	ghfake.FakeRepo(t, "wimpysworld", "tailor")

	dir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "social-assets")
	writeSocialFixture(t, dir)

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var stdout, stderr strings.Builder
	cmd := SocialCmd{
		Live:         true,
		OutputDir:    outDir,
		UpdateReadme: true,
		AssetsSHA:    "1234567890abcdef",
		dir:          dir,
		fetcher: &stubSocialFetcher{
			stats: social.StatsData{Commits: 50, OpenIssues: 1, ClosedIssues: 5, OpenPRs: 0, MergedPRs: 12, CreatedAt: start},
			stars: []social.StarPoint{
				{Date: start, Stars: 5},
				{Date: start.AddDate(0, 2, 0), Stars: 50},
			},
			sponsors:     social.SponsorsData{Enabled: false},
			contributors: []social.Contributor{{Login: "flexiondotorg"}},
		},
		output: output.New(&stdout, &stderr, output.Plain),
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("SocialCmd.Run() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "created "+filepath.Join(outDir, "stats-dark.svg")) ||
		!strings.Contains(out, "created "+filepath.Join(outDir, "stars-dark.svg")) ||
		strings.Contains(out, "sponsors-dark.svg") ||
		!strings.Contains(out, "updated README.md") {
		t.Errorf("unexpected live command output:\n%s", out)
	}

	readmeBytes, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile(README.md): %v", err)
	}
	wantURL := "https://raw.githubusercontent.com/wimpysworld/tailor/1234567890abcdef/stats-dark.svg"
	if !strings.Contains(string(readmeBytes), wantURL) {
		t.Errorf("README.md missing commit-pinned live URL %q:\n%s", wantURL, string(readmeBytes))
	}
}

func TestSocialHelpListsAllFlags(t *testing.T) {
	var stdout, stderr strings.Builder
	code := run([]string{"social", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("social --help exit code = %d, stderr:\n%s", code, stderr.String())
	}
	help := stdout.String()
	for _, flag := range []string{"--static", "--live", "--output-dir", "--update-readme", "--assets-sha", "--dry-run"} {
		if !strings.Contains(help, flag) {
			t.Errorf("social --help missing flag %q:\n%s", flag, help)
		}
	}
}

func TestMeasureCmdChecksSocialOG(t *testing.T) {
	encodePNG := func(width, height int) []byte {
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	tests := []struct {
		name       string
		configYAML string
		ogBytes    []byte
		wantLine   string
		wantAbsent bool
	}{
		{
			name:       "social enabled with valid 1280x640 reports present",
			configYAML: "license: none\nsocial:\n  enabled: true\nswatches: []\n",
			ogBytes:    encodePNG(1280, 640),
			wantLine:   "present:        pages/og.png",
		},
		{
			name:       "pages enabled with wrong dimensions reports warning",
			configYAML: "license: none\npages:\n  enabled: true\nswatches: []\n",
			ogBytes:    encodePNG(640, 480),
			wantLine:   "warning:        pages/og.png (dimensions 640x480; expected 1280x640 or 1200x630)",
		},
		{
			name:       "social enabled with absent og.png emits no og line",
			configYAML: "license: none\nsocial:\n  enabled: true\nswatches: []\n",
			wantAbsent: true,
		},
		{
			name:       "neither social nor pages enabled skips og.png",
			configYAML: "license: none\nswatches: []\n",
			ogBytes:    []byte("not a png"),
			wantAbsent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, tt.configYAML)
			if tt.ogBytes != nil {
				ogPath := filepath.Join(dir, "pages", "og.png")
				if err := os.MkdirAll(filepath.Dir(ogPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(ogPath, tt.ogBytes, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			t.Chdir(dir)
			var stdout, stderr strings.Builder
			cmd := MeasureCmd{output: output.New(&stdout, &stderr, output.Plain)}
			if err := cmd.Run(); err != nil {
				t.Fatalf("MeasureCmd.Run() error = %v", err)
			}
			out := stdout.String()
			if tt.wantAbsent {
				if strings.Contains(out, "pages/og.png") {
					t.Fatalf("unexpected pages/og.png in measure output:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, tt.wantLine) {
				t.Fatalf("measure output missing %q:\n%s", tt.wantLine, out)
			}
		})
	}
}
