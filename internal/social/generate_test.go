package social

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type mockLiveFetcher struct {
	stats            StatsData
	statsErr         error
	stars            []StarPoint
	starsErr         error
	sponsors         SponsorsData
	sponsorsErr      error
	discussionsCount int
	discussionsErr   error
	discordOnline    int
	discordMembers   int
	discordOK        bool
	discourseMembers int
	discourseOK      bool

	statsCalls       int
	starsCalls       int
	sponsorsCalls    int
	sponsorLoginSeen string
	discussionsCalls int
	discordCodesSeen []string
	discourseURLSeen string
}

func (m *mockLiveFetcher) FetchRepoStats(_ context.Context, _, _ string) (StatsData, error) {
	m.statsCalls++
	return m.stats, m.statsErr
}

func (m *mockLiveFetcher) FetchStarHistory(_ context.Context, _, _ string) ([]StarPoint, error) {
	m.starsCalls++
	return m.stars, m.starsErr
}

func (m *mockLiveFetcher) FetchSponsors(_ context.Context, login string) (SponsorsData, error) {
	m.sponsorsCalls++
	m.sponsorLoginSeen = login
	return m.sponsors, m.sponsorsErr
}

func (m *mockLiveFetcher) FetchDiscussionsCount(_ context.Context, _, _ string) (int, error) {
	m.discussionsCalls++
	return m.discussionsCount, m.discussionsErr
}

func (m *mockLiveFetcher) FetchDiscordInvite(_ context.Context, inviteCode string) (int, int, bool) {
	m.discordCodesSeen = append(m.discordCodesSeen, inviteCode)
	return m.discordOnline, m.discordMembers, m.discordOK
}

func (m *mockLiveFetcher) FetchDiscourseStats(_ context.Context, baseURL string) (int, bool) {
	m.discourseURLSeen = baseURL
	return m.discourseMembers, m.discourseOK
}

func writeTestFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", full, err)
	}
}

func sampleSocialConfigYAML() string {
	return `license: BlueOak-1.0.0
repository:
  description: "Bespoke project templates for GitHub repositories. 🪡"
  has_discussions: true
pages:
  enabled: true
  generator: static
  path: pages
  cname: tailor.wimpys.world
  links:
    website: https://tailor.wimpys.world
    bluesky: https://bsky.app/profile/wimpys.world
    discord: https://discord.gg/tailor123
    github_sponsors: https://github.com/sponsors/flexiondotorg
    mastodon: https://wimpysworld.social/@martin
    youtube: https://youtube.com/@wimpysworld
social:
  enabled: true
  theme: dark
  platforms: "Linux · macOS · Windows"
  readme:
    header: true
    community: auto
    packages:
      - apt
      - nix
    stats: true
    star_history:
      enabled: true
      min_stars: 25
      min_days: 30
    sponsors: auto
swatches: []
`
}

func sampleStyleCSS() string {
	return `:root[data-theme="dark"] {
  --mu-background-color: #14121d;
  --mu-card-background-color: #1c1927;
  --mu-primary: #a78bfa;
}
:root[data-theme="light"] {
  --mu-background-color: #faf9fe;
  --mu-primary: #6d28d9;
}
`
}

func sampleIconSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" fill="#a78bfa"/></svg>`
}

func sampleReadmeWithMarkers() string {
	return `# Tailor

<!-- tailor:social-header:start -->
<!-- tailor:social-header:end -->

Authored intro paragraph.

<!-- tailor:social-links:start -->
<!-- tailor:social-links:end -->

<!-- tailor:social-stats:start -->
existing stats block
<!-- tailor:social-stats:end -->

<!-- tailor:social-stars:start -->
<!-- tailor:social-stars:end -->

<!-- tailor:social-community:start -->
<!-- tailor:social-community:end -->

<!-- tailor:social-packages:start -->
<!-- tailor:social-packages:end -->

<!-- tailor:social-sponsors:start -->
<!-- tailor:social-sponsors:end -->

Footer notes.
`
}

func statusMap(res GenerateResult) map[string]AssetStatus {
	out := make(map[string]AssetStatus, len(res.Assets))
	for _, a := range res.Assets {
		out[a.Path] = a.Status
	}
	return out
}

func TestGenerateStaticOfflineAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
	writeTestFixtureFile(t, dir, "pages/style.css", sampleStyleCSS())
	writeTestFixtureFile(t, dir, "pages/icon.svg", sampleIconSVG())
	writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())

	// Neither Static nor Live set -> defaults to Static-only, works completely offline (Fetcher: nil).
	res, err := Generate(context.Background(), GenerateOptions{
		Dir:          dir,
		UpdateReadme: true,
		Owner:        "wimpysworld",
		Repo:         "tailor",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	got := statusMap(res)
	// First 3 non-community, non-funding links: website, bluesky, mastodon (youtube is 4th, discord is community, github_sponsors is funding).
	wantCreated := []string{
		"pages/social/hero-dark.svg",
		"pages/social/hero-light.svg",
		"pages/social/link-website-dark.svg",
		"pages/social/link-website-light.svg",
		"pages/social/link-bluesky-dark.svg",
		"pages/social/link-bluesky-light.svg",
		"pages/social/link-mastodon-dark.svg",
		"pages/social/link-mastodon-light.svg",
		"pages/social/community-dark.svg",
		"pages/social/community-light.svg",
		"pages/social/pkg-apt-dark.svg",
		"pages/social/pkg-apt-light.svg",
		"pages/social/pkg-nix-dark.svg",
		"pages/social/pkg-nix-light.svg",
		"pages/social/sponsors-dark.svg",
		"pages/social/sponsors-light.svg",
	}
	for _, p := range wantCreated {
		if got[p] != StatusCreated {
			t.Errorf("asset %s status = %q, want %q", p, got[p], StatusCreated)
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", p, err)
		}
		assertValidGitHubSVG(t, p, string(data))
	}
	if _, ok := got["pages/social/link-youtube-dark.svg"]; ok {
		t.Error("expected default links to cap at 3 non-community/non-funding links")
	}
	if _, ok := got["pages/social/link-discord-dark.svg"]; ok {
		t.Error("community link discord must not appear in default preferred links")
	}
	if _, ok := got["pages/social/link-github_sponsors-dark.svg"]; ok {
		t.Error("funding link github_sponsors must not appear in default preferred links")
	}
	if got["README.md"] != StatusUpdated {
		t.Errorf("README.md status = %q, want %q", got["README.md"], StatusUpdated)
	}

	// Verify hero contains extracted theme colour and stripped tagline (no trailing needle emoji).
	heroDark, _ := os.ReadFile(filepath.Join(dir, "pages/social/hero-dark.svg"))
	if !strings.Contains(string(heroDark), "#a78bfa") {
		t.Error("hero-dark.svg missing extracted dark primary colour #a78bfa")
	}
	if strings.Contains(string(heroDark), "🪡") {
		t.Error("hero-dark.svg did not strip trailing emoji from tagline")
	}

	// Verify README updated static markers (including static sponsors CTA) and left existing stats marker untouched.
	readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	readme := string(readmeBytes)
	if !strings.Contains(readme, `srcset="pages/social/hero-dark.svg"`) ||
		!strings.Contains(readme, `srcset="pages/social/link-mastodon-dark.svg"`) ||
		!strings.Contains(readme, `srcset="pages/social/pkg-apt-dark.svg"`) ||
		!strings.Contains(readme, `srcset="pages/social/sponsors-dark.svg"`) ||
		!strings.Contains(readme, `href="https://github.com/sponsors/flexiondotorg"`) {
		t.Errorf("README missing expected static marker blocks:\n%s", readme)
	}
	if !strings.Contains(readme, "existing stats block") {
		t.Errorf("static run clobbered existing live stats marker:\n%s", readme)
	}

	// Second run must report every file unchanged.
	res2, err := Generate(context.Background(), GenerateOptions{
		Dir:          dir,
		UpdateReadme: true,
		Owner:        "wimpysworld",
		Repo:         "tailor",
	})
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	for _, asset := range res2.Assets {
		if asset.Status != StatusUnchanged {
			t.Errorf("second run %s status = %q, want %q", asset.Path, asset.Status, StatusUnchanged)
		}
	}

	// Changing stylesheet causes SVGs to report StatusUpdated.
	writeTestFixtureFile(t, dir, "pages/style.css", `:root[data-theme="dark"] { --mu-primary: #22d3ee; }`)
	res3, err := Generate(context.Background(), GenerateOptions{
		Dir:   dir,
		Owner: "wimpysworld",
		Repo:  "tailor",
	})
	if err != nil {
		t.Fatalf("third Generate() error = %v", err)
	}
	if statusMap(res3)["pages/social/hero-dark.svg"] != StatusUpdated {
		t.Errorf("hero-dark.svg status after theme change = %q, want %q", statusMap(res3)["pages/social/hero-dark.svg"], StatusUpdated)
	}
}

func TestGenerateLiveToCustomOutputDirAndUpdateReadmeWithSHA(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "social-assets")
	writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
	writeTestFixtureFile(t, dir, "pages/style.css", sampleStyleCSS())
	writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fetcher := &mockLiveFetcher{
		stats: StatsData{
			Stars:        420,
			Release:      "v2.1.0",
			Language:     "Go",
			License:      "BlueOak-1.0.0",
			Contributors: 14,
		},
		stars: []StarPoint{
			{Date: start, Stars: 10},
			{Date: start.AddDate(0, 1, 0), Stars: 180},
			{Date: start.AddDate(0, 3, 0), Stars: 420},
		},
		sponsors: SponsorsData{
			Enabled: true,
			Handles: []string{"alice", "bob"},
		},
		discordOnline:  45,
		discordMembers: 320,
		discordOK:      true,
	}

	const sha = "deadbeef1234567890"
	res, err := Generate(context.Background(), GenerateOptions{
		Dir:          dir,
		Live:         true,
		OutputDir:    outDir,
		UpdateReadme: true,
		AssetsSHA:    sha,
		Owner:        "wimpysworld",
		Repo:         "tailor",
		Fetcher:      fetcher,
	})
	if err != nil {
		t.Fatalf("Generate(live) error = %v", err)
	}

	got := statusMap(res)
	wantFiles := []string{
		"stats-dark.svg", "stats-light.svg",
		"stars-dark.svg", "stars-light.svg",
		"sponsors-dark.svg", "sponsors-light.svg",
		"community-dark.svg", "community-light.svg",
	}
	for _, name := range wantFiles {
		full := filepath.Join(outDir, name)
		if got[full] != StatusCreated {
			t.Errorf("live asset %s status = %q, want %q", full, got[full], StatusCreated)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", full, err)
		}
		assertValidGitHubSVG(t, name, string(data))
	}

	// Verify static hero was NOT generated in live-only mode.
	if _, err := os.Stat(filepath.Join(outDir, "hero-dark.svg")); !os.IsNotExist(err) {
		t.Error("live-only mode must not write hero-dark.svg")
	}

	// Verify live counts were fetched and rendered.
	if fetcher.sponsorLoginSeen != "flexiondotorg" {
		t.Errorf("sponsor login = %q, want flexiondotorg from pages.links.github_sponsors", fetcher.sponsorLoginSeen)
	}
	if len(fetcher.discordCodesSeen) != 1 || fetcher.discordCodesSeen[0] != "tailor123" {
		t.Errorf("discord invite codes seen = %v, want [tailor123]", fetcher.discordCodesSeen)
	}
	commDark, _ := os.ReadFile(filepath.Join(outDir, "community-dark.svg"))
	if !strings.Contains(string(commDark), "45 online") || !strings.Contains(string(commDark), "320 members") {
		t.Errorf("community-dark.svg missing Discord live counts:\n%s", string(commDark))
	}

	// Verify README live markers use commit-pinned URLs and static markers stay untouched.
	readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	readme := string(readmeBytes)
	const prefix = "https://raw.githubusercontent.com/wimpysworld/tailor/" + sha + "/"
	for _, want := range []string{
		prefix + "stats-dark.svg",
		prefix + "stars-dark.svg",
		prefix + "sponsors-dark.svg",
		prefix + "community-dark.svg",
		`href="https://github.com/sponsors/flexiondotorg"`,
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README missing %q:\n%s", want, readme)
		}
	}
	// Static header marker must remain empty because Static was false.
	if strings.Contains(readme, "hero-dark.svg") {
		t.Errorf("live-only run should not populate static header marker:\n%s", readme)
	}
}

func TestGenerateLiveThresholdsAndSkips(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "live-out")
	writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
	writeTestFixtureFile(t, dir, "README.md", "<!-- tailor:social-stars:start -->\nstale stars\n<!-- tailor:social-stars:end -->\n")

	fetcher := &mockLiveFetcher{
		stats:    StatsData{Stars: 10, Language: "Go"},
		stars:    []StarPoint{{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Stars: 10}}, // below min_stars=25 and min_days=30
		sponsors: SponsorsData{Enabled: false},
	}

	res, err := Generate(context.Background(), GenerateOptions{
		Dir:          dir,
		Live:         true,
		OutputDir:    outDir,
		UpdateReadme: true,
		AssetsSHA:    "abcdef1",
		Owner:        "wimpysworld",
		Repo:         "tailor",
		Fetcher:      fetcher,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	got := statusMap(res)
	if got[filepath.Join(outDir, "stars-dark.svg")] != StatusCreated {
		t.Errorf("stars-dark.svg status = %q, want created (Star CTA fallback)", got[filepath.Join(outDir, "stars-dark.svg")])
	}
	if got[filepath.Join(outDir, "sponsors-dark.svg")] != StatusSkipped {
		t.Errorf("sponsors-dark.svg status = %q, want skipped", got[filepath.Join(outDir, "sponsors-dark.svg")])
	}
	starsData, err := os.ReadFile(filepath.Join(outDir, "stars-dark.svg"))
	if err != nil {
		t.Fatalf("ReadFile(stars-dark.svg): %v", err)
	}
	assertValidGitHubSVG(t, "stars-cta-dark", string(starsData))
	assertSVGDimensions(t, "stars-cta-dark", string(starsData), 840, 108)
	if !strings.Contains(string(starsData), "Star Tailor on GitHub") || !strings.Contains(string(starsData), "STAR") {
		t.Errorf("stars-dark.svg missing Star CTA copy:\n%s", string(starsData))
	}

	// Stars marker in README should now point to the commit-pinned Star CTA SVG.
	readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	wantURL := "https://raw.githubusercontent.com/wimpysworld/tailor/abcdef1/stars-dark.svg"
	if !strings.Contains(string(readmeBytes), wantURL) {
		t.Errorf("expected stars marker to contain %q:\n%s", wantURL, string(readmeBytes))
	}
}

func TestGenerateDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "does-not-exist-yet")
	writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
	origReadme := sampleReadmeWithMarkers()
	writeTestFixtureFile(t, dir, "README.md", origReadme)

	res, err := Generate(context.Background(), GenerateOptions{
		Dir:          dir,
		Static:       true,
		DryRun:       true,
		OutputDir:    outDir,
		UpdateReadme: true,
		Owner:        "wimpysworld",
		Repo:         "tailor",
	})
	if err != nil {
		t.Fatalf("Generate(dry-run) error = %v", err)
	}
	if !res.DryRun {
		t.Error("expected GenerateResult.DryRun = true")
	}
	if len(res.Assets) == 0 {
		t.Fatal("expected planned assets in dry-run result")
	}

	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Errorf("dry-run must not create output directory %s", outDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages", "social")); !os.IsNotExist(err) {
		t.Error("dry-run must not create pages/social")
	}
	readmeAfter, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if string(readmeAfter) != origReadme {
		t.Error("dry-run must not modify README.md")
	}
}

func TestGenerateRejectsSymlinksAndBadSHA(t *testing.T) {
	dir := t.TempDir()
	writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())

	if _, err := Generate(context.Background(), GenerateOptions{
		Dir:       dir,
		AssetsSHA: "../escape",
	}); err == nil {
		t.Error("expected error for invalid AssetsSHA")
	}

	if runtime.GOOS == "windows" {
		return
	}
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "pages", "social")); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(context.Background(), GenerateOptions{
		Dir:   dir,
		Owner: "wimpysworld",
		Repo:  "tailor",
	}); err == nil {
		t.Error("expected error when pages/social is a symlink")
	}
}

func TestGenerateDiscussionsAndForumAndPreferredLinks(t *testing.T) {
	t.Run("explicit preferred_links and live discussions count", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(t.TempDir(), "live")
		writeTestFixtureFile(t, dir, ".tailor.yml", `license: BlueOak-1.0.0
repository:
  description: "Test project"
  has_discussions: true
pages:
  enabled: true
  generator: static
  path: pages
  links:
    website: https://example.com
    youtube: https://youtube.com/@example
    mastodon: https://mastodon.social/@example
social:
  enabled: true
  readme:
    header: false
    preferred_links:
      - youtube
      - website
    community: discussions
    stats: false
    star_history:
      enabled: false
    sponsors: false
swatches: []
`)
		fetcher := &mockLiveFetcher{discussionsCount: 73}
		res, err := Generate(context.Background(), GenerateOptions{
			Dir:       dir,
			Static:    true,
			Live:      true,
			OutputDir: outDir,
			Owner:     "wimpysworld",
			Repo:      "tailor",
			Fetcher:   fetcher,
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		got := statusMap(res)
		if got[filepath.Join(outDir, "link-youtube-dark.svg")] != StatusCreated {
			t.Error("expected link-youtube-dark.svg from preferred_links")
		}
		if got[filepath.Join(outDir, "link-website-dark.svg")] != StatusCreated {
			t.Error("expected link-website-dark.svg from preferred_links")
		}
		if _, ok := got[filepath.Join(outDir, "link-mastodon-dark.svg")]; ok {
			t.Error("mastodon was not in preferred_links")
		}
		if fetcher.discussionsCalls != 1 {
			t.Errorf("discussionsCalls = %d, want 1", fetcher.discussionsCalls)
		}
		if fetcher.sponsorsCalls != 0 {
			t.Errorf("sponsorsCalls = %d, want 0 when sponsors=false", fetcher.sponsorsCalls)
		}
		commDark, _ := os.ReadFile(filepath.Join(outDir, "community-dark.svg"))
		if !strings.Contains(string(commDark), "73 discussions") {
			t.Errorf("community-dark.svg missing discussions count:\n%s", string(commDark))
		}
	})

	t.Run("live forum community uses Discourse stats with graceful fallback", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(t.TempDir(), "forum-live")
		writeTestFixtureFile(t, dir, ".tailor.yml", `license: BlueOak-1.0.0
repository:
  description: "Forum project"
pages:
  enabled: true
  generator: static
  path: pages
  links:
    forum: https://forum.example.org
social:
  enabled: true
  readme:
    header: false
    community: forum
    stats: false
    star_history:
      enabled: false
    sponsors: "true"
swatches: []
`)
		fetcher := &mockLiveFetcher{
			discourseMembers: 910,
			discourseOK:      true,
			sponsors:         SponsorsData{Enabled: false, Handles: nil},
		}
		res, err := Generate(context.Background(), GenerateOptions{
			Dir:       dir,
			Live:      true,
			OutputDir: outDir,
			Owner:     "wimpysworld",
			Repo:      "tailor",
			Fetcher:   fetcher,
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if fetcher.discourseURLSeen != "https://forum.example.org" {
			t.Errorf("discourseURLSeen = %q, want https://forum.example.org", fetcher.discourseURLSeen)
		}
		commDark, _ := os.ReadFile(filepath.Join(outDir, "community-dark.svg"))
		if !strings.Contains(string(commDark), "910 members") {
			t.Errorf("community-dark.svg missing Discourse members:\n%s", string(commDark))
		}
		// sponsors: true must render even with 0 handles.
		if statusMap(res)[filepath.Join(outDir, "sponsors-dark.svg")] != StatusCreated {
			t.Error("expected sponsors-dark.svg when sponsors: true even with 0 handles")
		}
	})
}
