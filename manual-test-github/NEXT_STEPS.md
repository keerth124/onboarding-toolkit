# Next Steps: GitHub Actions Onboarding

## Generated Summary

Platform: GitHub Actions

Conjur Cloud tenant: `my-tenant`

Authenticator type: `jwt`

Authenticator name: `github-keerth124`

Provisioning mode: `bootstrap`

This plan creates the GitHub authenticator, workloads, and group memberships.

Workload count: `8`

Identity claim: `repository`

Enforced claims: none in the MVP generator

Apps group to grant to safes: `conjur/authn-jwt/github-keerth124/apps`

## 1. Review the Generated Plan

Command:

```sh
conjur-onboard github generate --tenant my-tenant --work-dir manual-test-github
```

Expected outcome: `api/plan.json`, `api/01-create-authenticator.json`, `api/02-workloads.yml`, `api/03-add-group-members.jsonl`, and `integration/example-deploy.yml` are present and reviewable.

## 2. Validate Against the Tenant

Command:

```sh
CONJUR_API_KEY=<api-key> conjur-onboard github validate --tenant my-tenant --username <username> --work-dir manual-test-github
```

Expected outcome: validation can read all generated bodies and reach the tenant API.

## 3. Apply the Plan

Command:

```sh
CONJUR_API_KEY=<api-key> conjur-onboard github apply --tenant my-tenant --username <username> --work-dir manual-test-github
```

Expected outcome: the authenticator is created, workload policy is loaded, and `8` workload memberships are added to `conjur%2Fauthn-jwt%2Fgithub-keerth124%2Fapps`.

## 4. Grant Safe Access

COT does not grant access to safes. Grant the generated apps group to the safe or policy that contains the secrets this workflow should read:

```text
conjur/authn-jwt/github-keerth124/apps
```

Expected outcome: workloads in the apps group can read only the secrets that security approves.

## 5. Verify End to End

Add the sample workflow from `integration/example-deploy.yml` to a test repository and keep the `permissions: id-token: write` block. Replace the example secret path with a known test secret.

Expected outcome: the workflow fetches the test secret and the deployment step receives it through the configured environment variable.

## Troubleshooting

- HTTP 401 during validate or apply: check `CONJUR_API_KEY` and the `--username` value.
- HTTP 403 during authenticator creation: the tool identity likely needs create privileges on the authenticator policy branch, typically through `Authn_Admins`.
- GitHub workflow cannot obtain an OIDC token: confirm `permissions: id-token: write` is present at workflow or job level.
- Host not found during secret fetch: confirm the workflow repository matches one of the generated workload IDs under `data/github-apps/keerth124`.
- Secret not found or permission denied: grant the apps group access to the safe; COT intentionally does not generate safe grants.

## Known MVP Limitation

Synthetic claim analysis is generated from the documented GitHub OIDC schema. Live inspection and interactive claim selection are not implemented in this first GitHub slice.

Environment claims are recorded for review but not enforced by the MVP generator. Enforcing `environment` safely requires a compatible GitHub identity strategy so Conjur can map each token to the correct workload.
