# GitHub Discovery: Improve Fine-Grained Token Permission Messages

Labels: `github`, `discovery`, `auth`, `technical-debt`

## Background

Current discovery improves classic OAuth token scope handling using the `X-OAuth-Scopes` response header. Fine-grained personal access tokens and GitHub App tokens may not expose classic scopes in that header, so error messages can still be too generic.

The PRD requires missing platform permissions to be actionable, especially for GitHub discovery.

## Work To Do

- Improve auth error classification for GitHub discovery endpoints:
  - org metadata,
  - org repo listing,
  - selected repo lookup,
  - repo environments,
  - org/repo OIDC customization.
- Detect when `X-OAuth-Scopes` is empty and emit guidance for fine-grained tokens.
- Include endpoint-specific required permissions in messages. Examples:
  - Organization metadata: organization read access.
  - Repo listing/private repo discovery: repository metadata/read access.
  - Environments: repository metadata/read access.
  - OIDC customization: Actions or organization administration read access as required by GitHub.
- Where possible, read GitHub error response body and include the message without dumping large bodies.
- Keep secrets/tokens out of logs and errors.

## Acceptance Criteria

- A 403 with classic OAuth scopes still reports missing classic scopes where available.
- A 403 with no `X-OAuth-Scopes` reports likely fine-grained token permission requirements.
- Messages include the affected endpoint/resource, for example repo or org name.
- Tests cover classic token header, empty-scope fine-grained style response, 401, and 403.
- Error messages include remediation text such as `gh auth refresh -s repo,read:org` for classic `gh` tokens.

## References

GitHub fine-grained PAT behavior varies by endpoint, so tests should validate our message strategy rather than rely on one exact GitHub response body.
