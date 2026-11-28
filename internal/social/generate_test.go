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
	stats             StatsData
	statsErr          error
	stars             []StarPoint
	starsErr          error
	sponsors          SponsorsData
	sponsorsErr       error
	contributors      []Contributor
	contributorsTotal int
	contributorsErr   error
	discussionsCount  int
	discussionsErr    error
	discordOnline     int
	discordMembers    int
	discordOK         bool
	discourseMembers  int
	discourseOK       bool

	statsCalls        int
	starsCalls        int
	sponsorsCalls     int
	contributorsCalls int
	sponsorLoginSeen  string
	discussionsCalls  int
	discordCodesSeen  []string
	discourseURLSeen  string
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

func (m *mockLiveFetcher) FetchContributors(_ context.Context, _, _ string) ([]Contributor, int, error) {
	m.contributorsCalls++
	return m.contributors, m.contributorsTotal, m.contributorsErr
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

<!-- tailor:social-demo:start -->
<!-- tailor:social-demo:end -->

<!-- tailor:social-sponsors:start -->
<!-- tailor:social-sponsors:end -->

<!-- tailor:social-contributors:start -->
<!-- tailor:social-contributors:end -->

<!-- tailor:social-footer:start -->
<!-- tailor:social-footer:end -->

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
		"pages/social/footer-dark.svg",
		"pages/social/footer-light.svg",
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

	// Verify README links all enabled markers to branch-based raw.githubusercontent.com URLs
	// when owner and repo are provided.
	readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	readme := string(readmeBytes)
	const liveBranchPrefix = "https://raw.githubusercontent.com/wimpysworld/tailor/social-assets/"
	if !strings.Contains(readme, `srcset="`+liveBranchPrefix+`hero-dark.svg"`) ||
		!strings.Contains(readme, `srcset="`+liveBranchPrefix+`link-mastodon-dark.svg"`) ||
		!strings.Contains(readme, `srcset="`+liveBranchPrefix+`pkg-apt-dark.svg"`) ||
		!strings.Contains(readme, `srcset="`+liveBranchPrefix+`sponsors-dark.svg"`) ||
		!strings.Contains(readme, `href="https://github.com/sponsors/flexiondotorg"`) ||
		!strings.Contains(readme, `srcset="`+liveBranchPrefix+`footer-dark.svg"`) ||
		!strings.Contains(readme, `href="https://tailor.wimpys.world"`) {
		t.Errorf("README missing expected branch-based social marker blocks:\n%s", readme)
	}
	if strings.Contains(readme, "demo-dark.svg") {
		t.Error("README demo marker must stay untouched when no demo image exists")
	}
	for _, want := range []string{
		`srcset="` + liveBranchPrefix + `stats-dark.svg"`,
		`src="` + liveBranchPrefix + `stats-light.svg"`,
		`href="https://github.com/wimpysworld/tailor/pulse"`,
		`srcset="` + liveBranchPrefix + `stars-dark.svg"`,
		`href="https://github.com/wimpysworld/tailor/stargazers"`,
		`srcset="` + liveBranchPrefix + `contributors-dark.svg"`,
		`href="https://github.com/wimpysworld/tailor/graphs/contributors"`,
		`srcset="` + liveBranchPrefix + `community-dark.svg"`,
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README missing branch-based live marker %q:\n%s", want, readme)
		}
	}
	if strings.Contains(readme, "existing stats block") {
		t.Errorf("static run with owner/repo should replace placeholder stats marker:\n%s", readme)
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

func TestGenerateStaticReadmeBranchAndNoRepoBehavior(t *testing.T) {
	t.Run("custom live branch is used in static README update", func(t *testing.T) {
		dir := t.TempDir()
		cfgYAML := strings.Replace(sampleSocialConfigYAML(), "    sponsors: auto", "    sponsors: auto\n  live:\n    branch: live-social", 1)
		writeTestFixtureFile(t, dir, ".tailor.yml", cfgYAML)
		writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())

		if _, err := Generate(context.Background(), GenerateOptions{
			Dir:          dir,
			Static:       true,
			UpdateReadme: true,
			Owner:        "wimpysworld",
			Repo:         "tailor",
		}); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
		readme := string(readmeBytes)
		const prefix = "https://raw.githubusercontent.com/wimpysworld/tailor/live-social/"
		for _, want := range []string{
			prefix + "stats-dark.svg",
			prefix + "stars-dark.svg",
			prefix + "contributors-dark.svg",
			prefix + "community-dark.svg",
		} {
			if !strings.Contains(readme, want) {
				t.Errorf("README missing custom live branch URL %q:\n%s", want, readme)
			}
		}
	})

	t.Run("without owner/repo leaves live markers untouched and uses static community", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
		writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())

		if _, err := Generate(context.Background(), GenerateOptions{
			Dir:          dir,
			Static:       true,
			UpdateReadme: true,
		}); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
		readme := string(readmeBytes)
		if !strings.Contains(readme, "existing stats block") {
			t.Errorf("static run without owner/repo clobbered existing stats marker:\n%s", readme)
		}
		if !strings.Contains(readme, `srcset="pages/social/community-dark.svg"`) {
			t.Errorf("static run without owner/repo should use static community path:\n%s", readme)
		}
		if strings.Contains(readme, "raw.githubusercontent.com") {
			t.Errorf("static run without owner/repo must not write raw.githubusercontent.com URLs:\n%s", readme)
		}
	})
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
			Commits:      750,
			OpenIssues:   2,
			ClosedIssues: 41,
			OpenPRs:      0,
			MergedPRs:    180,
			CreatedAt:    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
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
		contributors: []Contributor{
			{Login: "flexiondotorg"},
			{Login: "alice"},
		},
		contributorsTotal: 2,
		discordOnline:     45,
		discordMembers:    320,
		discordOK:         true,
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
		"contributors-dark.svg", "contributors-light.svg",
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

	// Verify static hero and static sponsors were NOT generated in live-only mode.
	if _, err := os.Stat(filepath.Join(outDir, "hero-dark.svg")); !os.IsNotExist(err) {
		t.Error("live-only mode must not write hero-dark.svg")
	}
	if _, err := os.Stat(filepath.Join(outDir, "sponsors-dark.svg")); !os.IsNotExist(err) {
		t.Error("live-only mode must not write sponsors-dark.svg")
	}
	if fetcher.sponsorsCalls != 0 {
		t.Errorf("sponsorsCalls = %d, want 0 (sponsors is static)", fetcher.sponsorsCalls)
	}

	// Verify live counts were fetched and rendered.
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
		`href="https://github.com/wimpysworld/tailor/pulse"`,
		prefix + "stars-dark.svg",
		prefix + "contributors-dark.svg",
		`href="https://github.com/wimpysworld/tailor/graphs/contributors"`,
		prefix + "community-dark.svg",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README missing %q:\n%s", want, readme)
		}
	}
	// Static header and sponsors markers must remain untouched because Static was false.
	if strings.Contains(readme, "hero-dark.svg") || strings.Contains(readme, "sponsors-dark.svg") {
		t.Errorf("live-only run should not populate static header or sponsors markers:\n%s", readme)
	}

	// Without AssetsSHA, live UpdateReadme falls back to branch-based URLs.
	resBranch, err := Generate(context.Background(), GenerateOptions{
		Dir:          dir,
		Live:         true,
		OutputDir:    outDir,
		UpdateReadme: true,
		Owner:        "wimpysworld",
		Repo:         "tailor",
		Fetcher:      fetcher,
	})
	if err != nil {
		t.Fatalf("Generate(live without SHA) error = %v", err)
	}
	if statusMap(resBranch)["README.md"] != StatusUpdated {
		t.Errorf("README.md status = %q, want %q", statusMap(resBranch)["README.md"], StatusUpdated)
	}
	readmeBranch, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	const branchPrefix = "https://raw.githubusercontent.com/wimpysworld/tailor/social-assets/"
	for _, want := range []string{
		branchPrefix + "stats-dark.svg",
		branchPrefix + "stars-dark.svg",
		branchPrefix + "contributors-dark.svg",
		branchPrefix + "community-dark.svg",
	} {
		if !strings.Contains(string(readmeBranch), want) {
			t.Errorf("README missing branch-based live URL %q:\n%s", want, string(readmeBranch))
		}
	}
}

func TestGenerateLiveThresholdsAndSkips(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "live-out")
	writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
	writeTestFixtureFile(t, dir, "README.md", "<!-- tailor:social-stars:start -->\nstale stars\n<!-- tailor:social-stars:end -->\n")

	fetcher := &mockLiveFetcher{
		stats:    StatsData{Commits: 10, CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
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
	if _, ok := got[filepath.Join(outDir, "sponsors-dark.svg")]; ok {
		t.Errorf("sponsors-dark.svg must not be emitted in live-only mode, got status %q", got[filepath.Join(outDir, "sponsors-dark.svg")])
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
		// sponsors is static-only, so live-only mode does not fetch or write sponsors-dark.svg.
		if fetcher.sponsorsCalls != 0 {
			t.Errorf("sponsorsCalls = %d, want 0 (sponsors is static)", fetcher.sponsorsCalls)
		}
		if _, ok := statusMap(res)[filepath.Join(outDir, "sponsors-dark.svg")]; ok {
			t.Error("live-only mode must not emit sponsors-dark.svg")
		}
	})
}

func TestGenerateStaticDemoAndFooterOptions(t *testing.T) {
	t.Run("embeds demo.webp when within 512 KiB", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
		writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pages", "demo.webp"), makeTestWebPVP8X(960, 540), 0o644); err != nil {
			t.Fatal(err)
		}

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
		for _, p := range []string{"pages/social/demo-dark.svg", "pages/social/demo-light.svg"} {
			if got[p] != StatusCreated {
				t.Errorf("%s status = %q, want %q", p, got[p], StatusCreated)
			}
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", p, err)
			}
			assertValidGitHubSVG(t, p, string(data))
			assertSVGDimensions(t, p, string(data), 840, 509)
		}
		readme, _ := os.ReadFile(filepath.Join(dir, "README.md"))
		const prefix = "https://raw.githubusercontent.com/wimpysworld/tailor/social-assets/"
		if !strings.Contains(string(readme), `srcset="`+prefix+`demo-dark.svg"`) ||
			!strings.Contains(string(readme), `src="`+prefix+`demo-light.svg"`) {
			t.Errorf("README missing demo <picture> block:\n%s", string(readme))
		}
	})

	t.Run("falls back to demo-still.png when demo.webp is absent", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
		writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pages", "demo-still.png"), makeTestPNGHeader(840, 420), 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := Generate(context.Background(), GenerateOptions{
			Dir:          dir,
			UpdateReadme: true,
			Owner:        "wimpysworld",
			Repo:         "tailor",
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if statusMap(res)["pages/social/demo-dark.svg"] != StatusCreated {
			t.Fatalf("demo-dark.svg status = %q, want created", statusMap(res)["pages/social/demo-dark.svg"])
		}
		data, _ := os.ReadFile(filepath.Join(dir, "pages/social/demo-dark.svg"))
		assertValidGitHubSVG(t, "demo-png-dark", string(data))
		assertSVGDimensions(t, "demo-png-dark", string(data), 840, 456)
		if !strings.Contains(string(data), "data:image/png;base64,") {
			t.Error("expected embedded PNG data URI in demo-dark.svg")
		}
	})

	t.Run("oversized demo over 512 KiB falls back to plain img tag", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
		writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		oversized := make([]byte, maxDemoEmbedSize+1)
		copy(oversized, makeTestWebPVP8X(960, 540))
		if err := os.WriteFile(filepath.Join(dir, "pages", "demo.webp"), oversized, 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := Generate(context.Background(), GenerateOptions{
			Dir:          dir,
			UpdateReadme: true,
			Owner:        "wimpysworld",
			Repo:         "tailor",
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if statusMap(res)["pages/social/demo-dark.svg"] != StatusSkipped {
			t.Errorf("demo-dark.svg status = %q, want skipped", statusMap(res)["pages/social/demo-dark.svg"])
		}
		if _, err := os.Stat(filepath.Join(dir, "pages/social/demo-dark.svg")); !os.IsNotExist(err) {
			t.Error("oversized demo must not write pages/social/demo-dark.svg")
		}
		readme, _ := os.ReadFile(filepath.Join(dir, "README.md"))
		wantImg := `<img src="pages/demo.webp" alt="Tailor demo">`
		if !strings.Contains(string(readme), wantImg) {
			t.Errorf("README missing plain img fallback %q:\n%s", wantImg, string(readme))
		}
	})

	t.Run("rejects symlink demo image", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink test skipped on windows")
		}
		dir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "secret.webp")
		if err := os.WriteFile(outside, makeTestWebPVP8X(640, 360), 0o644); err != nil {
			t.Fatal(err)
		}
		writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "pages", "demo.webp")); err != nil {
			t.Fatal(err)
		}
		if _, err := Generate(context.Background(), GenerateOptions{
			Dir:   dir,
			Owner: "wimpysworld",
			Repo:  "tailor",
		}); err == nil {
			t.Fatal("expected error for symlink pages/demo.webp")
		}
	})

	t.Run("disabling demo and footer skips both assets", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFixtureFile(t, dir, ".tailor.yml", `license: BlueOak-1.0.0
pages:
  enabled: true
  path: pages
social:
  enabled: true
  readme:
    header: false
    demo: false
    sponsors: false
    footer: false
swatches: []
`)
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pages", "demo.webp"), makeTestWebPVP8X(640, 360), 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := Generate(context.Background(), GenerateOptions{
			Dir:   dir,
			Owner: "wimpysworld",
			Repo:  "tailor",
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		got := statusMap(res)
		if _, ok := got["pages/social/demo-dark.svg"]; ok {
			t.Error("demo-dark.svg should not be generated when demo: false")
		}
		if _, ok := got["pages/social/footer-dark.svg"]; ok {
			t.Error("footer-dark.svg should not be generated when footer: false")
		}
	})
}

func TestGenerateStaticAndLiveCombinedOutputDir(t *testing.T) {
	t.Run("writes static and live SVGs together with live community precedence", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(t.TempDir(), "social-assets")
		writeTestFixtureFile(t, dir, ".tailor.yml", sampleSocialConfigYAML())
		writeTestFixtureFile(t, dir, "pages/style.css", sampleStyleCSS())
		writeTestFixtureFile(t, dir, "pages/icon.svg", sampleIconSVG())
		writeTestFixtureFile(t, dir, "README.md", sampleReadmeWithMarkers())
		if err := os.WriteFile(filepath.Join(dir, "pages", "demo.webp"), makeTestWebPVP8X(960, 540), 0o644); err != nil {
			t.Fatal(err)
		}

		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		fetcher := &mockLiveFetcher{
			stats: StatsData{
				Commits:      750,
				OpenIssues:   2,
				ClosedIssues: 41,
				OpenPRs:      1,
				MergedPRs:    180,
				CreatedAt:    start,
			},
			stars: []StarPoint{
				{Date: start, Stars: 10},
				{Date: start.AddDate(0, 2, 0), Stars: 120},
			},
			contributors: []Contributor{
				{Login: "flexiondotorg"},
			},
			contributorsTotal: 1,
			discordOnline:     45,
			discordMembers:    320,
			discordOK:         true,
		}

		res, err := Generate(context.Background(), GenerateOptions{
			Dir:          dir,
			Static:       true,
			Live:         true,
			OutputDir:    outDir,
			UpdateReadme: true,
			Owner:        "wimpysworld",
			Repo:         "tailor",
			Fetcher:      fetcher,
		})
		if err != nil {
			t.Fatalf("Generate(static+live) error = %v", err)
		}

		got := statusMap(res)
		wantFiles := []string{
			"hero-dark.svg", "hero-light.svg",
			"link-website-dark.svg", "link-website-light.svg",
			"link-bluesky-dark.svg", "link-bluesky-light.svg",
			"link-mastodon-dark.svg", "link-mastodon-light.svg",
			"community-dark.svg", "community-light.svg",
			"pkg-apt-dark.svg", "pkg-apt-light.svg",
			"pkg-nix-dark.svg", "pkg-nix-light.svg",
			"demo-dark.svg", "demo-light.svg",
			"sponsors-dark.svg", "sponsors-light.svg",
			"footer-dark.svg", "footer-light.svg",
			"stats-dark.svg", "stats-light.svg",
			"stars-dark.svg", "stars-light.svg",
			"contributors-dark.svg", "contributors-light.svg",
		}
		for _, name := range wantFiles {
			full := filepath.Join(outDir, name)
			if got[full] != StatusCreated {
				t.Errorf("combined asset %s status = %q, want %q", full, got[full], StatusCreated)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", full, err)
			}
			assertValidGitHubSVG(t, name, string(data))
		}

		// community-{dark,light}.svg must be written once (live only, not static then overwritten).
		communityCount := 0
		for _, a := range res.Assets {
			if a.Path == filepath.Join(outDir, "community-dark.svg") {
				communityCount++
			}
		}
		if communityCount != 1 {
			t.Errorf("community-dark.svg emitted %d times, want 1", communityCount)
		}
		commDark, _ := os.ReadFile(filepath.Join(outDir, "community-dark.svg"))
		if !strings.Contains(string(commDark), "45 online") || !strings.Contains(string(commDark), "320 members") {
			t.Errorf("community-dark.svg missing live Discord counts:\n%s", string(commDark))
		}

		// All 10 README marker sets resolve to the social-assets branch.
		readmeBytes, _ := os.ReadFile(filepath.Join(dir, "README.md"))
		readme := string(readmeBytes)
		const prefix = "https://raw.githubusercontent.com/wimpysworld/tailor/social-assets/"
		for _, base := range []string{
			"hero", "link-website", "community", "pkg-apt", "demo",
			"stats", "stars", "sponsors", "contributors", "footer",
		} {
			if !strings.Contains(readme, prefix+base+"-dark.svg") || !strings.Contains(readme, prefix+base+"-light.svg") {
				t.Errorf("README missing unified branch URL for %q:\n%s", base, readme)
			}
		}
	})

	t.Run("static community platform still writes community SVG in combined mode", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(t.TempDir(), "social-assets")
		writeTestFixtureFile(t, dir, ".tailor.yml", `license: BlueOak-1.0.0
repository:
  description: "Matrix community project"
pages:
  enabled: true
  generator: static
  path: pages
  links:
    matrix: https://matrix.to/#/#tailor:matrix.org
social:
  enabled: true
  readme:
    header: true
    community: matrix
    stats: true
    star_history:
      enabled: false
    sponsors: false
    contributors: false
    footer: true
swatches: []
`)
		fetcher := &mockLiveFetcher{
			stats: StatsData{Commits: 42, CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		}
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
			t.Fatalf("Generate(static+live matrix) error = %v", err)
		}
		got := statusMap(res)
		for _, name := range []string{"hero-dark.svg", "community-dark.svg", "stats-dark.svg", "footer-dark.svg"} {
			if got[filepath.Join(outDir, name)] != StatusCreated {
				t.Errorf("%s status = %q, want created", name, got[filepath.Join(outDir, name)])
			}
		}
	})
}
