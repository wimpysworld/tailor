package alter

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
)

func TestPagesLinksReplace(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		prefix := "<footer>Authored content" + newline + "  " + pagesLinksStart
		suffix := pagesLinksEnd + newline + "<p>Copyright</p></footer>"
		data := []byte(prefix + newline + "  old links" + newline + "  " + suffix)
		links := map[string]string{"website": `https://example.com/?a=1&b="quoted"`, "email": "user@example.com", "x": ""}
		got, err := replacePagesLinks(data, &model.PagesSettings{Links: &links})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(got, []byte(prefix)) || !bytes.HasSuffix(got, []byte(suffix)) || bytes.Contains(got, []byte("old links")) {
			t.Fatal("changed authored content or retained old links")
		}
		for _, want := range []string{`href="https://example.com/?a=1&amp;b=&#34;quoted&#34;"`, `href="mailto:user@example.com"`, `aria-label="Website"`, `aria-label="Email"`} {
			if !strings.Contains(string(got), want) {
				t.Fatalf("missing %s", want)
			}
		}
		if strings.Contains(string(got), `aria-label="X"`) || strings.Index(string(got), `aria-label="Website"`) > strings.Index(string(got), `aria-label="Email"`) {
			t.Fatal("empty value rendered or order changed")
		}
		repeated, err := replacePagesLinks(got, &model.PagesSettings{Links: &links})
		if err != nil || !bytes.Equal(got, repeated) {
			t.Fatalf("not idempotent: %v", err)
		}
		cleared, err := replacePagesLinks(got, nil)
		if err != nil || string(cleared) != prefix+newline+"  "+suffix {
			t.Fatalf("clear failed: %q, %v", cleared, err)
		}
	}
}

func TestPagesLinksBadMarkers(t *testing.T) {
	for _, source := range []string{
		"", pagesLinksStart, pagesLinksEnd + "\n" + pagesLinksStart,
		pagesLinksStart + "\n" + pagesLinksStart + "\n" + pagesLinksEnd,
		"<p>" + pagesLinksStart + "\n" + pagesLinksEnd,
		pagesLinksStart + "\n" + pagesLinksEnd + "</p>",
	} {
		if _, err := replacePagesLinks([]byte(source), nil); err == nil {
			t.Fatalf("accepted malformed markers: %q", source)
		}
	}
}

func TestPagesLinksLifecycle(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		dir := t.TempDir()
		cfg := pagesTestConfig("static")
		cfg.Pages.Path = new("web/site")
		cfg.Pages.Links = &map[string]string{"website": "https://example.com"}
		original := "<h1>Keep me</h1>\n" + pagesLinksStart + "\n" + pagesLinksEnd + "\n"
		pagesTestFile(t, dir, "web/site/index.html", original)
		prepared, err := preparePagesSource(cfg, dir, mode)
		if err != nil {
			t.Fatal(err)
		}
		// Preserve an edit made after preflight, not its stale snapshot.
		original = "<!-- later edit -->\n" + original
		pagesTestFile(t, dir, "web/site/index.html", original)
		result, err := processPagesLinks(cfg, dir, mode, prepared)
		if err != nil || result == nil || result.Category != WouldOverwrite {
			t.Fatalf("result = %v, error = %v", result, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "web/site/index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !mode.ShouldWrite() {
			if string(data) != original {
				t.Fatal("dry run wrote content")
			}
		} else {
			if !bytes.HasPrefix(data, []byte("<!-- later edit -->\n<h1>Keep me</h1>")) || !bytes.Contains(data, []byte(`href="https://example.com"`)) {
				t.Fatal("content was not preserved or link was not written")
			}
			result, err = processPagesLinks(cfg, dir, mode, prepared)
			if err != nil || result.Category != NoChange {
				t.Fatalf("repeated result = %v, error = %v", result, err)
			}
		}
	}
}

func TestPagesLinksPreflight(t *testing.T) {
	dir := t.TempDir()
	cfg := pagesTestConfig("static")
	pagesTestFile(t, dir, "pages/index.html", "Authored site without markers")
	if _, err := preparePagesSource(cfg, dir, Apply); err != nil {
		t.Fatal("omitted links must leave site unmanaged:", err)
	}
	cfg.Pages.Links = &map[string]string{}
	if _, err := preparePagesSource(cfg, dir, Apply); err == nil {
		t.Fatal("missing markers accepted")
	}
	cfg.Pages.Enabled = new(false)
	if p, err := preparePagesSource(cfg, dir, Apply); err != nil || p != nil {
		t.Fatalf("disabled Pages inspected source: %v", err)
	}
	cfg.Pages.Enabled = new(true)
	outside := t.TempDir()
	pagesTestFile(t, outside, "target.html", pagesLinksStart+"\n"+pagesLinksEnd)
	if err := os.Remove(filepath.Join(dir, "pages/index.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "target.html"), filepath.Join(dir, "pages/index.html")); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePagesSource(cfg, dir, Apply); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestPagesLinksAllIcons(t *testing.T) {
	links := map[string]string{}
	for _, link := range model.PagesLinkTypes {
		links[link.Key] = "https://example.com"
	}
	links["email"] = "user@example.com"
	data, err := replacePagesLinks([]byte(pagesLinksStart+"\n"+pagesLinksEnd), &model.PagesSettings{Links: &links})
	if err != nil || bytes.Count(data, []byte("<a ")) != len(model.PagesLinkTypes) {
		t.Fatalf("rendered icons = %s, error = %v", data, err)
	}
}

func TestPagesLinksYAMLOrder(t *testing.T) {
	dir := t.TempDir()
	for _, links := range []string{
		"{email: user@example.com, x: '', website: https://example.com}",
		"{website: https://example.com, x: '', email: user@example.com}",
	} {
		pagesTestFile(t, dir, ".tailor.yml", "pages:\n  links: "+links+"\n")
		cfg, err := config.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, err := replacePagesLinks([]byte(pagesLinksStart+"\n"+pagesLinksEnd), cfg.Pages)
		if err != nil {
			t.Fatal(err)
		}
		emailFirst := bytes.Index(got, []byte(`aria-label="Email"`)) < bytes.Index(got, []byte(`aria-label="Website"`))
		if emailFirst != strings.HasPrefix(links, "{email:") || bytes.Count(got, []byte("<a ")) != 2 {
			t.Fatalf("rendered links do not follow YAML order: %s", got)
		}
	}
}
