# CI Token Requirements

## GitHub API permission model

### Endpoint matrix

| API category | Endpoint(s) | Required scope | `repo` alone sufficient | `GITHUB_TOKEN` in CI sufficient | Notes |
|---|---|---|---|---|---|
| Repo settings PATCH | `PATCH /repos/{owner}/{repo}` | Token-dependent repository administration access | Often | Depends on repository and job permissions | Tailor skips access errors; installation-token reads are unreliable for some merge and branch fields |
| Topics | `PUT /repos/{owner}/{repo}/topics` | Token-dependent repository metadata access | Often | Depends on repository and job permissions | Fine-grained tokens use Metadata write |
| Labels | `GET/POST/PATCH/DELETE /repos/{owner}/{repo}/labels` | Token-dependent issues access | Often | Depends on repository and job permissions | Fine-grained tokens use Issues write for writes |
| Actions workflow permissions | `GET/PUT /repos/{owner}/{repo}/actions/permissions/workflow` | Token-dependent repository administration access | Depends on token type and access | Depends on repository and job permissions | Tailor skips the fields when GitHub returns an access error |
| Licence fetch | `GET /licenses/{id}`, `GET /repos/{owner}/{repo}/license` | none (public) | Yes | Yes | Public endpoint; `repo` scope needed for private repos |
| File contents read/write | `GET/PUT /repos/{owner}/{repo}/contents/{path}` | `repo` (write) | Yes | Yes (contents:read/write) | Public read needs no auth; write always requires repo scope |
| Repository creation (user) | `POST /user/repos` | `repo` | Yes | No | `GITHUB_TOKEN` cannot create repos |
| Repository creation (org) | `POST /orgs/{org}/repos` | `repo`/`public_repo` | Yes | No | `GITHUB_TOKEN` cannot create repos in other orgs |

### Scope header semantics

- `X-OAuth-Scopes`: comma-separated scopes the token holds. Present only on classic PAT responses; absent for `GITHUB_TOKEN` and fine-grained PATs.
- `X-Accepted-OAuth-Scopes`: minimum scopes the endpoint accepts. Present on all authenticated requests.
- `X-Accepted-GitHub-Permissions`: present on 403 responses for fine-grained PATs; contains the required permission, e.g. `pull_requests=write`.

| Code | Meaning |
|---|---|
| 401 | Invalid or expired token |
| 403 | Insufficient scope or permission |
| 404 | Returned instead of 403 for private resources to avoid leaking existence |

## GitHub Actions behaviour

`GITHUB_TOKEN` is an installation token. Its effective access depends on the repository, workflow event, and job `permissions:`. Tailor does not assume that a named permission guarantees an endpoint will accept the token. It attempts each API operation and skips classified access errors so other settings can continue.

Tailor also probes `GET /user` to detect installation-token behaviour. When that request fails in GitHub Actions, username substitution falls back to `GITHUB_REPOSITORY_OWNER`. For settings that the installation token cannot manage, pass a suitably scoped PAT as `GH_TOKEN`.
