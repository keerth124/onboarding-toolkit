# Conjur Onboarding Toolkit

Conjur Onboarding Toolkit, `conjur-onboard`, generates reviewable CyberArk
Secrets Manager SaaS onboarding artifacts for CI/CD workloads.

The current implemented slice is GitHub Actions using GitHub OIDC and a Conjur
JWT authenticator.

## What It Does Today

- Discovers GitHub organization repositories and environments.
- Generates a GitHub Actions JWT authenticator body.
- Generates Conjur workload policy YAML for discovered repositories.
- Generates group membership API bodies for the authenticator `apps` group.
- Supports two GitHub provisioning modes:
  - `bootstrap`: create the GitHub authenticator, workloads, and memberships.
  - `workloads-only`: create only workloads and memberships for an existing
    org-level authenticator.
- Validates generated plans against a Secrets Manager SaaS tenant.
- Applies generated API calls and writes `apply-log.json`.
- Rolls back successful apply operations using `apply-log.json`.

## Prerequisites

- Go 1.22 or newer.
- GitHub access to the target organization.
- One GitHub auth option:
  - GitHub CLI, `gh`, authenticated with `repo` and `read:org` scopes.
  - `GITHUB_TOKEN`.
  - `--token <token>`.
- For tenant validation/apply/rollback:
  - Secrets Manager SaaS tenant subdomain, for example `my-tenant` from
    `https://my-tenant.secretsmgr.cyberark.cloud`.
  - Conjur username.
  - Conjur API key in `CONJUR_API_KEY`.

Current tenant auth uses Conjur API key auth:

```text
POST /authn/conjur/<username>/authenticate
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

## GitHub Quickstart

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

## Apply To A Tenant

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

## Manual Testing

See [docs/manual-testing.md](docs/manual-testing.md) for a fuller macOS and
Windows walkthrough, including a low-risk dry-run path, targeted repo discovery,
live tenant validation, apply, workloads-only, and rollback checks.

## Current Limitations

- GitHub live OIDC token inspection is not implemented yet.
- Interactive claim selection is not implemented yet.
- Environment claim enforcement is not emitted yet; the MVP generator produces
  repo-level workloads using the `repository` claim.
- Safe grants are not generated or applied. Grant the generated apps group to
  the appropriate safe after apply.
- Validation is conservative until the exact SaaS API endpoint shapes for every
  preflight check are confirmed.
