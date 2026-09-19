<h1 align="center">
  <img src="pages/icon.svg" width="256" height="256" alt="Tailor">
  <br />
  Tailor
</h1>

<p align="center"><b>Ready-to-wear project templates for GitHub repositories 👔</b></p>

<p align="center">Made with 💝 for 🐧🍏</p>

Tailor is a local terminal CLI that keeps GitHub repositories consistent with community health files, security policies, dev tooling, and repository settings.

Use it for solo projects or small teams that manage several repositories. Its embedded template files are called *swatches*.

## Install

### bin

```bash
bin install github.com/wimpysworld/tailor
bin update tailor
```

Requires [`bin`](https://github.com/marcosnils/bin). Tailor releases publish bare executables, no archive extraction needed.

### Homebrew

```bash
brew install wimpysworld/tap/tailor
```

### Nix

```bash
nix run github:wimpysworld/nix-packages#tailor -- --version
nix profile install github:wimpysworld/nix-packages#tailor
```

To use tailor in a flake configuration, add `nix-packages` as an input:

```nix
{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    wimpysworld-nix-packages = {
      url = "github:wimpysworld/nix-packages";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
}
```

Then reference tailor in your packages:

```nix
environment.systemPackages = [
  inputs.wimpysworld-nix-packages.packages.${system}.tailor
];
```

Available for `x86_64-linux`, `aarch64-linux`, and `aarch64-darwin`.

### Docker

```bash
docker run --rm ghcr.io/wimpysworld/tailor --version
```

Images are published to GHCR for `linux/amd64` and `linux/arm64`. Mount your project directory and pass a GitHub token:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -v "$PWD":/work -w /work \
  -e GH_TOKEN \
  ghcr.io/wimpysworld/tailor alter
```

The `--user` option uses your host identity for Git ownership checks and writes to the mounted checkout.

### Native packages

Releases include `.deb`, `.rpm`, `.apk`, and Arch Linux packages. Download the appropriate file from the [latest release](https://github.com/wimpysworld/tailor/releases/latest). Install it with your system package manager. The AUR package is [`tailor-bin`](https://aur.archlinux.org/packages/tailor-bin).

### Authentication

Tailor needs a valid GitHub authentication token for `fit`, `alter`, and `baste`. Set `GH_TOKEN` or `GITHUB_TOKEN` to use Tailor without the `gh` binary.

Alternatively, install the [GitHub CLI](https://cli.github.com/) and run `gh auth login`. Tailor can then read the token from the `gh` config file or keyring. The `measure` and `docket` commands do not require authentication.

## Quick start

`fit` writes `.tailor.yml` without copying swatches or changing repository settings. The default licence is BlueOak-1.0.0.

### New project

Create the project and enter its directory:

```bash
tailor fit ./my-project
cd my-project
```

### Existing project

Check local health files, then create the configuration:

```bash
cd existing-project
tailor measure
tailor fit .
```

If `.tailor.yml` already exists, skip `fit`. With a GitHub remote, `fit` imports only the repository's `description` and `homepage`. All other managed settings come from Tailor's embedded defaults.

### Review and apply

For either project, edit `.tailor.yml` to choose the licence, repository settings and files that Tailor manages. Set a swatch to `alteration: never` to leave its file untouched.

Repository settings require a GitHub remote. For a new local project, add its GitHub remote before the preview.

Preview the changes without writes:

```bash
tailor baste
```

After you review the preview, apply the changes:

```bash
tailor alter
```

> [!IMPORTANT]
> Swatches with `alteration: always` replace local file edits. Review the preview before each `alter` run.

## Development

Enter the pinned shell with `nix develop`. Run `just build-tailor` to build the stripped CLI. Run `just lint` for all enabled linters. `just lint-all` is a repository compatibility alias for `just lint`.

## Documentation

Read the [wiki](https://github.com/wimpysworld/tailor/wiki) for detailed guides and reference material.

## Support and contributing

Read [Support](SUPPORT.md) for help and [Contributing](CONTRIBUTING.md) before submitting a change. The [specification](docs/SPECIFICATION.md) defines Tailor's behaviour.
