# Conjur Onboarding Toolkit

Initial CLI for generating CyberArk Secrets Manager SaaS onboarding artifacts.
The first implemented platform slice is GitHub Actions with GitHub OIDC and a
JWT authenticator.

## GitHub MVP Flow

```sh
conjur-onboard github discover --org acme-corp
conjur-onboard github inspect --repo acme-corp/api-service
conjur-onboard github generate --tenant my-tenant
conjur-onboard github generate --tenant my-tenant --provisioning-mode workloads-only
CONJUR_API_KEY=<api-key> conjur-onboard github validate --tenant my-tenant --username <username>
CONJUR_API_KEY=<api-key> conjur-onboard github apply --tenant my-tenant --username <username>
CONJUR_API_KEY=<api-key> conjur-onboard github rollback --tenant my-tenant --username <username> --confirm
```

`discover` uses `--token`, `GITHUB_TOKEN`, or an authenticated `gh` CLI session.
Generated artifacts are written to the configured work directory, including
`api/plan.json`, reviewable API bodies, a GitHub Actions workflow snippet, and
`NEXT_STEPS.md`.

## Current Limitations

- GitHub live OIDC token inspection is not implemented yet.
- Interactive claim selection is not implemented yet.
- Environment claim enforcement is not emitted yet; the MVP generator produces
  repo-level workloads using the `repository` claim.
- `bootstrap` mode creates the GitHub authenticator. `workloads-only` mode
  assumes the org-level authenticator already exists.
- Rollback removes app-group memberships and generated workloads. It deletes the
  authenticator only when the current plan created it.
- Validation is intentionally conservative until the exact SaaS API endpoint
  shapes for every preflight check are confirmed.
