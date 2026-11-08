package main

import (
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

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
	var help strings.Builder
	parser, err := kong.New(&CLI{}, kong.Writers(&help, io.Discard), kong.Exit(func(int) {}))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = parser.Parse([]string{"alter", "--help"})
	if !strings.Contains(help.String(), "No longer required") {
		t.Fatalf("--adopt-wiki help does not mark the flag as no longer required:\n%s", help.String())
	}
	command := AlterCmd{LocalOnly: true, AdoptWiki: true}
	if err := command.Run(); err == nil || !strings.Contains(err.Error(), "--local-only") {
		t.Fatalf("local flag conflict: %v", err)
	}
}
