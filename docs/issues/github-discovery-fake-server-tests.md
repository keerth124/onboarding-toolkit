# GitHub Discovery: Add Fake GitHub Server Tests For API Workflows

Labels: `github`, `discovery`, `testing`, `technical-debt`

## Background

Current discovery has unit tests for pure helpers, but not for live API behavior. We need tests that exercise GitHub discovery against an in-process fake HTTP server so we can safely validate pagination, error handling, auth messages, warnings, and JSON shape without depending on real GitHub.

## Work To Do

- Refactor `internal/github/discover.go` to allow overriding the GitHub API base URL in tests.
- Keep production default as:

```text
https://api.github.com
```

- Do not expose the test API base URL as a public CLI flag.
- Add `httptest.Server` tests for:
  - org metadata fetch,
  - org OIDC subject customization fetch,
  - paginated org repo listing,
  - selected repo lookup from `RepoNames`,
  - paginated repo environments,
  - archived repo filtering,
  - warning behavior for optional endpoint 404s,
  - auth failure behavior for 401/403.
- Verify `discovery.json`-equivalent structs contain stable sorted output where expected.

## Acceptance Criteria

- `go test ./internal/github` covers the main discovery request paths without calling the real network.
- Tests verify pagination for repos and environments.
- Tests verify archived repos are omitted.
- Tests verify warnings are persisted for optional OIDC/environment failures.
- Tests verify auth failures mention the relevant required permission/scope.
- Production behavior remains unchanged.

## Implementation Notes

Prefer testing through `Discover(ctx, DiscoverConfig{...})` rather than testing every helper directly. Keep helper tests only where behavior is genuinely isolated.
