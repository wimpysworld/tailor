package alter_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
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
	mu                 sync.Mutex
	branch             string
	homepage           string
	exists             bool
	writes             []apiCall
	reads              []string
	actionsReads       int
	actionsForbiddenAt int
	actionsDisabledAt  int
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
			_ = json.NewEncoder(w).Encode(map[string]any{"private": false, "has_wiki": false, "default_branch": s.branch, "homepage": s.homepage, "description": "old", "permissions": map[string]bool{"admin": true}})
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
			s.actionsReads++
			switch s.actionsReads {
			case s.actionsForbiddenAt:
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
			case s.actionsDisabledAt:
				fmt.Fprint(w, `{"enabled":false,"allowed_actions":"all"}`)
			default:
				fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":true}`)
			}
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

var managedCoreAcceptancePaths = []string{
	"just/loader.just",
	"just/tailor.just",
	"nix/loader.nix",
}

var managedCoreAndPagesAcceptancePaths = []string{
	"just/loader.just",
	"just/tailor.just",
	"nix/loader.nix",
	"just/pages.just",
	"nix/pages.nix",
}

func pagesAcceptanceWithoutManagedPaths(snapshot map[string]string, managedPaths ...string) map[string]string {
	managed := make(map[string]struct{}, len(managedPaths))
	parents := make(map[string]struct{}, len(managedPaths))
	for _, name := range managedPaths {
		managed[name] = struct{}{}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			parents[parent] = struct{}{}
		}
	}

	filtered := make(map[string]string, len(snapshot))
	for name, content := range snapshot {
		if _, ok := managed[name]; ok {
			continue
		}
		if _, ok := parents[name]; ok && content == "directory" {
			continue
		}
		filtered[name] = content
	}
	return filtered
}

func TestPagesAcceptanceWithoutManagedPathsPreservesUserFiles(t *testing.T) {
	before := map[string]string{
		".":                  "directory",
		"just":               "directory",
		"just/loader.just":   "managed",
		"just/tailor.just":   "managed",
		"just/user.just":     "before",
		"just/custom/x.just": "before",
		"nix":                "directory",
		"nix/loader.nix":     "managed",
		"nix/user.nix":       "before",
		"nix/custom/x.nix":   "before",
	}
	after := map[string]string{
		".":                  "directory",
		"just":               "directory",
		"just/loader.just":   "updated managed",
		"just/tailor.just":   "updated managed",
		"just/user.just":     "after",
		"just/custom/x.just": "after",
		"nix":                "directory",
		"nix/loader.nix":     "updated managed",
		"nix/user.nix":       "after",
		"nix/custom/x.nix":   "after",
	}

	filteredBefore := pagesAcceptanceWithoutManagedPaths(before, managedCoreAcceptancePaths...)
	filteredAfter := pagesAcceptanceWithoutManagedPaths(after, managedCoreAcceptancePaths...)
	if reflect.DeepEqual(filteredBefore, filteredAfter) {
		t.Fatal("managed-path filter hid changes to user files")
	}
	for _, name := range []string{"just/user.just", "just/custom/x.just", "nix/user.nix", "nix/custom/x.nix"} {
		if _, ok := filteredAfter[name]; !ok {
			t.Errorf("managed-path filter removed user file %q", name)
		}
	}
}

func TestPagesAcceptanceActionsRecheckPreservesSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		missing     bool
		forbiddenAt int
		disabledAt  int
		wantReads   int
		wantError   string
	}{
		{name: "live forbidden missing source", missing: true, forbiddenAt: 2, wantReads: 2},
		{name: "live forbidden managed links", forbiddenAt: 2, wantReads: 2},
		{name: "preflight forbidden", forbiddenAt: 1, wantReads: 1},
		{name: "live disabled", disabledAt: 2, wantReads: 2, wantError: "effective Actions policy disables it"},
	} {
		for _, mode := range []alter.ApplyMode{alter.Apply, alter.Recut} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, mode), func(t *testing.T) {
				s, client := newPagesAcceptanceAPI(t)
				s.actionsForbiddenAt = tc.forbiddenAt
				s.actionsDisabledAt = tc.disabledAt
				dir := t.TempDir()
				writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\npages:\n  enabled: true\n  links: {}\nswatches:\n  - path: .github/workflows/tailor-pages.yml\n    alteration: always\n"))
				if !tc.missing {
					writeOnDisk(t, dir, "pages/index.html", []byte("<h1>My project</h1>\n<!-- tailor:links:start -->\n<a href=\"https://example.com\">Keep this link</a>\n<!-- tailor:links:end -->\n<p>My footer</p>\n"))
				}
				before := pagesAcceptanceSnapshot(t, dir)
				var output strings.Builder
				err := alter.Run(loadTestConfig(t, dir), dir, mode, client, &output, io.Discard)
				if tc.wantError != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("error = %v, want %q", err, tc.wantError)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					requireContains(t, output.String(), "pages.enabled")
					requireContains(t, output.String(), "insufficient scope")
				}
				if s.actionsReads != tc.wantReads || len(s.writes) != 0 {
					t.Fatalf("Actions reads = %d, want %d; remote writes = %v", s.actionsReads, tc.wantReads, s.writes)
				}
				after := pagesAcceptanceSnapshot(t, dir)
				if _, exists := after[swatch.PagesDestination]; exists {
					t.Error("Pages workflow was written")
				}
				if tc.missing {
					if _, exists := after["pages"]; exists {
						t.Error("missing Pages directory was created")
					}
				}
				if !reflect.DeepEqual(pagesAcceptanceWithoutManagedPaths(before, managedCoreAndPagesAcceptancePaths...), pagesAcceptanceWithoutManagedPaths(after, managedCoreAndPagesAcceptancePaths...)) {
					t.Error("unavailable Pages changed local files outside the managed core and Pages fragments")
				}
			})
		}
	}
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
				report, err := alter.Execute(loadTestConfig(t, dir), dir, mode, client, io.Discard, alter.Options{})
				if err != nil {
					t.Fatal(err)
				}
				if len(s.writes) != 0 || !reflect.DeepEqual(s.reads, []string{"/user"}) {
					t.Fatalf("disabled Pages made API calls: reads=%v writes=%v", s.reads, s.writes)
				}
				after := pagesAcceptanceSnapshot(t, dir)
				if !reflect.DeepEqual(pagesAcceptanceWithoutManagedPaths(before, managedCoreAcceptancePaths...), pagesAcceptanceWithoutManagedPaths(after, managedCoreAcceptancePaths...)) {
					t.Fatal("disabled Pages changed non-managed files")
				}
				for _, item := range report.Document.Items {
					if item.Domain == "Pages" {
						t.Fatalf("disabled Pages reported a remote result: %+v", item)
					}
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
		{"incomplete source", "always", "", "index.html", alter.Apply, true},
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
			if tc.missingSource {
				writeOnDisk(t, dir, "pages/README.md", []byte("keep"))
			}
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

func TestPagesNavigationAcceptance(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_discussions: true\npages:\n  enabled: true\n"))
	page := "<ul>\n<!-- tailor:navigation:start -->\n<!-- tailor:navigation:end -->\n</ul>\n"
	writeOnDisk(t, dir, "pages/index.html", []byte(page))
	before := pagesAcceptanceSnapshot(t, dir)
	output := captureAlterRun(t, loadTestConfig(t, dir), dir, alter.DryRun, client)
	requireContains(t, output, "would overwrite")
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("navigation preview wrote state")
	}
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Apply, client)
	data := pagesAcceptanceSnapshot(t, dir)["pages/index.html"]
	requireContains(t, data, `href="https://github.com/testowner/testrepo?tab=readme-ov-file">Documentation`)
	requireContains(t, data, `href="https://github.com/testowner/testrepo/discussions">Discussions`)
}

func TestPagesStarterAcceptance(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\npages:\n  enabled: true\n  path: web/site\n  links: {}\n"))
	before := pagesAcceptanceSnapshot(t, dir)
	output := captureAlterRun(t, loadTestConfig(t, dir), dir, alter.DryRun, client)
	for _, name := range []string{"index.html", "style.css", "theme.js", "icon.svg"} {
		requireContains(t, output, "web/site/"+name)
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("starter preview wrote state")
	}
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Apply, client)
	after := pagesAcceptanceSnapshot(t, dir)
	requireContains(t, after["web/site/index.html"], "<title>testrepo</title>")
	requireContains(t, after["web/site/index.html"], "https://github.com/testowner/testrepo/releases")
	requireContains(t, after[swatch.PagesDestination], "web/site")
	captureAlterRun(t, loadTestConfig(t, dir), dir, alter.Recut, client)
	if !reflect.DeepEqual(after, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("repeated starter application changed files")
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
