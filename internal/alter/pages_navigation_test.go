package alter

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func TestPagesLicense(t *testing.T) {
	for _, license := range []string{"BlueOak-1.0.0", "MIT", "Apache-2.0", "", "none", "custom&name"} {
		for _, newline := range []string{"\n", "\r\n"} {
			prefix := "<ul>" + newline + "  " + pagesLicenseStart
			suffix := "  " + pagesLicenseEnd + newline + "</ul>"
			data := []byte(prefix + newline + "  old link" + newline + suffix)
			got, err := replacePagesLicense(data, license, "https://github.com/owner/project")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(got, []byte(prefix)) || !bytes.HasSuffix(got, []byte(suffix)) {
				t.Fatal("changed content outside licence markers")
			}
			if license == "" || license == "none" {
				if bytes.Contains(got, []byte("<a ")) {
					t.Fatal("rendered an unconfigured licence")
				}
			} else if !bytes.Contains(got, []byte(`href="https://github.com/owner/project?tab=`+url.QueryEscape(license+"-1-ov-file")+`">License</a>`)) {
				t.Fatalf("incorrect licence URL: %s", got)
			}
			repeated, err := replacePagesLicense(got, license, "https://github.com/owner/project")
			if err != nil || !bytes.Equal(got, repeated) {
				t.Fatal("repeated rendering changed the licence link")
			}
		}
	}
	for _, source := range []string{pagesLicenseStart, pagesLicenseEnd, pagesLicenseEnd + "\n" + pagesLicenseStart} {
		if _, err := replacePagesLicense([]byte(source), "MIT", ""); err == nil {
			t.Fatal("accepted malformed licence markers")
		}
	}
}

func TestPagesLicenseLifecycle(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		dir := t.TempDir()
		cfg := pagesTestConfig("static")
		cfg.License = "MIT"
		original := pagesLicenseStart + "\n" + pagesLicenseEnd + "\n"
		pagesTestFile(t, dir, "pages/index.html", original)
		prepared, err := preparePagesSource(cfg, dir, mode)
		if err != nil {
			t.Fatal(err)
		}
		prepared.RepoURL = "https://github.com/owner/project"
		result, err := processStaticPages(cfg, dir, mode, prepared)
		if err != nil || result == nil || result.Category != WouldOverwrite {
			t.Fatalf("result %v, error %v", result, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "pages/index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if mode == DryRun {
			if string(data) != original {
				t.Fatal("preview wrote the licence link")
			}
		} else if !bytes.Contains(data, []byte("?tab=MIT-1-ov-file")) {
			t.Fatal("licence link does not use the configured identifier")
		}
	}
}

func TestPagesNavigation(t *testing.T) {
	for _, tt := range []struct {
		name              string
		wiki, discussions bool
		labels            []string
	}{
		{"readme", false, false, []string{"Documentation", "Download"}},
		{"discussions", false, true, []string{"Documentation", "Discussions", "Download"}},
		{"wiki", true, false, []string{"Overview", "Documentation", "Download"}},
		{"both", true, true, []string{"Overview", "Documentation", "Discussions", "Download"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, newline := range []string{"\n", "\r\n"} {
				prefix := "<ul>" + newline + "  <li>Authored link</li>" + newline + "  " + pagesNavigationStart
				suffix := "  " + pagesNavigationEnd + newline + "</ul>"
				data := []byte(prefix + newline + "  stale links" + newline + suffix)
				repo := &model.RepositorySettings{HasWiki: &tt.wiki, HasDiscussions: &tt.discussions}
				got, err := replacePagesNavigation(data, repo, "https://github.com/owner/project")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.HasPrefix(got, []byte(prefix)) || !bytes.HasSuffix(got, []byte(suffix)) {
					t.Fatal("changed content outside navigation markers")
				}
				if bytes.Count(got, []byte("<a ")) != len(tt.labels) {
					t.Fatalf("unexpected links: %s", got)
				}
				if bytes.Count(got, []byte(`target="_blank" rel="noopener noreferrer"`)) != len(tt.labels) {
					t.Fatal("header links must open in a new tab")
				}
				last := -1
				for _, label := range tt.labels {
					index := bytes.Index(got, []byte(">"+label+"</a>"))
					if index <= last {
						t.Fatalf("missing or misplaced %s: %s", label, got)
					}
					last = index
				}
				for suffix, want := range map[string]bool{"?tab=readme-ov-file": true, "/wiki": tt.wiki, "/discussions": tt.discussions} {
					if bytes.Contains(got, []byte(`href="https://github.com/owner/project`+suffix+`"`)) != want {
						t.Fatalf("incorrect link %s: %s", suffix, got)
					}
				}
				repeated, err := replacePagesNavigation(got, repo, "https://github.com/owner/project")
				if err != nil || !bytes.Equal(got, repeated) {
					t.Fatalf("repeated render changed navigation: %v", err)
				}
				cleared, err := replacePagesNavigation(got, nil, "https://github.com/owner/project")
				if err != nil || bytes.Count(cleared, []byte("<a ")) != 2 || !bytes.Contains(cleared, []byte(">Documentation</a>")) {
					t.Fatalf("disabled features remain: %s, %v", cleared, err)
				}
			}
		})
	}
}

func TestPagesFooterNavigation(t *testing.T) {
	for _, wiki := range []bool{false, true} {
		for _, discussions := range []bool{false, true} {
			cfg := pagesTestConfig("static")
			cfg.Repository = &model.RepositorySettings{HasWiki: &wiki, HasDiscussions: &discussions}
			dir := t.TempDir()
			pagesTestFile(t, dir, "pages/index.html", "<ul>\n"+pagesFooterNavigationStart+"\n"+pagesFooterNavigationEnd+"\n</ul>\n")
			prepared, err := preparePagesSource(cfg, dir, Apply)
			if err != nil {
				t.Fatal(err)
			}
			prepared.RepoURL = "https://github.com/owner/project"
			if _, err := processStaticPages(cfg, dir, Apply, prepared); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "pages/index.html"))
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"Documentation"}
			if wiki {
				want = []string{"Overview", "Documentation"}
			}
			if discussions {
				want = append(want, "Discussions")
			}
			if !wiki || !discussions {
				want = append(want, "Support")
			}
			if bytes.Count(data, []byte("<a ")) != len(want) {
				t.Fatalf("unexpected footer links: %s", data)
			}
			last := -1
			for _, label := range want {
				i := bytes.Index(data, []byte(">"+label+"</a>"))
				if i <= last {
					t.Fatalf("incorrect footer order: %s", data)
				}
				last = i
			}
			if (!wiki || !discussions) && !bytes.Contains(data, []byte(`href="https://github.com/owner/project/blob/HEAD/SUPPORT.md"`)) {
				t.Fatal("incorrect support URL")
			}
			result, err := processStaticPages(cfg, dir, Apply, prepared)
			if err != nil || result.Category != NoChange {
				t.Fatal("repeated footer rendering changed the page")
			}
		}
	}
}

func TestPagesNavigationPreflight(t *testing.T) {
	for _, source := range []string{
		pagesNavigationStart,
		pagesNavigationEnd,
		pagesFooterNavigationStart,
		pagesFooterNavigationEnd,
		pagesNavigationEnd + "\n" + pagesNavigationStart,
		pagesNavigationStart + "\n" + pagesNavigationStart + "\n" + pagesNavigationEnd,
		"<ul>" + pagesNavigationStart + "\n" + pagesNavigationEnd,
	} {
		dir := t.TempDir()
		pagesTestFile(t, dir, "pages/index.html", source)
		if _, err := preparePagesSource(pagesTestConfig("static"), dir, DryRun); err == nil {
			t.Fatalf("accepted malformed navigation: %s", source)
		}
	}
	source := []byte("<nav>My own navigation</nav>")
	got, err := replacePagesNavigation(source, &model.RepositorySettings{HasDiscussions: new(true)}, "https://github.com/owner/project")
	if err != nil || !bytes.Equal(source, got) {
		t.Fatal("changed an unmarked site")
	}
}

func TestPagesNavigationLifecycle(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		dir := t.TempDir()
		cfg := pagesTestConfig("static")
		cfg.Repository = &model.RepositorySettings{HasDiscussions: new(true)}
		cfg.Pages.Path = new("web/site")
		cfg.Pages.Links = &map[string]string{"website": "https://example.com"}
		original := pagesNavigationStart + "\n" + pagesNavigationEnd + "\n" + pagesLinksStart + "\n" + pagesLinksEnd + "\n"
		pagesTestFile(t, dir, "web/site/index.html", original)
		prepared, err := preparePagesSource(cfg, dir, mode)
		if err != nil {
			t.Fatal(err)
		}
		prepared.RepoURL = "https://github.com/owner/project"
		result, err := processStaticPages(cfg, dir, mode, prepared)
		if err != nil || result == nil || result.Category != WouldOverwrite {
			t.Fatalf("result %v, error %v", result, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "web/site/index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if mode == DryRun {
			if string(data) != original {
				t.Fatal("preview wrote the page")
			}
			continue
		}
		if !strings.Contains(string(data), ">Discussions</a>") || !strings.Contains(string(data), `href="https://example.com"`) {
			t.Fatal("navigation or social links are missing")
		}
		result, err = processStaticPages(cfg, dir, mode, prepared)
		if err != nil || result.Category != NoChange {
			t.Fatalf("repeated apply changed the page: %v, %v", result, err)
		}
		cfg.Pages.Links = nil
		cfg.Repository.HasDiscussions = new(false)
		result, err = processStaticPages(cfg, dir, mode, prepared)
		if err != nil || result.Category != WouldOverwrite {
			t.Fatalf("navigation without social configuration: %v, %v", result, err)
		}
		data, err = os.ReadFile(filepath.Join(dir, "web/site/index.html"))
		if err != nil || bytes.Contains(data, []byte(">Discussions</a>")) || !bytes.Contains(data, []byte(`href="https://example.com"`)) {
			t.Fatalf("failed to remove discussions or changed unmanaged social links: %v", err)
		}
	}
}
