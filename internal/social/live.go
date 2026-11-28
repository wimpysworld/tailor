package social

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
	defaultTelegramWebBase   = "https://t.me"
	defaultGitHubWebBase     = "https://github.com"
	starHistoryPerPage       = 100
	maxStarHistoryPages      = 15
	maxSponsorPages          = 5
	maxContributorsDisplayed = 20
	avatarFetchTimeout       = 5 * time.Second
	maxAvatarBytes           = 256 << 10
	maxLiveResponseBytes     = 2 << 20
	starHistoryAcceptHeader  = "application/vnd.github.star+json"
)

var (
	discordInviteCodeRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{2,64}$`)
	telegramPathSegmentRe = regexp.MustCompile(`^[A-Za-z0-9_+-]{1,128}$`)
	telegramPageExtraRe   = regexp.MustCompile(`(?is)<div\s+class="[^"]*\btgme_page_extra\b[^"]*"[^>]*>(.*?)</div>`)
	telegramMemberCountRe = regexp.MustCompile(`(?i)(\d[\d\s,\x{00a0}]*)\s+(?:members?|subscribers?)\b`)
	linkHeaderSegmentRe   = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="([^"]+)"`)
	publicSponsorHoverRe  = regexp.MustCompile(`data-hovercard-url="/(?:users|orgs)/([A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?)/hovercard"`)
	publicSponsorAltRe    = regexp.MustCompile(`alt="@([A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?)"`)
)

// StatsData holds live repository health metrics fetched from the GitHub API.
type StatsData struct {
	Commits      int
	OpenIssues   int
	ClosedIssues int
	OpenPRs      int
	MergedPRs    int
	CreatedAt    time.Time
}

// Contributor holds a human repository contributor's login and optional
// base64-encoded avatar data URI.
type Contributor struct {
	Login         string
	AvatarDataURI string
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
	FetchContributors(ctx context.Context, owner, repo string) ([]Contributor, int, error)
	FetchDiscussionsCount(ctx context.Context, owner, repo string) (int, error)
	FetchDiscordInvite(ctx context.Context, inviteCode string) (online int, members int, ok bool)
	FetchTelegramMembers(ctx context.Context, inviteURL string) (members int, ok bool)
	FetchDiscourseStats(ctx context.Context, baseURL string) (members int, ok bool)
}

// HTTPLiveFetcher implements LiveFetcher using Tailor's go-gh REST/GraphQL
// clients for GitHub endpoints and a bounded unauthenticated HTTP client for
// public Discord, Telegram, Discourse, and sponsors fallback endpoints.
type HTTPLiveFetcher struct {
	restClient      *api.RESTClient
	starClient      *api.RESTClient
	gqlClient       *api.GraphQLClient
	httpClient      *http.Client
	discordAPIBase  string
	telegramWebBase string
	githubWebBase   string
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
		restClient:      restClient,
		starClient:      starClient,
		gqlClient:       gqlClient,
		httpClient:      httpClient,
		discordAPIBase:  defaultDiscordAPIBase,
		telegramWebBase: defaultTelegramWebBase,
		githubWebBase:   defaultGitHubWebBase,
	}
}

const repoStatsGraphQLQuery = `query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    createdAt
    defaultBranchRef {
      target {
        ... on Commit {
          history {
            totalCount
          }
        }
      }
    }
    openIssues: issues(states: OPEN) {
      totalCount
    }
    closedIssues: issues(states: CLOSED) {
      totalCount
    }
    openPRs: pullRequests(states: OPEN) {
      totalCount
    }
    mergedPRs: pullRequests(states: MERGED) {
      totalCount
    }
  }
}`

// FetchRepoStats fetches commit count, open/closed issues, open/merged pull
// requests, and creation date for owner/repo via GitHub GraphQL.
func (f *HTTPLiveFetcher) FetchRepoStats(ctx context.Context, owner, repo string) (StatsData, error) {
	if f.gqlClient == nil {
		return StatsData{}, errors.New("GitHub GraphQL client is not configured")
	}

	var resp struct {
		Repository *struct {
			CreatedAt        string `json:"createdAt"`
			DefaultBranchRef *struct {
				Target *struct {
					History struct {
						TotalCount int `json:"totalCount"`
					} `json:"history"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
			OpenIssues struct {
				TotalCount int `json:"totalCount"`
			} `json:"openIssues"`
			ClosedIssues struct {
				TotalCount int `json:"totalCount"`
			} `json:"closedIssues"`
			OpenPRs struct {
				TotalCount int `json:"totalCount"`
			} `json:"openPRs"`
			MergedPRs struct {
				TotalCount int `json:"totalCount"`
			} `json:"mergedPRs"`
		} `json:"repository"`
	}
	vars := map[string]any{
		"owner": owner,
		"repo":  repo,
	}
	if err := f.gqlClient.DoWithContext(ctx, repoStatsGraphQLQuery, vars, &resp); err != nil {
		return StatsData{}, fmt.Errorf("fetching repository stats for %s/%s: %w", owner, repo, err)
	}
	if resp.Repository == nil {
		return StatsData{}, nil
	}

	stats := StatsData{
		OpenIssues:   max(resp.Repository.OpenIssues.TotalCount, 0),
		ClosedIssues: max(resp.Repository.ClosedIssues.TotalCount, 0),
		OpenPRs:      max(resp.Repository.OpenPRs.TotalCount, 0),
		MergedPRs:    max(resp.Repository.MergedPRs.TotalCount, 0),
	}
	if resp.Repository.DefaultBranchRef != nil && resp.Repository.DefaultBranchRef.Target != nil {
		stats.Commits = max(resp.Repository.DefaultBranchRef.Target.History.TotalCount, 0)
	}
	if raw := strings.TrimSpace(resp.Repository.CreatedAt); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			stats.CreatedAt = parsed.UTC()
		}
	}
	return stats, nil
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
// user and organization GraphQL queries, falling back to the public sponsors
// HTML page when GraphQL fails or returns no handles (for example when
// GITHUB_TOKEN lacks read:user/read:org scopes).
func (f *HTTPLiveFetcher) FetchSponsors(ctx context.Context, login string) (SponsorsData, error) {
	trimmed := strings.TrimSpace(login)
	if trimmed == "" {
		return SponsorsData{}, nil
	}
	if err := ctx.Err(); err != nil {
		return SponsorsData{}, err
	}

	var gqlData SponsorsData
	if f.gqlClient != nil {
		data, err := f.fetchSponsorsGraphQL(ctx, trimmed)
		if err == nil {
			if len(data.Handles) > 0 {
				return data, nil
			}
			gqlData = data
		}
		if ctx.Err() != nil {
			return SponsorsData{}, ctx.Err()
		}
	}

	fallback := f.fetchSponsorsPublicFallback(ctx, trimmed)
	if len(fallback.Handles) > 0 {
		return SponsorsData{
			Enabled: true,
			Handles: fallback.Handles,
		}, nil
	}
	return SponsorsData{
		Enabled: gqlData.Enabled || fallback.Enabled,
	}, nil
}

const userSponsorsGraphQLQuery = `query($login: String!, $cursor: String) {
  user(login: $login) {
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
}`

const orgSponsorsGraphQLQuery = `query($login: String!, $cursor: String) {
  organization(login: $login) {
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
}`

type sponsorableGraphQLNode struct {
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
}

func (f *HTTPLiveFetcher) fetchSponsorsGraphQL(ctx context.Context, login string) (SponsorsData, error) {
	data, found, err := f.fetchSponsorsGraphQLByKind(ctx, login, true)
	if err == nil && found {
		return data, nil
	}
	if ctx.Err() != nil {
		return SponsorsData{}, ctx.Err()
	}

	orgData, orgFound, orgErr := f.fetchSponsorsGraphQLByKind(ctx, login, false)
	if orgErr == nil && orgFound {
		return orgData, nil
	}
	if err != nil {
		return SponsorsData{}, err
	}
	if orgErr != nil {
		return SponsorsData{}, orgErr
	}
	return SponsorsData{}, nil
}

func (f *HTTPLiveFetcher) fetchSponsorsGraphQLByKind(ctx context.Context, login string, isUser bool) (SponsorsData, bool, error) {
	query := orgSponsorsGraphQLQuery
	if isUser {
		query = userSponsorsGraphQLQuery
	}

	var (
		result SponsorsData
		cursor any
		seen   = make(map[string]struct{})
	)

	for range maxSponsorPages {
		if err := ctx.Err(); err != nil {
			return SponsorsData{}, false, err
		}
		vars := map[string]any{
			"login":  login,
			"cursor": cursor,
		}
		var resp struct {
			User         *sponsorableGraphQLNode `json:"user"`
			Organization *sponsorableGraphQLNode `json:"organization"`
		}
		if err := f.gqlClient.DoWithContext(ctx, query, vars, &resp); err != nil {
			return SponsorsData{}, false, err
		}
		ownerNode := resp.Organization
		if isUser {
			ownerNode = resp.User
		}
		if ownerNode == nil {
			return SponsorsData{}, false, nil
		}
		if ownerNode.HasSponsorsListing {
			result.Enabled = true
		}
		for _, node := range ownerNode.SponsorshipsAsMaintainer.Nodes {
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
		if !ownerNode.SponsorshipsAsMaintainer.PageInfo.HasNextPage ||
			ownerNode.SponsorshipsAsMaintainer.PageInfo.EndCursor == "" {
			break
		}
		cursor = ownerNode.SponsorshipsAsMaintainer.PageInfo.EndCursor
	}
	return result, true, nil
}

func (f *HTTPLiveFetcher) fetchSponsorsPublicFallback(ctx context.Context, login string) SponsorsData {
	if f.httpClient == nil {
		return SponsorsData{}
	}
	base := strings.TrimRight(f.githubWebBase, "/")
	if base == "" {
		return SponsorsData{}
	}
	escLogin := url.PathEscape(login)
	htmlBody, ok := f.fetchPublicHTML(ctx, base+"/sponsors/"+escLogin, true)
	if !ok {
		return SponsorsData{}
	}
	handles := extractPublicSponsorHandles(htmlBody, login)
	if len(handles) == 0 && strings.Contains(htmlBody, "sponsors_partial") {
		if partial, ok := f.fetchPublicHTML(ctx, base+"/sponsors/"+escLogin+"/sponsors_partial?filter=active", false); ok {
			handles = extractPublicSponsorHandles(partial, login)
		}
	}
	return SponsorsData{
		Enabled: true,
		Handles: handles,
	}
}

func (f *HTTPLiveFetcher) fetchPublicHTML(ctx context.Context, endpoint string, requireSponsorsPath bool) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "tailor")
	req.Header.Set("Accept", "text/html")

	resp, err := f.httpClient.Do(req) //nolint:gosec // Endpoint is restricted to githubWebBase + escaped login.
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	if requireSponsorsPath && resp.Request != nil && resp.Request.URL != nil &&
		!strings.Contains(strings.ToLower(resp.Request.URL.Path), "/sponsors/") {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLiveResponseBytes))
	if err != nil {
		return "", true
	}
	return string(body), true
}

func extractPublicSponsorHandles(htmlBody, ownerLogin string) []string {
	hoverMatches := publicSponsorHoverRe.FindAllStringSubmatch(htmlBody, -1)
	altMatches := publicSponsorAltRe.FindAllStringSubmatch(htmlBody, -1)
	if len(hoverMatches) == 0 && len(altMatches) == 0 {
		return nil
	}
	ownerLower := strings.ToLower(strings.TrimSpace(ownerLogin))
	seen := make(map[string]struct{}, len(hoverMatches)+len(altMatches))
	var handles []string
	appendMatches := func(matches [][]string) {
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
	}
	appendMatches(hoverMatches)
	appendMatches(altMatches)
	return handles
}

// FetchContributors fetches human repository contributors from GitHub REST,
// filtering out bot accounts, recording the total human count, taking up to
// the top 20 contributors, and embedding base64 avatar data URIs when fetched
// cleanly.
func (f *HTTPLiveFetcher) FetchContributors(ctx context.Context, owner, repo string) ([]Contributor, int, error) {
	if f.restClient == nil {
		return nil, 0, errors.New("GitHub REST client is not configured")
	}
	escOwner := url.PathEscape(owner)
	escRepo := url.PathEscape(repo)
	contribPath := fmt.Sprintf("repos/%s/%s/contributors?per_page=100", escOwner, escRepo)

	resp, err := f.restClient.RequestWithContext(ctx, http.MethodGet, contribPath, nil)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusNoContent) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("fetching contributors for %s/%s: %w", owner, repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil, 0, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, 0, fmt.Errorf("unexpected HTTP %d from contributors endpoint", resp.StatusCode)
	}

	var entries []struct {
		Login     string `json:"login"`
		Type      string `json:"type"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLiveResponseBytes)).Decode(&entries); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("decoding contributors response: %w", err)
	}

	type humanEntry struct {
		login     string
		avatarURL string
	}
	humans := make([]humanEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		login := strings.TrimSpace(entry.Login)
		if login == "" || strings.EqualFold(entry.Type, "Bot") || strings.HasSuffix(strings.ToLower(login), "[bot]") {
			continue
		}
		key := strings.ToLower(login)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		humans = append(humans, humanEntry{
			login:     login,
			avatarURL: strings.TrimSpace(entry.AvatarURL),
		})
	}

	totalCount := len(humans)
	limit := min(totalCount, maxContributorsDisplayed)
	out := make([]Contributor, 0, limit)
	for i := range limit {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		entry := humans[i]
		out = append(out, Contributor{
			Login:         entry.login,
			AvatarDataURI: f.fetchAvatarDataURI(ctx, entry.avatarURL),
		})
	}
	return out, totalCount, nil
}

func (f *HTTPLiveFetcher) fetchAvatarDataURI(ctx context.Context, rawURL string) string {
	if f.httpClient == nil || strings.TrimSpace(rawURL) == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	q := u.Query()
	q.Set("s", "64")
	u.RawQuery = q.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, avatarFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "tailor")
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")

	resp, err := f.httpClient.Do(req) //nolint:gosec // Avatar URL comes from GitHub contributors API and is validated to http/https.
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxAvatarBytes {
		return ""
	}
	mimeType := http.DetectContentType(data)
	switch mimeType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return ""
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
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

// FetchTelegramMembers fetches a public Telegram preview page (https://t.me/...)
// and extracts the member or subscriber count from <div class="tgme_page_extra">.
// Any network, HTTP, URL, or parse error falls back gracefully with ok=false.
func (f *HTTPLiveFetcher) FetchTelegramMembers(ctx context.Context, inviteURL string) (members int, ok bool) {
	if f.httpClient == nil {
		return 0, false
	}
	relPath := extractTelegramPath(inviteURL)
	if relPath == "" {
		return 0, false
	}
	base := strings.TrimRight(f.telegramWebBase, "/")
	if base == "" {
		return 0, false
	}
	endpoint := base + "/" + relPath

	reqCtx, cancel := context.WithTimeout(ctx, defaultPublicHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "tailor")

	resp, err := f.httpClient.Do(req) //nolint:gosec // Endpoint is restricted to telegramWebBase + validated Telegram path.
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLiveResponseBytes))
	if err != nil {
		return 0, false
	}
	return parseTelegramMemberCount(string(body))
}

func extractTelegramPath(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return ""
	}
	switch strings.ToLower(u.Hostname()) {
	case "t.me", "www.t.me", "telegram.me", "www.telegram.me", "telegram.dog", "www.telegram.dog":
	default:
		return ""
	}
	trimmed := strings.Trim(u.Path, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 {
		return ""
	}
	for _, seg := range parts {
		if !telegramPathSegmentRe.MatchString(seg) {
			return ""
		}
	}
	return strings.Join(parts, "/")
}

func parseTelegramMemberCount(htmlBody string) (int, bool) {
	for _, match := range telegramPageExtraRe.FindAllStringSubmatch(htmlBody, -1) {
		if len(match) < 2 {
			continue
		}
		text := strings.ReplaceAll(html.UnescapeString(match[1]), "\u00a0", " ")
		m := telegramMemberCountRe.FindStringSubmatch(text)
		if len(m) < 2 {
			continue
		}
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, m[1])
		if digits == "" {
			continue
		}
		n, err := strconv.Atoi(digits)
		if err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
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
	head, rest, ok := strings.Cut(trimmedPath, "/")
	if !ok || !strings.EqualFold(head, "sponsors") || rest == "" {
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
