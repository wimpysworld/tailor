package social

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

type rewriteHostTransport struct {
	targetHost string
}

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = t.targetHost
	return http.DefaultTransport.RoundTrip(clone)
}

func newTestGitHubClients(t *testing.T, server *httptest.Server) (restClient, starClient *api.RESTClient, gqlClient *api.GraphQLClient) {
	t.Helper()
	transport := &rewriteHostTransport{targetHost: server.Listener.Addr().String()}

	var err error
	restClient, err = api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewRESTClient: %v", err)
	}
	starClient, err = api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Headers:   map[string]string{"Accept": starHistoryAcceptHeader},
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewRESTClient (star): %v", err)
	}
	gqlClient, err = api.NewGraphQLClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewGraphQLClient: %v", err)
	}
	return restClient, starClient, gqlClient
}

func TestHTTPLiveFetcherFetchRepoStats(t *testing.T) {
	t.Run("fetches 4-column health metrics via GraphQL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/graphql" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"repository":{"createdAt":"2026-03-14T10:20:30Z","defaultBranchRef":{"target":{"history":{"totalCount":750}}},"openIssues":{"totalCount":2},"closedIssues":{"totalCount":41},"openPRs":{"totalCount":3},"mergedPRs":{"totalCount":180}}}}`)
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

		got, err := fetcher.FetchRepoStats(context.Background(), "wimpysworld", "tailor")
		if err != nil {
			t.Fatalf("FetchRepoStats() error = %v", err)
		}
		want := StatsData{
			Commits:      750,
			OpenIssues:   2,
			ClosedIssues: 41,
			OpenPRs:      3,
			MergedPRs:    180,
			CreatedAt:    time.Date(2026, 3, 14, 10, 20, 30, 0, time.UTC),
		}
		if got != want {
			t.Errorf("FetchRepoStats() = %+v, want %+v", got, want)
		}
	})

	t.Run("handles empty repository with null defaultBranchRef", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/graphql" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"repository":{"createdAt":"","defaultBranchRef":null,"openIssues":{"totalCount":0},"closedIssues":{"totalCount":0},"openPRs":{"totalCount":0},"mergedPRs":{"totalCount":0}}}}`)
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

		got, err := fetcher.FetchRepoStats(context.Background(), "octocat", "empty")
		if err != nil {
			t.Fatalf("FetchRepoStats() error = %v", err)
		}
		if got.Commits != 0 || !got.CreatedAt.IsZero() {
			t.Errorf("FetchRepoStats(empty) = %+v, want zero commits and zero CreatedAt", got)
		}
	})
}

func TestHTTPLiveFetcherFetchStarHistory(t *testing.T) {
	t.Run("sends star+json accept header and aggregates multi-page history", func(t *testing.T) {
		var sawStarAccept atomic.Bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/wimpysworld/tailor/stargazers" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Header.Get("Accept") == starHistoryAcceptHeader {
				sawStarAccept.Store(true)
			}
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Query().Get("page") {
			case "1":
				w.Header().Set("Link", `<https://api.github.com/repos/wimpysworld/tailor/stargazers?per_page=100&page=2>; rel="last"`)
				fmt.Fprint(w, `[
					{"starred_at":"2025-01-01T10:00:00Z"},
					{"starred_at":"2025-01-01T15:00:00Z"},
					{"starred_at":"2025-01-15T08:00:00Z"}
				]`)
			case "2":
				fmt.Fprint(w, `[
					{"starred_at":"2025-02-10T12:00:00Z"},
					{"starred_at":"2025-02-10T18:00:00Z"}
				]`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

		pts, err := fetcher.FetchStarHistory(context.Background(), "wimpysworld", "tailor")
		if err != nil {
			t.Fatalf("FetchStarHistory() error = %v", err)
		}
		if !sawStarAccept.Load() {
			t.Error("expected Accept: application/vnd.github.star+json header")
		}
		if len(pts) != 3 {
			t.Fatalf("len(pts) = %d, want 3: %+v", len(pts), pts)
		}
		if pts[0].Stars != 2 || pts[1].Stars != 3 || pts[2].Stars != 5 {
			t.Errorf("cumulative counts = [%d, %d, %d], want [2, 3, 5]", pts[0].Stars, pts[1].Stars, pts[2].Stars)
		}
	})

	t.Run("samples large repos within maxStarHistoryPages requests", func(t *testing.T) {
		var requestCount atomic.Int32
		baseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/large/repo/stargazers" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			requestCount.Add(1)
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			w.Header().Set("Content-Type", "application/json")
			if page == 1 {
				w.Header().Set("Link", `<https://api.github.com/repos/large/repo/stargazers?per_page=100&page=100>; rel="last"`)
			}
			events := make([]stargazerEvent, 0, starHistoryPerPage)
			for i := range starHistoryPerPage {
				ts := baseDate.AddDate(0, 0, (page-1)*5+i/25)
				events = append(events, stargazerEvent{StarredAt: ts})
			}
			if err := json.NewEncoder(w).Encode(events); err != nil {
				t.Errorf("Encode: %v", err)
			}
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

		pts, err := fetcher.FetchStarHistory(context.Background(), "large", "repo")
		if err != nil {
			t.Fatalf("FetchStarHistory() error = %v", err)
		}
		if got := int(requestCount.Load()); got > maxStarHistoryPages {
			t.Errorf("fetched %d pages, want <= %d", got, maxStarHistoryPages)
		}
		if len(pts) < 2 {
			t.Fatalf("expected sampled star points, got %d", len(pts))
		}
		if pts[0].Stars != 1 {
			t.Errorf("first point stars = %d, want 1", pts[0].Stars)
		}
		if last := pts[len(pts)-1].Stars; last != 10000 {
			t.Errorf("last point stars = %d, want 10000", last)
		}
		for i := 1; i < len(pts); i++ {
			if pts[i].Date.Before(pts[i-1].Date) || pts[i].Stars < pts[i-1].Stars {
				t.Errorf("points not monotonic at %d: prev=%+v cur=%+v", i, pts[i-1], pts[i])
			}
		}
	})
}

func TestHTTPLiveFetcherFetchSponsors(t *testing.T) {
	t.Run("GraphQL returns paginated active sponsor handles for user", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/graphql" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if calls.Add(1) == 1 {
				fmt.Fprint(w, `{"data":{"user":{"hasSponsorsListing":true,"sponsorshipsAsMaintainer":{"nodes":[{"sponsorEntity":{"login":"alice"}},{"sponsorEntity":{"login":"acme-org"}}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"}}}}}`)
				return
			}
			fmt.Fprint(w, `{"data":{"user":{"hasSponsorsListing":true,"sponsorshipsAsMaintainer":{"nodes":[{"sponsorEntity":{"login":"bob"}},{"sponsorEntity":{"login":"Alice"}}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

		got, err := fetcher.FetchSponsors(context.Background(), "flexiondotorg")
		if err != nil {
			t.Fatalf("FetchSponsors() error = %v", err)
		}
		if !got.Enabled {
			t.Error("expected Enabled = true")
		}
		want := []string{"alice", "acme-org", "bob"}
		if strings.Join(got.Handles, ",") != strings.Join(want, ",") {
			t.Errorf("Handles = %v, want %v", got.Handles, want)
		}
	})

	t.Run("GraphQL falls through from user to organization", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/graphql" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var body struct {
				Query string `json:"query"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(body.Query, "user(login:") {
				fmt.Fprint(w, `{"data":{"user":null}}`)
				return
			}
			fmt.Fprint(w, `{"data":{"organization":{"hasSponsorsListing":true,"sponsorshipsAsMaintainer":{"nodes":[{"sponsorEntity":{"login":"org-backer"}}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

		got, err := fetcher.FetchSponsors(context.Background(), "wimpysworld")
		if err != nil {
			t.Fatalf("FetchSponsors() error = %v", err)
		}
		if !got.Enabled || len(got.Handles) != 1 || got.Handles[0] != "org-backer" {
			t.Errorf("FetchSponsors(org) = %+v, want Enabled=true Handles=[org-backer]", got)
		}
	})

	t.Run("falls back to public sponsors HTML when GraphQL returns empty nodes", func(t *testing.T) {
		gqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"user":{"hasSponsorsListing":true,"sponsorshipsAsMaintainer":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
		}))
		defer gqlSrv.Close()

		webSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/sponsors/flexiondotorg":
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<include-fragment src="/sponsors/flexiondotorg/sponsors_partial?filter=active"></include-fragment>`)
			case "/sponsors/flexiondotorg/sponsors_partial":
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<a data-hovercard-url="/users/flexiondotorg/hovercard"></a><a data-hovercard-url="/users/supporter1/hovercard"></a><img alt="@supporter2">`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer webSrv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, gqlSrv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, webSrv.Client())
		fetcher.githubWebBase = webSrv.URL

		got, err := fetcher.FetchSponsors(context.Background(), "flexiondotorg")
		if err != nil {
			t.Fatalf("FetchSponsors() error = %v", err)
		}
		if !got.Enabled {
			t.Error("expected Enabled = true from public fallback")
		}
		want := []string{"supporter1", "supporter2"}
		if strings.Join(got.Handles, ",") != strings.Join(want, ",") {
			t.Errorf("Handles = %v, want %v", got.Handles, want)
		}
	})

	t.Run("falls back to public sponsors page when GraphQL fails", func(t *testing.T) {
		gqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
		}))
		defer gqlSrv.Close()

		webSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/sponsors/flexiondotorg" {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<a data-hovercard-url="/users/flexiondotorg/hovercard"></a><a data-hovercard-url="/users/supporter1/hovercard"></a><a data-hovercard-url="/orgs/supporter-org/hovercard"></a>`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer webSrv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, gqlSrv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, webSrv.Client())
		fetcher.githubWebBase = webSrv.URL

		got, err := fetcher.FetchSponsors(context.Background(), "flexiondotorg")
		if err != nil {
			t.Fatalf("FetchSponsors() error = %v", err)
		}
		if !got.Enabled {
			t.Error("expected Enabled = true from public fallback")
		}
		want := []string{"supporter1", "supporter-org"}
		if strings.Join(got.Handles, ",") != strings.Join(want, ",") {
			t.Errorf("Handles = %v, want %v", got.Handles, want)
		}
	})

	t.Run("returns empty gracefully when sponsors unavailable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		restC, starC, gqlC := newTestGitHubClients(t, srv)
		fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())
		fetcher.githubWebBase = srv.URL

		got, err := fetcher.FetchSponsors(context.Background(), "nobody")
		if err != nil {
			t.Fatalf("FetchSponsors() unexpected error = %v", err)
		}
		if got.Enabled || len(got.Handles) != 0 {
			t.Errorf("got %+v, want empty SponsorsData", got)
		}
	})
}

func TestHTTPLiveFetcherFetchContributors(t *testing.T) {
	pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d}

	avatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("s") != "64" {
			t.Errorf("avatar request missing s=64 query: %s", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/u/1":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngMagic)
		case "/u/2":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html>not an image</html>")
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer avatarSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/wimpysworld/tailor/contributors" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[
			{"login":"dependabot[bot]","type":"Bot","avatar_url":"%s/u/bot"},
			{"login":"flexiondotorg","type":"User","avatar_url":"%s/u/1?v=4"},
			{"login":"renovate-bot","type":"Bot","avatar_url":"%s/u/bot2"},
			{"login":"custom[bot]","type":"User","avatar_url":"%s/u/bot3"},
			{"login":"alice","type":"User","avatar_url":"%s/u/2"},
			{"login":"bob","type":"User","avatar_url":"%s/u/500"}
		]`, avatarSrv.URL, avatarSrv.URL, avatarSrv.URL, avatarSrv.URL, avatarSrv.URL, avatarSrv.URL)
	}))
	defer apiSrv.Close()

	restC, starC, gqlC := newTestGitHubClients(t, apiSrv)
	fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, avatarSrv.Client())

	contributors, total, err := fetcher.FetchContributors(context.Background(), "wimpysworld", "tailor")
	if err != nil {
		t.Fatalf("FetchContributors() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3 human contributors", total)
	}
	if len(contributors) != 3 {
		t.Fatalf("len(contributors) = %d, want 3", len(contributors))
	}
	if contributors[0].Login != "flexiondotorg" || !strings.HasPrefix(contributors[0].AvatarDataURI, "data:image/png;base64,") {
		t.Errorf("contributors[0] = %+v, want flexiondotorg with PNG data URI", contributors[0])
	}
	if contributors[1].Login != "alice" || contributors[1].AvatarDataURI != "" {
		t.Errorf("contributors[1] = %+v, want alice with empty AvatarDataURI on non-image MIME", contributors[1])
	}
	if contributors[2].Login != "bob" || contributors[2].AvatarDataURI != "" {
		t.Errorf("contributors[2] = %+v, want bob with empty AvatarDataURI on HTTP 500", contributors[2])
	}
}

func TestHTTPLiveFetcherFetchDiscussionsCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"repository":{"hasDiscussionsEnabled":true,"discussions":{"totalCount":42}}}}`)
	}))
	defer srv.Close()

	restC, starC, gqlC := newTestGitHubClients(t, srv)
	fetcher := NewHTTPLiveFetcher(restC, starC, gqlC, srv.Client())

	count, err := fetcher.FetchDiscussionsCount(context.Background(), "wimpysworld", "tailor")
	if err != nil {
		t.Fatalf("FetchDiscussionsCount() error = %v", err)
	}
	if count != 42 {
		t.Errorf("count = %d, want 42", count)
	}
}

func TestHTTPLiveFetcherFetchDiscordInvite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/invites/validCode":
			if r.URL.Query().Get("with_counts") != "true" {
				t.Errorf("missing with_counts=true query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"approximate_presence_count":87,"approximate_member_count":640}`)
		case "/invites/badJSON":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{invalid`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	fetcher := NewHTTPLiveFetcher(nil, nil, nil, srv.Client())
	fetcher.discordAPIBase = srv.URL

	online, members, ok := fetcher.FetchDiscordInvite(context.Background(), "validCode")
	if !ok || online != 87 || members != 640 {
		t.Fatalf("FetchDiscordInvite(validCode) = (%d, %d, %v), want (87, 640, true)", online, members, ok)
	}

	if _, _, ok := fetcher.FetchDiscordInvite(context.Background(), "missing"); ok {
		t.Error("expected ok=false on 404 invite")
	}
	if _, _, ok := fetcher.FetchDiscordInvite(context.Background(), "badJSON"); ok {
		t.Error("expected ok=false on malformed JSON")
	}
	if _, _, ok := fetcher.FetchDiscordInvite(context.Background(), "../bad/path"); ok {
		t.Error("expected ok=false on invalid invite code")
	}
}

func TestHTTPLiveFetcherFetchDiscourseStats(t *testing.T) {
	t.Run("parses users_count and legacy user_count", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/modern/about.json":
				fmt.Fprint(w, `{"about":{"stats":{"users_count":512}}}`)
			case "/legacy/about.json":
				fmt.Fprint(w, `{"about":{"stats":{"user_count":205}}}`)
			case "/broken/about.json":
				fmt.Fprint(w, `not json`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		fetcher := NewHTTPLiveFetcher(nil, nil, nil, srv.Client())

		members, ok := fetcher.FetchDiscourseStats(context.Background(), srv.URL+"/modern/")
		if !ok || members != 512 {
			t.Errorf("modern FetchDiscourseStats = (%d, %v), want (512, true)", members, ok)
		}

		members, ok = fetcher.FetchDiscourseStats(context.Background(), srv.URL+"/legacy")
		if !ok || members != 205 {
			t.Errorf("legacy FetchDiscourseStats = (%d, %v), want (205, true)", members, ok)
		}

		if _, ok := fetcher.FetchDiscourseStats(context.Background(), srv.URL+"/broken"); ok {
			t.Error("expected ok=false for malformed JSON")
		}
		if _, ok := fetcher.FetchDiscourseStats(context.Background(), srv.URL+"/missing"); ok {
			t.Error("expected ok=false for 404")
		}
		if _, ok := fetcher.FetchDiscourseStats(context.Background(), "ftp://invalid.example"); ok {
			t.Error("expected ok=false for non-HTTP scheme")
		}
	})
}

func TestLiveHelpers(t *testing.T) {
	discordCases := map[string]string{
		"https://discord.gg/tailor123":             "tailor123",
		"https://discord.com/invite/AbC_123-xy":    "AbC_123-xy",
		"https://discordapp.com/invite/teamInvite": "teamInvite",
		"https://wimpysworld.io/discord":           "",
		"https://discord.com/channels/123/456":     "",
		"not a url":                                "",
	}
	for raw, want := range discordCases {
		if got := ExtractDiscordInviteCode(raw); got != want {
			t.Errorf("ExtractDiscordInviteCode(%q) = %q, want %q", raw, got, want)
		}
	}

	sponsorCases := map[string]string{
		"https://github.com/sponsors/flexiondotorg":                          "flexiondotorg",
		"https://github.com/sponsors/wimpysworld/":                           "wimpysworld",
		"https://github.com/sponsors/flexiondotorg?frequency=recurring#tier": "flexiondotorg",
		"https://patreon.com/wimpysworld":                                    "",
		"https://github.com/wimpysworld/tailor":                              "",
	}
	for raw, want := range sponsorCases {
		if got := ExtractGitHubSponsorsLogin(raw); got != want {
			t.Errorf("ExtractGitHubSponsorsLogin(%q) = %q, want %q", raw, got, want)
		}
	}

	if got := parseLinkLastPage(`<https://api.github.com/r?page=2>; rel="next", <https://api.github.com/r?page=42>; rel="last"`); got != 42 {
		t.Errorf("parseLinkLastPage() = %d, want 42", got)
	}
	if got := parseLinkLastPage(`<https://api.github.com/r?page=2>; rel="next"`); got != 0 {
		t.Errorf("parseLinkLastPage(no last) = %d, want 0", got)
	}
}
