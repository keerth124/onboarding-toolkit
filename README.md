# Conjur Onboarding Toolkit

Conjur Onboarding Toolkit, `conjur-onboard`, generates reviewable CyberArk
Conjur onboarding artifacts for CI/CD workloads.

The current implemented slice is GitHub Actions using GitHub OIDC and a Conjur
JWT authenticator.

The implementation is now organized around reusable platform contracts. GitHub
is the first adapter, while Conjur generation, apply, validate, rollback, and
shared CLI wiring are structured for reuse by future platforms.

## What It Does Today

- Discovers repositories and environments for a GitHub organization or user
  owner.
- Generates a GitHub Actions JWT authenticator body.
- Generates Conjur workload policy YAML for discovered repositories.
- Adds JWT claim annotations to generated workloads, including the GitHub
  `repository` claim used by the authenticator.
- Generates group membership API bodies for the authenticator `apps` group on
  SaaS.
- Generates policy-load grant fallback artifacts for Conjur Enterprise and
  Secrets Manager Self-Hosted.
- Supports two GitHub provisioning modes:
  - `bootstrap`: create the GitHub authenticator, workloads, and memberships.
  - `workloads-only`: create only workloads and memberships for an existing
    org-level authenticator.
- Validates generated plans against a Secrets Manager SaaS tenant or
  self-hosted Conjur endpoint.
- Applies generated API calls and writes `apply-log.json`.
- Rolls back successful apply operations using `apply-log.json`.

## Prerequisites

- Go 1.22 or newer.
- GitHub access to the target organization or user-owned repository.
- One GitHub auth option:
  - GitHub CLI, `gh`, authenticated with `repo` and `read:org` scopes.
  - `GITHUB_TOKEN`.
  - `--token <token>`.
- For SaaS validation/apply/rollback:
  - Secrets Manager SaaS tenant subdomain, for example `my-tenant` from
    `https://my-tenant.secretsmgr.cyberark.cloud/api`.
- For Conjur Enterprise or Secrets Manager Self-Hosted validation/apply/rollback:
  - Full appliance URL, for example `https://conjur.example.com`.
  - Optional Conjur account name if it is not `conjur`.
- For all Conjur targets:
  - Conjur username.
  - Conjur API key in `CONJUR_API_KEY`.

For complete discovery of repositories owned by your own GitHub user account,
authenticate `gh` with:

```sh
gh auth refresh -s repo,read:org
```

Current Conjur auth uses API key auth. SaaS tenant mode uses this URL shape:

```text
POST https://<tenant>.secretsmgr.cyberark.cloud/api/authn/conjur/<username>/authenticate
```

Self-hosted mode uses the provided appliance URL without appending `/api`:

```text
POST https://<conjur-host>/authn/<account>/<username>/authenticate
```

CyberArk Identity session auth is still a PRD target, not implemented in this
slice.

## Build

macOS or Linux:

```sh
mkdir -p bin
go build -o ./bin/conjur-onboard .
./bin/conjur-onboard --help
```

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force bin
go build -o .\bin\conjur-onboard.exe .
.\bin\conjur-onboard.exe --help
```

If Go is not on your Windows `PATH`, use the full path:

```powershell
& 'C:\Program Files\Go\bin\go.exe' build -o .\bin\conjur-onboard.exe .
```

You can also run from source without building:

```sh
go run . github --help
```

## GitHub SaaS Quickstart

Use a stable work directory so each step reads the artifacts from the previous
step.

macOS or Linux:

```sh
export WORK_DIR=./manual-test-github
export GITHUB_TOKEN=<github-token>

./bin/conjur-onboard github discover --org acme-corp --work-dir "$WORK_DIR"
./bin/conjur-onboard github inspect --repo acme-corp/api-service --work-dir "$WORK_DIR"
./bin/conjur-onboard github generate --tenant my-tenant --work-dir "$WORK_DIR"
./bin/conjur-onboard github validate --tenant my-tenant --dry-run --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
$env:WORK_DIR = ".\manual-test-github"
$env:GITHUB_TOKEN = "<github-token>"

.\bin\conjur-onboard.exe github discover --org acme-corp --work-dir $env:WORK_DIR
.\bin\conjur-onboard.exe github inspect --repo acme-corp/api-service --work-dir $env:WORK_DIR
.\bin\conjur-onboard.exe github generate --tenant my-tenant --work-dir $env:WORK_DIR
.\bin\conjur-onboard.exe github validate --tenant my-tenant --dry-run --work-dir $env:WORK_DIR
```

Review these generated files before applying:

- `discovery.json`
- `claims-analysis.json`
- `api/01-create-authenticator.json`
- `api/02-workloads.yml`
- `api/03-add-group-members.jsonl`
- `api/plan.json`
- `integration/example-deploy.yml`
- `NEXT_STEPS.md`

`api/plan.json` is the stable contract consumed by `validate`, `apply`, and
`rollback`. It records platform-neutral Conjur operations plus expected
authenticator metadata, including `authenticator_type`,
`authenticator_subtype`, `authenticator_name`, and `identity_path`.

## Apply To Conjur

Run a live validation first:

macOS or Linux:

```sh
export CONJUR_API_KEY=<conjur-api-key>

./bin/conjur-onboard github validate \
  --tenant my-tenant \
  --username admin \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
$env:CONJUR_API_KEY = "<conjur-api-key>"

.\bin\conjur-onboard.exe github validate `
  --tenant my-tenant `
  --username admin `
  --work-dir $env:WORK_DIR
```

Apply:

macOS or Linux:

```sh
./bin/conjur-onboard github apply \
  --tenant my-tenant \
  --username admin \
  --work-dir "$WORK_DIR"
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github apply `
  --tenant my-tenant `
  --username admin `
  --work-dir $env:WORK_DIR
```

Rollback, if needed:

macOS or Linux:

```sh
./bin/conjur-onboard github rollback \
  --tenant my-tenant \
  --username admin \
  --work-dir "$WORK_DIR" \
  --confirm
```

Windows PowerShell:

```powershell
.\bin\conjur-onboard.exe github rollback `
  --tenant my-tenant `
  --username admin `
  --work-dir $env:WORK_DIR `
  --confirm
```

Rollback deletes generated workloads and removes generated group memberships. It
deletes the authenticator only when the current plan created it.

## Workloads-Only Mode

Use this after the org-level GitHub authenticator already exists. This is the
normal recurring mode for onboarding additional repositories in the same GitHub
organization.

```sh
./bin/conjur-onboard github generate \
  --tenant my-tenant \
  --provisioning-mode workloads-only \
  --work-dir "$WORK_DIR"
```

If the existing authenticator does not use the default `github-<org>` name, pass
an override:

```sh
./bin/conjur-onboard github generate \
  --tenant my-tenant \
  --provisioning-mode workloads-only \
  --authenticator-name existing-authenticator-name \
  --work-dir "$WORK_DIR"
```

## Self-Hosted Or Enterprise Endpoint

For Conjur Enterprise or Secrets Manager Self-Hosted, generate with the full
appliance URL and the self-hosted target mode:

```sh
./bin/conjur-onboard github generate \
  --conjur-url https://conjur.example.com \
  --conjur-target self-hosted \
  --work-dir "$WORK_DIR"
```

For self-hosted targets, the tool uses `--conjur-url` as provided and does not
append `/api`. The SaaS `/api` base suffix is added only when you use
`--tenant`.

Self-hosted plans still use the manage-authenticators REST endpoint, but they do
not use the SaaS group-membership endpoint. Instead, generation emits
`api/04-grant-authenticator-access.yml` and adds a policy-load operation that
grants generated workloads to `conjur/authn-jwt/<authenticator>/apps`.

Apply with the same endpoint and, if needed, a non-default Conjur account:

```sh
CONJUR_API_KEY=<api-key> ./bin/conjur-onboard github apply \
  --conjur-url https://conjur.example.com \
  --account myaccount \
  --username admin \
  --work-dir "$WORK_DIR"
```

## Manual Testing

See [docs/manual-testing.md](docs/manual-testing.md) for a fuller macOS and
Windows walkthrough, including a low-risk dry-run path, targeted repo discovery,
live Conjur validation, apply, workloads-only, self-hosted, and rollback checks.

## Architecture

The codebase is split so future platforms can reuse the non-platform-specific
pieces:

- `internal/platform`: shared platform contracts, normalized discovery,
  workload, claim, authenticator, integration artifact, and adapter types.
- `internal/github`: GitHub discovery, claim analysis, and the GitHub platform
  adapter. The adapter owns GitHub-specific defaults such as `repository` claim
  identity, `github_actions` subtype, `github-<org>` authenticator names, and
  `data/github-apps/<org>` identity paths.
- `internal/conjur`: platform-neutral Conjur artifact generation for JWT
  onboarding plans. It consumes `internal/platform` types and does not import
  GitHub internals.
- `internal/core`: platform-neutral plan loading, validation, apply, rollback,
  logs, and operation execution.
- `cmd/shared`: reusable CLI flag handling and shared `validate`, `apply`, and
  `rollback` command builders.
- `cmd/github`: GitHub-specific command registration and flows for `discover`,
  `inspect`, `generate`, and `express`.

Validation compares generic expected authenticator fields declared in
`api/plan.json`; it does not hardcode GitHub-specific subtype behavior.
Rollback prefers explicit operation metadata such as `rollback_kind`,
`workload_ids`, `workload_id`, and `member_kind`, while preserving compatibility
with existing operation IDs.

## Current Limitations

- GitHub live OIDC token inspection is not implemented yet.
- Interactive claim selection is not implemented yet.
- Environment claim enforcement is not emitted yet; the MVP generator produces
  repo-level workloads using the `repository` claim.
- Safe grants are not generated or applied. Grant the generated apps group to
  the appropriate safe after apply.
- Validation is conservative until the exact SaaS API endpoint shapes for every
  preflight check are confirmed.
