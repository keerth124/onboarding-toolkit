# Product Requirements Document: Conjur Onboarding Toolkit (COT)

**Status:** Draft v1.1
**Owner:** TBD
**Last updated:** 2026-04-20

**Changelog from v1.0:**
- Scope narrowed to **CyberArk Secrets Manager SaaS (Conjur Cloud)** as the v1 target. Conjur Enterprise and Conjur OSS deferred to post-v1.
- Authenticator provisioning reworked to use the SaaS REST API (`POST /api/authenticators`) instead of generated policy YAML for the authenticator itself.
- Workload grants reworked to use the SaaS REST API (`POST /api/groups/{identifier}/members`) against the auto-created `apps` group.
- Added tool authentication section covering how COT authenticates to the SaaS tenant.
- Added Conjur Cloud authentication flag for Terraform and Ansible consumer configurations.

---

## 1. Overview

### 1.1 Problem

CyberArk Secrets Manager SaaS (Conjur Cloud) adoption is gated less by the product itself than by the configuration distance between "I want my workload to use Conjur" and "I have a configured authenticator, workloads with the correct identity claims, grants to the right safes, and a correctly configured integration on the consuming platform."

Today this gap is bridged by:
- Professional Services engagements
- Documentation that is comprehensive but fragmented across platforms
- Trial-and-error with opaque JWT claim mismatches
- Tribal knowledge about which claims to use for `token_app_property`

This slows deployment, increases support load, and produces inconsistent security postures across customers (some overly permissive because they keyed off `repository_owner` alone, others brittle because they keyed off `workflow_ref`).

The Secrets Manager SaaS v2 REST API significantly reduces the mechanical complexity here — authenticator creation is a single POST, and the auto-created `apps` / `operators` groups remove the need to hand-author most policy YAML. COT's role is to bridge the remaining gap: discovering what the customer's platform looks like, translating that into correctly-shaped API calls, and producing the matching consumer-side integration configuration.

### 1.2 Solution

**Conjur Onboarding Toolkit (COT)** — a CLI that for each supported platform:

1. **Discovers** the customer's platform identity configuration (orgs, repos/projects, OIDC issuer, JWKS URI, available claims).
2. **Inspects** a sample JWT to surface identity-bearing claims with security annotations.
3. **Generates** the set of Secrets Manager SaaS REST API calls (create authenticator, create workloads, add workloads to the authenticator's `apps` group), along with any supplementary policy YAML for resources the API does not cover, plus a pre-filled integration snippet for the consuming platform.
4. **Validates** and optionally **applies** the generated calls against a target Secrets Manager SaaS tenant.

COT has two operating modes:

- **Default (Express) mode:** one command runs end-to-end using opinionated best-practice defaults aligned with CyberArk Professional Services recommendations. Most customers use this.
- **Advanced (Interactive) mode:** the discover → inspect → generate steps can be run individually, with interactive claim selection and review, for customers who need non-standard configurations.

### 1.3 Supported platforms (initial scope)

| Platform | Identity mechanism | Priority |
|---|---|---|
| GitHub Actions | GitHub OIDC | P0 |
| GitLab CI/CD | GitLab OIDC | P0 |
| Jenkins | Conjur JWT plugin (self-issued) | P0 |
| Azure DevOps | Azure DevOps Workload Identity Federation | P1 |
| Terraform Cloud / HCP Terraform | TFC workload identity tokens | P1 |
| Ansible (AAP/AWX) | AAP OIDC where available, API key fallback | P2 |

### 1.4 Out of scope (v1)

- **Conjur Enterprise and Conjur OSS support.** v1 targets Secrets Manager SaaS (Conjur Cloud) exclusively, to take advantage of the v2 REST API's simpler provisioning surface. Support for Enterprise and OSS is deferred to a later release and will likely require falling back to policy-YAML-based provisioning for those targets.
- Hosted web UI — CLI only for v1.
- Secret provisioning itself — COT configures access; populating secret values is the customer's responsibility.
- Platforms not listed above (CircleCI, Bitbucket Pipelines, AWS CodeBuild, etc.) — considered for v2 based on demand.
- Secrets rotation or lifecycle management.
- Replacing the official Conjur CLI — COT wraps and complements it.

### 1.5 Non-goals

- COT does not replace the official platform-specific integration actions/plugins (`cyberark/conjur-action`, the Conjur Jenkins plugin, the `cyberark.conjur` Ansible collection, etc.). COT generates configuration that uses those official integrations.
- COT does not make security decisions on the customer's behalf that deviate from documented best practices without explicit opt-in.

---

## 2. Goals and success metrics

### 2.1 Goals

1. Reduce time-to-first-successful-secret-fetch for a new Conjur customer on a supported platform from days to under 30 minutes.
2. Reduce PS engagement hours spent on repetitive authenticator and policy setup by 50%.
3. Produce consistent, documented, reviewable policy artifacts that customers can commit to version control.
4. Make the security implications of claim selection explicit and visible, rather than buried in documentation.

### 2.2 Success metrics

- **Adoption:** 500 unique CLI installs in first 6 months post-GA.
- **Completion rate:** ≥80% of users who run `discover` also run `apply` (proxy for end-to-end success).
- **Time to first secret:** Median time from `install` to first successful secret retrieval < 30 minutes, measured via opt-in telemetry.
- **Policy reuse:** ≥60% of generated policies are committed to customer version control (measured via survey).
- **Support deflection:** Reduction in tickets tagged "authn-jwt-configuration" by 40% within 12 months of GA.

---

## 3. Personas

### 3.1 Primary: Platform / DevOps Engineer

- Has admin on their CI/CD platform (GitHub org owner, Jenkins admin, etc.).
- Has been given a Conjur account and appliance URL by their security team.
- Is comfortable on a terminal, has `gh`, `glab`, `az`, or equivalent CLI tools installed.
- Wants to get their first pipeline pulling a secret today, without reading 40 pages of docs.
- Typically uses **Express mode**.

### 3.2 Secondary: Security / IAM Engineer

- Owns the Conjur instance and authenticator policy.
- Reviews and approves what the Platform Engineer generates before it is loaded.
- Cares deeply about claim selection, scope of trust, and auditability.
- Typically uses **Advanced mode** or reviews Express-generated output before `apply`.

### 3.3 Tertiary: CyberArk Professional Services Engineer

- Runs COT during customer engagements to bootstrap setup quickly.
- Uses Advanced mode to customize for non-standard customer topologies.
- Provides feedback that drives default/recommendation improvements.

---

## 4. User experience

### 4.1 Installation

Distributed as:
- Homebrew tap: `brew install cyberark/tap/conjur-onboard`
- Standalone binaries for Linux/macOS/Windows on GitHub Releases
- Docker image: `cyberark/conjur-onboard:latest`

No runtime dependencies beyond the relevant platform CLI for discovery (e.g., `gh` for GitHub). COT will detect and prompt if the dependency is missing.

### 4.2 Conjur target and authentication

COT has three distinct authentication concerns, which the spec keeps separate to avoid the confusion that currently exists in the field:

**(a) The Conjur target.** v1 targets **Secrets Manager SaaS (Conjur Cloud)** exclusively. The target is identified by tenant subdomain: `https://<subdomain>.secretsmgr.cyberark.cloud`. This is configured via `--tenant <subdomain>` or `CONJUR_TENANT` environment variable.

**(b) How COT authenticates to the SaaS tenant (tool auth).** COT itself must authenticate to the tenant to call the authenticator and group-membership APIs. v1 supports:

- **CyberArk Identity user session**, via the tenant's authenticate-user endpoint. This is the expected path for Platform and Security engineers running COT interactively.
- **API key for a bootstrap admin workload**, for CI/automation use of COT itself.

Tool-auth tokens are cached in the OS keychain for the session's duration and never written to disk in plaintext. The token is sent as `Authorization: Token token="<token>"` on every API call, per the documented SaaS API contract.

The authenticated tool-auth identity must have `create` privileges on the `authn-<type>` policy branch (for example, membership in `Authn_Admins`) to create authenticators, and `update` on the relevant group policies to add members. COT detects permission failures at each step and prints a specific message about which policy privilege is missing, rather than surfacing a bare HTTP 403.

**(c) How workloads authenticate to Conjur (workload auth).** This is separate from (b) — it is the identity mechanism the customer's CI/CD workloads use to retrieve secrets. v1 supports two families of workload auth:

**JWT family.** The workload presents a JWT (issued by the CI/CD platform or by a JWT-capable system like the Jenkins Conjur plugin) and Conjur validates it against the configured authenticator. Authenticator `type` is `jwt` in the SaaS API.

**Cloud IAM family.** The workload presents its underlying cloud identity (AWS IAM role via STS, GCP service account identity, Azure managed identity) and Conjur validates it by calling the respective cloud provider. No JWT is minted or signed by the workload or the platform. Authenticator `type` is `aws_iam`, `gcp`, or `azure` in the SaaS API. This is the right choice when the workload is running on cloud infrastructure and already has an IAM identity attached — common for Terraform runners and Ansible control nodes on EC2, GCE, or Azure VMs.

| Workload auth method | API `type` | Applicable platforms | When to use |
|---|---|---|---|
| `jwt` | `jwt` | GitHub Actions, GitLab CI, Jenkins, Azure DevOps, Terraform Cloud, Ansible (AAP with OIDC) | Platform issues OIDC tokens the workload can present |
| `aws-iam` | `aws_iam` | Terraform, Ansible | Workload runs on EC2 / ECS / EKS with an attached IAM role |
| `gcp-iam` | `gcp` *(singleton — see note)* | Terraform, Ansible | Workload runs on GCE / GKE / Cloud Run with a service account |
| `azure-managed-identity` | `azure` *(confirm API type value — see §11.9)* | Terraform, Ansible | Workload runs on an Azure VM, AKS, or Function with a managed identity |
| `certificate` | `certificate` | *(not surfaced as a platform-level option in v1; available via advanced mode)* | mTLS-based workload auth |

The `--workload-auth` flag controls both which authenticator is created by COT and which consumer-side integration snippet is generated.

- For **GitHub, GitLab, Jenkins, Azure DevOps**: only `jwt` is supported. The flag is ignored.
- For **Terraform and Ansible**: `jwt`, `aws-iam`, `gcp-iam`, and `azure-managed-identity` are all valid. The flag is required in non-interactive mode and prompted interactively. The choice has real implications documented in §6.5 and §6.6 — JWT requires the platform to issue tokens (Terraform Cloud does; OSS Terraform does not), while cloud IAM requires the workload to be running on cloud infrastructure with an appropriate identity attached.

**Cloud IAM authenticator shape differs fundamentally from JWT.** The v2 API body for `aws_iam`, `gcp`, and `azure` types does not use `data.jwks_uri`, `data.issuer`, `data.audience`, or `data.identity` — those are JWT-specific. Cloud IAM authenticators are created with just `type`, `name`, and `enabled`. The workload-to-identity binding happens on the workload resource itself, via **annotations** matching the workload's cloud identity attributes:

- `aws_iam`: workload ID is the AWS IAM role ARN path (e.g., `data/<account-id>/<role-name>`). No annotations required; the role ARN *is* the identity.
- `gcp`: annotations like `authn-gcp/project-id`, `authn-gcp/service-account-email`, `authn-gcp/service-account-id`, `authn-gcp/instance-name`. Combination is AND-correlated. Minimum: two annotations.
- `azure`: annotations `authn-azure/subscription-id` and `authn-azure/resource-group` are required; optional `authn-azure/user-assigned-identity` or `authn-azure/system-assigned-identity` for per-workload granularity.

The GCP authenticator is a **singleton** — only one may exist per tenant, and its name is always `conjur/authn-gcp` with no suffix. COT must detect an existing GCP authenticator and skip creation (treat as success) rather than attempting to create a second one. AWS IAM and GCP are documented as "predefined" authenticators in Secrets Manager SaaS, meaning they may be auto-provisioned by the tenant — COT's discovery must check authenticator existence before attempting creation for these types.

The shared provisioning core in §5.8 must branch on authenticator type: JWT authenticators use claim-based identity binding (`token_app_property` + `identity_path`), cloud IAM authenticators use annotation-based identity binding on the workload resource itself.

**Platform auth.** Separately from Conjur auth, COT reuses existing CLI auth where possible (`gh auth status`, `glab auth status`, `az account show`) to discover platform configuration. Fall back to prompting for a PAT. Platform tokens may be cached in OS keychain when explicitly requested.

### 4.3 Command structure

```
conjur-onboard <platform> <command> [flags]

Platforms:
  github, gitlab, jenkins, azure-devops, terraform, ansible

Commands:
  express     Run discover → inspect → generate end-to-end with defaults (most common)
  discover    Gather platform identity configuration
  inspect     Decode and analyze a sample JWT
  generate    Produce Conjur policy, API script, and integration snippet
  validate    Dry-run generated policy against a Conjur instance
  apply       Load policy to Conjur
  rollback    Reverse a prior apply using generated rollback script

Global flags:
  --work-dir       Directory for generated artifacts (default: ./conjur-onboard-<platform>-<timestamp>)
  --config         Path to saved configuration file
  --non-interactive  Suppress prompts; fail on missing values
  --verbose, -v    Verbose logging
  --dry-run        Print actions without executing
```

### 4.4 The two modes

**Express mode (default path, ~90% of users):**

```
$ conjur-onboard github express --org acme-corp --conjur-url https://conjur.acme-corp.com
```

Runs discovery, applies recommended claim defaults (documented per platform in §6), generates all artifacts, prints a summary, and prompts once: "Review the generated policy at `./conjur-onboard-github-20260420/policy/` then run `conjur-onboard github apply` to load it. Proceed to apply now? [y/N]"

Recommended defaults are visible, not hidden — the summary explicitly states "Using recommended claims: `repository`, `environment`. These bind workload identity to the repo and deployment environment. To customize, re-run with `conjur-onboard github inspect --interactive`."

**Advanced mode (power users, non-standard setups):**

```
$ conjur-onboard github discover --org acme-corp
$ conjur-onboard github inspect --interactive
$ conjur-onboard github generate --identity-claims repository,workflow_ref,environment
$ conjur-onboard github validate --conjur-url https://conjur.acme-corp.com
$ conjur-onboard github apply
```

Each step writes state to the working directory so later steps pick up where earlier ones left off.

### 4.5 Output structure

Every run produces a self-contained working directory:

```
conjur-onboard-<platform>-<timestamp>/
├── config.yml                    # Persisted inputs + selected options
├── discovery.json                # Raw discovery output
├── claims-analysis.json          # Claim selection with rationale
├── api/
│   ├── 01-create-authenticator.json    # Body for POST /api/authenticators
│   ├── 02-workloads.yml                # Policy YAML for workload creation (see note below)
│   ├── 03-add-group-members.jsonl      # One JSON object per POST /api/groups/.../members call
│   └── plan.json                       # Ordered manifest of all API calls with metadata
├── scripts/
│   ├── apply.sh                  # Executes the API plan in order; idempotent
│   └── rollback.sh               # Inverse operations (DELETE member, delete authenticator)
├── integration/
│   ├── example-config.<ext>      # Platform-specific integration snippet
│   └── README.md                 # How to adapt it
└── NEXT_STEPS.md                 # Human-readable walkthrough, the actual deliverable
```

**Note on workload creation:** the v2 REST API documentation covers authenticator creation and group membership but does not (at the time of this PRD) document a dedicated "create workload" endpoint separate from policy loading. Workload resources are therefore created via the existing policy load endpoint (`POST /policies/...`), with a minimal YAML body that creates the workload resource under the policy branch specified in the authenticator's `identity_path`. This is documented as open question §11.7 and the implementer should verify current API coverage before committing to this approach — if a dedicated workload-creation endpoint exists, COT should use it.

---

## 5. Functional requirements

### 5.1 Discovery

Each platform adapter must produce a normalized `discovery.json` containing at minimum:

- Platform identifier and version (where available)
- Organization / tenant identifier (both human-readable and internal ID where distinct)
- OIDC issuer URL
- JWKS URI
- List of workload-capable entities (repos, projects, pipelines, workspaces) with associated metadata (environments, protected refs, service connections, etc.)
- Existing OIDC customizations (e.g., GitHub's `sub` claim customization at org level)
- Any authenticator-relevant tenant-wide settings

Discovery must be idempotent and must not mutate the platform state (no resources created on the customer's platform).

**Exception:** GitHub live-mode claim inspection may optionally open a PR adding an inspector workflow; this is explicitly opt-in and is part of `inspect`, not `discover`.

### 5.2 Claim inspection

For each platform, COT must:

1. Obtain or construct a representative JWT (synthetic from documented schema, or live from the platform where feasible).
2. Decode and display each claim with:
   - The claim name and value
   - A classification: `identity-strong`, `identity-weak`, `scope`, `metadata`, `ephemeral`
   - A human-readable explanation of what selecting it for identity means
   - Security implications (e.g., "`repository_owner` alone grants any repo in the org")
3. Present the recommended default selection with rationale.
4. In interactive mode, allow the user to modify the selection and warn on risky combinations.
5. Persist the selection to `claims-analysis.json`.

### 5.3 Authenticator and workload provisioning generation

The generator produces an ordered **plan** of API calls and (where unavoidable) supplementary policy YAML. The plan is written to `api/plan.json` as a manifest, and each operation's body is written as a standalone artifact alongside so it can be reviewed independently.

**5.3.1 Authenticator body generation.** For the target authenticator, the generator produces a JSON body for `POST https://<subdomain>.secretsmgr.cyberark.cloud/api/authenticators` matching the documented v2 contract. The body must:

The per-`type` body contract is:

**5.3.1.a JWT authenticator (`type: jwt`)** — used by GitHub, GitLab, Jenkins, Azure DevOps, Terraform Cloud, and Ansible with AAP OIDC.

- Set `subtype` to the matching platform subtype where one exists in the documented list (`github_actions`, `gitlab`, `jenkins`). For Azure DevOps, Terraform, and Ansible, `subtype` is omitted.
- Set `name` to a deterministic value derived from the platform and org/tenant identifier (e.g., `github-acme-corp`), sanitized to allowed characters (`A-Z a-z 0-9 - _`).
- Set `enabled` to `true` by default; `--create-disabled` flag overrides for customers who want to stage the authenticator before going live.
- Populate `data.jwks_uri`, `data.issuer`, and `data.audience` from discovery.
- Populate `data.identity.token_app_property` with the selected identity claim and `data.identity.identity_path` with the workload branch (e.g., `data/<platform>-apps`).
- Populate `data.identity.enforced_claims` with any additional claims the user selected as required (beyond `token_app_property`).
- Populate `data.identity.claim_aliases` where aliases improve readability (e.g., mapping `workspace` to `environment` for GitLab).

**5.3.1.b AWS IAM authenticator (`type: aws_iam`)** — used by Terraform and Ansible when `--workload-auth aws-iam`.

- Set `name` to a deterministic value (e.g., `aws-iam-default` or customer-supplied via `--name-suffix`). AWS IAM is documented as a predefined authenticator, so the default name convention may already exist on the tenant — generator must handle HTTP 409 as success.
- Set `enabled` to `true`.
- No `data` object is populated — AWS IAM authenticator has no authenticator-level configuration variables.

**5.3.1.c GCP authenticator (`type: gcp`)** — used by Terraform and Ansible when `--workload-auth gcp-iam`.

- GCP is a **singleton** per tenant; `name` is ignored by the API (authenticator is always `conjur/authn-gcp`). Generator attempts creation; if HTTP 409 (already exists), treat as success and proceed.
- Set `enabled` to `true`.
- No `data` object is populated.

**5.3.1.d Azure authenticator (`type: azure`)** — used by Terraform and Ansible when `--workload-auth azure-managed-identity`. The API `type` value for Azure is not present in the create-authenticator doc snapshot used to draft this PRD; see §11.9 for the verification open question.

- Set `name` to a deterministic value (e.g., `azure-<tenant-guid-short>`).
- Set `enabled` to `true`.
- Azure authenticator may require configuration variables (subscription ID / Azure AD tenant ID) depending on API version — verify against current docs at implementation time.

**5.3.2 Workload resources.**

The API auto-creates the `conjur/<authn-type>/<authn-name>/apps` group when the authenticator is created; COT does not generate this group. COT must create the actual workload resources at the path appropriate to the authenticator type.

**For JWT authenticators**, workloads live at `<identity_path>/<workload-id>`. Workload `id` matches the `token_app_property` claim value. No annotations required by the authenticator itself, but the generator may add annotations for operator readability.

**For cloud IAM authenticators**, workloads require identity-proving annotations:

- `aws_iam`: workload `id` encodes the AWS account ID and role name (convention: `data/<account-id>/<role-name>`). No annotations required — the path itself is the identity. The IAM authenticator validates by calling STS against the role ARN derived from the workload's path.
- `gcp`: workload annotations must include at least two of `authn-gcp/project-id`, `authn-gcp/service-account-id`, `authn-gcp/service-account-email`, `authn-gcp/instance-name` (GCE only). AND-correlated at validation time.
- `azure`: workload annotations must include `authn-azure/subscription-id` and `authn-azure/resource-group`, plus optionally `authn-azure/user-assigned-identity` or `authn-azure/system-assigned-identity`.

Until a dedicated workload-creation REST endpoint is confirmed (see §11.7), workload creation uses the policy load endpoint with a minimal YAML body per branch. Example for GCP:

```yaml
- !policy
  id: data/gcp-apps
  body:
    - !host
      id: tf-runner-prod
      annotations:
        authn-gcp/project-id: acme-prod-12345
        authn-gcp/service-account-email: terraform-runner@acme-prod.iam.gserviceaccount.com
    # one per workload
```

This YAML is emitted to `api/02-workloads.yml`. The shared core is responsible for producing the correct annotation shape based on authenticator type.

**5.3.3 Group membership additions.** For each workload, the generator emits one JSON body for `POST /api/groups/{identifier}/members` against the authenticator's auto-created `apps` group. The group identifier is computed as:

```
conjur/authn-<type>/<authn-name>/apps
```

URL-encoded when placed in the path. The body references the workload by its full path (e.g., `data/github-apps/acme-corp/api-service-production`) and `kind: workload`.

Each membership addition is emitted as a line in `api/03-add-group-members.jsonl` along with a metadata record in `api/plan.json` that records the target group, method, and expected response code.

**5.3.4 Determinism.** All generated JSON bodies and YAML files must be deterministic — the same inputs produce byte-identical outputs. Map keys are sorted; JSON is pretty-printed with a fixed indent; line endings are LF. This is required for diff review and version control.

**5.3.5 What is NOT generated.** COT does not generate safe grants (the assignment of workloads or groups to specific secret safes). Safe grants are a security decision that varies per customer environment and must be done as a reviewed step documented in `NEXT_STEPS.md`. COT's output includes the group reference the customer should grant — typically the authenticator's `apps` group or a customer-created subgroup — but does not execute the grant itself.

### 5.4 Integration snippet generation

Each platform adapter must produce a ready-to-paste integration configuration for the consuming platform's official Conjur integration. The snippet matches the selected workload auth method (§4.2):

- GitHub Actions (`jwt`): workflow YAML using `cyberark/conjur-action`
- GitLab CI (`jwt`): `.gitlab-ci.yml` job using the Conjur integration
- Jenkins (`jwt`): Jenkinsfile fragment using the Conjur credentials provider
- Azure DevOps (`jwt`): pipeline YAML with the Conjur task and service connection definition
- Terraform (`jwt`): provider block using JWT auth method pointing at the tenant's authenticate endpoint
- Terraform (`conjur-cloud-auth`): provider block using Conjur Cloud service token auth
- Ansible (`jwt`): playbook fragment using the `cyberark.conjur` collection's lookup plugin, configured for JWT
- Ansible (`conjur-cloud-auth`): same collection, configured for Conjur Cloud service token auth

Snippets must include comments indicating customer-specific values and a paired README explaining how to adapt them. Snippets must reference the correct tenant URL format (`https://<subdomain>.secretsmgr.cyberark.cloud`) and must use the documented API version header (`Accept: application/x.secretsmgr.v2beta+json`) where applicable.

### 5.5 Validation

`validate` must:

- Connect to the target SaaS tenant using the tool-auth token from §4.2.
- For each API call in `api/plan.json`, perform a pre-flight check appropriate to the call:
  - Authenticator: `GET /api/authenticators/<name>` to detect conflict; `POST` with a dry-run flag if supported by the API, otherwise just the conflict check.
  - Group membership: verify the group exists (`GET /api/groups/<identifier>`) and the workload path exists.
- Report conflicts, permission failures, or missing referenced resources.
- Exit non-zero on failure with a clear remediation hint (e.g., "Authenticator `github-acme-corp` already exists. Use `--name-suffix` or delete the existing authenticator.").

### 5.6 Apply

`apply` must:

- Require successful `validate` first (or `--skip-validate` with a warning).
- Execute the API plan in order: authenticator creation first, then workload policy load, then group-membership additions.
- For each call, record the request, response status, and response body to `apply-log.json`.
- Print a summary of resources created, with IDs returned by the API.
- On partial failure, stop at the first error and print the rollback command. Do not attempt to continue past a failed call.
- Be idempotent: re-running `apply` against an already-applied state must detect existing resources (HTTP 409 on authenticator creation, existing group memberships) and treat them as success with a "no change" note, rather than failing.

### 5.7 Rollback

`rollback` must:

- Read `apply-log.json` to know what was created.
- Produce inverse operations in reverse order:
  - `DELETE /api/groups/{identifier}/members/{kind}/{role-id}` for each added membership.
  - Workload policy deletions for each created workload.
  - `DELETE /api/authenticators/<name>` for the authenticator (endpoint to be confirmed — see §11.8).
- Require explicit `--confirm` before executing.
- Warn clearly that rollback deletes workloads and their audit history.
- Handle "already deleted" responses (HTTP 404) gracefully — they indicate prior manual cleanup, not an error.

### 5.8 Shared provisioning core

All six adapters must emit API plans via a shared internal library, not duplicate generation logic. Adapters are responsible for:
- Discovery
- Claim inspection
- The final integration snippet

The shared core is responsible for:
- Authenticator JSON body construction per §5.3.1
- Workload resource generation per §5.3.2
- Group membership body generation per §5.3.3
- Plan manifest construction
- Apply/validate/rollback execution against the SaaS API
- Deterministic output formatting

---

## 6. Platform-specific requirements

### 6.1 GitHub

**Discovery inputs:** org name, optionally a specific repo list.
**Auth:** `gh` CLI session or `GITHUB_TOKEN` with `repo` and `read:org` scopes.
**Discovery outputs:**
- Org metadata including enterprise tier
- Repo list with visibility, default branch, environments
- Whether org-level `sub` claim customization is configured

**OIDC config (derived, not asked):**
- Issuer: `https://token.actions.githubusercontent.com`
- JWKS URI: `https://token.actions.githubusercontent.com/.well-known/jwks`

**SaaS API body:**
- `type`: `jwt`
- `subtype`: `github_actions`
- `name`: `github-<org>` (sanitized)
- `data.identity.identity_path`: `data/github-apps/<org>`
- `data.identity.token_app_property`: `repository` (default) or customer selection
- `data.audience`: `conjur-cloud` (default; overridable via `--audience`)

**Recommended default identity claims:**
- `repository` (primary — set as `token_app_property`)
- `environment` (added to `enforced_claims` when present on discovery)

**Inspection modes:**
- `--mode synthetic` (default): construct expected claims from documented schema
- `--mode live`: create PR with inspector workflow (requires explicit opt-in)

**Integration snippet:** GitHub Actions workflow using `cyberark/conjur-action@v2` (or current GA version at generation time). Tenant URL is `https://<subdomain>.secretsmgr.cyberark.cloud`.

### 6.2 GitLab

**Discovery inputs:** GitLab URL (defaults to `https://gitlab.com`), group path, PAT or OAuth session.
**Auth:** `glab` CLI session or GitLab PAT with `read_api` scope.
**Discovery outputs:**
- Group and subgroup structure
- Projects under the group(s), recursively
- Environments per project, protected refs

**OIDC config:**
- Issuer: `<GITLAB_URL>`
- JWKS URI: `<GITLAB_URL>/oauth/discovery/keys`

**SaaS API body:**
- `type`: `jwt`
- `subtype`: `gitlab`
- `name`: `gitlab-<group-slug>` (sanitized)
- `data.identity.identity_path`: `data/gitlab-apps/<group-slug>`
- `data.identity.token_app_property`: `project_path`
- `data.audience`: `conjur-cloud` (default)

**Recommended default identity claims:**
- `project_path` (primary)
- `environment` (as enforced claim when present)

**Inspection modes:**
- `--mode synthetic` (default)
- `--mode live`: instructions for user to add a one-shot echo job to `.gitlab-ci.yml`, with the decoded token posted to an artifact.

**Integration snippet:** `.gitlab-ci.yml` job fragment using `CI_JOB_JWT_V2` with the Conjur integration.

### 6.3 Jenkins

**Discovery inputs:** Jenkins URL, API token.
**Auth:** Jenkins API token with read permissions on the relevant jobs/folders.
**Discovery outputs:**
- Jenkins version and Conjur JWT plugin version
- Folder and job structure
- Currently configured claim mappings in the JWT plugin
- Any custom claims the admin has added

**OIDC config (self-derived since Jenkins is the issuer):**
- Issuer: `<JENKINS_URL>`
- JWKS URI: `<JENKINS_URL>/jwtauth/conjur-jwk-set` (confirm against installed plugin version at runtime)

**SaaS API body:**
- `type`: `jwt`
- `subtype`: `jenkins`
- `name`: `jenkins-<jenkins-hostname-slug>` (sanitized)
- `data.identity.identity_path`: `data/jenkins-apps/<hostname-slug>`
- `data.identity.token_app_property`: `jenkins_name` (or customer selection — Jenkins identity is often composed, see §6.3 note)
- `data.audience`: `conjur-cloud` (default)

**Note on composite identity:** Jenkins workload identity often needs both `jenkins_parent_full_name` (folder path) and `jenkins_name` (job name). Since `token_app_property` is a single claim, the recommended pattern is to use `jenkins_name` as `token_app_property` and add `jenkins_parent_full_name` to `enforced_claims`, with workload names matching the job name under a folder-based `identity_path` branch. Claim aliases may be used to simplify this.

**Recommended default identity claims:**
- `jenkins_name` (primary — set as `token_app_property`)
- `jenkins_parent_full_name` (added to `enforced_claims`)

**Inspection modes:**
- The plugin can mint sample tokens directly; live mode is the default here and is not invasive.

**Integration snippet:** Jenkinsfile fragment using `withCredentials` and the Conjur credentials provider, pre-configured with the generated workload identity.

### 6.4 Azure DevOps

**Discovery inputs:** organization URL, PAT with project-read scope.
**Auth:** `az devops` CLI session or PAT.
**Discovery outputs:**
- Org ID (GUID — fetched, not asked)
- Projects
- Service connections per project (critical — this is the workload identity boundary)
- Pipelines and their linked service connections

**OIDC config:**
- Issuer: `https://vstoken.dev.azure.com/<org-id-guid>`
- JWKS URI: `https://vstoken.dev.azure.com/<org-id-guid>/.well-known/jwks`

**SaaS API body:**
- `type`: `jwt`
- `subtype`: *(omitted — not in documented subtype list)*
- `name`: `azure-devops-<org-slug>`
- `data.identity.identity_path`: `data/azure-devops-apps/<org-slug>`
- `data.identity.token_app_property`: `sub`
- `data.audience`: `conjur-cloud` (default)

**Recommended default identity claims:**
- `sub` (`sc://<org>/<project>/<service-connection-name>`) — primary

**Special considerations:**
- Workload identity is bound to service connections, not pipelines. The inspector must surface this clearly because it is a common misconception.
- The tool should detect projects without configured WIF service connections and note them.

**Integration snippet:** pipeline YAML with generic service connection definition + Conjur task.

### 6.5 Terraform

Terraform is the first platform where `--workload-auth` is a mandatory up-front decision, because the provider supports four distinct authentication modes and the choice depends on *where the Terraform runner executes*, not on the Terraform code itself. COT prompts (or requires the flag non-interactively) before any discovery begins, and the discovery and generation flow branches from there.

**Supported workload auth values for Terraform:**

| Flag value | Applicable when | Runner must be on |
|---|---|---|
| `jwt` | Using Terraform Cloud / HCP Terraform or Terraform Enterprise | TFC/TFE remote runners or agents |
| `aws-iam` | Using OSS or Enterprise Terraform runners on AWS | EC2, ECS, EKS with an attached IAM role |
| `gcp-iam` | OSS or Enterprise Terraform runners on GCP | GCE, GKE, Cloud Run with a service account |
| `azure-managed-identity` | OSS or Enterprise Terraform runners on Azure | Azure VM, AKS with a managed identity |

**Plain OSS Terraform with no cloud identity** is not supported — the tool prints a helpful message explaining that the runner needs *some* verifiable identity (either platform-issued JWT or cloud IAM) and points at alternatives.

---

**6.5.a Terraform with `--workload-auth jwt`**

**Discovery inputs:** Terraform Cloud / Enterprise URL (default `https://app.terraform.io`), org name, TFC API token.
**Auth:** TFC API token with read scope on the target org.
**Discovery outputs:** projects, workspaces, workspace execution modes.

**OIDC config:**
- Issuer: `<TFC_URL>` (or customer's TFE URL)
- JWKS URI: `<TFC_URL>/.well-known/jwks`

**SaaS API body:** JWT authenticator per §5.3.1.a.
- `name`: `terraform-<org-slug>`
- `subtype`: omitted
- `data.identity.token_app_property`: `terraform_full_workspace`
- `data.identity.identity_path`: `data/terraform-apps/<org-slug>`
- `data.identity.enforced_claims`: `["terraform_run_phase"]` (mandatory because plan and apply have different blast radius)

**Recommended default identity claims:**
- `terraform_full_workspace` (primary — set as `token_app_property`)
- `terraform_run_phase` (enforced claim — plan vs apply)

**Integration snippet:** Terraform provider block:
```hcl
provider "conjur" {
  appliance_url = "https://<subdomain>.secretsmgr.cyberark.cloud/api"
  account       = "conjur"
  authn_type    = "jwt"
  service_id    = "terraform-<org-slug>"
  host_id       = "host/data/terraform-apps/<org-slug>/<workspace-name>"
}
```
Plus TFC workspace variable set definitions for the JWT pass-through.

---

**6.5.b Terraform with `--workload-auth aws-iam`**

**Discovery inputs:** AWS account IDs, IAM role names/ARNs for the Terraform runners.
- Auto-discovered via `aws` CLI if available: `aws sts get-caller-identity` to confirm runner identity, `aws iam list-roles --path-prefix` or explicit `--roles` flag for batch input.
- In interactive mode, the user provides or confirms the list of role ARNs used by Terraform runners.

**SaaS API body:** AWS IAM authenticator per §5.3.1.b.
- `name`: `aws-iam-default` (or `--name-suffix`)
- No JWKS, issuer, audience, or identity claims — cloud IAM doesn't use them.

**Workload resources:** one per IAM role, at `data/<account-id>/<role-name>`, no annotations (path encodes identity).

**Integration snippet:** Terraform provider block:
```hcl
provider "conjur" {
  appliance_url = "https://<subdomain>.secretsmgr.cyberark.cloud/api"
  account       = "conjur"
  authn_type    = "iam"
  service_id    = "default"
  host_id       = "host/data/<account-id>/<role-name>"
}
```
No JWT variable needed. Includes a note in `NEXT_STEPS.md` explaining that the runner must have AWS credentials available via standard providers (instance profile, environment variables, etc.).

---

**6.5.c Terraform with `--workload-auth gcp-iam`**

**Discovery inputs:** GCP project IDs and service account emails used by Terraform runners.
- Auto-discovered via `gcloud` CLI if available: `gcloud iam service-accounts list --project=<project>`.
- Or explicit `--service-accounts` flag / interactive prompt.

**SaaS API body:** GCP authenticator per §5.3.1.c (singleton, no name needed). If already exists on tenant, skip creation and proceed.

**Workload resources:** one per service account, with annotations:
```yaml
- !host
  id: tf-runner-<safe-name>
  annotations:
    authn-gcp/project-id: <project-id>
    authn-gcp/service-account-email: <sa-email>
```

**Integration snippet:** Terraform provider block:
```hcl
provider "conjur" {
  appliance_url = "https://<subdomain>.secretsmgr.cyberark.cloud/api"
  account       = "conjur"
  authn_type    = "gcp"
  host_id       = "host/data/gcp-apps/tf-runner-<safe-name>"
}
```
No `service_id` because GCP authenticator is singleton.

---

**6.5.d Terraform with `--workload-auth azure-managed-identity`**

**Discovery inputs:** Azure subscription IDs, resource group names, and managed identity details.
- Auto-discovered via `az` CLI: `az account list`, `az identity list --resource-group <rg>`.
- Or explicit flags / interactive prompts.

**SaaS API body:** Azure authenticator per §5.3.1.d.

**Workload resources:** annotations per §5.3.2 (`authn-azure/subscription-id`, `authn-azure/resource-group`, plus optional managed identity for per-workload granularity).

**Integration snippet:** Terraform provider block:
```hcl
provider "conjur" {
  appliance_url = "https://<subdomain>.secretsmgr.cyberark.cloud/api"
  account       = "conjur"
  authn_type    = "azure"
  service_id    = "<authenticator-name>"
  host_id       = "host/data/azure-apps/<workload-id>"
}
```

---

**Shared for all Terraform variants:**
- `NEXT_STEPS.md` includes a section explaining the chosen auth method's prerequisites (IAM role trust policy, GCP service account bindings, Azure managed identity assignment).
- The generated provider block is written to `integration/provider.tf` so it can be directly copied into the customer's Terraform configuration.
- A companion `integration/tfc-variable-set.json` is produced for the `jwt` variant, describing the TFC variable set to configure.

### 6.6 Ansible

Like Terraform, Ansible requires `--workload-auth` up front because the right answer depends on where the Ansible control node runs.

**Supported workload auth values for Ansible:**

| Flag value | Applicable when | Control node must be on |
|---|---|---|
| `jwt` | Using AAP (Ansible Automation Platform) with OIDC-capable version | AAP execution environments |
| `aws-iam` | Ansible control node on AWS | EC2 with attached IAM role |
| `gcp-iam` | Ansible control node on GCP | GCE with service account |
| `azure-managed-identity` | Ansible control node on Azure | Azure VM with managed identity |

**Plain Ansible from a workstation** is not supported for first-class integration — the tool explains that retrieving secrets from Conjur from a human-operated workstation should use the CyberArk Identity user-auth flow directly, not an automated workload identity.

---

**6.6.a Ansible with `--workload-auth jwt` (AAP)**

**Discovery inputs:** AAP URL, OAuth2 or API token.
**Discovery outputs:** AAP version (to verify OIDC support), organizations, job templates.

**OIDC config:** derived from AAP settings if OIDC token issuance is enabled on the instance. Generator must verify minimum AAP version supporting OIDC and exit with a helpful message if not met.

**SaaS API body:** JWT authenticator per §5.3.1.a.
- `name`: `ansible-aap-<org-slug>`
- `subtype`: omitted
- `data.identity.token_app_property`: job-template-identifying claim (verify claim names against installed AAP version)
- `data.identity.identity_path`: `data/ansible-apps/<org-slug>`

**Integration snippet:** Playbook fragment using the `cyberark.conjur` collection with JWT connection configuration.

---

**6.6.b–d Ansible with cloud IAM workload auth (`aws-iam`, `gcp-iam`, `azure-managed-identity`)**

These variants follow the same shape as the corresponding Terraform variants (§6.5.b–d). Discovery inputs, API body generation, and workload resource creation are identical — the only difference is the integration snippet, which is a playbook fragment rather than a Terraform provider block.

**Integration snippet (AWS example):**
```yaml
- hosts: localhost
  vars:
    ansible_connection: local
  tasks:
    - name: Fetch secret from Conjur
      set_fact:
        db_password: "{{ lookup('cyberark.conjur.conjur_variable',
                                'data/vault/db-safe/db/password',
                                config_file='./conjur.conf') }}"
```

With a generated `conjur.conf`:
```ini
[default]
appliance_url = https://<subdomain>.secretsmgr.cyberark.cloud/api
account = conjur
authn_type = iam
host_id = host/data/<account-id>/<role-name>
```

Analogous configurations are generated for GCP and Azure variants using the appropriate `authn_type` and host_id paths.

---

**Shared for all Ansible variants:**
- `NEXT_STEPS.md` includes collection installation instructions: `ansible-galaxy collection install cyberark.conjur`.
- The generated config file is written to `integration/conjur.conf` with clear annotations on customer-specific values.
- A minimal test playbook is generated in `integration/test-connection.yml` that the customer can run immediately to verify end-to-end connectivity.

---

---

## 7. User stories and acceptance criteria

### Epic A: Core CLI and shared infrastructure

#### A1. CLI installation and discovery of platform adapters

**As a** Platform Engineer
**I want** to install COT with a single command on my OS
**So that** I can begin onboarding without complex setup

**Acceptance criteria:**
- Given a macOS user, when they run `brew install cyberark/tap/conjur-onboard`, then the `conjur-onboard` binary is installed and `conjur-onboard --version` prints a version.
- Given a Linux or Windows user, when they download the release binary for their platform, then the binary runs without requiring additional runtime installation.
- Given a user with Docker, when they run `docker run --rm cyberark/conjur-onboard:latest --help`, then the help text is printed.
- When the user runs `conjur-onboard --help`, all six platform adapters are listed.

#### A2. Express mode end-to-end run

**As a** Platform Engineer
**I want** a single command that runs the full discover → inspect → generate flow with best-practice defaults
**So that** I do not have to learn the internal step sequence

**Acceptance criteria:**
- Given valid platform credentials and a Secrets Manager SaaS tenant subdomain, when the user runs `conjur-onboard <platform> express --tenant <subdomain>`, then within a single command run: discovery completes, recommended claims or cloud identity attributes are selected, and API call bodies + integration artifacts are generated in the working directory.
- For platforms where `--workload-auth` is required (Terraform, Ansible), Express mode prompts interactively if the flag is not supplied; in `--non-interactive` mode, absence of `--workload-auth` is an error with a message listing valid values.
- The final output prints a plain-language summary including: platform, org/tenant identifier, authenticator type and name, number of workloads generated, identity mechanism (claims used for JWT, or annotation pattern for cloud IAM), and next steps.
- The summary explicitly labels defaults as "recommended" and points to the command to re-run in advanced mode.
- If any step fails, the command exits non-zero with a remediation hint, and no partial artifacts are left in an ambiguous state (either complete or clearly marked incomplete).

#### A3. Working directory and idempotent re-runs

**As a** Platform Engineer
**I want** all generated artifacts written to a single self-contained directory
**So that** I can commit them, review them, and re-run safely

**Acceptance criteria:**
- Every command writes to a working directory at `./conjur-onboard-<platform>-<timestamp>/` by default, overridable with `--work-dir`.
- When `--work-dir` points at an existing COT working directory, subsequent commands resume from prior state instead of starting over.
- Running `generate` twice with identical inputs produces byte-identical outputs in `api/` and `integration/`.
- A `.gitignore` is written to the working directory excluding anything containing live secrets (e.g., retrieved tokens or cached auth) while permitting API bodies, integration files, and `NEXT_STEPS.md` to be committed.

#### A4. Shared provisioning core library

**As a** COT maintainer
**I want** a single shared library used by all adapters for authenticator body generation, workload creation, and API execution
**So that** bug fixes apply everywhere and output is consistent

**Acceptance criteria:**
- Authenticator JSON body construction, workload YAML generation, group membership body generation, and API execution are implemented in one internal module, consumed by all six adapters.
- The module correctly branches by authenticator type (`jwt`, `aws_iam`, `gcp`, `azure`, `certificate`) — JWT bodies populate the `data.identity` object; cloud IAM bodies do not.
- Unit tests verify each authenticator type produces a body matching the documented v2 API contract and round-trips through the API schema without validation errors.
- A change to the JWT body template requires modifying only one file.

#### A5. Tenant authentication for COT itself

**As a** Platform Engineer
**I want** COT to authenticate to my Secrets Manager SaaS tenant using my existing CyberArk Identity session
**So that** I don't have to manage a separate credential for this tool

**Acceptance criteria:**
- Given a user with an active CyberArk Identity session, when they first run any command that requires tenant auth, COT uses an OAuth2 authorization-code flow to obtain a tenant token without requiring them to paste credentials.
- The tenant token is cached in the OS keychain for the duration of the session.
- `--api-key` flag with an API key ID and secret is supported for CI/automation use, with the secret read from `CONJUR_API_KEY` environment variable (never from a command-line argument).
- When a tool-auth token lacks required privileges for a specific API call (e.g., `create` on the authenticator policy), the error message names the missing privilege and the group membership that would grant it (e.g., "Your identity needs `create` on `conjur/authn-jwt`, typically via membership in `Authn_Admins`.").

### Epic B: GitHub adapter (P0)

#### B1. GitHub discovery

**As a** Platform Engineer with admin on a GitHub org
**I want** COT to enumerate my repos and detect OIDC configuration automatically
**So that** I don't have to hand-list repos or look up issuer URLs

**Acceptance criteria:**
- Given a user authenticated to `gh` with `repo` and `read:org` scopes, when they run `conjur-onboard github discover --org <org>`, then `discovery.json` is written containing all non-archived repos, each repo's environments, and the OIDC issuer and JWKS URI.
- If the user's `gh` session lacks required scopes, the command exits with a message naming the missing scopes and the `gh auth refresh` command to fix them.
- If `gh` is not installed, the command prints installation instructions and accepts `--token` as an alternative.
- If org-level `sub` claim customization is configured, it is detected and noted in `discovery.json`.

#### B2. GitHub claim inspection (synthetic)

**As a** Security Engineer
**I want** to see the claims that would be in a GitHub OIDC token for my repo, with annotations
**So that** I can make an informed identity-binding decision

**Acceptance criteria:**
- `conjur-onboard github inspect --mode synthetic --repo <org>/<repo>` prints the expected decoded JWT with every claim annotated.
- Each identity-bearing claim has a classification and a one-line security explanation.
- The recommended default selection (`repository`, `environment`) is marked with rationale.
- Selecting a risky combination in interactive mode produces a clear warning (e.g., selecting only `repository_owner` prints a warning about org-wide impersonation).

#### B3. GitHub claim inspection (live)

**As a** Platform Engineer
**I want** optionally to inspect a real GitHub OIDC token from one of my repos
**So that** I can verify the claim values match my expectations before generating policy

**Acceptance criteria:**
- `conjur-onboard github inspect --mode live --repo <org>/<repo>` opens a PR to the target repo adding `.github/workflows/conjur-onboard-inspect.yml`.
- The PR description explains what the workflow does and invites a reviewer.
- After the workflow runs once on PR merge or manual dispatch, its artifact contains the decoded token, which COT reads and displays.
- The inspector workflow is a single file, contains no secrets, and is removable via a follow-up cleanup command.

#### B4. GitHub authenticator and workload provisioning

**As a** Platform Engineer
**I want** COT to generate the SaaS API calls that create my GitHub authenticator and workloads
**So that** I can apply them to my tenant without hand-crafting JSON

**Acceptance criteria:**
- Generated `api/01-create-authenticator.json` is a valid body for `POST /api/authenticators` with `type: jwt`, `subtype: github_actions`, a sanitized `name` derived from the org, and a `data.identity` object containing `token_app_property`, `identity_path`, and any enforced claims.
- Generated `api/02-workloads.yml` is a policy YAML fragment creating one workload per repo (and per environment where applicable) at the authenticator's `identity_path`.
- Generated `api/03-add-group-members.jsonl` contains one JSON line per workload, each suitable as a body for `POST /api/groups/{url-encoded-apps-group-id}/members` with `kind: workload` and the workload's full path as `id`.
- `api/plan.json` lists all calls in execution order with method, path, and expected success response code per call.
- When `apply` runs successfully, the authenticator, workloads, and group memberships all exist on the tenant, verifiable via GET calls on each resource.

#### B5. GitHub integration snippet

**As a** Platform Engineer
**I want** a ready-to-paste workflow YAML using the official GitHub Action
**So that** I can immediately consume Conjur secrets

**Acceptance criteria:**
- `integration/example-deploy.yml` is a valid GitHub Actions workflow using the current GA version of `cyberark/conjur-action`.
- It includes the required `permissions: id-token: write` block.
- It references the correct tenant URL (`https://<subdomain>.secretsmgr.cyberark.cloud`), authenticator service-id (`github-<org>`), and `host-id` matching a workload created by B4.
- A companion `README.md` explains each field and how to customize the secret paths.

### Epic C: GitLab adapter (P0)

#### C1. GitLab discovery

**As a** Platform Engineer with admin on a GitLab group
**I want** COT to enumerate my projects recursively and detect OIDC configuration
**So that** I can onboard a whole group at once

**Acceptance criteria:**
- Given a GitLab PAT with `read_api`, when the user runs `conjur-onboard gitlab discover --group <path>`, then `discovery.json` contains projects from the group and all subgroups recursively.
- Self-hosted GitLab instances are supported via `--url`.
- The OIDC issuer and JWKS URI are correctly derived from the GitLab URL.
- Protected branches and environments per project are recorded.

#### C2. GitLab authenticator and workload provisioning

**As a** Platform Engineer
**I want** generated API calls that match my GitLab group structure
**So that** workload identity follows my existing project organization

**Acceptance criteria:**
- `api/01-create-authenticator.json` has `type: jwt`, `subtype: gitlab`, `name: gitlab-<group-slug>`, and `data.identity.token_app_property: project_path`.
- `api/02-workloads.yml` creates one workload per project under the authenticator's `identity_path` (`data/gitlab-apps/<group-slug>`). Workload IDs match the `project_path` claim value.
- When `environment` is present on a project's jobs, the project's workload has `environment` as an enforced claim for the authenticator; per-environment workloads may also be generated if `--per-environment` is passed.
- For projects with no environments configured, a single workload is created with `project_path` as the identity.
- All generated API bodies apply successfully against a test tenant.

#### C3. GitLab integration snippet

**As a** Platform Engineer
**I want** a `.gitlab-ci.yml` fragment ready to paste into my pipelines
**So that** I can fetch secrets from Conjur without reading integration docs

**Acceptance criteria:**
- The fragment uses `CI_JOB_JWT_V2` and the currently documented Conjur authentication pattern.
- It references the correct authenticator ID and host.
- The README explains how to adapt it for different environments and jobs.

### Epic D: Jenkins adapter (P0)

#### D1. Jenkins discovery with self-derived JWKS

**As a** Jenkins admin
**I want** COT to derive JWKS URI and issuer from my Jenkins URL
**So that** I don't have to look up plugin endpoints manually

**Acceptance criteria:**
- Given a Jenkins URL and API token, when the user runs `conjur-onboard jenkins discover --url <url>`, then the Conjur JWT plugin version is detected and the JWKS URI is derived accordingly.
- If the Conjur JWT plugin is not installed or is below the minimum version, the command exits with a clear message pointing to plugin installation docs.
- Folder and job hierarchy is enumerated.
- Custom claim mappings configured in the plugin are detected and included in `discovery.json`.

#### D2. Jenkins live token inspection

**As a** Jenkins admin
**I want** to see the actual JWT the plugin would issue for a specific job
**So that** I can validate claims before committing policy

**Acceptance criteria:**
- `conjur-onboard jenkins inspect --job <folder/job>` queries the plugin for a sample token for that job.
- The decoded token is displayed with claim annotations matching §5.2.
- No job is actually executed; this is a plugin API call only.

#### D3. Jenkins policy and Jenkinsfile generation

**As a** Platform Engineer
**I want** policy and a Jenkinsfile fragment matching my Jenkins job structure
**So that** credentials binding mirrors my existing folder organization

**Acceptance criteria:**
- `api/01-create-authenticator.json` has `type: jwt`, `subtype: jenkins`, `name: jenkins-<hostname-slug>`, and a `data.identity` block with `token_app_property: jenkins_name` and `enforced_claims: [jenkins_parent_full_name]`.
- Workloads are created under `data/jenkins-apps/<hostname-slug>/<folder-path>/<job>`.
- Generated Jenkinsfile uses `withCredentials` with the Conjur credentials provider and the correct host ID.

### Epic E: Azure DevOps adapter (P1)

#### E1. Azure DevOps discovery with service connections

**As an** Azure DevOps admin
**I want** COT to enumerate my service connections, not just my pipelines
**So that** workload identity is bound to the correct entity

**Acceptance criteria:**
- Discovery fetches org ID as a GUID, not asking the user to type it.
- Service connections per project are listed in `discovery.json` with their type (Azure Resource Manager, generic, etc.).
- Projects without WIF-capable service connections are flagged with a note that WIF must be set up before COT-generated configs will work.
- The derived OIDC issuer and JWKS URI reflect the correct org-id GUID format.

#### E2. Azure DevOps claim inspection clarity

**As a** Security Engineer
**I want** the inspector to make clear that workload identity is bound to service connections
**So that** I do not mistakenly bind to pipelines

**Acceptance criteria:**
- When `inspect` runs for Azure DevOps, the output contains an explanatory paragraph that the `sub` claim encodes the service connection path (`sc://<org>/<project>/<service-connection-name>`) and that this is the identity boundary.
- The recommended default `token_app_property` is `sub`.
- Interactive mode does not offer per-pipeline identity as a valid option without a warning naming the consequences.

#### E3. Azure DevOps authenticator and pipeline generation

**As a** Platform Engineer
**I want** generated API calls and a pipeline YAML matching my Azure DevOps projects
**So that** I can onboard pipelines without learning Azure DevOps-specific Conjur docs

**Acceptance criteria:**
- `api/01-create-authenticator.json` has `type: jwt`, `subtype` omitted (not in documented list), `name: azure-devops-<org-slug>`, and `data.identity.token_app_property: sub`.
- Workloads are created under `data/azure-devops-apps/<org-slug>`, one per service connection, with workload IDs sanitized from the `sc://...` path.
- `integration/pipeline.yml` is a valid Azure Pipelines YAML fragment using the CyberArk Conjur Service Connector task with JWT auth, referencing the correct service connection and workload.
- A companion `integration/service-connection-setup.md` documents the exact steps to create the required Azure Resource Manager service connection, since this is a prerequisite COT cannot create via API.

### Epic F: Terraform adapter (P1)

#### F1. Terraform workload-auth selection

**As a** Platform Engineer
**I want** COT to prompt me for the right auth method based on where my Terraform runners live
**So that** I don't create the wrong authenticator type

**Acceptance criteria:**
- When the user runs any Terraform command without `--workload-auth`, interactive mode prompts with the four options (`jwt`, `aws-iam`, `gcp-iam`, `azure-managed-identity`) and one-line descriptions of when each applies.
- In `--non-interactive` mode, missing `--workload-auth` for Terraform exits non-zero with a message listing valid values.
- Selecting `jwt` without providing a TFC/TFE URL prompts for it (or uses `--tfc-url` if passed).
- Selecting `aws-iam`, `gcp-iam`, or `azure-managed-identity` prompts for (or uses flags for) the corresponding cloud account identifiers.

#### F2. Terraform JWT variant (TFC/TFE)

**Acceptance criteria:**
- Given TFC credentials, `conjur-onboard terraform discover --workload-auth jwt --tfc-org <org>` enumerates workspaces and their execution modes.
- Self-hosted TFE is supported via `--tfc-url`.
- Generated authenticator has `type: jwt`, `name: terraform-<org-slug>`, `token_app_property: terraform_full_workspace`, and `enforced_claims: ["terraform_run_phase"]`.
- Generated `integration/provider.tf` uses `authn_type = "jwt"` and the correct `service_id` and `host_id`.
- Generated `integration/tfc-variable-set.json` describes the TFC variable set to configure for JWT pass-through.
- `NEXT_STEPS.md` includes a note explaining why plan and apply phases have different identities and how the enforced `terraform_run_phase` claim uses that.

#### F3. Terraform AWS IAM variant

**Acceptance criteria:**
- Given `--workload-auth aws-iam` and one or more role ARNs (via `--roles`, a file, or interactive prompt), the generator produces an AWS IAM authenticator body.
- If `aws` CLI is installed and authenticated, `conjur-onboard terraform discover --workload-auth aws-iam` can auto-discover candidate roles from the user's account.
- Generated authenticator body has `type: aws_iam`, `name: aws-iam-default` (or `--name-suffix`), no `data` object.
- If an AWS IAM authenticator already exists on the tenant with the target name, the generator treats HTTP 409 as success and proceeds to workload creation.
- Workloads are created at `data/<account-id>/<role-name>`, one per role ARN.
- Generated `integration/provider.tf` uses `authn_type = "iam"` and the correct `host_id`.

#### F4. Terraform GCP variant

**Acceptance criteria:**
- Given `--workload-auth gcp-iam` and one or more GCP service accounts (via `--service-accounts`, a file, or interactive prompt), the generator produces a GCP authenticator body.
- If `gcloud` CLI is installed and authenticated, service accounts can be auto-discovered.
- Generated authenticator body has `type: gcp`, no name suffix. If a GCP authenticator already exists (singleton), the generator treats HTTP 409 as success.
- Workloads are created at `data/gcp-apps/<sanitized-sa-name>` with annotations `authn-gcp/project-id` and `authn-gcp/service-account-email`.
- Generated `integration/provider.tf` uses `authn_type = "gcp"` and omits `service_id`.

#### F5. Terraform Azure variant

**Acceptance criteria:**
- Given `--workload-auth azure-managed-identity` and Azure subscription/resource-group/identity details, the generator produces an Azure authenticator body.
- If `az` CLI is installed and authenticated, Azure identities can be auto-discovered.
- Generated authenticator body has `type: azure` (pending verification per §11.9), `name: azure-<tenant-slug>`.
- Workloads are created under `data/azure-apps/` with annotations `authn-azure/subscription-id`, `authn-azure/resource-group`, and optionally `authn-azure/user-assigned-identity` or `authn-azure/system-assigned-identity`.
- Generated `integration/provider.tf` uses `authn_type = "azure"` and the correct `service_id` and `host_id`.

### Epic G: Ansible adapter (P2)

#### G1. Ansible workload-auth selection

**As a** Platform Engineer
**I want** COT to prompt me for the right auth method based on where my Ansible control node runs
**So that** I get the right authenticator for my environment

**Acceptance criteria:**
- Running any Ansible command without `--workload-auth` in interactive mode prompts with four options (`jwt`, `aws-iam`, `gcp-iam`, `azure-managed-identity`) and one-line descriptions.
- In `--non-interactive` mode, missing `--workload-auth` for Ansible exits non-zero with a message listing valid values.
- Selecting `jwt` requires AAP URL and token; the tool verifies AAP version supports OIDC before proceeding.
- Selecting any cloud IAM variant prompts for (or uses flags for) the corresponding cloud account identifiers.

#### G2. Ansible JWT variant (AAP with OIDC)

**Acceptance criteria:**
- Given AAP credentials and an OIDC-capable AAP version, discovery extracts issuer, JWKS URI, and available claims from AAP settings.
- Generated authenticator body has `type: jwt`, `name: ansible-aap-<org-slug>`, and appropriate `token_app_property` for the AAP OIDC claim schema.
- If the installed AAP version does not support OIDC, the command exits with a helpful message recommending either an upgrade or switching to a cloud IAM variant.
- Generated playbook fragment uses the `cyberark.conjur` collection with JWT connection config.

#### G3. Ansible cloud IAM variants (aws-iam, gcp-iam, azure-managed-identity)

**Acceptance criteria:**
- Each variant produces authenticator bodies and workload resources following the same contracts as the equivalent Terraform variants (F3, F4, F5).
- Generated `integration/conjur.conf` contains the correct `authn_type`, `host_id`, and `appliance_url` for the cloud auth method.
- Generated `integration/test-connection.yml` is a minimal Ansible playbook that verifies end-to-end connectivity by fetching a single test variable and printing success or a specific error.
- `NEXT_STEPS.md` includes collection installation (`ansible-galaxy collection install cyberark.conjur`) and a verification step using the test playbook.

### Epic H: Validation, apply, and rollback

#### H1. API plan validation against live tenant

**As a** Security Engineer
**I want** to validate the generated API plan against my Secrets Manager SaaS tenant before applying
**So that** I catch conflicts and permission issues before any change

**Acceptance criteria:**
- `conjur-onboard <platform> validate --tenant <subdomain>` connects to the tenant using the tool-auth token and for each call in `api/plan.json` performs an appropriate pre-flight check (GET to detect existing resources, policy validation endpoint for YAML fragments, etc.).
- Detected conflicts (authenticator already exists with conflicting configuration, workload already exists at the same path) are reported with the specific resource ID and a remediation hint.
- Missing tool-auth privileges are reported by policy name, not by bare HTTP 403.
- Validation never mutates tenant state.
- Exits non-zero on any detected issue.

#### H2. Apply with per-call audit log

**As a** Platform Engineer
**I want** the apply step to execute the plan exactly as generated and record what happened
**So that** I have an audit trail and a reliable rollback path

**Acceptance criteria:**
- `apply` requires prior successful `validate` unless `--skip-validate` is passed (with a prominent warning in the output).
- Calls execute in the order specified in `api/plan.json`: authenticator creation, workload policy load, group membership additions.
- Each call records request method, path, request body, response status, and response body to `apply-log.json` with a timestamp.
- A summary prints at the end: authenticator created (with ID), N workloads created, M group memberships added.
- On partial failure, `apply` stops at the first error, prints the rollback command, and exits non-zero. It does not attempt to continue past a failed call.
- Re-running `apply` against an already-applied state detects existing resources (HTTP 409 on authenticator, existing group memberships) and treats them as success with a "no change" indicator rather than failing.

#### H3. Rollback with confirmation

**As a** Platform Engineer
**I want** a reliable rollback command that undoes what apply did
**So that** I can clean up a misconfigured onboarding attempt

**Acceptance criteria:**
- `rollback` reads `apply-log.json` to determine what was created.
- Produces inverse operations in reverse order: group membership DELETEs, then workload policy deletions, then authenticator DELETE.
- Requires explicit `--confirm` before executing any destructive operation.
- Prints a clear warning that rollback deletes workloads and their audit history.
- Handles HTTP 404 responses on individual DELETE calls (indicating prior manual cleanup) gracefully, logging but not aborting.
- On completion, moves `apply-log.json` to `apply-log.rolled-back.json` so re-running `rollback` is a clean no-op.

### Epic I: Documentation and `NEXT_STEPS.md`

#### I1. Per-run walkthrough document

**As a** Platform Engineer
**I want** a single markdown document that walks me through everything I need to do
**So that** I don't have to hunt through CLI output or docs

**Acceptance criteria:**
- Every run produces `NEXT_STEPS.md` containing: summary of what was generated (authenticator type, name, workload count), ordered step list to validate, apply, and test, the exact commands to run at each step, and troubleshooting guidance for the top 3–5 failure modes for the selected platform and auth method.
- Steps are numbered and each has a verifiable outcome.
- Platform and auth-method-specific gotchas are called out inline:
  - GitHub: `permissions: id-token: write` requirement
  - GitLab: `CI_JOB_JWT_V2` vs `CI_JOB_JWT`
  - Terraform `jwt`: run-phase semantics and the difference between plan and apply identity
  - AWS IAM: role trust policy requirements for STS `GetCallerIdentity`
  - GCP: required service account scopes and audience claim
  - Azure: managed identity assignment to the resource
- A final "Verify end-to-end" section provides a concrete test case (fetch a known test secret via the configured integration) and the expected output.

#### I2. Command help and examples

**As a** Platform Engineer
**I want** thorough help output with concrete examples
**So that** I can use the CLI without reading external docs

**Acceptance criteria:**
- Every command has `--help` output with at least one complete invocation example.
- Running `conjur-onboard <platform>` with no command prints a platform-specific recommended flow, including any required `--workload-auth` value.
- The top-level `conjur-onboard --help` lists all platforms with a one-line description and an example of the most common invocation for each.

---

## 8. Non-functional requirements

### 8.1 Security

- COT never writes secrets (platform tokens, tenant auth tokens, cloud credentials) to disk outside OS-standard credential stores.
- All network communication uses TLS; certificate verification is on by default. The `--insecure` flag is not provided — Secrets Manager SaaS tenants do not require it, and offering it invites accidental misuse.
- Generated API bodies and policy YAML contain no secrets. Authenticator creation bodies reference only public configuration (JWKS URIs, issuers, audiences).
- The tool is explicit about what it does: every operation that mutates platform state (e.g., opening a PR for live inspection) requires interactive confirmation unless `--non-interactive` is explicitly passed.
- The `Accept: application/x.secretsmgr.v2beta+json` header is used on all tenant API calls. COT tracks the API version internally and warns if the tenant returns responses from a different version than expected.

### 8.2 Reliability

- Every command is idempotent. Re-running `generate` with the same inputs produces identical outputs. Re-running `apply` against an already-applied state detects existing resources and is a no-op.
- No partial writes: generation writes to a temp directory and atomically moves to the working directory on success.
- API retry policy: transient failures (HTTP 5xx, connection timeouts) are retried with exponential backoff up to 3 times. Authentication failures (401), permission failures (403), and validation errors (4xx except 409) are not retried.

### 8.3 Observability

- Opt-in anonymous telemetry tracks command invocation, completion status, platform, and workload-auth method selected. No customer identifiers, org names, tenant subdomains, or content from generated artifacts is transmitted.
- Telemetry opt-in is prompted once on first run and can be changed via `conjur-onboard config telemetry on|off`.
- Verbose mode (`-v`, `-vv`) provides increasing detail for debugging. `-vv` logs full API request/response bodies with secrets redacted.

### 8.4 Compatibility

- Target: CyberArk Secrets Manager SaaS (Conjur Cloud). The v2 REST API version is pinned at `application/x.secretsmgr.v2beta+json` as of this PRD; implementation must track the header and adapt when the API GAs.
- Conjur Enterprise and Conjur OSS are **not supported in v1**. Generated API bodies will not work against these targets; commands detect and refuse non-SaaS URLs with a clear message.
- Platform CLI versions: minimum tested versions of `gh`, `glab`, `az`, `aws`, `gcloud`, and `tf` documented at release. Missing CLIs produce install-instruction messages, not silent failures.
- Operating systems: Linux (x86_64, arm64), macOS (x86_64, arm64), Windows (x86_64).

### 8.5 Performance

- Discovery for a 100-repo GitHub org completes in under 30 seconds on a typical broadband connection.
- Generation is effectively instantaneous (<2 seconds) once discovery and inspection are complete.
- `apply` for a 100-workload plan completes in under 2 minutes, including all per-workload group-membership additions (one POST each).

### 8.6 Localization

- v1: English only. String externalization should be in place to enable future localization without code changes.

---

## 9. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Official integrations (GitHub Action, Jenkins plugin, etc.) change config shape | Generated snippets become stale | Version-pin integration references; CI test against latest integration releases; template integration snippets are data-driven, not hardcoded strings |
| v2 REST API changes between `v2beta` and GA | Generated API bodies become invalid | Track the `Accept` header version at the library layer; CI tests exercise the documented schema; pin a minimum tested API version and warn if the tenant returns a different version than expected |
| Customer has unusual or non-standard platform configuration | Generated API plan is wrong | Advanced mode exists for customization; clear warning in Express mode output that defaults may not fit all cases; `NEXT_STEPS.md` always recommends review before apply |
| Claim schemas on platforms evolve | Synthetic inspection becomes inaccurate | Live-mode inspection is always available as a verification path; synthetic schemas are stored as data files updated independently of core code |
| Customers apply plan that conflicts with existing resources | Broken tenant state | Mandatory `validate` before `apply`; apply produces per-call audit log; rollback is available; HTTP 409 on authenticator creation is handled idempotently |
| Platform API rate limits during discovery of very large orgs | Timeout or partial results | Pagination, retry with backoff, and `--repos-from-file` escape hatch for bulk inputs |
| Customers misunderstand Express "recommended" defaults as "only option" | Security misconfigurations | Express output explicitly labels defaults as recommended and shows how to customize; `NEXT_STEPS.md` describes the security implication of each default |
| Jenkins plugin endpoint changes (JWKS path) across versions | Wrong JWKS URI | Detect plugin version at discovery; maintain a version→endpoint map; warn and ask for confirmation on unknown versions |
| GCP singleton authenticator already exists on tenant | Apply fails on second customer run | Detect HTTP 409 on GCP authenticator creation and treat as success; surface clearly in apply summary |
| Cloud IAM authenticator API body contract is not fully documented in v2 API surface | Bodies may not match GA shape | Implementer verifies against current docs at build time; §12.2 lists specific references to check; fallback to policy-load API for types whose v2 body is undocumented |
| Workload creation does not have a dedicated v2 REST endpoint | Reliance on policy load API | Use policy load until workload-creation REST endpoint is available; abstract behind the shared core so swapping is a single-location change |
| Customer's CyberArk Identity session grants access to multiple tenants | Ambiguity about target tenant | `--tenant` flag is always required explicitly; COT never defaults to a "last used" tenant to avoid cross-tenant mistakes |
| Customer runs `aws-iam` / `gcp-iam` onboarding from a workstation without the target cloud identity | Generated integration snippet doesn't work | `NEXT_STEPS.md` explicitly documents that the generated `host_id` only works from a runner with the matching cloud identity attached, and includes a verification step |

---

## 10. Release phases

### Phase 1: MVP (P0 platforms, Express mode, JWT only)
- Core CLI and shared provisioning core (API plan execution)
- GitHub, GitLab, Jenkins adapters with JWT workload auth only
- Tenant auth via CyberArk Identity session
- Express mode, validate, apply, rollback
- `NEXT_STEPS.md` generation
- Binary distribution (brew + GitHub releases)

### Phase 2: Advanced mode and P1 platforms
- Interactive claim inspection for all P0 platforms
- Azure DevOps adapter (JWT)
- Terraform adapter with `jwt` variant only
- Docker image
- Opt-in telemetry
- API key tool-auth for automation use

### Phase 3: Cloud IAM workload auth and P2 platforms
- Cloud IAM support for Terraform and Ansible adapters: `aws-iam`, `gcp-iam`, `azure-managed-identity`
- Ansible JWT (AAP OIDC) adapter
- Live inspection for GitHub
- Richer error messages based on Phase 1/2 field feedback
- CI tests against customer-representative topologies

### Phase 4 (post-v1, informed by adoption data)
- Conjur Enterprise and Conjur OSS support (policy-load-based fallback for non-SaaS targets)
- Additional platforms (CircleCI, Bitbucket, CodeBuild, etc.)
- Hosted web UI consideration
- Integration with CyberArk Identity and other CyberArk products

---

## 11. Open questions

1. Should COT produce Terraform modules representing the generated configuration as an alternative output format, for customers who manage Conjur via infrastructure-as-code?
2. What is the long-term relationship between COT and the official Conjur CLI? Should COT ultimately be folded in as a subcommand (`conjur onboard ...`)?
3. For the GitHub live-inspection PR, should we offer an alternative that uses `act` locally instead of opening a PR, for customers who prohibit automated PRs?
4. Should the tool produce CI/CD configuration in the customer's existing version control via PR, rather than just writing files locally? (v2 consideration.)
5. For customers with very large orgs (1000+ repos), is per-repo workload generation the right default, or should we generate per-repo-group workloads with fewer, broader identity bindings?
6. How should COT handle customers who later need Conjur OSS or Enterprise support — separate tool, mode flag, or a shared core with pluggable provisioning backends?
7. **Workload creation endpoint.** The SaaS v2 REST API documentation covers authenticator creation and group membership. Does v2 include a dedicated workload creation endpoint? If so, COT should use it in preference to the policy load API. If not, when is one expected, and should COT abstract this behind a provisioning interface so it can be swapped without user-visible changes?
8. **Authenticator deletion endpoint.** The documented create-authenticator endpoint is `POST /api/authenticators`. Is the delete counterpart `DELETE /api/authenticators/{name}`, or does it require a different form? Rollback depends on this being confirmed.
9. **Azure authenticator v2 API body shape.** The v2 create-authenticator doc snapshot used to draft this PRD lists `jwt`, `aws_iam`, and `certificate` as supported `type` values. GCP and Azure authenticators are documented elsewhere as `authn-gcp` and `authn-azure`. Confirm at implementation time: (a) the exact `type` value strings the v2 POST accepts for GCP and Azure; (b) whether Azure requires configuration variables in the `data` object (Azure AD tenant ID, etc.) at authenticator creation time; (c) whether GCP's singleton constraint is enforced server-side or client-must-enforce.
10. **CyberArk Identity OAuth flow for tenant auth.** Confirm the documented OAuth2 authorization-code flow (endpoints, scopes, PKCE requirements) for obtaining a tenant auth token via CyberArk Identity. If no public OAuth flow exists, tenant auth falls back to API key only for v1.
11. **Authenticator rename / update semantics.** The v2 API documents create and delete. Is there an update endpoint for authenticator `data`? If a customer re-runs COT with different claim choices for an existing authenticator, what's the idempotent path — delete-then-recreate, in-place update, or refuse and require advanced mode?
12. **Azure DevOps Service Connection service principal.** When Workload Identity Federation is used, can COT discover the service principal federated subject claim automatically, or must the user provide it manually? Affects how automatic discovery can scope identity.

---

## 12. Appendix

### 12.1 Glossary

- **Secrets Manager SaaS / Conjur Cloud:** The hosted CyberArk tenant at `https://<subdomain>.secretsmgr.cyberark.cloud`. v1 target for COT.
- **Authenticator:** A Conjur construct that validates incoming credentials and maps them to workload identities. Types in v1 scope: `jwt`, `aws_iam`, `gcp`, `azure`.
- **`apps` group:** Auto-created group at `conjur/<authn-type>/<authn-name>/apps` when an authenticator is created via the v2 API. Membership in this group grants a workload `authenticate` and `read` on the authenticator.
- **`operators` group:** Auto-created group at `conjur/<authn-type>/<authn-name>/operators`. Membership grants management and status privileges on the authenticator.
- **Workload / Host:** A Conjur identity representing a non-human actor (pipeline, job, service). In policy, `!host` or `!workload` depending on Conjur edition and API version.
- **Identity claim / `token_app_property`:** For JWT authenticators, the claim whose value Conjur uses to look up the matching workload.
- **`identity_path`:** For JWT authenticators, the policy branch under which workloads live.
- **`enforced_claims`:** Additional JWT claims that must match annotations on the workload at authentication time, beyond `token_app_property`.
- **JWKS:** JSON Web Key Set; the public keys the issuer publishes for JWT signature verification.
- **Cloud IAM authenticator:** An authenticator that validates the workload's underlying cloud identity (AWS IAM role, GCP service account, Azure managed identity) rather than a JWT the workload mints. Identity binding uses resource annotations on the workload, not claim matching.
- **Safe:** A logical grouping of secrets in CyberArk Secrets Manager.
- **Tool auth / Workload auth:** Two distinct Conjur authentication concerns — how COT authenticates to the tenant to provision resources (tool auth) vs. how the customer's workloads authenticate to retrieve secrets (workload auth). Always kept distinct in this spec.

### 12.2 References to verify at build time

Before implementation begins, the team must verify current documentation for:

**API contract:**
- v2 REST API version header (`application/x.secretsmgr.v2beta+json`) — confirm GA value and tracking strategy.
- `POST /api/authenticators` body shapes for all `type` values in scope: `jwt`, `aws_iam`, `gcp`, `azure`. Specifically the GCP and Azure bodies, which were not shown in the snapshot used to draft this PRD (§11.9).
- `DELETE /api/authenticators/{name}` endpoint existence and exact shape (§11.8).
- `POST /api/groups/{identifier}/members` and its `DELETE` counterpart — confirmed in the doc snapshot.
- Workload creation endpoint (§11.7) — confirm whether a v2 REST endpoint exists or policy load is the correct path.
- Authenticator update / rename semantics (§11.11).

**Integration versions:**
- Current GA versions of platform integrations: `cyberark/conjur-action` (GitHub), Conjur GitLab integration, Conjur Jenkins plugin, CyberArk Conjur Service Connector (Azure DevOps), Conjur Terraform provider, `cyberark.conjur` Ansible collection.
- Exact JWKS URI paths for each platform, especially the Jenkins plugin which has varied across versions.

**Platform identity schemas:**
- GitHub Actions OIDC claim schema (current version).
- GitLab `CI_JOB_JWT_V2` claim schema.
- Jenkins Conjur JWT plugin claim mapping documentation.
- Azure DevOps Workload Identity Federation subject format.
- Terraform Cloud workload identity token claim schema.
- AAP OIDC claim schema (versions that support it).

**Cloud IAM specifics:**
- AWS IAM authenticator trust requirements (what IAM permissions the target role must grant to Conjur).
- GCP authenticator audience claim value and service account token format requirements.
- Azure authenticator required configuration variables (Azure AD tenant ID, etc.) — see §11.9.

**CyberArk Identity integration:**
- OAuth2 authorization-code flow details for tenant auth (§11.10).
- Required scopes and consent prompts.
