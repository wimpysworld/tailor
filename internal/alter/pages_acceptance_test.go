package alter_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// pagesAcceptanceAPI keeps live state across command runs and records every write.
type pagesAcceptanceAPI struct {
	mu       sync.Mutex
	branch   string
	homepage string
	exists   bool
	writes   []apiCall
	reads    []string
}

func newPagesAcceptanceAPI(t *testing.T) (*pagesAcceptanceAPI, *api.RESTClient) {
	t.Helper()
	s := &pagesAcceptanceAPI{branch: "main", homepage: "https://github.com/testowner/testrepo"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		const repo = "/repos/testowner/testrepo"
		if r.Method != http.MethodGet {
			body, _ := io.ReadAll(r.Body)
			s.writes = append(s.writes, apiCall{Method: r.Method, Path: r.URL.Path, Body: string(body)})
			switch {
			case r.Method == http.MethodPost && r.URL.Path == repo+"/pages":
				s.exists = true
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `{}`)
			case r.Method == http.MethodPatch && r.URL.Path == repo:
				var body struct {
					Homepage *string `json:"homepage"`
				}
				if err := json.Unmarshal([]byte(s.writes[len(s.writes)-1].Body), &body); err != nil {
					t.Error(err)
				}
				if body.Homepage != nil {
					s.homepage = *body.Homepage
				}
				fmt.Fprint(w, `{}`)
			default:
				t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		s.reads = append(s.reads, r.URL.Path)
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"login":"testuser"}`)
		case repo:
			w.Header().Set("X-OAuth-Scopes", "repo")
			_ = json.NewEncoder(w).Encode(map[string]any{"private": false, "default_branch": s.branch, "homepage": s.homepage, "description": "old", "permissions": map[string]bool{"admin": true}})
		case repo + "/pages":
			if !s.exists {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"Not Found"}`)
				return
			}
			fmt.Fprint(w, `{"build_type":"workflow","html_url":"https://testowner.github.io/testrepo/","https_enforced":true}`)
		case repo + "/environments/github-pages":
			fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":null,"protection_rules":[{"type":"wait_timer","wait_timer":30}],"can_admins_bypass":false}`)
		case repo + "/actions/permissions":
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":true}`)
		case repo + "/actions/permissions/workflow":
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
		case repo + "/private-vulnerability-reporting", repo + "/automated-security-fixes":
			fmt.Fprint(w, `{"enabled":false}`)
		case repo + "/vulnerability-alerts":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected read: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	ghfake.FakeRepo(t, "testowner", "testrepo")
	return s, testutil.NewTestClient(t, server)
}

func pagesAcceptanceSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := make(map[string]string)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := root.Readlink(path)
			files[path] = "symlink:" + target
			return err
		}
		if entry.IsDir() {
			files[path] = "directory"
			return nil
		}
		data, err := root.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPagesAcceptanceDisabledLeavesPagesUnmanaged(t *testing.T) {
	for _, section := range []string{"", "pages:\n  enabled: false\n"} {
		for _, mode := range []alter.ApplyMode{alter.Apply, alter.Recut, alter.DryRun} {
			t.Run(fmt.Sprintf("%q/%d", section, mode), func(t *testing.T) {
				s, client := newPagesAcceptanceAPI(t)
				dir := t.TempDir()
				writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\n"+section+"swatches:\n  - path: .github/workflows/tailor-pages.yml\n    alteration: always\n"))
				writeOnDisk(t, dir, swatch.PagesDestination, []byte("name: user-owned\n"))
				writeOnDisk(t, dir, ".gitignore", []byte("keep/\n"))
				before := pagesAcceptanceSnapshot(t, dir)
				output := captureAlterRun(t, loadTestConfig(t, dir), dir, mode, client)
				if len(s.writes) != 0 || !reflect.DeepEqual(s.reads, []string{"/user"}) {
					t.Fatalf("disabled Pages made API calls: reads=%v writes=%v", s.reads, s.writes)
				}
				if !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) || strings.Contains(output, "pages.") {
					t.Fatalf("disabled Pages changed files or reported Pages results: %s", output)
				}
			})
		}
	}
}

func TestPagesAcceptanceConflictsBlockAllWrites(t *testing.T) {
	for _, tc := range []struct {
		name, alteration, workflow, want string
		mode                             alter.ApplyMode
		missingSource                    bool
	}{
		{"missing source", "always", "", "source", alter.Apply, true},
		{"unowned recut", "always", "name: custom\n", "ownership conflict", alter.Recut, false},
		{"missing never", "never", "", "missing", alter.Apply, false},
		{"first-fit mismatch", "first-fit", "old", "incompatible", alter.Apply, false},
		{"never recut mismatch", "never", "old", "incompatible", alter.Recut, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, client := newPagesAcceptanceAPI(t)
			dir := t.TempDir()
			writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  description: changed\npages:\n  enabled: true\nswatches:\n  - path: .github/workflows/tailor-pages.yml\n    alteration: "+tc.alteration+"\n"))
			writeOnDisk(t, dir, ".github/workflows/tailor.yml", []byte("retired but not yet removed"))
			if !tc.missingSource {
				writeOnDisk(t, dir, "pages/index.html", []byte("site"))
			}
			workflow := []byte(tc.workflow)
			if tc.workflow == "old" {
				var err error
				workflow, err = swatch.PagesContent("static", "pages", "old")
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(workflow) != 0 {
				writeOnDisk(t, dir, swatch.PagesDestination, workflow)
			}
			before := pagesAcceptanceSnapshot(t, dir)
			err := alter.Run(loadTestConfig(t, dir), dir, tc.mode, client, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
				t.Fatalf("conflict allowed changes: writes=%v", s.writes)
			}
		})
	}
}

func TestPagesAcceptancePreviewIsReadOnly(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\npages:\n  enabled: true\n  generator: hugo\n  cname: www.example.com\n"))
	writeOnDisk(t, dir, "pages/hugo.toml", []byte("title = 'Site'\n"))
	writeOnDisk(t, dir, ".gitignore", []byte("keep/"))
	writeOnDisk(t, dir, ".github/workflows/custom.yml", []byte("name: custom\n"))
	before := pagesAcceptanceSnapshot(t, dir)
	output := captureAlterRun(t, loadTestConfig(t, dir), dir, alter.DryRun, client)
	for _, want := range []string{"pages.build_type", "homepage", swatch.PagesDestination, ".gitignore"} {
		requireContains(t, output, want)
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatalf("preview changed API or filesystem state: writes=%v", s.writes)
	}
}

func TestPagesLinksAcceptance(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\npages:\n  enabled: true\n  links:\n    website: https://example.com\n    email: user@example.com\n"))
	page := "<h1>My project</h1>\n<!-- tailor:links:start -->\n<!-- tailor:links:end -->\n<p>My footer</p>\n"
	writeOnDisk(t, dir, "pages/index.html", []byte(page))
	before := pagesAcceptanceSnapshot(t, dir)
	output := captureAlterRun(t, loadTestConfig(t, dir), dir, alter.DryRun, client)
	requireContains(t, output, "pages/index.html")
	requireContains(t, output, "would overwrite")
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("baste wrote state")
	}
	output = captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Apply, client)
	requireContains(t, output, "pages/index.html")
	requireContains(t, output, "overwritten")
	data := pagesAcceptanceSnapshot(t, dir)["pages/index.html"]
	if !strings.Contains(data, `href="https://example.com"`) || !strings.Contains(data, `href="mailto:user@example.com"`) || !strings.HasPrefix(data, "<h1>My project</h1>\n") || !strings.HasSuffix(data, "<p>My footer</p>\n") {
		t.Fatalf("incorrect page: %s", data)
	}
	before = pagesAcceptanceSnapshot(t, dir)
	writes := len(s.writes)
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Recut, client)
	if len(s.writes) != writes || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("repeated recut changed state")
	}
}

func TestPagesLinksConflictBlocksAllWrites(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  description: changed\npages:\n  enabled: true\n  links: {website: https://example.com}\n"))
	writeOnDisk(t, dir, "pages/index.html", []byte("No markers"))
	writeOnDisk(t, dir, ".github/workflows/tailor.yml", []byte("Retired workflow"))
	before := pagesAcceptanceSnapshot(t, dir)
	err := alter.Run(loadTestConfig(t, dir), dir, alter.Apply, client, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "pages.links") {
		t.Fatalf("expected marker error, got %v", err)
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("marker conflict allowed writes")
	}
}

func TestPagesAcceptanceRepeatedApplyAndDefaultBranchChange(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  homepage: https://github.com/testowner/testrepo # tailor: inferred homepage \"https://github.com/testowner/testrepo\"\npages:\n  enabled: true\n"))
	writeOnDisk(t, dir, "pages/index.html", []byte("site"))
	writeOnDisk(t, dir, ".github/workflows/custom.yml", []byte("name: custom\n"))
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Apply, client)
	if len(s.writes) != 2 || s.writes[0].Method != http.MethodPost || s.writes[0].Body != `{"build_type":"workflow"}` || s.homepage != "https://testowner.github.io/testrepo/" {
		t.Fatalf("first apply = %v, homepage = %q", s.writes, s.homepage)
	}
	before := pagesAcceptanceSnapshot(t, dir)
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Apply, client)
	if len(s.writes) != 2 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatalf("matching second apply changed state: %v", s.writes)
	}
	s.mu.Lock()
	s.branch = "release"
	s.mu.Unlock()
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Apply, client)
	after := pagesAcceptanceSnapshot(t, dir)
	if !strings.Contains(after[swatch.PagesDestination], `"release"`) || strings.Contains(after[swatch.PagesDestination], `"main"`) {
		t.Fatal("workflow did not follow the changed default branch")
	}
	delete(before, swatch.PagesDestination)
	delete(after, swatch.PagesDestination)
	if len(s.writes) != 2 || !reflect.DeepEqual(before, after) {
		t.Fatalf("branch change touched unrelated files or remote state: %v", s.writes)
	}
}

func TestPagesAcceptanceRecutKeepsGeneratorIgnore(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	s.exists = true
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\npages:\n  enabled: true\n  generator: hugo\nswatches:\n  - path: .gitignore\n    alteration: first-fit\n"))
	writeOnDisk(t, dir, "pages/hugo.toml", []byte("title = 'Site'\n"))
	writeOnDisk(t, dir, ".gitignore", []byte("old-rule/\n/pages/public/\n"))
	for range 2 {
		captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Recut, client)
		content := pagesAcceptanceSnapshot(t, dir)[".gitignore"]
		if strings.Count(content, "/pages/public/\n") != 1 || strings.Contains(content, "old-rule/") {
			t.Fatalf("recut lost or duplicated the generator rule: %q", content)
		}
	}
}
