# Manual Testing Guide

This guide is for manually testing the current GitHub Actions slice of
`conjur-onboard` on macOS, Linux, or Windows.

The safest first pass is:

1. Build the CLI.
2. Run GitHub discovery for one or two selected repositories.
3. Generate artifacts.
4. Run `validate --dry-run`.
5. Review the generated files.
6. Run live `validate`.
7. Apply only in a test Conjur environment or with test repositories.

## Test Inputs

Collect these before starting:

- GitHub organization or user owner: `GITHUB_ORG`
- One test repository owned by that account: `GITHUB_REPO`, for example
  `api-service`
- Secrets Manager SaaS tenant subdomain: `CONJUR_TENANT`
- Conjur Enterprise or Secrets Manager Self-Hosted appliance URL:
  `CONJUR_URL`
- Optional Conjur account name for self-hosted targets: `CONJUR_ACCOUNT`
- Conjur username: `CONJUR_USERNAME`
- Conjur API key: `CONJUR_API_KEY`
- Optional existing authenticator name for workloads-only testing
- Optional `--insecure-skip-tls-verify` for local self-signed Conjur endpoints

When testing Conjur API key auth manually, send the API key as the raw request
body with `Content-Type: text/plain`. Bruno should use a plain text body, not
JSON or form data.

The tenant value is the subdomain only. Use `my-tenant`, not
`https://my-tenant.secretsmgr.cyberark.cloud/api`. The tool adds the SaaS
`/api` base path internally.

For self-hosted targets, use the appliance URL as-is, for example
`https://conjur.example.com`. The tool does not append `/api` to
`--conjur-url`. Authenticator creation uses
`/authenticators/<account-name>` on that URL.

If the local endpoint uses a self-signed certificate, add
`--insecure-skip-tls-verify` to live `validate`, `apply`, `rollback`, or
`express --apply` commands. Do not use this flag for production endpoints.

## 1. Build And Smoke Test

macOS or Linux:

```sh
mkdir -p bin
go build -o ./bin/conjur-onboard .
./bin/conjur-onboard --help
./bin/conjur-onboard github --help
```

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force bin
go build -o .\bin\conjur-onboard.exe .
.\bin\conjur-onboard.exe --help
.\bin\conjur-onboard.exe github --help
```

Expected result: help text prints and lists the `github` command plus
`discover`, `inspect`, `generate`, `validate`, `apply`, `rollback`, and
`express`.

## 2. Configure GitHub Auth

Option A: GitHub CLI.

macOS, Linux, or Windows PowerShell:

```sh
gh auth login
gh auth refresh -s repo,read:org
```

Option B: environment variable.

macOS or Linux:

```sh
export GITHUB_TOKEN=<github-token>
```

Windows PowerShell:

```powershell
$env:GITHUB_TOKEN = "<github-token>"
```

The token must be able to read the target owner and repositories. For private
repositories, use a token with repository read access. For user-owned public
repositories such as `keerth124/onboarding-toolkit`, set `GITHUB_ORG` to the
username, for example `keerth124`.

When `GITHUB_ORG` is your own GitHub username, discovery uses GitHub's
authenticated-user repository endpoint so it can include repositories you own
that are not returned by the public user listing. If the count looks too low,
refresh `gh` with the documented scopes:

```sh
gh auth refresh -s repo,read:org
```

## 3. Create A Small Repo List

For early tests, use `--repos-from-file` to avoid onboarding every repository
visible for the owner.

macOS or Linux:

```sh
cat > repos.txt <<'EOF'
api-service
EOF
```

Windows PowerShell:

```powershell
Set-Content -Path .\repos.txt -Value "api-service"
```

Use either `repo-name` or `owner/repo-name` per line. Blank lines are ignored.

## 4. Discovery

macOS or Linux:

```sh
export WORK_DIR=./manual-test-github
export GITHUB_ORG=acme-corp

./bin/conjur-onboard github discover \
  --org "$GITHUB_ORG" \
  --repos-from-file repos.txt \
  --work-dir "$WORK_DIR" \
  --verbose
```

Windows PowerShell:

```powershell
$env:WORK_DIR = ".\manual-test-github"
$env:GITHUB_ORG = "acme-corp"

.\bin\conjur-onboard.exe github discover `
  --org $env:GITHUB_ORG `
  --repos-from-file .\repos.txt `
  --work-dir $env:WORK_DIR `
  --verbose
```

Expected result:

- `discovery.json` exists in the work directory.
- The output reports the selected repository count.
- Warnings about environment or OIDC subject customization are acceptable for
  this MVP, but should be reviewed.

## 5. Claim Inspection

macOS or Linux:

```sh
./bin/conjur-onboard github inspect \
  --repo "$GITHUB_ORG/api-service" \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github inspect `
  --repo "$env:GITHUB_ORG/api-service" `
  --work-dir $env:WORK_DIR
```

Expected result:

- `claims-analysis.json` exists.
- `repository` is selected as `token_app_property`.
- Enforced claims are `none` unless you explicitly pass `--enforced-claims`.

This slice supports synthetic inspection only. `--mode live` is intentionally
not implemented yet.

## 6. Generate Bootstrap Artifacts

macOS or Linux:

```sh
export CONJUR_TENANT=my-tenant

./bin/conjur-onboard github generate \
  --tenant "$CONJUR_TENANT" \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
$env:CONJUR_TENANT = "my-tenant"

.\bin\conjur-onboard.exe github generate `
  --tenant $env:CONJUR_TENANT `
  --work-dir $env:WORK_DIR
```

Expected result:

- `api/01-create-authenticator.json`
- `api/02-workloads.yml`
- `api/03-add-group-members.jsonl`
- `api/plan.json`
- `integration/example-deploy.yml`
- `integration/README.md`
- `NEXT_STEPS.md`

Review `api/plan.json` and confirm:

- `provisioning_mode` is `bootstrap`.
- `authenticator_type` is `jwt`.
- `authenticator_subtype` is `github_actions`.
- `authenticator_name` is `github-<org>`.
- The first operation is `create-authenticator`.
- SaaS plans include `load-identity-branch` and `load-workload-policy`.
- `load-identity-branch` appends to `data` and creates the nested platform and
  org/controller branches, for example `data/github-apps/<org>`.
- `load-workload-policy` appends hosts directly to the leaf branch such as
  `/policies/conjur/policy/data%2Fgithub-apps%2F<org>`.
- The apply identity must be able to append policy under `data`.
- Generated rollback-capable operations include `metadata.rollback_kind`.
- Workload IDs are under `data/github-apps/<org>/...` and use the repository
  name only, for example `data/github-apps/acme/api`.
- Each generated host has a JWT repository annotation such as
  `authn-jwt/github-<org>/repository: <owner>/<repo>`.

## 7. Local Dry-Run Validation

This does not contact Conjur.

macOS or Linux:

```sh
./bin/conjur-onboard github validate \
  --tenant "$CONJUR_TENANT" \
  --dry-run \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github validate `
  --tenant $env:CONJUR_TENANT `
  --dry-run `
  --work-dir $env:WORK_DIR
```

Expected result:

- All operation body files are readable.
- `validate-log.json` is written.
- No Conjur credentials are required.

## 8. Local Dry-Run Apply

This does not contact Conjur.

macOS or Linux:

```sh
./bin/conjur-onboard github apply \
  --tenant "$CONJUR_TENANT" \
  --dry-run \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github apply `
  --tenant $env:CONJUR_TENANT `
  --dry-run `
  --work-dir $env:WORK_DIR
```

Expected result:

- `apply-log.json` is written with dry-run entries.
- No Conjur credentials are required.

If you plan to run a real apply afterward, remove the dry-run `apply-log.json`
or use a fresh work directory so rollback testing is not confused by dry-run
state.

## 9. Live Conjur Validation

macOS or Linux:

```sh
export CONJUR_USERNAME=admin
export CONJUR_API_KEY=<conjur-api-key>

./bin/conjur-onboard github validate \
  --tenant "$CONJUR_TENANT" \
  --username "$CONJUR_USERNAME" \
  --work-dir "$WORK_DIR" \
  --verbose
```

Windows PowerShell:

```powershell
$env:CONJUR_USERNAME = "admin"
$env:CONJUR_API_KEY = "<conjur-api-key>"

.\bin\conjur-onboard.exe github validate `
  --tenant $env:CONJUR_TENANT `
  --username $env:CONJUR_USERNAME `
  --work-dir $env:WORK_DIR `
  --verbose
```

Expected result in `bootstrap` mode:

- Missing `github-<org>` authenticator is OK.
- Existing compatible `github-<org>` authenticator produces a warning.
- Existing conflicting authenticator fails validation.

Expected result in `workloads-only` mode:

- Missing authenticator fails validation.
- Existing compatible authenticator passes.

## 10. Apply Bootstrap Plan

Apply only after reviewing the generated artifacts and confirming this is a
test Conjur environment or intended test scope.

macOS or Linux:

```sh
./bin/conjur-onboard github apply \
  --tenant "$CONJUR_TENANT" \
  --username "$CONJUR_USERNAME" \
  --work-dir "$WORK_DIR" \
  --verbose
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github apply `
  --tenant $env:CONJUR_TENANT `
  --username $env:CONJUR_USERNAME `
  --work-dir $env:WORK_DIR `
  --verbose
```

Expected result:

- Authenticator create operation succeeds or is treated as no-change if the API
  returns an idempotent status.
- Workload policy load succeeds.
- Group memberships are added.
- `apply-log.json` records every call.

## 11. Test Workloads-Only Mode

Use a new work directory and a repo that was not part of the bootstrap test if
possible.

macOS or Linux:

```sh
export WORKLOADS_ONLY_DIR=./manual-test-github-workloads-only

./bin/conjur-onboard github discover \
  --org "$GITHUB_ORG" \
  --repos-from-file repos.txt \
  --work-dir "$WORKLOADS_ONLY_DIR"

./bin/conjur-onboard github inspect \
  --repo "$GITHUB_ORG/api-service" \
  --work-dir "$WORKLOADS_ONLY_DIR"

./bin/conjur-onboard github generate \
  --tenant "$CONJUR_TENANT" \
  --provisioning-mode workloads-only \
  --work-dir "$WORKLOADS_ONLY_DIR"
```

Windows PowerShell:

```powershell
$env:WORKLOADS_ONLY_DIR = ".\manual-test-github-workloads-only"

.\bin\conjur-onboard.exe github discover `
  --org $env:GITHUB_ORG `
  --repos-from-file .\repos.txt `
  --work-dir $env:WORKLOADS_ONLY_DIR

.\bin\conjur-onboard.exe github inspect `
  --repo "$env:GITHUB_ORG/api-service" `
  --work-dir $env:WORKLOADS_ONLY_DIR

.\bin\conjur-onboard.exe github generate `
  --tenant $env:CONJUR_TENANT `
  --provisioning-mode workloads-only `
  --work-dir $env:WORKLOADS_ONLY_DIR
```

Review `api/plan.json` and confirm:

- `provisioning_mode` is `workloads-only`.
- `authenticator_subtype` remains `github_actions`.
- There is no `create-authenticator` operation.
- The group membership path still targets the existing authenticator apps
  group.

Then run live validation and apply with the same command shape from sections 9
and 10, replacing `WORK_DIR` with `WORKLOADS_ONLY_DIR`.

If the existing authenticator is not named `github-<org>`, regenerate with:

```sh
--authenticator-name <existing-authenticator-name>
```

## 11a. Test Self-Hosted / Enterprise Mode

Use `--conjur-url` instead of `--tenant` and set `--conjur-target self-hosted`
when generating artifacts. Use the self-hosted appliance URL as-is; the tool
does not append `/api` for self-hosted targets.

macOS or Linux:

```sh
export CONJUR_URL=https://conjur.example.com
export CONJUR_ACCOUNT=conjur

./bin/conjur-onboard github generate \
  --conjur-url "$CONJUR_URL" \
  --conjur-target self-hosted \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
$env:CONJUR_URL = "https://conjur.example.com"
$env:CONJUR_ACCOUNT = "conjur"

.\bin\conjur-onboard.exe github generate `
  --conjur-url $env:CONJUR_URL `
  --conjur-target self-hosted `
  --work-dir $env:WORK_DIR
```

Expected result:

- `api/00-authenticator-branch.yml` exists.
- `api/04-grant-authenticator-access.yml` exists.
- `api/01-create-authenticator.json` does not include `subtype`.
- `api/plan.json` uses `/authenticators/{account}` for
  `create-authenticator`.
- `api/plan.json` uses `/policies/{account}/policy/root` for self-hosted
  policy loads.
- `api/plan.json` includes `load-authenticator-grants`.
- `api/plan.json` does not include `add-group-member-*` operations.
- `load-authenticator-grants` includes metadata noting manual policy review
  rollback behavior.

Apply with:

macOS or Linux:

```sh
./bin/conjur-onboard github apply \
  --conjur-url "$CONJUR_URL" \
  --account "$CONJUR_ACCOUNT" \
  --username "$CONJUR_USERNAME" \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github apply `
  --conjur-url $env:CONJUR_URL `
  --account $env:CONJUR_ACCOUNT `
  --username $env:CONJUR_USERNAME `
  --work-dir $env:WORK_DIR
```

If your Conjur account is not `conjur`, pass the correct account with
`--account`; `apply`, `validate`, and `rollback` use that value for the
self-hosted `/authenticators/<account-name>` endpoint.

## 12. Rollback

Rollback is destructive. It removes group memberships and deletes generated
workloads. In bootstrap mode, it also deletes the authenticator only if the
current plan successfully created it.

Dry-run first:

macOS or Linux:

```sh
./bin/conjur-onboard github rollback \
  --tenant "$CONJUR_TENANT" \
  --dry-run \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github rollback `
  --tenant $env:CONJUR_TENANT `
  --dry-run `
  --work-dir $env:WORK_DIR
```

Execute:

macOS or Linux:

```sh
./bin/conjur-onboard github rollback \
  --tenant "$CONJUR_TENANT" \
  --username "$CONJUR_USERNAME" \
  --work-dir "$WORK_DIR" \
  --confirm \
  --verbose
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github rollback `
  --tenant $env:CONJUR_TENANT `
  --username $env:CONJUR_USERNAME `
  --work-dir $env:WORK_DIR `
  --confirm `
  --verbose
```

Expected result:

- `rollback-log.json` is written.
- `apply-log.json` is moved to `apply-log.rolled-back.json` after successful
  non-dry-run rollback.
- Re-running rollback should be a clean no-op because `apply-log.json` is gone.

## 13. GitHub Workflow Verification

After apply, inspect `integration/example-deploy.yml`.

Before placing it in a repository:

- Replace `data/vault/example/safe/test-secret` with a real test variable path.
- Confirm the workflow keeps:
  - `permissions: id-token: write`
  - `permissions: contents: read`
- Grant the generated apps group to the safe or policy containing that test
  secret.

The apps group is:

```text
conjur/authn-jwt/<authenticator-name>/apps
```

COT intentionally does not create safe grants.

## Troubleshooting

- `GitHub token required`: set `GITHUB_TOKEN`, pass `--token`, or authenticate
  with `gh auth login`.
- `GitHub token scopes are missing`: run `gh auth refresh -s repo,read:org` or
  use a token with equivalent access.
- User-owned discovery returns fewer repositories than expected: confirm the
  requested owner matches the authenticated `gh` user and run
  `gh auth refresh -s repo,read:org`; public user listings do not include every
  repository the authenticated user can access.
- `CONJUR_API_KEY environment variable is required`: set it in the shell running
  `validate`, `apply`, or `rollback`.
- HTTP 401 from Conjur auth: check `--tenant` or `--conjur-url`, `--account`,
  `--username`, and `CONJUR_API_KEY`.
- TLS certificate errors against a local self-signed endpoint: retry the live
  command with `--insecure-skip-tls-verify`.
- HTTP 403 on Conjur API calls: the tool identity likely lacks authenticator or
  policy management privileges.
- `workloads-only mode requires existing authenticator`: run bootstrap first or
  pass `--authenticator-name` for the existing org authenticator.
- GitHub Actions cannot fetch an OIDC token: confirm `permissions:
  id-token: write` is present in the workflow.

## Known Limitations During Manual Testing

- GitHub live OIDC token inspection is not implemented.
- Interactive claim selection is not implemented.
- Environment claim enforcement is not emitted by the MVP generator.
- Workload creation currently uses policy loading.
- Conjur auth currently uses API key auth, not CyberArk Identity session
  auth.
- Some live SaaS endpoint shapes still need confirmation against a real tenant.
