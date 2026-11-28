package social

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
)

const (
	maxGeneratedAssetSize = 4 << 20
	maxReadmeFileSize     = 4 << 20
	maxDemoEmbedSize      = 512 * 1024
)

var hexSHARegexp = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// AssetStatus describes the outcome of processing one generated asset or README file.
type AssetStatus string

const (
	// StatusCreated indicates a new file was (or in dry-run would be) created.
	StatusCreated AssetStatus = "created"
	// StatusUpdated indicates an existing file was (or in dry-run would be) updated.
	StatusUpdated AssetStatus = "updated"
	// StatusUnchanged indicates an existing file already matched the rendered content.
	StatusUnchanged AssetStatus = "unchanged"
	// StatusSkipped indicates an asset was skipped by configuration, thresholds, or missing target.
	StatusSkipped AssetStatus = "skipped"
)

// AssetResult records the status of one file handled by Generate.
type AssetResult struct {
	Path   string
	Status AssetStatus
	Reason string
}

// GenerateOptions controls static and live social SVG generation and README
// marker updates.
type GenerateOptions struct {
	// Dir is the project root directory. Defaults to ".".
	Dir string
	// Config supplies a pre-loaded Tailor configuration. When nil, Generate loads
	// .tailor.yml from Dir.
	Config *config.Config
	// Static enables static SVG generation. Defaults to true when neither Static
	// nor Live is explicitly set.
	Static bool
	// Live enables live SVG generation via Fetcher.
	Live bool
	// DryRun previews planned changes without creating directories or writing files.
	DryRun bool
	// OutputDir overrides the SVG output directory. Defaults to <pagesPath>/social.
	OutputDir string
	// UpdateReadme updates existing <!-- tailor:social-* --> markers in README.md.
	UpdateReadme bool
	// AssetsSHA is an optional commit SHA of the live social-assets branch used
	// for commit-pinned raw.githubusercontent.com URLs in README.md. When empty,
	// README.md uses branch-based URLs targeting social.live.branch.
	AssetsSHA string
	// Owner is the GitHub repository owner.
	Owner string
	// Repo is the GitHub repository name.
	Repo string
	// Fetcher provides live GitHub and community metrics when Live is true.
	Fetcher LiveFetcher
}

// GenerateResult reports every planned or applied asset outcome from Generate.
type GenerateResult struct {
	DryRun bool
	Assets []AssetResult
}

// Generate orchestrates theme extraction, project asset resolution, static and
// live SVG rendering, safe rooted file writes, and optional README marker updates.
func Generate(ctx context.Context, opts GenerateOptions) (result GenerateResult, retErr error) {
	result.DryRun = opts.DryRun

	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		dir = "."
	}

	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("accessing project directory %q: %w", dir, err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 || !dirInfo.IsDir() {
		return GenerateResult{}, fmt.Errorf("project directory %q must be a regular directory, not a symlink", dir)
	}

	sha := strings.TrimSpace(opts.AssetsSHA)
	if sha != "" && !hexSHARegexp.MatchString(sha) {
		return GenerateResult{}, fmt.Errorf("invalid assets SHA %q: expected 7 to 64 hexadecimal characters", opts.AssetsSHA)
	}

	runLive := opts.Live
	runStatic := opts.Static || !opts.Live

	owner := strings.TrimSpace(opts.Owner)
	repo := strings.TrimSpace(opts.Repo)
	if runLive {
		if opts.Fetcher == nil {
			return GenerateResult{}, errors.New("live social asset generation requires a LiveFetcher")
		}
		if owner == "" || repo == "" {
			return GenerateResult{}, errors.New("live social asset generation requires repository owner and name")
		}
	}

	cfg := opts.Config
	if cfg == nil {
		cfg, err = config.Load(dir)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("loading .tailor.yml: %w", err)
		}
	}

	projectRoot, err := os.OpenRoot(dir)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer func() { retErr = errors.Join(retErr, projectRoot.Close()) }()

	pagesPath := ""
	if cfg.Pages != nil && cfg.Pages.Path != nil {
		pagesPath = *cfg.Pages.Path
	}
	cleanedPages, err := cleanRelativeDir(pagesPath)
	if err != nil {
		return GenerateResult{}, err
	}

	theme, err := ExtractTheme(projectRoot, cleanedPages, cfg.Social)
	if err != nil {
		return GenerateResult{}, err
	}

	repoFullName := formatRepoFullName(owner, repo, dir)
	assets, err := ResolveProjectAssets(projectRoot, cleanedPages, cfg, repoFullName)
	if err != nil {
		return GenerateResult{}, err
	}

	target, closeTarget, err := openOutputTarget(projectRoot, cleanedPages, opts.OutputDir, opts.DryRun)
	if err != nil {
		return GenerateResult{}, err
	}
	defer func() {
		if closeTarget != nil {
			retErr = errors.Join(retErr, closeTarget())
		}
	}()

	links := resolvePreferredLinks(cfg)
	comm := resolveCommunity(cfg, owner, repo)
	commIsLive := isLiveCommunity(comm)
	packages := resolvePackages(cfg, repoFullName, repo)

	var staticRes staticGenResult
	if runStatic {
		var err error
		staticRes, err = generateStaticAssets(projectRoot, target, cleanedPages, cfg, theme, assets, links, comm, commIsLive, runLive, packages, opts.DryRun, &result)
		if err != nil {
			return result, err
		}
	}

	var liveRes liveGenResult
	if runLive {
		var err error
		liveRes, err = generateLiveAssets(
			ctx, target, cfg, theme, assets.Title, comm, commIsLive, owner, repo, opts.Fetcher, opts.DryRun, &result,
		)
		if err != nil {
			return result, err
		}
	}

	if opts.UpdateReadme {
		if err := updateProjectReadme(
			projectRoot, cfg, assets, target.staticRelDir, owner, repo, sha,
			runStatic, runLive, links, comm, commIsLive, packages, staticRes, liveRes, opts.DryRun, &result,
		); err != nil {
			return result, err
		}
	}

	return result, nil
}

func formatRepoFullName(owner, repo, dir string) string {
	if owner != "" && repo != "" {
		return owner + "/" + repo
	}
	if repo != "" {
		return repo
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return filepath.Base(abs)
	}
	return filepath.Base(dir)
}

type staticGenResult struct {
	demoSVG         bool
	demoFallbackSrc string
	footer          bool
}

func generateStaticAssets(
	projectRoot *os.Root,
	target *outputTarget,
	cleanedPages string,
	cfg *config.Config,
	theme Theme,
	assets ProjectAssets,
	links []ReadmeLinkEntry,
	comm *ReadmeCommunityEntry,
	commIsLive bool,
	runLive bool,
	packages []ReadmePackageEntry,
	dryRun bool,
	result *GenerateResult,
) (staticGenResult, error) {
	var staticRes staticGenResult

	if cfg.EffectiveSocialReadmeHeader() {
		heroIn := HeroInput{Assets: assets}
		if assets.IconPath != "" && assets.IconKind != IconKindNone {
			data, exists, err := readRootRegularFile(projectRoot, assets.IconPath, maxIconSize)
			if err != nil {
				return staticRes, fmt.Errorf("reading project icon %q: %w", assets.IconPath, err)
			}
			if exists {
				heroIn.IconData = data
				switch assets.IconKind {
				case IconKindSVG:
					heroIn.IconMIME = "image/svg+xml"
				case IconKindPNG:
					heroIn.IconMIME = "image/png"
				}
			}
		}
		pair, err := RenderHero(heroIn, theme)
		if err != nil {
			return staticRes, fmt.Errorf("rendering hero SVG: %w", err)
		}
		if err := writeSVGPair(target, "hero", pair, dryRun, result); err != nil {
			return staticRes, err
		}
	}

	for _, link := range links {
		pair, err := RenderLinkPill(LinkPillInput{Key: link.Key, Label: link.Label}, theme)
		if err != nil {
			return staticRes, fmt.Errorf("rendering link pill %q: %w", link.Key, err)
		}
		if err := writeSVGPair(target, "link-"+link.Key, pair, dryRun, result); err != nil {
			return staticRes, err
		}
	}

	// Write static community banner when the platform is static or when running static-only.
	if comm != nil && (!runLive || !commIsLive) {
		pair, err := RenderCommunityBanner(CommunityBannerInput{Platform: comm.Platform}, theme)
		if err != nil {
			return staticRes, fmt.Errorf("rendering static community SVG: %w", err)
		}
		if err := writeSVGPair(target, "community", pair, dryRun, result); err != nil {
			return staticRes, err
		}
	}

	for _, pkg := range packages {
		pair, err := RenderPackageCard(pkg.Host, theme)
		if err != nil {
			return staticRes, fmt.Errorf("rendering package card %q: %w", pkg.Host, err)
		}
		if err := writeSVGPair(target, "pkg-"+pkg.Host, pair, dryRun, result); err != nil {
			return staticRes, err
		}
	}

	if cfg.EffectiveSocialDemo() {
		demoSVG, fallbackSrc, err := generateStaticDemo(projectRoot, target, cleanedPages, theme, assets.Title, dryRun, result)
		if err != nil {
			return staticRes, err
		}
		staticRes.demoSVG = demoSVG
		staticRes.demoFallbackSrc = fallbackSrc
	}

	// Write static sponsors CTA banner when sponsors is enabled.
	if staticSponsorsEnabled(cfg) {
		pair, err := RenderSponsors(SponsorsInput{}, theme)
		if err != nil {
			return staticRes, fmt.Errorf("rendering static sponsors SVG: %w", err)
		}
		if err := writeSVGPair(target, "sponsors", pair, dryRun, result); err != nil {
			return staticRes, err
		}
	}

	if cfg.EffectiveSocialFooter() {
		pair, err := RenderFooter(theme)
		if err != nil {
			return staticRes, fmt.Errorf("rendering footer SVG: %w", err)
		}
		if err := writeSVGPair(target, "footer", pair, dryRun, result); err != nil {
			return staticRes, err
		}
		staticRes.footer = true
	}

	return staticRes, nil
}

func generateStaticDemo(
	projectRoot *os.Root,
	target *outputTarget,
	cleanedPages string,
	theme Theme,
	projectTitle string,
	dryRun bool,
	result *GenerateResult,
) (demoSVG bool, fallbackSrc string, err error) {
	candidates := []struct {
		filename string
		mime     string
	}{
		{filename: "demo.webp", mime: "image/webp"},
		{filename: "demo-still.png", mime: "image/png"},
	}

	for _, cand := range candidates {
		relPath := path.Join(cleanedPages, cand.filename)
		if err := checkRootParents(projectRoot, relPath); err != nil {
			return false, "", err
		}
		info, err := projectRoot.Lstat(filepath.FromSlash(relPath))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return false, "", fmt.Errorf("inspecting %q: %w", relPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, "", fmt.Errorf("%q is a symlink", relPath)
		}
		if !info.Mode().IsRegular() {
			return false, "", fmt.Errorf("%q is not a regular file", relPath)
		}
		if info.Size() > maxDemoEmbedSize {
			recordSkippedPair(target, "demo", "demo image exceeds 512 KiB embed limit", result)
			return false, relPath, nil
		}

		data, exists, err := readRootRegularFile(projectRoot, relPath, maxDemoEmbedSize)
		if err != nil {
			return false, "", fmt.Errorf("reading demo image %q: %w", relPath, err)
		}
		if !exists {
			continue
		}
		imgW, imgH, err := decodeDemoImageDimensions(data, cand.mime)
		if err != nil {
			return false, "", fmt.Errorf("decoding demo image %q dimensions: %w", relPath, err)
		}
		pair, err := RenderDemo(theme, projectTitle, data, cand.mime, imgW, imgH)
		if err != nil {
			return false, "", fmt.Errorf("rendering demo SVG: %w", err)
		}
		if err := writeSVGPair(target, "demo", pair, dryRun, result); err != nil {
			return false, "", err
		}
		return true, "", nil
	}

	return false, "", nil
}

var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

func decodeDemoImageDimensions(data []byte, mime string) (width, height int, err error) {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		if len(data) < 24 || !bytes.Equal(data[:8], pngSignature) || string(data[12:16]) != "IHDR" {
			return 0, 0, errors.New("invalid PNG header")
		}
		w := int(binary.BigEndian.Uint32(data[16:20]))
		h := int(binary.BigEndian.Uint32(data[20:24]))
		if w <= 0 || h <= 0 {
			return 0, 0, fmt.Errorf("invalid PNG dimensions %dx%d", w, h)
		}
		return w, h, nil

	case "image/webp":
		if len(data) < 16 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
			return 0, 0, errors.New("invalid WebP RIFF header")
		}
		switch string(data[12:16]) {
		case "VP8X":
			if len(data) < 30 {
				return 0, 0, errors.New("truncated WebP VP8X header")
			}
			w := 1 + (int(data[24]) | int(data[25])<<8 | int(data[26])<<16)
			h := 1 + (int(data[27]) | int(data[28])<<8 | int(data[29])<<16)
			if w <= 0 || h <= 0 {
				return 0, 0, fmt.Errorf("invalid WebP VP8X dimensions %dx%d", w, h)
			}
			return w, h, nil
		case "VP8 ":
			if len(data) < 30 {
				return 0, 0, errors.New("truncated WebP VP8 header")
			}
			if data[20]&1 != 0 || data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
				return 0, 0, errors.New("invalid WebP VP8 keyframe header")
			}
			w := int(binary.LittleEndian.Uint16(data[26:28]) & 0x3fff)
			h := int(binary.LittleEndian.Uint16(data[28:30]) & 0x3fff)
			if w <= 0 || h <= 0 {
				return 0, 0, fmt.Errorf("invalid WebP VP8 dimensions %dx%d", w, h)
			}
			return w, h, nil
		case "VP8L":
			if len(data) < 25 || data[20] != 0x2f {
				return 0, 0, errors.New("invalid WebP VP8L header")
			}
			bits := binary.LittleEndian.Uint32(data[21:25])
			w := int(bits&0x3fff) + 1
			h := int((bits>>14)&0x3fff) + 1
			if w <= 0 || h <= 0 {
				return 0, 0, fmt.Errorf("invalid WebP VP8L dimensions %dx%d", w, h)
			}
			return w, h, nil
		default:
			return 0, 0, fmt.Errorf("unsupported WebP chunk %q", string(data[12:16]))
		}
	default:
		return 0, 0, fmt.Errorf("unsupported demo image MIME %q", mime)
	}
}

func staticSponsorsEnabled(cfg *config.Config) bool {
	switch cfg.EffectiveSocialSponsors() {
	case "true":
		return true
	case "auto":
		return cfg != nil && cfg.Pages != nil && cfg.Pages.Links != nil &&
			strings.TrimSpace((*cfg.Pages.Links)["github_sponsors"]) != ""
	default:
		return false
	}
}

type liveGenResult struct {
	stars        bool
	community    bool
	contributors bool
}

func generateLiveAssets(
	ctx context.Context,
	target *outputTarget,
	cfg *config.Config,
	theme Theme,
	projectTitle string,
	comm *ReadmeCommunityEntry,
	commIsLive bool,
	owner, repo string,
	fetcher LiveFetcher,
	dryRun bool,
	result *GenerateResult,
) (liveGenResult, error) {
	var live liveGenResult

	if cfg.EffectiveSocialReadmeStats() {
		stats, err := fetcher.FetchRepoStats(ctx, owner, repo)
		if err != nil {
			return live, err
		}
		pair, err := RenderStats(StatsInput(stats), theme)
		if err != nil {
			return live, fmt.Errorf("rendering stats SVG: %w", err)
		}
		if err := writeSVGPair(target, "stats", pair, dryRun, result); err != nil {
			return live, err
		}
	}

	if cfg.EffectiveSocialStarHistoryEnabled() {
		points, err := fetcher.FetchStarHistory(ctx, owner, repo)
		if err != nil {
			return live, err
		}
		pair, ok, err := RenderStarHistory(StarHistoryInput{
			Points:       points,
			MinStars:     cfg.EffectiveSocialStarHistoryMinStars(),
			MinDays:      cfg.EffectiveSocialStarHistoryMinDays(),
			Title:        "Star History",
			ProjectTitle: projectTitle,
		}, theme)
		if err != nil {
			return live, fmt.Errorf("rendering star history SVG: %w", err)
		}
		if ok {
			if err := writeSVGPair(target, "stars", pair, dryRun, result); err != nil {
				return live, err
			}
			live.stars = true
		} else {
			recordSkippedPair(target, "stars", "below star history threshold", result)
		}
	}

	if cfg.EffectiveSocialContributors() {
		contributors, totalCount, err := fetcher.FetchContributors(ctx, owner, repo)
		if err != nil {
			return live, err
		}
		if len(contributors) > 0 {
			pair, err := RenderContributors(theme, projectTitle, contributors, totalCount)
			if err != nil {
				return live, fmt.Errorf("rendering contributors SVG: %w", err)
			}
			if err := writeSVGPair(target, "contributors", pair, dryRun, result); err != nil {
				return live, err
			}
			live.contributors = true
		} else {
			recordSkippedPair(target, "contributors", "no human contributors found", result)
		}
	}

	if comm != nil && commIsLive {
		commIn := CommunityBannerInput{Platform: comm.Platform}
		switch comm.Platform {
		case "discord":
			if code := ExtractDiscordInviteCode(comm.URL); code != "" {
				if online, members, ok := fetcher.FetchDiscordInvite(ctx, code); ok {
					commIn.OnlineCount = online
					commIn.MemberCount = members
				}
			}
		case "discussions":
			count, err := fetcher.FetchDiscussionsCount(ctx, owner, repo)
			if err != nil {
				return live, err
			}
			commIn.DiscussionCount = count
		case "forum":
			if comm.URL != "" {
				if members, ok := fetcher.FetchDiscourseStats(ctx, comm.URL); ok {
					commIn.MemberCount = members
				}
			}
		}
		pair, err := RenderCommunityBanner(commIn, theme)
		if err != nil {
			return live, fmt.Errorf("rendering live community SVG: %w", err)
		}
		if err := writeSVGPair(target, "community", pair, dryRun, result); err != nil {
			return live, err
		}
		live.community = true
	}

	return live, nil
}

func updateProjectReadme(
	projectRoot *os.Root,
	cfg *config.Config,
	assets ProjectAssets,
	staticRelDir string,
	owner, repo, sha string,
	runStatic, runLive bool,
	links []ReadmeLinkEntry,
	comm *ReadmeCommunityEntry,
	commIsLive bool,
	packages []ReadmePackageEntry,
	staticRes staticGenResult,
	live liveGenResult,
	dryRun bool,
	result *GenerateResult,
) error {
	raw, exists, err := readRootRegularFile(projectRoot, "README.md", maxReadmeFileSize)
	if err != nil {
		return fmt.Errorf("reading README.md: %w", err)
	}
	if !exists {
		result.Assets = append(result.Assets, AssetResult{
			Path:   "README.md",
			Status: StatusSkipped,
			Reason: "README.md not found",
		})
		return nil
	}

	blocksIn := ReadmeBlocksInput{
		StaticDir:    staticRelDir,
		Owner:        owner,
		Repo:         repo,
		LiveBranch:   cfg.EffectiveSocialLiveBranch(),
		LiveSHA:      sha,
		ProjectTitle: assets.Title,
	}

	if runStatic {
		blocksIn.Header = cfg.EffectiveSocialReadmeHeader()
		blocksIn.HeaderURL = assets.FooterURL
		blocksIn.Links = links
		blocksIn.Packages = packages
		blocksIn.Demo = staticRes.demoSVG
		blocksIn.DemoFallbackSrc = staticRes.demoFallbackSrc
		if staticSponsorsEnabled(cfg) {
			blocksIn.Sponsors = true
			blocksIn.SponsorsURL = resolveSponsorsURL(cfg, owner)
		}
		blocksIn.Footer = staticRes.footer
	}
	hasRepoCoords := owner != "" && repo != ""
	if comm != nil {
		switch {
		case live.community:
			blocksIn.Community = comm
			blocksIn.LiveCommunity = true
		case runStatic:
			blocksIn.Community = comm
			blocksIn.LiveCommunity = commIsLive && hasRepoCoords
		}
	}
	if runLive {
		blocksIn.Stats = cfg.EffectiveSocialReadmeStats()
		blocksIn.StarHistory = live.stars
		blocksIn.Contributors = live.contributors
	} else if runStatic && hasRepoCoords {
		blocksIn.Stats = cfg.EffectiveSocialReadmeStats()
		blocksIn.StarHistory = cfg.EffectiveSocialStarHistoryEnabled()
		blocksIn.Contributors = cfg.EffectiveSocialContributors()
	}

	blocks := FormatReadmeBlocks(blocksIn)
	if runLive {
		if cfg.EffectiveSocialStarHistoryEnabled() && !live.stars {
			blocks[MarkerStars] = ""
		}
		if cfg.EffectiveSocialContributors() && !live.contributors {
			blocks[MarkerContributors] = ""
		}
	}

	updated, changed, err := UpdateReadmeMarkers(string(raw), blocks)
	if err != nil {
		return fmt.Errorf("updating README.md social markers: %w", err)
	}
	if !changed {
		result.Assets = append(result.Assets, AssetResult{
			Path:   "README.md",
			Status: StatusUnchanged,
		})
		return nil
	}
	if !dryRun {
		if err := writeRootFileAtomic(projectRoot, "README.md", []byte(updated)); err != nil {
			return fmt.Errorf("writing README.md: %w", err)
		}
	}
	result.Assets = append(result.Assets, AssetResult{
		Path:   "README.md",
		Status: StatusUpdated,
	})
	return nil
}

func resolvePreferredLinks(cfg *config.Config) []ReadmeLinkEntry {
	if cfg == nil || cfg.Pages == nil || cfg.Pages.Links == nil {
		return nil
	}
	linksMap := *cfg.Pages.Links

	if preferred := cfg.EffectiveSocialPreferredLinks(); len(preferred) > 0 {
		seen := make(map[string]struct{}, len(preferred))
		out := make([]ReadmeLinkEntry, 0, len(preferred))
		for _, rawKey := range preferred {
			key := strings.ToLower(strings.TrimSpace(rawKey))
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			urlVal := strings.TrimSpace(linksMap[key])
			if urlVal == "" {
				continue
			}
			info, ok := LookupLinkIcon(key)
			if !ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ReadmeLinkEntry{Key: key, Label: info.Label, URL: urlVal})
		}
		return out
	}

	// Default: first 3 non-community, non-funding pages.links entries in declared order.
	const maxDefaultLinks = 3
	out := make([]ReadmeLinkEntry, 0, maxDefaultLinks)
	for _, linkType := range cfg.Pages.OrderedLinks() {
		if len(out) >= maxDefaultLinks {
			break
		}
		if isCommunityLinkKey(linkType.Key) || isFundingLinkKey(linkType.Key) {
			continue
		}
		urlVal := strings.TrimSpace(linksMap[linkType.Key])
		if urlVal == "" {
			continue
		}
		if _, ok := LookupLinkIcon(linkType.Key); !ok {
			continue
		}
		out = append(out, ReadmeLinkEntry{Key: linkType.Key, Label: linkType.Label, URL: urlVal})
	}
	return out
}

func isCommunityLinkKey(key string) bool {
	switch key {
	case "discord", "matrix", "irc", "slack", "forum":
		return true
	default:
		return false
	}
}

func isFundingLinkKey(key string) bool {
	switch key {
	case "patreon", "github_sponsors", "kofi":
		return true
	default:
		return false
	}
}

func resolveCommunity(cfg *config.Config, owner, repo string) *ReadmeCommunityEntry {
	mode := cfg.EffectiveSocialCommunity()
	if mode == "none" {
		return nil
	}

	linksMap := map[string]string{}
	if cfg != nil && cfg.Pages != nil && cfg.Pages.Links != nil {
		linksMap = *cfg.Pages.Links
	}

	discussionsURL := ""
	if owner != "" && repo != "" {
		discussionsURL = fmt.Sprintf("https://github.com/%s/%s/discussions", owner, repo)
	}

	if mode == "auto" {
		if cfg != nil && cfg.Pages != nil {
			for _, linkType := range cfg.Pages.OrderedLinks() {
				if !isCommunityLinkKey(linkType.Key) {
					continue
				}
				if urlVal := strings.TrimSpace(linksMap[linkType.Key]); urlVal != "" {
					return &ReadmeCommunityEntry{
						Platform: linkType.Key,
						URL:      urlVal,
					}
				}
			}
		}
		if cfg != nil && cfg.Repository != nil && cfg.Repository.HasDiscussions != nil && *cfg.Repository.HasDiscussions {
			return &ReadmeCommunityEntry{
				Platform: "discussions",
				URL:      discussionsURL,
			}
		}
		return nil
	}

	if mode == "discussions" {
		return &ReadmeCommunityEntry{
			Platform: "discussions",
			URL:      discussionsURL,
		}
	}

	return &ReadmeCommunityEntry{
		Platform: mode,
		URL:      strings.TrimSpace(linksMap[mode]),
	}
}

func isLiveCommunity(comm *ReadmeCommunityEntry) bool {
	if comm == nil {
		return false
	}
	switch comm.Platform {
	case "discussions":
		return true
	case "discord":
		return ExtractDiscordInviteCode(comm.URL) != ""
	case "forum":
		return strings.TrimSpace(comm.URL) != ""
	default:
		return false
	}
}

func resolvePackages(cfg *config.Config, repoFullName, repo string) []ReadmePackageEntry {
	hosts := cfg.EffectiveSocialPackages()
	if len(hosts) == 0 {
		return nil
	}
	pkgName := strings.ToLower(strings.TrimSpace(repo))
	if pkgName == "" {
		fallback := strings.TrimSpace(repoFullName)
		if _, after, ok := strings.Cut(fallback, "/"); ok {
			fallback = strings.TrimSpace(after)
		}
		pkgName = strings.ToLower(fallback)
	}
	out := make([]ReadmePackageEntry, 0, len(hosts))
	for _, rawHost := range hosts {
		host := strings.ToLower(strings.TrimSpace(rawHost))
		if _, ok := LookupPackageHost(host); !ok {
			continue
		}
		out = append(out, ReadmePackageEntry{Host: host, Package: pkgName})
	}
	return out
}

func resolveSponsorsURL(cfg *config.Config, owner string) string {
	if cfg != nil && cfg.Pages != nil && cfg.Pages.Links != nil {
		if raw := strings.TrimSpace((*cfg.Pages.Links)["github_sponsors"]); raw != "" {
			return raw
		}
	}
	if owner != "" {
		return "https://github.com/sponsors/" + owner
	}
	return ""
}

type outputTarget struct {
	root          *os.Root
	relPrefix     string // POSIX path inside root ("" for root itself)
	displayPrefix string // prefix for AssetResult.Path
	staticRelDir  string // repo-relative POSIX path for README static links
	external      bool
}

func (t *outputTarget) displayPath(filename string) string {
	if t.external {
		return filepath.Join(t.displayPrefix, filename)
	}
	if t.displayPrefix == "" || t.displayPrefix == "." {
		return filename
	}
	return path.Join(t.displayPrefix, filename)
}

func (t *outputTarget) rootRelPath(filename string) string {
	if t.relPrefix == "" || t.relPrefix == "." {
		return filename
	}
	return path.Join(t.relPrefix, filename)
}

func openOutputTarget(projectRoot *os.Root, cleanedPages, outputDir string, dryRun bool) (*outputTarget, func() error, error) {
	defaultStaticDir := path.Join(cleanedPages, "social")
	trimmed := strings.TrimSpace(outputDir)
	if trimmed == "" {
		return &outputTarget{
			root:          projectRoot,
			relPrefix:     defaultStaticDir,
			displayPrefix: defaultStaticDir,
			staticRelDir:  defaultStaticDir,
		}, nil, nil
	}

	if !filepath.IsAbs(trimmed) {
		if cleaned, err := cleanRelativeDir(trimmed); err == nil {
			return &outputTarget{
				root:          projectRoot,
				relPrefix:     cleaned,
				displayPrefix: cleaned,
				staticRelDir:  cleaned,
			}, nil, nil
		}
	}

	// External or absolute output directory (for example an orphan social-assets branch checkout).
	cleanExternal := filepath.Clean(trimmed)
	info, err := os.Lstat(cleanExternal)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("output directory %q must not be a symlink", outputDir)
		}
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("output directory %q is not a directory", outputDir)
		}
	case errors.Is(err, fs.ErrNotExist):
		if dryRun {
			return &outputTarget{
				root:          nil,
				relPrefix:     "",
				displayPrefix: cleanExternal,
				staticRelDir:  defaultStaticDir,
				external:      true,
			}, nil, nil
		}
		if err := os.MkdirAll(cleanExternal, 0o755); err != nil {
			return nil, nil, fmt.Errorf("creating output directory %q: %w", outputDir, err)
		}
	default:
		return nil, nil, fmt.Errorf("inspecting output directory %q: %w", outputDir, err)
	}

	extRoot, err := os.OpenRoot(cleanExternal)
	if err != nil {
		return nil, nil, fmt.Errorf("opening output directory %q: %w", outputDir, err)
	}
	return &outputTarget{
		root:          extRoot,
		relPrefix:     "",
		displayPrefix: cleanExternal,
		staticRelDir:  defaultStaticDir,
		external:      true,
	}, extRoot.Close, nil
}

func writeSVGPair(target *outputTarget, baseName string, pair SVGPair, dryRun bool, result *GenerateResult) error {
	variants := []struct {
		suffix  string
		content string
	}{
		{"-dark.svg", pair.Dark},
		{"-light.svg", pair.Light},
	}
	for _, v := range variants {
		fileName := baseName + v.suffix
		status, err := writeTargetFile(target, fileName, []byte(v.content), dryRun)
		if err != nil {
			return err
		}
		result.Assets = append(result.Assets, AssetResult{
			Path:   target.displayPath(fileName),
			Status: status,
		})
	}
	return nil
}

func recordSkippedPair(target *outputTarget, baseName, reason string, result *GenerateResult) {
	for _, suffix := range []string{"-dark.svg", "-light.svg"} {
		result.Assets = append(result.Assets, AssetResult{
			Path:   target.displayPath(baseName + suffix),
			Status: StatusSkipped,
			Reason: reason,
		})
	}
}

func writeTargetFile(target *outputTarget, fileName string, content []byte, dryRun bool) (AssetStatus, error) {
	if target.root == nil {
		// External output directory does not exist yet in dry-run mode.
		return StatusCreated, nil
	}

	relPath := target.rootRelPath(fileName)
	existing, exists, err := readRootRegularFile(target.root, relPath, maxGeneratedAssetSize)
	if err != nil {
		return "", fmt.Errorf("inspecting %q: %w", target.displayPath(fileName), err)
	}

	status := StatusCreated
	if exists {
		if bytes.Equal(existing, content) {
			return StatusUnchanged, nil
		}
		status = StatusUpdated
	}
	if dryRun {
		return status, nil
	}

	if err := ensureRootParents(target.root, relPath); err != nil {
		return "", fmt.Errorf("preparing output directory for %q: %w", target.displayPath(fileName), err)
	}
	if err := writeRootFileAtomic(target.root, relPath, content); err != nil {
		return "", fmt.Errorf("writing %q: %w", target.displayPath(fileName), err)
	}
	return status, nil
}

func ensureRootParents(root *os.Root, relPath string) error {
	parent := path.Dir(relPath)
	if parent == "." || parent == "" {
		return nil
	}
	current := ""
	for component := range strings.SplitSeq(parent, "/") {
		if current == "" {
			current = component
		} else {
			current = current + "/" + component
		}
		osPath := filepath.FromSlash(current)
		info, err := root.Lstat(osPath)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("parent %q is a symlink", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("parent %q is not a directory", current)
			}
		case errors.Is(err, fs.ErrNotExist):
			if err := root.Mkdir(osPath, 0o755); err != nil {
				return fmt.Errorf("creating directory %q: %w", current, err)
			}
		default:
			return fmt.Errorf("checking parent %q: %w", current, err)
		}
	}
	return nil
}

func writeRootFileAtomic(root *os.Root, relPath string, content []byte) (retErr error) {
	osPath := filepath.FromSlash(relPath)
	if info, err := root.Lstat(osPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q is a symlink", relPath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", relPath)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	var randBytes [8]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return fmt.Errorf("generating temp filename: %w", err)
	}
	tmpRel := path.Join(path.Dir(relPath), ".tailor-social-"+hex.EncodeToString(randBytes[:])+".tmp")
	tmpOS := filepath.FromSlash(tmpRel)

	f, err := root.OpenFile(tmpOS, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.Remove(tmpOS)
		}
	}()

	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Re-verify target has not become a symlink before replacing.
	if info, err := root.Lstat(osPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is a symlink", relPath)
	}
	if err := root.Rename(tmpOS, osPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
