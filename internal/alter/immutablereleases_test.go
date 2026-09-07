package alter_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestProcessImmutableReleases(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		mode                 alter.ApplyMode
		desired, live, owner bool
		status               int
		want                 string
		writes               int
	}{
		{"preview enable", alter.DryRun, true, false, false, 204, "would set:", 0},
		{"preview disable", alter.DryRun, false, true, false, 204, "would set:", 0},
		{"enable", alter.Apply, true, false, false, 204, "set:", 1},
		{"disable", alter.Apply, false, true, false, 204, "set:", 1},
		{"recut", alter.Recut, true, false, false, 204, "set:", 1},
		{"no change", alter.Apply, false, false, false, 204, "no change:", 0},
		{"enforced preview", alter.DryRun, false, true, true, 204, "would skip (enforced by owner):", 0},
		{"enforced apply", alter.Apply, false, true, true, 204, "would skip (enforced by owner):", 0},
		{"enforced enabled", alter.Apply, true, true, true, 204, "no change:", 0},
		{"write forbidden", alter.Apply, true, false, false, 403, "would skip (insufficient scope):", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/o/r/immutable-releases" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if r.Method == http.MethodGet {
					fmt.Fprintf(w, `{"enabled":%t,"enforced_by_owner":%t}`, tt.live, tt.owner)
					return
				}
				writes++
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{ImmutableReleases: &model.ImmutableReleasesSettings{Enabled: new(tt.desired)}}
			results, err := alter.ProcessImmutableReleases(cfg, tt.mode, repoTarget(testutil.NewTestClient(t, server), "o", "r", true))
			if err != nil {
				t.Fatal(err)
			}
			output := alter.FormatOutput(results, nil, nil, tt.mode)
			if !strings.Contains(output, tt.want) || !strings.Contains(output, "immutable_releases.enabled") || writes != tt.writes {
				t.Fatalf("output = %q, writes = %d", output, writes)
			}
			if tt.status == 403 && strings.Contains(output, "set:") {
				t.Fatalf("false success: %s", output)
			}
		})
	}
}

func TestProcessImmutableReleasesOmission(t *testing.T) {
	for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply, alter.Recut} {
		for _, cfg := range []*config.Config{{}, {ImmutableReleases: &model.ImmutableReleasesSettings{}}} {
			if _, err := config.MergeDefaults(cfg); err != nil {
				t.Fatal(err)
			}
			results, err := alter.ProcessImmutableReleases(cfg, mode, repoTarget(nil, "o", "r", true))
			if err != nil || len(results) != 0 {
				t.Fatalf("results = %v, error = %v", results, err)
			}
		}
	}
}

func TestProcessImmutableReleasesReadFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusConflict, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s", r.Method)
				}
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{ImmutableReleases: &model.ImmutableReleasesSettings{Enabled: new(false)}}
			results, err := alter.ProcessImmutableReleases(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "o", "r", true))
			if status != http.StatusForbidden {
				if err == nil {
					t.Fatal("read failure did not stop processing")
				}
				return
			}
			if err != nil || len(results) != 1 || results[0].Category != alter.WouldSkipSetup {
				t.Fatalf("access error did not produce a nonfatal skip: %v, %v", results, err)
			}
		})
	}
}
