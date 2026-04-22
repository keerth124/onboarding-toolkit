# Slice 3: GitHub Adapter Migration

## Goal

Make GitHub the first implementation of the platform adapter contracts and route GitHub generation through the generic Conjur/JWT generator.

## Motivation

The architecture is not proven until the existing GitHub platform uses it. This slice moves GitHub-specific decisions out of generic Conjur generation while preserving current user-facing behavior.

## Scope

- Add a GitHub adapter implementation.
- Convert GitHub discovery output into normalized platform workloads.
- Move GitHub-specific claim defaults into the GitHub adapter.
- Move GitHub-specific authenticator metadata into the GitHub adapter.
- Move GitHub integration snippet generation behind the adapter.
- Move GitHub next-step text behind the adapter or a platform template.
- Keep all existing `conjur-onboard github` commands working.

## GitHub-Specific Responsibilities

The GitHub adapter should own:

- Platform ID: `github`.
- Human platform name: `GitHub Actions`.
- Authenticator subtype: `github_actions`.
- Default token app property: `repository`.
- Supported identity claims and enforced claims.
- OIDC issuer: `https://token.actions.githubusercontent.com`.
- JWKS URI: `https://token.actions.githubusercontent.com/.well-known/jwks`.
- Default authenticator name: `github-<org>`.
- Identity path: `data/github-apps/<org>`.
- Repository-to-workload mapping.
- GitHub Actions workflow snippet.
- GitHub troubleshooting text.

## Out Of Scope

- Adding GitLab, Jenkins, or Azure DevOps.
- Changing GitHub discovery API calls.
- Implementing live OIDC token inspection.
- Large CLI cleanup beyond what is necessary to call the adapter.

## Acceptance Criteria

- `conjur-onboard github discover` still writes compatible `discovery.json`.
- `conjur-onboard github inspect` still writes compatible `claims-analysis.json`.
- `conjur-onboard github generate` still writes the expected API artifacts.
- `conjur-onboard github express` still works.
- Existing GitHub unit tests pass.
- New tests cover GitHub adapter workload mapping and metadata.

## Residual Risk

This slice migrates active behavior. The main risk is accidentally changing generated file content or operation metadata relied on by apply and rollback.
