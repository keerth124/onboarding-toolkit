# GitHub Discovery: Detect Repo-Level OIDC Subject Claim Customization

Labels: `github`, `discovery`, `technical-debt`

## Background

The GitHub PRD requires discovery to detect OIDC customizations that affect token subject and claim shape. Current discovery checks org-level customization via:

```text
GET /orgs/{org}/actions/oidc/customization/sub
```

but does not inspect repository-level overrides.

GitHub supports repo-level OIDC subject customization via:

```text
GET /repos/{owner}/{repo}/actions/oidc/customization/sub
```

This matters because a repository can use a custom subject template that makes generated Conjur identity defaults invalid.

## Work To Do

- Extend `internal/github/discover.go` to call repo-level OIDC customization for each non-archived discovered repo.
- Add a field to `RepoInfo`, for example:

```go
type RepoOIDCSubCustomization struct {
	UsesDefault      bool     `json:"uses_default"`
	IncludeClaimKeys []string `json:"include_claim_keys,omitempty"`
	Warning          string   `json:"warning,omitempty"`
}
```

- Sort `include_claim_keys` deterministically before writing `discovery.json`.
- Treat 404 as a per-repo warning, not a fatal discovery failure.
- Treat 401/403 as a clear auth/permission failure.
- Add repo-level warnings to both the repo object and top-level `discovery.json.warnings`.
- Later generation should warn if repo-level OIDC customization conflicts with selected claims.

## Acceptance Criteria

- `conjur-onboard github discover --org <org>` records repo-level OIDC subject customization for each non-archived repo.
- `conjur-onboard github discover --org <org> --repos-from-file repos.txt` does the same for selected repos.
- One repo returning 404 does not fail all discovery.
- Token permission failures produce a remediation message.
- Unit tests cover 200, 403, and 404 behavior.

## Reference

https://docs.github.com/en/rest/actions/oidc#get-the-customization-template-for-an-oidc-subject-claim-for-a-repository
