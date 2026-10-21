package swatch_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvrcWatches(t *testing.T) {
	t.Setenv("BASH_ENV", os.DevNull)
	content := mustSwatchContent(t, ".envrc")
	own, err := os.ReadFile("../../.envrc")
	if err != nil {
		t.Fatal(err)
	}
	if string(own) != string(content) {
		t.Fatal("repository .envrc differs from its swatch")
	}
	for _, program := range []string{"bash", "direnv"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skip(program + " is required for native watch tests")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".envrc"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "bash", "-euc", `
	unset DIRENV_WATCHES
	nix_direnv_manual_reload() { manual=1; }
	watch_file() { eval "$(direnv watch bash "$@")"; }
	use() {
	  test "$manual" = 1
	  test "$*" = flake
	  direnv current nix
	  direnv current nix/hooks
	}
	source .envrc
	direnv show_dump "$DIRENV_WATCHES" > watches.json
	! grep -F '*.nix' watches.json
	mkdir -p nix/hooks
	! direnv current nix
	! direnv current nix/hooks
	for file in nix/loader.nix nix/hooks.nix nix/go.nix 'nix/with space.nix' nix/hooks/core.nix; do
	  touch "$file"
	done
	source .envrc
	for file in nix/loader.nix nix/hooks.nix nix/go.nix 'nix/with space.nix' nix/hooks/core.nix; do
	  direnv current "$file"
	done
	touch cache
	for file in nix/go.nix nix/hooks/core.nix; do
	  touch -t 200001010000 "$file"
	  source .envrc
	  printf 'changed\n' > "$file"
	  ! direnv current "$file"
	  test "$file" -nt cache
	  source .envrc
	done
	for directory in nix nix/hooks; do
	  touch -t 200001010000 "$directory"
	  source .envrc
	  touch "$directory/new.nix"
	  ! direnv current "$directory"
	  test "$directory" -nt cache
	  source .envrc
	  direnv current "$directory/new.nix"
	  touch -t 200001010000 "$directory"
	  source .envrc
	  rm "$directory/new.nix"
	  ! direnv current "$directory"
	  ! direnv current "$directory/new.nix"
	  test "$directory" -nt cache
	  source .envrc
	done
	mkdir unrelated
	touch unrelated/ignored.nix
	direnv current nix
	direnv current nix/hooks
	! direnv current unrelated/ignored.nix
	`)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("native direnv watches: %v\n%s", err, output)
	}
}

func TestEnvrcWatchOrder(t *testing.T) {
	t.Setenv("BASH_ENV", os.DevNull)
	dir := t.TempDir()
	content := mustSwatchContent(t, ".envrc")
	if err := os.WriteFile(filepath.Join(dir, ".envrc"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "bash", "-euc", `
	mkdir -p nix/hooks
	touch nix/loader.nix nix/hooks.nix 'nix/package space.nix' nix/hooks/core.nix
	nix_direnv_manual_reload() { echo manual; }
	watch_file() { printf 'watch %s\n' "$@"; }
	use() { echo "use $*"; }
	source .envrc
	`)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("watch order: %v\n%s", err, output)
	}
	want := "manual\nwatch nix\nwatch nix/hooks\nwatch nix/lib\nwatch nix/shells\nwatch nix/hooks.nix\nwatch nix/loader.nix\nwatch nix/package space.nix\nwatch nix/hooks/core.nix\nuse flake\n"
	if strings.TrimSpace(string(output)) != strings.TrimSpace(want) {
		t.Fatalf("watch order = %q, want %q", output, want)
	}
}

func TestEnvrcNixDirenvCache(t *testing.T) {
	source := os.Getenv("TAILOR_TEST_NIX_DIRENV")
	if source == "" {
		t.Skip("TAILOR_TEST_NIX_DIRENV must name an installed nix-direnv direnvrc")
	}
	t.Setenv("BASH_ENV", os.DevNull)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".envrc"), mustSwatchContent(t, ".envrc"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "bash", "-ec", `
	unset DIRENV_WATCHES
	direnv=$(command -v direnv)
	source "$TAILOR_TEST_NIX_DIRENV"
	watch_file() { eval "$("$direnv" watch bash "$@")"; }
	use() { use_flake; }
	log_status() { echo "$*" >&2; }
	log_error() { echo "$*" >&2; }
	direnv_layout_dir() { echo "$PWD/cache"; }
	_nix_direnv_preflight() { :; }
	_nix_argsum_suffix() { :; }
	_nix_refresh_gcroots() { :; }
	_nix_clean_old_gcroots() { :; }
	_nix_add_gcroot() { touch "$2"; }
	_nix() {
	  case "$1" in
	    print-dev-env) echo build >> builds; echo 'export FIXTURE_CACHE=renewed' ;;
	    flake) echo '{}' ;;
	    *) return 1 ;;
	  esac
	}
	mkdir -p cache nix/hooks
	touch cache/flake-profile builds
	echo 'export FIXTURE_CACHE=old' > cache/flake-profile.rc
	for operation in modify create remove hook-create hook-remove; do
	  for file in nix/*.nix nix/hooks/*.nix; do
	    if [ -e "$file" ]; then touch -t 200001010000 "$file"; fi
	  done
	  touch -t 200001010000 cache/flake-profile.rc nix nix/hooks
	  case "$operation" in
	    modify) echo changed > nix/go.nix ;;
	    create) touch nix/new.nix ;;
	    remove) rm nix/new.nix ;;
	    hook-create) touch nix/hooks/new.nix ;;
	    hook-remove) rm nix/hooks/new.nix ;;
	  esac
	  unset _nix_direnv_force_reload
	  before=$(wc -l < builds)
	  source .envrc
	  after=$(wc -l < builds)
	  test "$before" = "$after"
	  _nix_direnv_force_reload=1
	  source .envrc
	  test "$FIXTURE_CACHE" = renewed
	  test "$(wc -l < builds)" -eq "$((before + 1))"
	done
	`)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("nix-direnv cache invalidation: %v\n%s", err, output)
	}
}
