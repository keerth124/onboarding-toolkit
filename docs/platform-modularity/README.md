# Platform Modularity Guide

This guide explains how `conjur-onboard` is structured so new CI/CD platforms
can reuse the Conjur generation, validation, apply, rollback, and shared CLI
plumbing that was built for GitHub.

The current implemented platform is GitHub Actions. Future platforms such as
Jenkins, GitLab, Azure DevOps, or Buildkite should follow the same adapter
shape instead of copying Conjur-specific logic.

## Current Architecture

The codebase is split into four layers:

- `internal/platform`: platform-neutral contracts and data types.
- `internal/<platform>`: platform-specific discovery, claim analysis, workload
  mapping, authenticator defaults, and generated integration snippets.
- `internal/conjur`: reusable Conjur artifact generation for JWT onboarding.
- `internal/core`: platform-neutral `validate`, `apply`, and `rollback`
  execution against a generated `api/plan.json`.
- `cmd/shared`: reusable CLI wiring for live Conjur commands.
- `cmd/<platform>`: platform-specific command registration and command flow.

GitHub is the reference implementation:

- `internal/github/adapter.go` implements the platform adapter behavior.
- `cmd/github/generation_config.go` converts GitHub discovery output into a
  generic `conjur.GenerateConfig`.
- `cmd/github/github.go` wires GitHub commands into the shared apply,
  validate, and rollback builders.
- `cmd/root.go` explicitly registers the GitHub command.

This is modular, but it is not a dynamic plugin runtime yet. Adding a new
platform currently means adding a new package and registering a new Cobra
command in `cmd/root.go`.

## Shared Contracts

The adapter contract lives in `internal/platform/contracts.go`.

A platform adapter must provide:

- `Descriptor`: stable platform ID and display metadata.
- `Discover`: platform-specific resource discovery.
- `InspectClaims`: live or synthetic identity claim inspection.
- `DefaultClaimSelection`: safe default JWT claim strategy.
- `Authenticator`: Conjur JWT authenticator metadata.
- `Workloads`: normalized Conjur workload identities.
- `IntegrationArtifacts`: platform-side snippets such as workflow files or
  Jenkinsfile examples.
- `NextSteps`: platform-specific user guidance.

The most important normalized types are:

- `platform.Discovery`: common discovery output.
- `platform.ClaimAnalysis`: available claims and selected claim strategy.
- `platform.Authenticator`: Conjur JWT authenticator settings.
- `platform.Workload`: one Conjur workload identity to create.
- `platform.GenerationInput`: all generic inputs needed for generation.

## What Gets Reused

A new adapter should reuse these pieces without platform-specific forks:

- Conjur JWT authenticator body generation.
- SaaS versus self-hosted endpoint selection.
- Self-hosted authenticator parent branch generation.
- Workload policy generation.
- SaaS group membership body generation.
- Self-hosted group access policy fallback.
- `api/plan.json` ordering and metadata.
- Dry-run and live `validate`.
- Dry-run and live `apply`.
- Rollback mapping for common generated operations.
- Shared Conjur flags:
  - `--tenant`
  - `--conjur-url`
  - `--account`
  - `--username`
  - `--insecure-skip-tls-verify`
  - `CONJUR_API_KEY`

The adapter should not build raw Conjur API request loops or duplicate apply
logic. It should generate platform-neutral inputs and let `internal/conjur` and
`internal/core` handle the Conjur side.

## What Belongs In The Adapter

Platform-specific code belongs in `internal/<platform>`:

- API clients and discovery logic for that platform.
- The platform's OIDC issuer and JWKS URL discovery or defaults.
- Claim recommendations and warnings.
- Claim-to-Conjur identity strategy.
- Mapping platform resources to `platform.Workload`.
- Authenticator naming and identity path conventions.
- Platform-specific generated files.
- Platform-specific `NEXT_STEPS.md` content.

For GitHub, the adapter maps repositories to workloads and keeps the Conjur
host path under the org branch:

```text
data/github-apps/<org>/<repo>
```

The JWT `repository` annotation still uses GitHub's full `owner/repo` claim
value because that is what GitHub emits in the token.

## Generation Flow

The current GitHub flow is the model for new platforms:

1. Platform command loads platform discovery output.
2. Platform command loads or creates claim analysis.
3. Adapter builds `platform.GenerationInput`.
4. Adapter returns:
   - `platform.Authenticator`
   - `[]platform.Workload`
   - `[]platform.IntegrationArtifact`
   - `NEXT_STEPS.md` content
5. Command passes those values into `conjur.Generate`.
6. `internal/conjur` writes:
   - `api/00-authenticator-branch.yml` for self-hosted bootstrap plans
   - `api/01-create-authenticator.json`
   - `api/02-workloads.yml`
   - `api/03-add-group-members.jsonl` for SaaS
   - `api/04-grant-authenticator-access.yml` for self-hosted
   - `api/plan.json`
   - integration artifacts
   - `NEXT_STEPS.md`

After generation, `validate`, `apply`, and `rollback` consume `api/plan.json`
through `internal/core`; they do not call the platform adapter.

## Adding A New Adapter

Use this checklist when adding a platform such as Jenkins.

1. Create `internal/<platform>`.

   Example:

   ```text
   internal/jenkins/
     adapter.go
     discover.go
     claims.go
     claims_test.go
     adapter_test.go
   ```

2. Implement `platform.Adapter`.

   Start with the GitHub adapter as the reference:

   ```go
   type Adapter struct{}

   func (Adapter) Descriptor() platform.Descriptor {
       return platform.Descriptor{
           ID:          "jenkins",
           DisplayName: "Jenkins",
           Description: "Jenkins workloads via OIDC",
       }
   }
   ```

3. Normalize discovery output.

   Map Jenkins objects into `platform.Resource`. Depending on the Jenkins
   setup, resources might be jobs, folders, multibranch projects, pipelines, or
   controller-scoped identities.

4. Decide the identity boundary.

   Pick a stable identity path and host ID shape. For example:

   ```text
   data/jenkins/<controller-or-folder>/<job>
   ```

   Keep paths readable and avoid duplicating parent names inside child host IDs.

5. Define claim recommendations.

   The adapter should choose the safest available JWT claim for
   `TokenAppProperty`. For Jenkins, this depends on the OIDC plugin/provider
   being used. Common candidates might be job full name, folder path, pipeline
   ID, or subject. Avoid broad claims that would collapse many jobs into one
   Conjur workload identity.

6. Build authenticator metadata.

   The adapter must provide:

   - `Type`: usually `jwt`
   - `Subtype`: only when the target API supports or requires one
   - `Name`: stable, sanitized authenticator name
   - `Issuer`
   - `JWKSURI`
   - `Audience`
   - `IdentityPath`
   - `TokenAppProperty`
   - `EnforcedClaims`

7. Build workloads.

   Each workload needs:

   - `FullPath`: full Conjur host path
   - `HostID`: ID relative to the identity policy branch
   - `DisplayName`: user-facing name
   - `SourceID`: platform source object ID
   - `Annotations`: JWT annotations such as `authn-jwt/<authenticator>/<claim>`

8. Generate integration artifacts.

   For Jenkins this might be:

   ```text
   integration/Jenkinsfile
   integration/README.md
   ```

   These files should show the Conjur URL, authenticator ID, host ID, and where
   a secret path belongs.

9. Add `cmd/<platform>`.

   Mirror the GitHub command package shape:

   ```text
   cmd/jenkins/
     jenkins.go
     discover.go
     inspect.go
     generate.go
     generation_config.go
   ```

   Use `cmd/shared.NewValidateCmd`, `cmd/shared.NewApplyCmd`, and
   `cmd/shared.NewRollbackCmd` instead of platform-specific copies.

10. Register the command.

    Add the new command to `cmd/root.go`:

    ```go
    rootCmd.AddCommand(jenkins.NewJenkinsCmd(shared.GlobalFlags{...}))
    ```

11. Add tests.

    Minimum useful tests:

    - Adapter descriptor validation.
    - Discovery normalization.
    - Claim analysis defaults.
    - Authenticator metadata.
    - Workload path and annotation mapping.
    - Generation config produces expected `api/plan.json`.
    - Self-hosted generation emits account-aware policy paths.

## Jenkins-Specific Notes

Before implementing Jenkins, confirm these details:

- Which Jenkins OIDC plugin or identity provider is in scope.
- The token issuer URL.
- The JWKS URI.
- The audience expected by Conjur.
- The exact token claims available in a job.
- Whether job identity should be scoped by controller, folder, organization,
  multibranch project, or job.
- Whether the Jenkins plugin can fetch Conjur secrets using host ID and JWT in
  the same way the generated Conjur authenticator expects.

Do not assume GitHub's `repository` claim model applies to Jenkins. The
adapter contract is reusable, but the claim strategy must match the platform's
real token.

## Current Limitations

- Platform command registration is explicit in Go code; there is no dynamic
  plugin loader yet.
- The generic generator is JWT-focused.
- Live claim inspection is platform-specific and only synthetic GitHub
  inspection exists today.
- Safe grants are not generated; users still grant the generated authenticator
  apps group to safes manually.
- Rollback covers common generated operations, but policy-load rollback remains
  manual-review oriented.

