# GitHub wiki

Tailor imports an existing public GitHub wiki into `wiki/` automatically, then prepares a workflow to publish source changes.

[Setup](#setup) · [Workflow](#workflow) · [Import](#import-the-existing-wiki) · [No pages](#wiki-without-pages) · [Recovery](#readiness-and-recovery) · [Publication](#publication) · [Disable](#disable-publishing)

## Setup

Set `repository.has_wiki: true`, then run `tailor baste` to preview setup. Apply with `tailor alter` after review. Wiki publishing supports public repositories only. The source directory is `wiki/`, independent of Pages and `pages/`.

`fit` writes `has_wiki: false`, including for repositories with an enabled wiki. Set it to `true` before `alter` to keep the wiki enabled.

After local safety checks, `alter` enables a disabled wiki through a GitHub API PATCH containing only `has_wiki: true`, then continues in the same run. Enablement remains in effect if a later check fails. A wiki without pages does not stop other alterations. Other wiki blockers stop them until wiki readiness passes.

The source must contain only directories and regular files, with no symlinks or `.git` metadata. Copy shared documentation into `wiki/` instead of linking to it. Reserved `.tailor-wiki-*` names block adoption and publication, except the root `wiki/.tailor-wiki-base`. Retain any such files, including abandoned temporary files, for manual review before retrying.

Tailor preserves existing wiki starter pages, including under `alter --recut` or `always`. Their `never` mode skips creation. Imports can replace same-path files, including starter pages, with backups.

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

If the remote wiki is readable but not adopted, `alter` imports it automatically, pinned to its current commit. It does not ask for approval or need a flag, including in noninteractive runs. Ready, adopted sources remain untouched, including uncommitted edits. A pending import completes before normal alterations.

1. Optional: run `tailor baste --verbose` to review additions, replacements and the baseline change.
2. Run `tailor alter`. It imports the wiki, then applies the other declared changes.
3. Review the imported files, `wiki/.tailor-wiki-base`, workflow and other changes from `alter`.
4. Commit and push through your normal project workflow to publish from the default branch.

`--adopt-wiki` is no longer required. `alter` accepts it for compatibility, and it has no effect. `--local-only` rejects `--adopt-wiki`.

The import preserves binary bytes and executable modes. Local-only files and identical files remain unchanged. Before replacement, Tailor saves the original bytes and modes in private Git metadata. It writes the full remote commit ID to `wiki/.tailor-wiki-base` last. Tailor never stages, commits or pushes wiki files from the CLI.

## Wiki without pages

GitHub creates the wiki repository only when you save the first page. Until then, Tailor reports that the wiki has no pages. This does not stop the run. `alter` creates missing starter pages in `wiki/` and the workflow when their alteration modes permit it, then applies the other declared changes. The `never` mode skips starter pages and requires an existing compatible workflow.

1. Create and save the first page at `https://github.com/OWNER/REPO/wiki`. Use the exact repository URL that Tailor reports. You can use the content of `wiki/Home.md`.
2. Run `tailor alter` again. It imports the new page automatically.

Tailor does not create the first page through an API or an initial push. Until the first page exists, the workflow logs that it skips publication and succeeds.

## Readiness and recovery

Run `tailor baste` for a full preview without prompts or persistent writes. Wiki reads use isolated temporary Git storage. A disabled wiki produces guidance until `alter` enables it. A disabled wiki does not prove that pages are missing. Readiness blockers alone do not fail the preview.

Tailor treats a "not found" answer as a wiki without pages only when GitHub reports a public repository with the wiki enabled. If Tailor cannot read the remote wiki for another reason, fix the reported access or network error before retrying. When absence and denied access are indistinguishable, open the exact `/wiki` URL and check access first. Create a page only if none exists.

Recovery state and backups remain under `tailor/wiki-imports/<transaction>` in the actual Git directory for the worktree. Tailor reports the full path, including for linked worktrees. These files are not project files and never belong in `wiki/`. Keep them after completion.

For a valid interrupted import, run `tailor alter` again. It resumes the import automatically. Tailor checks the pinned remote commit, journal, backups, file modes, and before/after hashes before resuming. Valid incomplete staging can resume too.

Changed remote content, changed local files, corrupt state, unknown backups, or a missing valid journal require manual review. Unsafe paths, unresolved conflict markers, path collisions and detected races also stop adoption. Keep the reported recovery directory and affected files. Compare those files with the journal and backups before any manual repair. Do not delete recovery data or write a baseline to bypass a stop.

An import is not one atomic transaction. A failure can leave some imported files in place. The baseline is written last, but completed file writes are not rolled back. Avoid concurrent edits during import.

The baseline contains one full 40-character lowercase hexadecimal commit ID. Readiness accepts a baseline that matches remote HEAD when every remote file exists locally. Local edits remain permitted. Otherwise, readiness checks the publisher's `Tailor-Wiki-Source` trailer, matching remote tree, and source ancestry in local `HEAD`.

Independent edits through the Wiki tab require a reviewed reimport. If local project history is missing, fetch the history and use the current project branch before considering reimport. For a shallow clone:

```bash
git fetch --unshallow
```

Then rerun `tailor baste`. A malformed baseline requires manual review rather than automatic replacement.

Imports support the Linux and macOS releases. Safe import locking is explicitly unsupported on non-Unix platforms. Snapshots allow 10,000 files, 16 MiB per file and 64 MiB total content. These limits do not bound the temporary clone's history or disk use.

## Publication

A successful readiness check does not prove that publication succeeded. Check the generated workflow run after the push.

After adoption, the source directory controls the published tree, including deletions. The publisher excludes the baseline, preserves wiki history and never force-pushes. Missing wikis, unknown access and concurrent changes stop publication without replacing content.

The workflow uses GitHub's built-in `GITHUB_TOKEN` with `contents: write`. Token support rests on upstream implementation evidence, not a live Tailor publication. Tests use offline fixtures. Tailor does not create a personal access token or configure a secret.

## Disable publishing

Set `repository.has_wiki: false` and run `tailor alter` to disable the wiki and remove only Tailor's marked workflow. Commit and push the removal to stop future workflow runs. Local source files and remote wiki history remain intact. An unmarked workflow remains untouched and needs manual removal.

An omitted setting leaves wiki files unmanaged for that run. With [default merging](Configuration#default-merging) enabled, Tailor saves `has_wiki: false`, so the next run removes the marked workflow. To stop wiki management across later runs, disable default merging and omit `repository.has_wiki`.
