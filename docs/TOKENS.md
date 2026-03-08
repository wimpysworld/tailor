# CI Token Requirements

## GitHub API permission model

### Endpoint matrix

| API category | Endpoint(s) | Required scope | `repo` alone sufficient | `GITHUB_TOKEN` in CI sufficient | Notes |
|---|---|---|---|---|---|
| Repo settings PATCH | `PATCH /repos/{owner}/{repo}` | `repo` | Yes | Yes (contents:write) | Standard settings work with repo write access, admin-only fields use separate endpoints |
| Topics | `PUT /repos/{owner}/{repo}/topics` | `repo` | Yes | Yes (contents:write) | Fine-grained: Metadata write |
| Labels | `GET/POST/PATCH/DELETE /repos/{owner}/{repo}/labels` | `repo` | Yes | Yes (issues:write for writes) | Labels sit under issues permissions in fine-grained |
| Private vulnerability reporting | `PUT/DELETE /repos/{owner}/{repo}/private-vulnerability-reporting` | `repo` | Yes | No | Requires repo admin or security manager role |
| Vulnerability alerts | `PUT/DELETE /repos/{owner}/{repo}/vulnerability-alerts` | `repo` | Yes | No | Requires admin role; fine-grained needs security-events:write |
| Automated security fixes | `PUT/DELETE /repos/{owner}/{repo}/automated-security-fixes` | `repo` | Yes | No | Same constraints as vulnerability alerts |
| Actions workflow permissions | `GET/PUT /repos/{owner}/{repo}/actions/permissions/workflow` | `repo` | Yes | Yes (actions:write) | Repo-level only; org-level requires admin:org |
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
| 403 | Insufficient scope or insufficient role |
| 404 | Returned instead of 403 for private resources to avoid leaking existence |

Security feature endpoints (e.g. `GET /repos/{owner}/{repo}/vulnerability-alerts`) return 404 when the feature is disabled - not a scope error in the read path.
