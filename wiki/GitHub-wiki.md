# GitHub wiki

Tailor publishes documentation from `wiki/` to the GitHub wiki of a public repository.

[Setup](#setup) · [Workflow](#workflow) · [Import](#import-the-existing-wiki) · [Recovery](#readiness-and-recovery) · [Publication](#publication) · [Disable](#disable-publishing)

## Setup

Set `repository.has_wiki: true`, then run `tailor alter`. Tailor checks local safety, enables the wiki through GitHub's API if needed, and checks readiness before other changes. Wiki publishing supports public repositories only. The source directory is `wiki/`, independent of Pages and `pages/`.

Existing-project `fit` preserves the live `has_wiki` setting. New configs default to `false`.

The source must contain only directories and regular files, with no symlinks or `.git` metadata. Copy shared documentation into `wiki/` instead of linking to it.

Tailor preserves existing wiki starter pages, including under `alter --recut` or `always`. Their `never` mode skips creation.

## Workflow

The workflow path is `.github/workflows/tailor-wiki.yml`, with default mode `always`. Its first line must be `# Managed by Tailor: wiki`. An existing unmarked workflow blocks setup before writes, including with `--recut`.

| Workflow mode | Behaviour when the wiki is enabled |
|---|---|
| `always` | Creates or updates the marked workflow. |
| `first-fit` | Creates a missing workflow. Preserves an existing compatible workflow, unless `--recut` applies. |
| `never` | Requires an existing compatible workflow and never writes it, including with `--recut`. |

A protected workflow must match the generated YAML semantics, including execution and permissions. Comments and formatting can differ. Custom steps or changed settings can block setup under `first-fit` or `never`.

The workflow publishes when `wiki/` files or the workflow change on the default branch. For manual publication, select the current default branch. Publication requires its latest commit. Older runs stop if the branch advances before the publisher checks it.

## Import the existing wiki

If setup is incomplete, `alter` exits with an error and the next steps. The wiki can remain enabled, but Tailor writes no config, swatches or other repository settings. The local CLI does not import files, push Git commits or change wiki history. Complete setup as follows:

1. If Tailor reports a missing wiki, open `https://github.com/OWNER/REPO/wiki` and create and save the first page.
2. Rerun `tailor alter` to check the wiki and receive import instructions.
3. Clone `https://github.com/OWNER/REPO.wiki.git` into a separate directory.
4. Review conflicts with local files. Manually import all wiki files into `wiki/`, except `.git`.
5. After the import, record the clone's `git rev-parse HEAD` output in `wiki/.tailor-wiki-base`.
6. Run `tailor baste` to check adoption and preview the remaining changes.
7. After readiness passes, run `tailor alter` to apply the remaining changes, including the workflow and any missing starter pages.
8. Review and commit the imported files, baseline and workflow, then push to the default branch.

## Readiness and recovery

The baseline is the imported commit's full 40-character lowercase hexadecimal ID. It records explicit adoption of the imported wiki. A missing or stale baseline blocks initial setup. Every remote file must exist locally, so a baseline alone does not complete adoption.

Independent edits through the Wiki tab also block readiness. Import those edits and update the baseline before another `tailor alter` run.

After publication, Tailor checks the source commit recorded in the wiki history. That commit must be an ancestor of local `HEAD`, with its wiki tree available locally. Ordinary publication does not require a baseline update after each run.

If a shallow, outdated or rewritten checkout lacks that history, fetch the project history and use the current project branch. For a shallow clone, fetch the complete history with:

```bash
git fetch --unshallow
```

Run `tailor baste` to check again. If the history remains unavailable, [review and import the wiki again](#import-the-existing-wiki), then update the baseline.

If Tailor cannot read the remote wiki, it reports an access or network error without assuming that the wiki is missing. Follow first-page guidance only if the wiki has no saved page. Otherwise, fix access and rerun `tailor alter`.

Run `tailor baste` for a full preview without writes. If wiki readiness is blocked, a `Next steps:` section follows the complete preview. Changes labelled `would copy` and other pending changes wait until readiness passes.

For a disabled wiki, the steps start with `tailor alter` to enable it through GitHub's API. The steps also give the repository's exact `/wiki` URL and conditional first-page and manual import instructions. A disabled wiki does not prove that its pages are missing. Confirmed access or network failures instead give check-and-retry steps, without page creation or import instructions.

## Publication

A successful readiness check does not prove that publication succeeded. Check the generated workflow run after the push.

After adoption, the source directory controls the published tree, including deletions. The publisher preserves wiki history and never force-pushes. Missing wikis, unknown access and concurrent changes stop publication without replacing content.

The workflow uses GitHub's built-in `GITHUB_TOKEN` with `contents: write`. Token support is verified against upstream implementation evidence, not a live Tailor publication. Tailor does not create a personal access token or configure a secret.

## Disable publishing

Set `repository.has_wiki: false` and run `tailor alter` to disable the wiki and remove only Tailor's marked workflow. Commit and push the removal to stop future workflow runs. Local source files and remote wiki history remain intact. An unmarked workflow remains untouched and needs manual removal.

An omitted setting leaves wiki files unmanaged for that run. With [default merging](Configuration#default-merging) enabled, Tailor saves `has_wiki: false`, so the next run removes the marked workflow. To stop wiki management across later runs, disable default merging and omit `repository.has_wiki`.
