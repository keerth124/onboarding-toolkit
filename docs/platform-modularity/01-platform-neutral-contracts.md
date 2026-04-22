# Slice 1: Platform-Neutral Contracts

## Goal

Define the reusable platform model without changing current GitHub behavior.

This slice creates the shared vocabulary future platform adapters will use, while leaving the existing GitHub commands and generator path intact until the migration slices.

## Motivation

The current project has a reusable execution core, but the generation path is GitHub-shaped. Before moving code around, define the target contracts so each later refactor has a stable destination.

## Scope

- Add a new package such as `internal/platform`.
- Define normalized types for platform discovery and generation inputs.
- Define the minimum adapter contract needed by GitHub and future platforms.
- Keep the existing GitHub implementation working as-is.
- Avoid rewriting command behavior in this slice.

## Proposed Types

The exact names can change during implementation, but the contract should cover:

- `Discovery`: normalized platform discovery output.
- `Workload`: one Conjur workload identity to generate.
- `ClaimSelection`: selected token claim strategy.
- `ClaimAnalysis`: available and recommended claim metadata.
- `Adapter`: platform-specific behavior provider.
- `IntegrationArtifact`: generated platform-side snippets or documents.

## Adapter Responsibilities

A platform adapter should be responsible for:

- Discovering platform resources.
- Inspecting or synthesizing relevant identity claims.
- Choosing default claim strategy.
- Mapping discovered resources to Conjur workloads.
- Providing authenticator metadata such as type, subtype, issuer, JWKS URI, and audience defaults.
- Providing platform-specific integration artifacts.
- Providing platform-specific next-step notes or troubleshooting text.

## Out Of Scope

- Migrating GitHub generation to the new contracts.
- Changing `conjur-onboard github` command behavior.
- Generalizing apply, validate, or rollback.
- Adding a second platform.

## Acceptance Criteria

- The new platform contract package compiles.
- Existing tests still pass.
- No generated GitHub artifact shape intentionally changes.
- The contract is documented enough that GitLab or Jenkins could implement it without reading GitHub internals first.

## Residual Risk

The first contract will likely need small adjustments during Slice 2 and Slice 3. Keep it minimal so later changes are cheap.
