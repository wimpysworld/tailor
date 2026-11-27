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

	t.Run("updates all seven canonical marker names", func(t *testing.T) {
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
		Stats:       true,
		StarHistory: true,
		Sponsors:    true,
		SponsorsURL: "https://github.com/sponsors/flexiondotorg",
	}

	blocks := FormatReadmeBlocks(in)

	if len(blocks) != len(ReadmeMarkerNames) {
		t.Fatalf("FormatReadmeBlocks() returned %d blocks, want %d", len(blocks), len(ReadmeMarkerNames))
	}

	// Verify static header block.
	header := blocks[MarkerHeader]
	wantHeader := `<a href="https://tailor.wimpys.world"><picture><source media="(prefers-color-scheme: dark)" srcset="pages/social/hero-dark.svg"><img src="pages/social/hero-light.svg" alt="Tailor"></picture></a>`
	if !strings.Contains(header, wantHeader) {
		t.Errorf("header block missing expected markup:\n%s", header)
	}

	// Verify preferred links + mailto normalisation.
	links := blocks[MarkerLinks]
	if !strings.Contains(links, `srcset="pages/social/link-bluesky-dark.svg"`) ||
		!strings.Contains(links, `src="pages/social/link-bluesky-light.svg"`) ||
		!strings.Contains(links, `href="mailto:code@wimpress.io"`) {
		t.Errorf("links block missing expected markup:\n%s", links)
	}

	// Verify static community block.
	comm := blocks[MarkerCommunity]
	if !strings.Contains(comm, `href="https://wimpysworld.io/discord"`) ||
		!strings.Contains(comm, `srcset="pages/social/community-dark.svg"`) {
		t.Errorf("community block unexpected:\n%s", comm)
	}

	// Verify Haberdasher package anchors and custom override.
	pkgs := blocks[MarkerPackages]
	if !strings.Contains(pkgs, `href="https://apt.wimpys.world/#tailor"`) ||
		!strings.Contains(pkgs, `srcset="pages/social/pkg-apt-dark.svg"`) ||
		!strings.Contains(pkgs, `href="https://nix.wimpys.world/#tailor"`) ||
		!strings.Contains(pkgs, `href="https://brew.wimpys.world/#custom"`) {
		t.Errorf("packages block unexpected:\n%s", pkgs)
	}

	// Verify commit-pinned raw.githubusercontent.com live URLs.
	const livePrefix = "https://raw.githubusercontent.com/wimpysworld/tailor/deadbeef1234567890/"
	if !strings.Contains(blocks[MarkerStats], `srcset="`+livePrefix+`stats-dark.svg"`) ||
		!strings.Contains(blocks[MarkerStats], `src="`+livePrefix+`stats-light.svg"`) {
		t.Errorf("stats block missing commit-pinned live URLs:\n%s", blocks[MarkerStats])
	}
	if !strings.Contains(blocks[MarkerStars], `srcset="`+livePrefix+`stars-dark.svg"`) ||
		!strings.Contains(blocks[MarkerStars], `href="https://github.com/wimpysworld/tailor/stargazers"`) {
		t.Errorf("stars block missing commit-pinned live URLs:\n%s", blocks[MarkerStars])
	}
	if !strings.Contains(blocks[MarkerSponsors], `srcset="`+livePrefix+`sponsors-dark.svg"`) ||
		!strings.Contains(blocks[MarkerSponsors], `href="https://github.com/sponsors/flexiondotorg"`) {
		t.Errorf("sponsors block missing commit-pinned live URLs:\n%s", blocks[MarkerSponsors])
	}

	// Verify LiveCommunity switches community banner to commit-pinned live URL.
	in.LiveCommunity = true
	liveBlocks := FormatReadmeBlocks(in)
	if !strings.Contains(liveBlocks[MarkerCommunity], `srcset="`+livePrefix+`community-dark.svg"`) {
		t.Errorf("live community block missing commit-pinned URL:\n%s", liveBlocks[MarkerCommunity])
	}
}
