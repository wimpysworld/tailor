package gh

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestReadImmutableReleases(t *testing.T) {
	for _, tt := range []struct {
		name         string
		status       int
		body         string
		admin        bool
		workflow     int
		wantError    bool
		wantEnabled  bool
		wantEnforced bool
	}{
		{"enabled", 200, `{"enabled":true,"enforced_by_owner":false}`, false, 200, false, true, false},
		{"disabled JSON", 200, `{"enabled":false,"enforced_by_owner":false}`, false, 200, false, false, false},
		{"owner", 200, `{"enabled":true,"enforced_by_owner":true}`, false, 200, false, true, true},
		{"disabled 404", 404, `{}`, true, 200, false, false, false},
		{"ambiguous 404", 404, `{}`, false, 200, true, false, false},
		{"token denied", 404, `{}`, true, 403, true, false, false},
		{"forbidden", 403, `{}`, false, 200, true, false, false},
		{"server", 500, `{}`, false, 200, true, false, false},
		{"missing enabled", 200, `{"enforced_by_owner":false}`, false, 200, true, false, false},
		{"missing owner", 200, `{"enabled":true}`, false, 200, true, false, false},
		{"malformed", 200, `no`, false, 200, true, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s", r.Method)
				}
				switch r.URL.Path {
				case "/repos/o/r/immutable-releases":
					w.WriteHeader(tt.status)
					fmt.Fprint(w, tt.body)
				case "/repos/o/r":
					fmt.Fprintf(w, `{"permissions":{"admin":%t}}`, tt.admin)
				case "/repos/o/r/actions/permissions/workflow":
					w.WriteHeader(tt.workflow)
					fmt.Fprint(w, `{}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			state, err := ReadImmutableReleases(newTestClient(t, server), "o", "r")
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v", err)
			}
			if err == nil && (*state.Enabled != tt.wantEnabled || *state.EnforcedByOwner != tt.wantEnforced) {
				t.Fatalf("state = %+v", state)
			}
		})
	}
}

func TestApplyImmutableReleases(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, status := range []int{204, 403, 404, 409, 422, 500} {
			t.Run(fmt.Sprintf("%t/%d", enabled, status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					want := http.MethodDelete
					if enabled {
						want = http.MethodPut
					}
					if r.Method != want || r.URL.Path != "/repos/o/r/immutable-releases" {
						t.Errorf("request = %s %s", r.Method, r.URL.Path)
					}
					body, _ := io.ReadAll(r.Body)
					if len(body) != 0 {
						t.Errorf("body = %s", body)
					}
					w.WriteHeader(status)
				}))
				t.Cleanup(server.Close)
				err := ApplyImmutableReleases(newTestClient(t, server), "o", "r", enabled)
				if (err != nil) != (status != 204) {
					t.Fatalf("error = %v", err)
				}
				if status == 409 {
					var httpErr *api.HTTPError
					if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusConflict {
						t.Fatalf("conflict = %v", err)
					}
				}
			})
		}
	}
}
