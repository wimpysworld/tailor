package alter_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestWikiConflictBlocksAllWrites(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: .tailor.yml\n    alteration: always\n"))
	writeOnDisk(t, dir, swatch.WikiDestination, []byte("name: user-owned\n"))
	before := pagesAcceptanceSnapshot(t, dir)
	var stdout, stderr strings.Builder
	err := alter.Run(loadTestConfig(t, dir), dir, alter.Apply, client, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "ownership conflict") {
		t.Fatalf("error=%v", err)
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("conflict caused writes")
	}
}

func TestWikiAndPagesPreviewTogether(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\npages:\n  enabled: true\nswatches:\n  - path: wiki/Home.md\n    alteration: first-fit\n"))
	writeOnDisk(t, dir, "pages/index.html", []byte("site"))
	before := pagesAcceptanceSnapshot(t, dir)
	output := captureAlterRun(t, loadTestConfig(t, dir), dir, alter.DryRun, client)
	for _, name := range []string{swatch.WikiDestination, swatch.PagesDestination, "wiki/Home.md"} {
		if !strings.Contains(output, name) {
			t.Fatalf("preview missing %s: %s", name, output)
		}
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("preview wrote files or remote state")
	}
}
