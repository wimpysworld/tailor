package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/gh"
)

const (
	defaultPublicHTTPTimeout = 10 * time.Second
	defaultDiscordAPIBase    = "https://discord.com/api/v10"
	defaultGitHubWebBase     = "https://github.com"
	starHistoryPerPage       = 100
	maxStarHistoryPages      = 15
	maxSponsorPages          = 5
	maxLiveResponseBytes     = 2 << 20
	starHistoryAcceptHeader  = "application/vnd.github.star+json"
)

var (
	discordInviteCodeRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{2,64}$`)
	linkHeaderSegmentRe  = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="([^"]+)"`)
	publicSponsorHoverRe = regexp.MustCompile(`data-hovercard-url="/(?:users|orgs)/([A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?)/hovercard"`)
)

// StatsData holds live repository metrics fetched from the GitHub API.
type StatsData struct {
	Stars        int
	Release      string
	Language     string
	License      string
	Contributors int
}

// SponsorsData holds live GitHub Sponsors state and active public sponsor
// handles for an account.
type SponsorsData struct {
	Enabled bool
	Handles []string
}

// LiveFetcher fetches live metrics for README social SVGs. Implementations are
// mockable so tests never require live network access.
type LiveFetcher interface {
	FetchRepoStats(ctx context.Context, owner, repo string) (StatsData, error)
	FetchStarHistory(ctx context.Context, owner, repo string) ([]StarPoint, error)
	FetchSponsors(ctx context.Context, login string) (SponsorsData, error)
	FetchDiscussionsCount(ctx context.Context, owner, repo string) (int, error)
	FetchDiscordInvite(ctx context.Context, inviteCode string) (online int, members int, ok bool)
	FetchDiscourseStats(ctx context.Context, baseURL string) (members int, ok bool)
}

// HTTPLiveFetcher implements LiveFetcher using Tailor's go-gh REST/GraphQL
// clients for GitHub endpoints and a bounded unauthenticated HTTP client for
// public Discord, Discourse, and sponsors fallback endpoints.
type HTTPLiveFetcher struct {
	restClient     *api.RESTClient
	starClient     *api.RESTClient
	gqlClient      *api.GraphQLClient
	httpClient     *http.Client
	discordAPIBase string
	githubWebBase  string
}

// NewGitHubLiveFetcher creates an HTTPLiveFetcher bound to the given GitHub
// host using Tailor's existing go-gh authentication path.
func NewGitHubLiveFetcher(host string) (*HTTPLiveFetcher, error) {
	resolved := gh.ResolveHost(host)
	restClient, err := gh.NewRESTClient(resolved)
	if err != nil {
		return nil, fmt.Errorf("creating GitHub REST client: %w", err)
	}
	starClient, err := gh.NewRESTClientWithHeaders(resolved, map[string]string{
		"Accept": starHistoryAcceptHeader,
	})
	if err != nil {
		return nil, fmt.Errorf("creating GitHub stargazers client: %w", err)
	}
	gqlClient, err := gh.NewGraphQLClient(resolved)
	if err != nil {
		return nil, fmt.Errorf("creating GitHub GraphQL client: %w", err)
	}
	return NewHTTPLiveFetcher(restClient, starClient, gqlClient, nil), nil
}

// NewHTTPLiveFetcher creates an HTTPLiveFetcher from explicit clients. When
// httpClient is nil, a bounded client with a 10-second timeout is used.
func NewHTTPLiveFetcher(restClient, starClient *api.RESTClient, gqlClient *api.GraphQLClient, httpClient *http.Client) *HTTPLiveFetcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultPublicHTTPTimeout}
	}
	return &HTTPLiveFetcher{
		restClient:     restClient,
		starClient:     starClient,
		gqlClient:      gqlClient,
		httpClient:     httpClient,
		discordAPIBase: defaultDiscordAPIBase,
		githubWebBase:  defaultGitHubWebBase,
	}
}

// FetchRepoStats fetches stars, latest release tag, primary language, licence
// SPDX identifier, and contributor count for owner/repo.
func (f *HTTPLiveFetcher) FetchRepoStats(ctx context.Context, owner, repo string) (StatsData, error) {
	if f.restClient == nil {
		return StatsData{}, errors.New("GitHub REST client is not configured")
	}
	escOwner := url.PathEscape(owner)
	escRepo := url.PathEscape(repo)

	var repoPayload struct {
		StargazersCount int    `json:"stargazers_count"`
		Language        string `json:"language"`
		License         *struct {
			SPDXID string `json:"spdx_id"`
			Name   string `json:"name"`
		} `json:"license"`
	}
	repoPath := fmt.Sprintf("repos/%s/%s", escOwner, escRepo)
	if err := f.restClient.DoWithContext(ctx, http.MethodGet, repoPath, nil, &repoPayload); err != nil {
		return StatsData{}, fmt.Errorf("fetching repository stats for %s/%s: %w", owner, repo, err)
	}

	stats := StatsData{
		Stars:    repoPayload.StargazersCount,
		Language: strings.TrimSpace(repoPayload.Language),
	}
	if repoPayload.License != nil {
		spdx := strings.TrimSpace(repoPayload.License.SPDXID)
		name := strings.TrimSpace(repoPayload.License.Name)
		if spdx != "" && !strings.EqualFold(spdx, "NOASSERTION") {
			stats.License = spdx
		} else if name != "" && !strings.EqualFold(name, "Other") {
			stats.License = name
		}
	}

	releaseTag, err := f.fetchLatestReleaseTag(ctx, escOwner, escRepo)
	if err != nil {
		return StatsData{}, fmt.Errorf("fetching latest release for %s/%s: %w", owner, repo, err)
	}
	stats.Release = releaseTag

	contributors, err := f.fetchContributorCount(ctx, escOwner, escRepo)
	if err != nil {
		return StatsData{}, fmt.Errorf("fetching contributor count for %s/%s: %w", owner, repo, err)
	}
	stats.Contributors = contributors

	return stats, nil
}

func (f *HTTPLiveFetcher) fetchLatestReleaseTag(ctx context.Context, escOwner, escRepo string) (string, error) {
	var payload struct {
		TagName string `json:"tag_name"`
	}
	relPath := fmt.Sprintf("repos/%s/%s/releases/latest", escOwner, escRepo)
	if err := f.restClient.DoWithContext(ctx, http.MethodGet, relPath, nil, &payload); err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(payload.TagName), nil
}

func (f *HTTPLiveFetcher) fetchContributorCount(ctx context.Context, escOwner, escRepo string) (int, error) {
	contribPath := fmt.Sprintf("repos/%s/%s/contributors?per_page=1&anon=false", escOwner, escRepo)
	resp, err := f.restClient.RequestWithContext(ctx, http.MethodGet, contribPath, nil)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusNoContent) {
			return 0, nil
		}
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return 0, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("unexpected HTTP %d from contributors endpoint", resp.StatusCode)
	}

	if lastPage := parseLinkLastPage(resp.Header.Get("Link")); lastPage > 0 {
		return lastPage, nil
	}

	var entries []json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLiveResponseBytes)).Decode(&entries); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, nil
		}
		return 0, fmt.Errorf("decoding contributors response: %w", err)
	}
	return len(entries), nil
}

type stargazerEvent struct {
	StarredAt time.Time `json:"starred_at"`
}

// FetchStarHistory fetches stargazer timestamps using the
// application/vnd.github.star+json media type. Large repositories are sampled
// evenly across pages up to maxStarHistoryPages so fetches stay fast and
// respect rate limits.
func (f *HTTPLiveFetcher) FetchStarHistory(ctx context.Context, owner, repo string) ([]StarPoint, error) {
	client := f.starClient
	if client == nil {
		client = f.restClient
	}
	if client == nil {
		return nil, errors.New("GitHub stargazers client is not configured")
	}

	escOwner := url.PathEscape(owner)
	escRepo := url.PathEscape(repo)

	page1, lastPage, err := fetchStargazerPage(ctx, client, escOwner, escRepo, 1)
	if err != nil {
		return nil, fmt.Errorf("fetching star history for %s/%s: %w", owner, repo, err)
	}
	if len(page1) == 0 {
		return nil, nil
	}
	if lastPage < 1 {
		lastPage = 1
	}

	if lastPage <= maxStarHistoryPages {
		all := make([]time.Time, 0, lastPage*starHistoryPerPage)
		for _, ev := range page1 {
			if !ev.StarredAt.IsZero() {
				all = append(all, ev.StarredAt.UTC())
			}
		}
		for page := 2; page <= lastPage; page++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			events, _, err := fetchStargazerPage(ctx, client, escOwner, escRepo, page)
			if err != nil {
				if isPaginationLimitError(err) {
					break
				}
				return nil, fmt.Errorf("fetching star history page %d for %s/%s: %w", page, owner, repo, err)
			}
			if len(events) == 0 {
				break
			}
			for _, ev := range events {
				if !ev.StarredAt.IsZero() {
					all = append(all, ev.StarredAt.UTC())
				}
			}
		}
		return aggregateStarPoints(all), nil
	}

	// Sample evenly across large repositories so we stay within maxStarHistoryPages calls.
	pages := sampleStarPages(lastPage, maxStarHistoryPages)
	rawPoints := make([]StarPoint, 0, len(pages)+1)
	if first, ok := firstValidTimestamp(page1); ok {
		rawPoints = append(rawPoints, StarPoint{Date: utcDay(first), Stars: 1})
	}
	if last, ok := lastValidTimestamp(page1); ok {
		rawPoints = append(rawPoints, StarPoint{Date: utcDay(last), Stars: len(page1)})
	}

	for _, page := range pages[1:] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		events, _, err := fetchStargazerPage(ctx, client, escOwner, escRepo, page)
		if err != nil {
			if isPaginationLimitError(err) {
				break
			}
			return nil, fmt.Errorf("fetching sampled star history page %d for %s/%s: %w", page, owner, repo, err)
		}
		if len(events) == 0 {
			continue
		}
		if ts, ok := lastValidTimestamp(events); ok {
			cumulative := (page-1)*starHistoryPerPage + len(events)
			rawPoints = append(rawPoints, StarPoint{Date: utcDay(ts), Stars: cumulative})
		}
	}

	return compactStarPoints(rawPoints), nil
}

func fetchStargazerPage(ctx context.Context, client *api.RESTClient, escOwner, escRepo string, page int) ([]stargazerEvent, int, error) {
	reqPath := fmt.Sprintf("repos/%s/%s/stargazers?per_page=%d&page=%d", escOwner, escRepo, starHistoryPerPage, page)
	resp, err := client.RequestWithContext(ctx, http.MethodGet, reqPath, nil)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusNoContent) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil, 0, nil
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, 0, errPaginationLimit
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, 0, fmt.Errorf("unexpected HTTP %d from stargazers endpoint", resp.StatusCode)
	}

	lastPage := parseLinkLastPage(resp.Header.Get("Link"))
	var events []stargazerEvent
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLiveResponseBytes)).Decode(&events); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, lastPage, nil
		}
		return nil, 0, fmt.Errorf("decoding stargazers response: %w", err)
	}
	return events, lastPage, nil
}

var errPaginationLimit = errors.New("stargazer pagination limit reached")

func isPaginationLimitError(err error) bool {
	if errors.Is(err, errPaginationLimit) {
		return true
	}
	var httpErr *api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity
}

func firstValidTimestamp(events []stargazerEvent) (time.Time, bool) {
	for _, ev := range events {
		if !ev.StarredAt.IsZero() {
			return ev.StarredAt.UTC(), true
		}
	}
	return time.Time{}, false
}

func lastValidTimestamp(events []stargazerEvent) (time.Time, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if !events[i].StarredAt.IsZero() {
			return events[i].StarredAt.UTC(), true
		}
	}
	return time.Time{}, false
}

func utcDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func aggregateStarPoints(timestamps []time.Time) []StarPoint {
	if len(timestamps) == 0 {
		return nil
	}
	sorted := slices.Clone(timestamps)
	slices.SortFunc(sorted, func(a, b time.Time) int {
		return a.Compare(b)
	})

	points := make([]StarPoint, 0, min(len(sorted), 365))
	for i, ts := range sorted {
		day := utcDay(ts)
		count := i + 1
		if len(points) > 0 && points[len(points)-1].Date.Equal(day) {
			points[len(points)-1].Stars = count
			continue
		}
		points = append(points, StarPoint{Date: day, Stars: count})
	}
	return points
}

func compactStarPoints(points []StarPoint) []StarPoint {
	if len(points) == 0 {
		return nil
	}
	sorted := slices.Clone(points)
	slices.SortFunc(sorted, func(a, b StarPoint) int {
		if cmp := a.Date.Compare(b.Date); cmp != 0 {
			return cmp
		}
		return a.Stars - b.Stars
	})

	out := make([]StarPoint, 0, len(sorted))
	for _, pt := range sorted {
		if len(out) > 0 && out[len(out)-1].Date.Equal(pt.Date) {
			if pt.Stars > out[len(out)-1].Stars {
				out[len(out)-1].Stars = pt.Stars
			}
			continue
		}
		out = append(out, pt)
	}
	return out
}

func sampleStarPages(lastPage, maxPages int) []int {
	if lastPage <= 0 {
		return nil
	}
	if maxPages < 2 || lastPage <= maxPages {
		out := make([]int, lastPage)
		for i := range lastPage {
			out[i] = i + 1
		}
		return out
	}

	out := make([]int, 0, maxPages)
	prev := 0
	for i := range maxPages {
		frac := float64(i) / float64(maxPages-1)
		page := 1 + int(math.Round(frac*float64(lastPage-1)))
		if page <= prev {
			page = prev + 1
		}
		if page > lastPage {
			page = lastPage
		}
		out = append(out, page)
		prev = page
		if page == lastPage {
			break
		}
	}
	if out[len(out)-1] != lastPage {
		out[len(out)-1] = lastPage
	}
	return out
}

// FetchSponsors fetches active public GitHub Sponsors handles for login via
// GraphQL, falling back to the public sponsors page when GraphQL is
// unavailable. It returns an empty SponsorsData without error when sponsors is
// disabled or inaccessible.
func (f *HTTPLiveFetcher) FetchSponsors(ctx context.Context, login string) (SponsorsData, error) {
	trimmed := strings.TrimSpace(login)
	if trimmed == "" {
		return SponsorsData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return SponsorsData{}, err
	}

	if f.gqlClient != nil {
		data, err := f.fetchSponsorsGraphQL(ctx, trimmed)
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return SponsorsData{}, ctx.Err()
		}
	}

	return f.fetchSponsorsPublicFallback(ctx, trimmed), nil
}

const sponsorsGraphQLQuery = `query($login: String!, $cursor: String) {
  repositoryOwner(login: $login) {
    ... on Sponsorable {
      hasSponsorsListing
      sponsorshipsAsMaintainer(first: 100, activeOnly: true, includePrivate: false, after: $cursor) {
        nodes {
          sponsorEntity {
            ... on User { login }
            ... on Organization { login }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}`

func (f *HTTPLiveFetcher) fetchSponsorsGraphQL(ctx context.Context, login string) (SponsorsData, error) {
	var (
		result SponsorsData
		cursor any
		seen   = make(map[string]struct{})
	)

	for range maxSponsorPages {
		if err := ctx.Err(); err != nil {
			return SponsorsData{}, err
		}
		vars := map[string]any{
			"login":  login,
			"cursor": cursor,
		}
		var resp struct {
			RepositoryOwner *struct {
				HasSponsorsListing       bool `json:"hasSponsorsListing"`
				SponsorshipsAsMaintainer struct {
					Nodes []struct {
						SponsorEntity *struct {
							Login string `json:"login"`
						} `json:"sponsorEntity"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"sponsorshipsAsMaintainer"`
			} `json:"repositoryOwner"`
		}
		if err := f.gqlClient.DoWithContext(ctx, sponsorsGraphQLQuery, vars, &resp); err != nil {
			return SponsorsData{}, err
		}
		if resp.RepositoryOwner == nil {
			return SponsorsData{}, nil
		}
		if resp.RepositoryOwner.HasSponsorsListing {
			result.Enabled = true
		}
		for _, node := range resp.RepositoryOwner.SponsorshipsAsMaintainer.Nodes {
			if node.SponsorEntity == nil {
				continue
			}
			handle := strings.TrimSpace(node.SponsorEntity.Login)
			if handle == "" {
				continue
			}
			key := strings.ToLower(handle)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			result.Handles = append(result.Handles, handle)
		}
		if len(result.Handles) > 0 {
			result.Enabled = true
		}
		if !resp.RepositoryOwner.SponsorshipsAsMaintainer.PageInfo.HasNextPage ||
			resp.RepositoryOwner.SponsorshipsAsMaintainer.PageInfo.EndCursor == "" {
			break
		}
		cursor = resp.RepositoryOwner.SponsorshipsAsMaintainer.PageInfo.EndCursor
	}
	return result, nil
}

func (f *HTTPLiveFetcher) fetchSponsorsPublicFallback(ctx context.Context, login string) SponsorsData {
	if f.httpClient == nil {
		return SponsorsData{}
	}
	base := strings.TrimRight(f.githubWebBase, "/")
	if base == "" {
		return SponsorsData{}
	}
	endpoint := base + "/sponsors/" + url.PathEscape(login)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SponsorsData{}
	}
	req.Header.Set("User-Agent", "tailor")
	req.Header.Set("Accept", "text/html")

	resp, err := f.httpClient.Do(req) //nolint:gosec // Endpoint is restricted to githubWebBase + escaped login.
	if err != nil {
		return SponsorsData{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SponsorsData{}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLiveResponseBytes))
	if err != nil {
		return SponsorsData{Enabled: true}
	}
	return SponsorsData{
		Enabled: true,
		Handles: extractPublicSponsorHandles(string(body), login),
	}
}

func extractPublicSponsorHandles(htmlBody, ownerLogin string) []string {
	matches := publicSponsorHoverRe.FindAllStringSubmatch(htmlBody, -1)
	if len(matches) == 0 {
		return nil
	}
	ownerLower := strings.ToLower(strings.TrimSpace(ownerLogin))
	seen := make(map[string]struct{}, len(matches))
	var handles []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		handle := strings.TrimSpace(m[1])
		key := strings.ToLower(handle)
		if handle == "" || key == ownerLower {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		handles = append(handles, handle)
	}
	return handles
}

const discussionsCountGraphQLQuery = `query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    hasDiscussionsEnabled
    discussions {
      totalCount
    }
  }
}`

// FetchDiscussionsCount fetches the total GitHub Discussions count for
// owner/repo via GraphQL. It returns 0 when Discussions are disabled.
func (f *HTTPLiveFetcher) FetchDiscussionsCount(ctx context.Context, owner, repo string) (int, error) {
	if f.gqlClient == nil {
		return 0, errors.New("GitHub GraphQL client is not configured")
	}
	var resp struct {
		Repository *struct {
			HasDiscussionsEnabled bool `json:"hasDiscussionsEnabled"`
			Discussions           struct {
				TotalCount int `json:"totalCount"`
			} `json:"discussions"`
		} `json:"repository"`
	}
	vars := map[string]any{
		"owner": owner,
		"repo":  repo,
	}
	if err := f.gqlClient.DoWithContext(ctx, discussionsCountGraphQLQuery, vars, &resp); err != nil {
		return 0, fmt.Errorf("fetching discussions count for %s/%s: %w", owner, repo, err)
	}
	if resp.Repository == nil || !resp.Repository.HasDiscussionsEnabled {
		return 0, nil
	}
	return max(resp.Repository.Discussions.TotalCount, 0), nil
}

// FetchDiscordInvite queries the public Discord invite API for approximate
// online and total member counts. Any network, HTTP, or parse error falls back
// gracefully with ok=false.
func (f *HTTPLiveFetcher) FetchDiscordInvite(ctx context.Context, inviteCode string) (online int, members int, ok bool) {
	code := strings.TrimSpace(inviteCode)
	if !discordInviteCodeRe.MatchString(code) || f.httpClient == nil {
		return 0, 0, false
	}
	base := strings.TrimRight(f.discordAPIBase, "/")
	if base == "" {
		return 0, 0, false
	}
	endpoint := base + "/invites/" + url.PathEscape(code) + "?with_counts=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, 0, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tailor")

	resp, err := f.httpClient.Do(req) //nolint:gosec // Endpoint is restricted to discordAPIBase + validated invite code.
	if err != nil {
		return 0, 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, false
	}
	var payload struct {
		ApproximatePresenceCount int `json:"approximate_presence_count"`
		ApproximateMemberCount   int `json:"approximate_member_count"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLiveResponseBytes)).Decode(&payload); err != nil {
		return 0, 0, false
	}
	if payload.ApproximatePresenceCount < 0 {
		payload.ApproximatePresenceCount = 0
	}
	if payload.ApproximateMemberCount < 0 {
		payload.ApproximateMemberCount = 0
	}
	if payload.ApproximatePresenceCount == 0 && payload.ApproximateMemberCount == 0 {
		return 0, 0, false
	}
	return payload.ApproximatePresenceCount, payload.ApproximateMemberCount, true
}

// FetchDiscourseStats queries a public Discourse forum's /about.json endpoint
// for total user count. Any error falls back gracefully with ok=false.
func (f *HTTPLiveFetcher) FetchDiscourseStats(ctx context.Context, baseURL string) (members int, ok bool) {
	if f.httpClient == nil {
		return 0, false
	}
	normalized, valid := normalizePublicHTTPURL(baseURL)
	if !valid {
		return 0, false
	}
	endpoint := normalized + "/about.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tailor")

	resp, err := f.httpClient.Do(req) //nolint:gosec // Public forum URL validated by normalizePublicHTTPURL.
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	var payload struct {
		About struct {
			Stats struct {
				UsersCount int `json:"users_count"`
				UserCount  int `json:"user_count"`
			} `json:"stats"`
		} `json:"about"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLiveResponseBytes)).Decode(&payload); err != nil {
		return 0, false
	}
	count := payload.About.Stats.UsersCount
	if count <= 0 {
		count = payload.About.Stats.UserCount
	}
	if count <= 0 {
		return 0, false
	}
	return count, true
}

// ExtractDiscordInviteCode extracts a Discord invite code from a discord.gg or
// discord.com/invite URL. It returns an empty string for non-invite or vanity
// redirect URLs.
func ExtractDiscordInviteCode(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	trimmedPath := strings.Trim(u.Path, "/")
	if trimmedPath == "" {
		return ""
	}
	var code string
	switch host {
	case "discord.gg":
		code, _, _ = strings.Cut(trimmedPath, "/")
	case "discord.com", "www.discord.com", "discordapp.com", "www.discordapp.com":
		rest, ok := strings.CutPrefix(trimmedPath, "invite/")
		if !ok {
			return ""
		}
		code, _, _ = strings.Cut(rest, "/")
	default:
		return ""
	}
	if !discordInviteCodeRe.MatchString(code) {
		return ""
	}
	return code
}

// ExtractGitHubSponsorsLogin extracts the GitHub account login from a
// https://github.com/sponsors/<login> URL, or returns an empty string.
func ExtractGitHubSponsorsLogin(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return ""
	}
	trimmedPath := strings.Trim(u.Path, "/")
	rest, ok := strings.CutPrefix(trimmedPath, "sponsors/")
	if !ok || rest == "" {
		return ""
	}
	login, _, _ := strings.Cut(rest, "/")
	return strings.TrimSpace(login)
}

func normalizePublicHTTPURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", false
	}
	if u.Hostname() == "" || u.User != nil {
		return "", false
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), true
}

func parseLinkLastPage(linkHeader string) int {
	if strings.TrimSpace(linkHeader) == "" {
		return 0
	}
	matches := linkHeaderSegmentRe.FindAllStringSubmatch(linkHeader, -1)
	for _, m := range matches {
		if len(m) < 3 || m[2] != "last" {
			continue
		}
		u, err := url.Parse(m[1])
		if err != nil {
			continue
		}
		pageStr := u.Query().Get("page")
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			return page
		}
	}
	return 0
}
