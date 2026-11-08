package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/creack/pty"
	"github.com/wimpysworld/tailor/internal/alter"
)

func TestConfirmWikiAdoption(t *testing.T) {
	for _, tc := range []struct {
		name, input       string
		interactive, want bool
	}{
		{"yes", "yes\n", true, true}, {"y", "Y\n", true, true}, {"no", "n\n", true, false}, {"default no", "\n", true, false}, {"EOF", "", true, false}, {"unterminated EOF", "yes", true, false}, {"nonterminal", "yes\n", false, false}, {"invalid", "maybe\nyes\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr strings.Builder
			plan := alter.WikiAdoptionPlan{Head: strings.Repeat("a", 40), Additions: []string{"wiki/new.md"}, Replacements: []string{"wiki/\x1b[31mHome.md"}, Backups: []string{"wiki/Home.md"}}
			got, err := confirmWikiAdoption(strings.NewReader(tc.input), &stderr, tc.interactive, plan)
			if err != nil || got != tc.want {
				t.Fatalf("got=%t err=%v", got, err)
			}
			prompts := strings.Count(stderr.String(), "[y/N]")
			wantPrompts := 0
			if tc.interactive {
				wantPrompts = 1
			}
			if prompts != wantPrompts || strings.Contains(stderr.String(), "\x1b") {
				t.Fatalf("prompt: %q", stderr.String())
			}
		})
	}
}

type wikiPromptErrorReader struct{}

func (wikiPromptErrorReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }
func TestConfirmWikiAdoptionReadFailure(t *testing.T) {
	got, err := confirmWikiAdoption(wikiPromptErrorReader{}, io.Discard, true, alter.WikiAdoptionPlan{})
	if got || err == nil {
		t.Fatalf("got=%t err=%v", got, err)
	}
}

func TestWikiPromptChannels(t *testing.T) {
	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminal.Close()
	input, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer output.Close()
	for _, tc := range []struct {
		name   string
		stdin  *os.File
		stderr io.Writer
		ci     string
		want   bool
	}{
		{"terminal input and prompt", terminal, terminal, "", true},
		{"redirected stdin", input, terminal, "", false},
		{"redirected stderr", terminal, output, "", false},
		{"CI", terminal, terminal, "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CI", tc.ci)
			// Result stdout is redirected independently of the input and prompt channels.
			previous := os.Stdout
			os.Stdout = output
			t.Cleanup(func() { os.Stdout = previous })
			if got := wikiPromptInteractive(tc.stdin, tc.stderr); got != tc.want {
				t.Fatalf("interactive=%t want %t", got, tc.want)
			}
		})
	}
}

func TestWikiPromptBoundsFilePreview(t *testing.T) {
	plan := alter.WikiAdoptionPlan{Head: strings.Repeat("a", 40)}
	for i := range 1000 {
		plan.Additions = append(plan.Additions, fmt.Sprintf("wiki/new-%d.md", i))
		plan.Replacements = append(plan.Replacements, fmt.Sprintf("wiki/old-%d.md", i))
	}
	plan.Backups = plan.Replacements
	var stderr strings.Builder
	approved, err := confirmWikiAdoption(strings.NewReader("yes\n"), &stderr, true, plan)
	if err != nil || !approved {
		t.Fatalf("approved=%t err=%v", approved, err)
	}
	got := stderr.String()
	if strings.Count(got, "\n") > 18 || strings.Count(got, "[y/N]") != 1 {
		t.Fatalf("unbounded or repeated prompt: %s", got)
	}
	for _, want := range []string{"1000 additions", "1000 replacements", "1000 backups", "add: 995 more files", "replace and back up: 995 more files", "Approval covers all files", "tailor baste --verbose", "private Git metadata", "Approve only this wiki import?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
}

func TestAdoptWikiFlagScope(t *testing.T) {
	for _, args := range [][]string{{"alter", "--adopt-wiki"}, {"alter", "--recut", "--adopt-wiki"}} {
		var cli CLI
		parser, err := kong.New(&cli, kong.Writers(io.Discard, io.Discard))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(args); err != nil || !cli.Alter.AdoptWiki {
			t.Fatalf("parse %v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"baste", "--adopt-wiki"}, {"measure", "--adopt-wiki"}, {"fit", "project", "--adopt-wiki"}} {
		var cli CLI
		parser, err := kong.New(&cli, kong.Writers(io.Discard, io.Discard))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(args); err == nil {
			t.Fatalf("accepted unsupported flag: %v", args)
		}
	}
	command := AlterCmd{LocalOnly: true, AdoptWiki: true}
	if err := command.Run(); err == nil || !strings.Contains(err.Error(), "--local-only") {
		t.Fatalf("local flag conflict: %v", err)
	}
}
