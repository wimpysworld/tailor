package social

import (
	"strings"
	"testing"
)

func TestUpdateReadmeMarkers(t *testing.T) {
	t.Run("replaces only declared markers and preserves surrounding prose", func(t *testing.T) {
		readme := `# Tailor

<!-- tailor:social-header:start -->
old hero
<!-- tailor:social-header:end -->

Authored intro paragraph that must remain untouched.

<!-- tailor:social-stats:start -->
old stats
<!-- tailor:social-stats:end -->

## Install

<!-- tailor:social-packages:start -->
<!-- tailor:social-packages:end -->

Footer notes stay intact.
`
		blocks := map[string]string{
			MarkerHeader:   `<p align="center"><picture><source media="(prefers-color-scheme: dark)" srcset="pages/social/hero-dark.svg"><img src="pages/social/hero-light.svg" alt="Tailor"></picture></p>`,
			MarkerStats:    `<p align="center"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/wimpysworld/tailor/abc1234/stats-dark.svg"><img src="https://raw.githubusercontent.com/wimpysworld/tailor/abc1234/stats-light.svg" alt="Tailor statistics"></picture></p>`,
			MarkerPackages: `<p align="center"><a href="https://apt.wimpys.world/#tailor"><picture><source media="(prefers-color-scheme: dark)" srcset="pages/social/pkg-apt-dark.svg"><img src="pages/social/pkg-apt-light.svg" alt="Debian and Ubuntu"></picture></a></p>`,
			MarkerSponsors: `<p align="center">not injected because README has no sponsors markers</p>`,
		}

		updated, changed, err := UpdateReadmeMarkers(readme, blocks)
		if err != nil {
			t.Fatalf("UpdateReadmeMarkers() error = %v", err)
		}
		if !changed {
			t.Fatal("expected changed = true on first update")
		}

		if strings.Contains(updated, "old hero") || strings.Contains(updated, "old stats") {
			t.Errorf("old marker contents were not replaced:\n%s", updated)
		}
		if !strings.Contains(updated, "Authored intro paragraph that must remain untouched.") {
			t.Error("surrounding prose was modified")
		}
		if !strings.Contains(updated, "Footer notes stay intact.") {
			t.Error("trailing prose was modified")
		}
		if strings.Contains(updated, "not injected") {
			t.Error("blocks without existing markers must not be injected")
		}
		if !strings.Contains(updated, "pages/social/hero-dark.svg") ||
			!strings.Contains(updated, "abc1234/stats-dark.svg") ||
			!strings.Contains(updated, "pages/social/pkg-apt-dark.svg") {
			t.Errorf("expected updated marker contents in output:\n%s", updated)
		}

		// Idempotency check.
		second, changed2, err := UpdateReadmeMarkers(updated, blocks)
		if err != nil {
			t.Fatalf("second UpdateReadmeMarkers() error = %v", err)
		}
		if changed2 {
			t.Error("expected changed = false on idempotent second update")
		}
		if second != updated {
			t.Errorf("second pass altered output:\n%s", second)
		}
	})

	t.Run("README without markers is untouched", func(t *testing.T) {
		readme := "# Plain project\n\nNo markers here at all.\n"
		blocks := FormatReadmeBlocks(ReadmeBlocksInput{
			ProjectTitle: "Plain",
			Header:       true,
			Stats:        true,
		})
		updated, changed, err := UpdateReadmeMarkers(readme, blocks)
		if err != nil {
			t.Fatalf("UpdateReadmeMarkers() error = %v", err)
		}
		if changed {
			t.Error("expected changed = false when README has no markers")
		}
		if updated != readme {
			t.Error("README without markers was modified")
		}
	})

	t.Run("empty block clears existing marker cleanly and idempotently", func(t *testing.T) {
		readme := "Before\n<!-- tailor:social-stars:start -->\n<p>old stars</p>\n<!-- tailor:social-stars:end -->\nAfter\n"
		updated, changed, err := UpdateReadmeMarkers(readme, map[string]string{MarkerStars: ""})
		if err != nil {
			t.Fatalf("UpdateReadmeMarkers() error = %v", err)
		}
		if !changed {
			t.Fatal("expected changed = true when clearing block")
		}
		want := "Before\n<!-- tailor:social-stars:start -->\n<!-- tailor:social-stars:end -->\nAfter\n"
		if updated != want {
			t.Errorf("cleared output = %q, want %q", updated, want)
		}
		second, changed2, err := UpdateReadmeMarkers(updated, map[string]string{MarkerStars: ""})
		if err != nil || changed2 || second != want {
			t.Errorf("second clear was not idempotent: changed=%v err=%v out=%q", changed2, err, second)
		}
	})

	t.Run("preserves CRLF line endings and marker indentation", func(t *testing.T) {
		readme := "Intro\r\n  <!-- tailor:social-links:start -->\r\n  old\r\n  <!-- tailor:social-links:end -->\r\nOutro\r\n"
		updated, changed, err := UpdateReadmeMarkers(readme, map[string]string{MarkerLinks: "line1\nline2"})
		if err != nil {
			t.Fatalf("UpdateReadmeMarkers() error = %v", err)
		}
		if !changed {
			t.Fatal("expected changed = true")
		}
		want := "Intro\r\n  <!-- tailor:social-links:start -->\r\nline1\r\nline2\r\n  <!-- tailor:social-links:end -->\r\nOutro\r\n"
		if updated != want {
			t.Errorf("CRLF output = %q, want %q", updated, want)
		}
	})

	t.Run("updates all eight canonical marker names", func(t *testing.T) {
		var sb strings.Builder
		blocks := make(map[string]string, len(ReadmeMarkerNames))
		for _, name := range ReadmeMarkerNames {
			sb.WriteString(MarkerStart(name) + "\n")
			sb.WriteString(MarkerEnd(name) + "\n\n")
			blocks[name] = "<p>block-" + name + "</p>"
		}
		updated, changed, err := UpdateReadmeMarkers(sb.String(), blocks)
		if err != nil {
			t.Fatalf("UpdateReadmeMarkers() error = %v", err)
		}
		if !changed {
			t.Fatal("expected changed = true")
		}
		for _, name := range ReadmeMarkerNames {
			if !strings.Contains(updated, "<p>block-"+name+"</p>") {
				t.Errorf("missing updated block for %q", name)
			}
		}
	})

	t.Run("rejects malformed or unknown markers", func(t *testing.T) {
		cases := []struct {
			name    string
			content string
			blocks  map[string]string
		}{
			{
				name:    "unknown marker key",
				content: "# Hi\n",
				blocks:  map[string]string{"bogus": "x"},
			},
			{
				name:    "missing end marker",
				content: "<!-- tailor:social-header:start -->\nhello\n",
				blocks:  map[string]string{MarkerHeader: "x"},
			},
			{
				name:    "missing start marker",
				content: "hello\n<!-- tailor:social-header:end -->\n",
				blocks:  map[string]string{MarkerHeader: "x"},
			},
			{
				name:    "duplicate start marker",
				content: "<!-- tailor:social-header:start -->\n<!-- tailor:social-header:start -->\n<!-- tailor:social-header:end -->\n",
				blocks:  map[string]string{MarkerHeader: "x"},
			},
			{
				name:    "reversed markers",
				content: "<!-- tailor:social-header:end -->\n<!-- tailor:social-header:start -->\n",
				blocks:  map[string]string{MarkerHeader: "x"},
			},
			{
				name:    "non-standalone start marker",
				content: "prefix <!-- tailor:social-header:start -->\n<!-- tailor:social-header:end -->\n",
				blocks:  map[string]string{MarkerHeader: "x"},
			},
			{
				name:    "overlapping markers",
				content: "<!-- tailor:social-header:start -->\n<!-- tailor:social-stats:start -->\n<!-- tailor:social-header:end -->\n<!-- tailor:social-stats:end -->\n",
				blocks:  map[string]string{MarkerHeader: "x"},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if _, _, err := UpdateReadmeMarkers(tc.content, tc.blocks); err == nil {
					t.Errorf("%s: expected error, got nil", tc.name)
				}
			})
		}
	})
}

func TestFormatReadmeBlocks(t *testing.T) {
	in := ReadmeBlocksInput{
		StaticDir:    "pages/social",
		Owner:        "wimpysworld",
		Repo:         "tailor",
		LiveSHA:      "deadbeef1234567890",
		ProjectTitle: "Tailor",
		Header:       true,
		HeaderURL:    "https://tailor.wimpys.world",
		Links: []ReadmeLinkEntry{
			{Key: "website", URL: "https://tailor.wimpys.world"},
			{Key: "bluesky", URL: "https://bsky.app/profile/wimpys.world"},
			{Key: "email", URL: "code@wimpress.io"},
		},
		Community: &ReadmeCommunityEntry{
			Platform: "discord",
			URL:      "https://wimpysworld.io/discord",
		},
		Packages: []ReadmePackageEntry{
			{Host: "apt", Package: "Tailor"},
			{Host: "nix", Package: "tailor"},
			{Host: "brew", URL: "https://brew.wimpys.world/#custom"},
		},
		Demo:         true,
		Stats:        true,
		StarHistory:  true,
		Sponsors:     true,
		SponsorsURL:  "https://github.com/sponsors/flexiondotorg",
		Contributors: true,
		Footer:       true,
	}

	blocks := FormatReadmeBlocks(in)

	if len(blocks) != len(ReadmeMarkerNames) {
		t.Fatalf("FormatReadmeBlocks() returned %d blocks, want %d", len(blocks), len(ReadmeMarkerNames))
	}

	// Verify all 10 README SVG sets resolve to commit-pinned raw.githubusercontent.com URLs when Owner and Repo are set.
	const livePrefix = "https://raw.githubusercontent.com/wimpysworld/tailor/deadbeef1234567890/"

	header := blocks[MarkerHeader]
	wantHeader := `<a href="https://tailor.wimpys.world"><picture><source media="(prefers-color-scheme: dark)" srcset="` + livePrefix + `hero-dark.svg"><img src="` + livePrefix + `hero-light.svg" alt="Tailor"></picture></a>`
	if !strings.Contains(header, wantHeader) {
		t.Errorf("header block missing expected markup:\n%s", header)
	}

	// Verify preferred links + mailto normalisation.
	links := blocks[MarkerLinks]
	if !strings.Contains(links, `srcset="`+livePrefix+`link-bluesky-dark.svg"`) ||
		!strings.Contains(links, `src="`+livePrefix+`link-bluesky-light.svg"`) ||
		!strings.Contains(links, `href="mailto:code@wimpress.io"`) {
		t.Errorf("links block missing expected markup:\n%s", links)
	}

	// Verify community block uses raw.githubusercontent.com URLs even when LiveCommunity is false.
	comm := blocks[MarkerCommunity]
	if !strings.Contains(comm, `href="https://wimpysworld.io/discord"`) ||
		!strings.Contains(comm, `srcset="`+livePrefix+`community-dark.svg"`) ||
		!strings.Contains(comm, `src="`+livePrefix+`community-light.svg"`) {
		t.Errorf("community block unexpected:\n%s", comm)
	}

	// Verify Haberdasher package anchors and custom override.
	pkgs := blocks[MarkerPackages]
	if !strings.Contains(pkgs, `href="https://apt.wimpys.world/#tailor"`) ||
		!strings.Contains(pkgs, `srcset="`+livePrefix+`pkg-apt-dark.svg"`) ||
		!strings.Contains(pkgs, `src="`+livePrefix+`pkg-apt-light.svg"`) ||
		!strings.Contains(pkgs, `href="https://nix.wimpys.world/#tailor"`) ||
		!strings.Contains(pkgs, `href="https://brew.wimpys.world/#custom"`) {
		t.Errorf("packages block unexpected:\n%s", pkgs)
	}

	if !strings.Contains(blocks[MarkerStats], `srcset="`+livePrefix+`stats-dark.svg"`) ||
		!strings.Contains(blocks[MarkerStats], `src="`+livePrefix+`stats-light.svg"`) ||
		!strings.Contains(blocks[MarkerStats], `href="https://github.com/wimpysworld/tailor/pulse"`) {
		t.Errorf("stats block missing commit-pinned URLs or /pulse link:\n%s", blocks[MarkerStats])
	}
	if !strings.Contains(blocks[MarkerStars], `srcset="`+livePrefix+`stars-dark.svg"`) ||
		!strings.Contains(blocks[MarkerStars], `href="https://github.com/wimpysworld/tailor"`) {
		t.Errorf("stars block missing commit-pinned URLs or repository link:\n%s", blocks[MarkerStars])
	}
	if !strings.Contains(blocks[MarkerSponsors], `srcset="`+livePrefix+`sponsors-dark.svg"`) ||
		!strings.Contains(blocks[MarkerSponsors], `src="`+livePrefix+`sponsors-light.svg"`) ||
		!strings.Contains(blocks[MarkerSponsors], `href="https://github.com/sponsors/flexiondotorg"`) {
		t.Errorf("sponsors block missing commit-pinned URLs:\n%s", blocks[MarkerSponsors])
	}
	if !strings.Contains(blocks[MarkerContributors], `srcset="`+livePrefix+`contributors-dark.svg"`) ||
		!strings.Contains(blocks[MarkerContributors], `src="`+livePrefix+`contributors-light.svg"`) ||
		!strings.Contains(blocks[MarkerContributors], `href="https://github.com/wimpysworld/tailor/graphs/contributors"`) {
		t.Errorf("contributors block missing commit-pinned URLs or /graphs/contributors link:\n%s", blocks[MarkerContributors])
	}

	if !strings.Contains(blocks[MarkerDemo], `srcset="`+livePrefix+`demo-dark.svg"`) ||
		!strings.Contains(blocks[MarkerDemo], `src="`+livePrefix+`demo-light.svg"`) ||
		!strings.Contains(blocks[MarkerDemo], `alt="Tailor demo"`) {
		t.Errorf("demo block unexpected:\n%s", blocks[MarkerDemo])
	}
	if !strings.Contains(blocks[MarkerFooter], `href="https://tailor.wimpys.world"`) ||
		!strings.Contains(blocks[MarkerFooter], `srcset="`+livePrefix+`footer-dark.svg"`) ||
		!strings.Contains(blocks[MarkerFooter], `src="`+livePrefix+`footer-light.svg"`) ||
		!strings.Contains(blocks[MarkerFooter], `alt="Fitted by Tailor"`) {
		t.Errorf("footer block unexpected:\n%s", blocks[MarkerFooter])
	}

	// Verify DemoFallbackSrc renders a plain centred <img> pointing to main without <picture>.
	fallbackBlocks := FormatReadmeBlocks(ReadmeBlocksInput{
		Owner:           "wimpysworld",
		Repo:            "tailor",
		ProjectTitle:    "Tailor",
		DemoFallbackSrc: "pages/demo.webp",
	})
	wantFallback := "<p align=\"center\">\n  <img src=\"pages/demo.webp\" alt=\"Tailor demo\">\n</p>"
	if fallbackBlocks[MarkerDemo] != wantFallback {
		t.Errorf("demo fallback block =\n%s\nwant:\n%s", fallbackBlocks[MarkerDemo], wantFallback)
	}

	// Verify branch-based URL fallback for all 10 SVG sets when LiveSHA is empty.
	branchIn := in
	branchIn.LiveSHA = ""
	branchIn.LiveBranch = ""
	defaultBranchBlocks := FormatReadmeBlocks(branchIn)
	const defaultBranchPrefix = "https://raw.githubusercontent.com/wimpysworld/tailor/social-assets/"
	for _, tc := range []struct {
		marker string
		asset  string
	}{
		{MarkerHeader, "hero"},
		{MarkerLinks, "link-website"},
		{MarkerCommunity, "community"},
		{MarkerPackages, "pkg-apt"},
		{MarkerDemo, "demo"},
		{MarkerStats, "stats"},
		{MarkerStars, "stars"},
		{MarkerSponsors, "sponsors"},
		{MarkerContributors, "contributors"},
		{MarkerFooter, "footer"},
	} {
		if !strings.Contains(defaultBranchBlocks[tc.marker], `srcset="`+defaultBranchPrefix+tc.asset+`-dark.svg"`) ||
			!strings.Contains(defaultBranchBlocks[tc.marker], `src="`+defaultBranchPrefix+tc.asset+`-light.svg"`) {
			t.Errorf("%s block missing default branch URL:\n%s", tc.marker, defaultBranchBlocks[tc.marker])
		}
	}

	// Verify explicit LiveBranch overrides the default branch name when LiveSHA is empty.
	branchIn.LiveBranch = "custom-social"
	customBranchBlocks := FormatReadmeBlocks(branchIn)
	const customBranchPrefix = "https://raw.githubusercontent.com/wimpysworld/tailor/custom-social/"
	if !strings.Contains(customBranchBlocks[MarkerHeader], `srcset="`+customBranchPrefix+`hero-dark.svg"`) ||
		!strings.Contains(customBranchBlocks[MarkerStats], `srcset="`+customBranchPrefix+`stats-dark.svg"`) ||
		!strings.Contains(customBranchBlocks[MarkerCommunity], `srcset="`+customBranchPrefix+`community-dark.svg"`) {
		t.Errorf("custom LiveBranch URLs missing:\nheader=%s\nstats=%s\ncommunity=%s", customBranchBlocks[MarkerHeader], customBranchBlocks[MarkerStats], customBranchBlocks[MarkerCommunity])
	}

	// Verify missing Owner/Repo falls back to static relative paths for all 10 SVG sets.
	noRepoIn := branchIn
	noRepoIn.Owner = ""
	noRepoBlocks := FormatReadmeBlocks(noRepoIn)
	for _, tc := range []struct {
		marker string
		asset  string
	}{
		{MarkerHeader, "hero"},
		{MarkerLinks, "link-website"},
		{MarkerCommunity, "community"},
		{MarkerPackages, "pkg-apt"},
		{MarkerDemo, "demo"},
		{MarkerStats, "stats"},
		{MarkerStars, "stars"},
		{MarkerSponsors, "sponsors"},
		{MarkerContributors, "contributors"},
		{MarkerFooter, "footer"},
	} {
		if !strings.Contains(noRepoBlocks[tc.marker], `srcset="pages/social/`+tc.asset+`-dark.svg"`) ||
			!strings.Contains(noRepoBlocks[tc.marker], `src="pages/social/`+tc.asset+`-light.svg"`) {
			t.Errorf("missing owner should fall back to static path for %s:\n%s", tc.marker, noRepoBlocks[tc.marker])
		}
	}
}

func TestBalancePackageRows(t *testing.T) {
	cases := []struct {
		n         int
		wantSizes []int
	}{
		{n: 0, wantSizes: nil},
		{n: 1, wantSizes: []int{1}},
		{n: 2, wantSizes: []int{2}},
		{n: 3, wantSizes: []int{3}},
		{n: 4, wantSizes: []int{4}},
		{n: 5, wantSizes: []int{3, 2}},
		{n: 6, wantSizes: []int{3, 3}},
		{n: 7, wantSizes: []int{4, 3}},
		{n: 8, wantSizes: []int{4, 4}},
		{n: 9, wantSizes: []int{3, 3, 3}},
	}

	for _, tc := range cases {
		items := make([]string, tc.n)
		for i := range items {
			items[i] = string(rune('a' + i))
		}
		rows := balancePackageRows(items)
		if len(rows) != len(tc.wantSizes) {
			t.Fatalf("balancePackageRows(%d) returned %d rows, want %d", tc.n, len(rows), len(tc.wantSizes))
		}
		var flattened []string
		for i, row := range rows {
			if len(row) != tc.wantSizes[i] {
				t.Errorf("balancePackageRows(%d) row %d len = %d, want %d", tc.n, i, len(row), tc.wantSizes[i])
			}
			flattened = append(flattened, row...)
		}
		if strings.Join(flattened, "") != strings.Join(items, "") {
			t.Errorf("balancePackageRows(%d) did not preserve item order: got %v, want %v", tc.n, flattened, items)
		}
	}
}

func TestFormatReadmeBlocksBalancedPackageRows(t *testing.T) {
	allHosts := []string{"apt", "rpm", "pkg", "apk", "flatpak", "brew", "scoop", "nix"}

	cases := []struct {
		count    int
		wantRows []int
		wantBrs  int
	}{
		{count: 3, wantRows: []int{3}, wantBrs: 0},
		{count: 4, wantRows: []int{4}, wantBrs: 0},
		{count: 5, wantRows: []int{3, 2}, wantBrs: 1},
		{count: 6, wantRows: []int{3, 3}, wantBrs: 1},
		{count: 7, wantRows: []int{4, 3}, wantBrs: 1},
		{count: 8, wantRows: []int{4, 4}, wantBrs: 1},
	}

	for _, tc := range cases {
		pkgs := make([]ReadmePackageEntry, tc.count)
		for i := range tc.count {
			pkgs[i] = ReadmePackageEntry{Host: allHosts[i], Package: "tailor"}
		}
		blocks := FormatReadmeBlocks(ReadmeBlocksInput{
			StaticDir: "pages/social",
			Packages:  pkgs,
		})
		block := blocks[MarkerPackages]
		if !strings.HasPrefix(block, "<p align=\"center\">\n") || !strings.HasSuffix(block, "\n</p>") {
			t.Fatalf("count=%d: unexpected wrapper:\n%s", tc.count, block)
		}
		if gotBrs := strings.Count(block, "<br>"); gotBrs != tc.wantBrs {
			t.Errorf("count=%d: <br> count = %d, want %d\n%s", tc.count, gotBrs, tc.wantBrs, block)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(block, "<p align=\"center\">\n"), "\n</p>")
		parts := strings.Split(inner, "\n  <br>\n")
		if len(parts) != len(tc.wantRows) {
			t.Fatalf("count=%d: got %d row chunks, want %d\n%s", tc.count, len(parts), len(tc.wantRows), block)
		}
		for i, part := range parts {
			gotTiles := strings.Count(part, "<a href=")
			if gotTiles != tc.wantRows[i] {
				t.Errorf("count=%d: row %d has %d tiles, want %d\n%s", tc.count, i, gotTiles, tc.wantRows[i], part)
			}
		}
	}
}
