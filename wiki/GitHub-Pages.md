# GitHub Pages

Tailor configures GitHub Pages for public repositories, with a deployment workflow and the `github-pages` environment. Pages is disabled by default.

[Setup](#setup) · [Local preview](#local-preview) · [Workflow](#workflow) · [Existing site](#existing-site-and-environment) · [Domain](#custom-domain-and-homepage) · [Links](#static-pages-links) · [Starter](#static-pages-starter) · [Navigation](#static-pages-navigation)

## Setup

For static Pages, Tailor can create a starter site. Hugo and Jekyll need existing source files. Edit these fields in `.tailor.yml`:

```yaml
pages:
  enabled: true           # Default: false. Omission leaves Pages unmanaged.
  generator: static       # static, hugo, or jekyll. Default: static.
  path: pages             # Existing source directory, relative to the project.
  # branch: main          # Omit to follow the current repository default branch.
  # cname: www.example.com # Omit to preserve the domain. "" clears it.
```

Run `tailor baste` to preview.

Run `tailor alter` to apply. Commit the source and generated workflow to the selected branch to deploy.

| Generator | Required source | Build |
| --- | --- | --- |
| `static` | `index.html` | Uploads the source without changing URLs. Use URLs that support the deployment base path. |
| `hugo` | Recognised Hugo configuration and local themes or pinned modules/submodules | Hugo Extended 0.165.0 writes `<path>/public`. Modules require `go.mod` and matching `go.sum` entries. |
| `jekyll` | `_config.yml`, `Gemfile`, and a complete `Gemfile.lock` with Jekyll 4.4.1 | Ruby 3.3.12 builds `<path>/_site`. |

The default source path is `pages`. Symlinks in the source or its parents are rejected. Hugo module replacements are unsupported. Jekyll path dependencies must stay inside the source, and Git dependencies require pinned commits.

Inspected inputs, including static `index.html` and Hugo/Jekyll configuration and dependency files, must be regular files no larger than 1 MiB.

For Hugo and Jekyll, Tailor does not create a site or add Node, Sass, or custom build commands.

The workflow uses pinned actions and GitHub-hosted runners. Pushes deploy only the selected literal branch, never pull requests. Hugo and Jekyll use GitHub's Pages metadata for project prefixes and custom domains. The effective Actions policy must allow the required actions, including `ruby/setup-ruby` for Jekyll.

Tailor reports blocked actions without bypassing the policy or changing repository-wide workflow permissions.

> [!IMPORTANT]
> Pages currently requires a classic token with `repo` scope. Use a repository administrator account to permit environment creation or branch-policy updates. Tailor skips Pages with `insufficient scope` when it cannot prove permissions, including with fine-grained tokens.

## Local preview

When `pages.enabled` is true, Tailor adds the `just pages` recipe and miniserve package. The recipe binds only to `http://127.0.0.1:18473`.

| Generator | Preview directory | Required file |
| --- | --- | --- |
| `static` | The effective `pages.path`, including a custom path | `<pages.path>/index.html` |
| `hugo` | `<pages.path>/public` | `<pages.path>/public/index.html` |
| `jekyll` | `<pages.path>/_site` | `<pages.path>/_site/index.html` |

Build Hugo or Jekyll before preview. If the required file is absent, the recipe stops and prints the matching command:

```bash
hugo --source "<pages.path>"
bundle exec jekyll build --source "<pages.path>" --destination "<pages.path>/_site"
```

Run the command for the selected generator, then run `just pages`. Tailor never serves raw Hugo or Jekyll source files.

## Workflow

The workflow path is `.github/workflows/tailor-pages.yml`, with default mode `always`. Tailor refuses an existing file without the first-line marker `# Managed by Tailor: pages`, even with `--recut`.

| Workflow mode | Behaviour |
| --- | --- |
| `always` | Creates or updates the marked workflow for the selected generator, path and branch. |
| `first-fit` | Creates a missing workflow. Preserves an existing compatible workflow, unless `--recut` applies. |
| `never` | Requires an existing compatible workflow and never writes it, including with `--recut`. |

A protected workflow must run the same steps with the same settings as the generated YAML. Comments and formatting can differ, but changes to execution or permissions block setup.

## Existing site and environment

Enabling Pages management converts an existing branch-based Pages site to workflow publishing. Prepare the source at `pages.path` before applying the change. Tailor leaves other workflows untouched.

If `github-pages` is absent, Tailor creates the environment with a custom deployment policy for the selected branch. For an existing custom policy, Tailor adds that branch and preserves other patterns, secrets, reviewers and timers.

A protected-branches-only policy blocks an ineligible branch. Tailor reports the conflict without weakening the restriction. Select an eligible `pages.branch`, or review the `github-pages` policy under repository **Settings → Environments**. Run `tailor baste` to check again before applying changes.

## Custom domain and homepage

Set `cname` to a domain without a scheme or path. No `CNAME` file is required. DNS and account-level domain verification remain manual. For subdomains, point the DNS CNAME to `<owner>.github.io`, without a repository path.

For apex domains, follow [GitHub's DNS instructions](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site). Complete any required TXT verification in account or organisation Pages settings. Tailor enables HTTPS when the certificate is ready. If DNS, verification or the certificate is pending, complete the reported step and rerun `tailor alter`.

An explicit `repository.homepage`, including `""`, wins. Otherwise, Tailor replaces only a live homepage that points to this repository's GitHub URL. The inline comment `# tailor: inferred homepage <URL>` identifies an inferred config value. Remove that comment, or edit `repository.homepage`, to make the value explicit.

`fit` copies the GitHub homepage exactly. Non-empty imported values carry the inferred marker. An imported empty homepage stays explicit, so Pages leaves it empty.

Tailor updates the homepage only after successful Pages setup.

For Hugo and Jekyll, Tailor appends the output directory to `.gitignore`, unless its swatch mode is `never`. Existing text stays unchanged, and tracked files stay tracked. Static adds no ignore rule.

Omitting `pages` or setting `enabled: false` stops Pages management without deleting the site, workflow or environment. Default merging and `--recut` never enable Pages.

## Static Pages links

`pages.links` manages optional connection icons for `generator: static` only. Non-empty values with Hugo or Jekyll are validation errors. An omitted generator means static.

```yaml
pages:
  enabled: true
  generator: static
  links:
    website: https://example.com
    mastodon: https://fosstodon.org/@example
    email: hello@example.com
    feed: https://example.com/feed.xml
```

The supported keys are `website`, `x`, `bluesky`, `mastodon`, `discord`, `matrix`, `slack`, `linkedin`, `youtube`, `twitch`, `podcast`, `instagram`, `pixelfed`, `tiktok`, `peertube`, `steam`, `itchio`, `patreon`, `github_sponsors`, `kofi`, `forum`, `email`, and `feed`. Use full HTTPS URLs without credentials. `email` takes a bare address without mail headers. Icons follow the key order in `pages.links`.

Tailor preserves this order when it writes the config. Empty strings add no icon. Unknown keys, nulls and non-string values are rejected.

Put these markers on separate lines inside the footer of `<pages.path>/index.html`:

```html
<!-- tailor:links:start -->
<!-- tailor:links:end -->
```

Tailor owns only the content between the markers. It replaces that content with labelled icon links and preserves all other bytes, including under `--recut`. Missing, repeated, reversed or inline markers stop preflight before local or remote writes. The output stays within the 1 MiB input limit.

Omit `links` to leave the page untouched. Use `links: {}` or all-empty values to clear the marked section. New configs include Martin Wimpress’s published links from <https://wimpysworld.link/>. Services without a matching personal URL stay empty.

Default merging never adds these links to existing configs and preserves explicit declarations, including an empty mapping. Disabled Pages leaves links unmanaged. A skipped Pages setup also skips the links update. `baste` reports `would overwrite` for a changed page without writing.

`alter` reports `overwritten`. Repeated application makes no change. An empty mapping with Hugo or Jekyll has no effect.

The generated section includes its own minimal layout and CSS masks, so it needs no JavaScript or project-specific icon classes. It uses pinned Simple Icons 16.30.0 and Octicons 19.36.0 CDN URLs. Slack and LinkedIn use generic organisation and briefcase icons. Other service links use brand icons. Website, podcast, forum, email and feed use generic icons.

GitHub navigation stays separate from these optional connections.

## Static Pages starter

When Pages is enabled with `generator: static`, Tailor creates a starter in a missing or empty `pages.path` (default `pages`). The four embedded sources are `pages/index.html`, `pages/style.css`, `pages/theme.js` and `pages/icon.svg`. Their destination names stay fixed beneath `pages.path`.

The starter uses the default azure theme from µCSS 1.4.9 (`@digicreon/mucss@1.4.9/dist/mu.css`), Work Sans and Fira Code, with pinned CDN dependencies. Custom presentation uses µCSS theme variables for light, dark and system modes. A labelled native selector lets visitors choose System, Light or Dark. System follows live operating system changes and clears the stored override. Explicit choices persist when browser storage is available. Theme colour metadata uses the matching µCSS page background. Without JavaScript, the selector stays hidden and the site follows the operating system. The skip link moves focus to `main`, where a compact marker on the first heading replaces the full-container outline. It needs no build step. Edit the HTML for your introduction, features and installation instructions. Replace `icon.svg` to use your project icon.

One icon supplies the header, footer and favicon.

Optional examples include a three-slide gallery, a screenshot with a caption, a YouTube video, store graphics and a native HTML FAQ. Edit or remove each example before publication. The gallery supports scrolling, swiping and keyboard links without automatic rotation. The video loads lazily and starts only when the visitor plays it.

The store examples load official graphics from their publishers without download links. Keep the badges for your stores and wrap each image in a link to your product listing. Keep the original colours and proportions. The examples cover App Store, Google Play, Mac App Store, Microsoft Store, Snap Store, Flathub, Steam and itch.io.

Unlike the pinned CSS and font dependencies, these publisher-hosted assets can change upstream.

Tailor substitutes the repository name, configured description and repository URLs only when it creates the starter. The copyright year is dynamic. Edit the copyright holder in the footer. No author identity is inferred from the repository owner.

The starter files default to `first-fit`. Tailor never replaces an existing site with starter content, including with `--recut` or `always`. It updates only the opted-in marked sections. A non-empty directory without `index.html` remains an error.

Missing assets in an existing site are not added. If any required starter file uses `never`, creation stops before writes. Supply an existing site to use that mode.

`baste` reports all four proposed files without writing them. Disabled or skipped Pages creates no files. Hugo and Jekyll do not use this starter. The starter sources are excluded from generic swatch processing and use the Pages stage instead.

## Static Pages navigation

Put these markers on separate lines inside the header navigation list to let Tailor manage its repository links:

```html
<!-- tailor:navigation:start -->
<!-- tailor:navigation:end -->
```

The README link comes first. When `repository.has_wiki: true`, its label is `Overview`, followed by a `Documentation` link to the wiki. Otherwise, the README link is `Documentation`. When `repository.has_discussions: true`, a `Discussions` link follows.

A `Download` link to `/releases` always comes last in the header. Omitted or false flags omit the corresponding link.

Tailor uses the repository that it manages to build these URLs. It preserves content outside the markers, including custom header links. No markers means no navigation changes. Malformed markers stop preflight before writes.

This works only for enabled static Pages, independently of `pages.links`. Disabled or skipped Pages and Hugo or Jekyll sites stay unchanged. Run `tailor baste` to preview.

Run `tailor alter` to update the links.

The footer uses `<!-- tailor:footer-navigation:start -->` and `<!-- tailor:footer-navigation:end -->` inside its Resources list. It follows the same README, Wiki and Discussions order. Append `Support` linking to `/blob/HEAD/SUPPORT.md` when either Wiki or Discussions is disabled or omitted. Hide Support only when both are enabled.

Releases stays in the separate Project list.

The footer License link can also follow `license:`. Put these markers around its list item:

```html
<!-- tailor:license:start -->
<!-- tailor:license:end -->
```

Tailor keeps the label `License` and builds `?tab=<license>-1-ov-file` from the configured identifier. An empty value or `none` removes the marked link. Unmarked licence links stay unchanged. The same static Pages, preview and marker rules apply.
