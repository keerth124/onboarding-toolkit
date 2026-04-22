# Slice 2: Generic Conjur/JWT Generation

Status: Implemented.

Implemented primarily in:

- `internal/conjur/generate.go`
- `internal/conjur/authenticator.go`
- `internal/conjur/workload.go`
- `internal/conjur/groups.go`
- `cmd/github/generation_config.go`

## Goal

Extract reusable Conjur artifact generation so GitHub is no longer embedded in `internal/conjur`.

After this slice, the generator should consume platform-neutral inputs and produce the same core Conjur artifacts for any JWT-based platform.

## Motivation

Today `internal/conjur` imports `internal/github`, accepts GitHub discovery types, and hardcodes GitHub-specific names, identity paths, operation descriptions, and claim behavior. This makes a second platform likely to copy generation code instead of reusing it.

## Scope

- Refactor generic JWT authenticator generation to use platform-neutral inputs.
- Refactor workload policy generation to use normalized workloads.
- Keep group membership generation reusable.
- Keep self-hosted grant fallback reusable.
- Build `core.Plan` from generic Conjur generation inputs.
- Remove direct `internal/conjur` dependency on `internal/github`.
- Preserve existing operation semantics where possible.

## Reusable Generation Pieces

This slice should make the following platform-neutral:

- Target-aware authenticator create body and operation generation for JWT
  authenticators.
- Self-hosted parent branch policy generation before authenticator creation.
- `api/02-workloads.yml` policy generation.
- `api/03-add-group-members.jsonl` generation.
- `api/04-grant-authenticator-access.yml` self-hosted fallback.
- `api/plan.json` operation ordering.
- Authenticator naming override behavior.
- SaaS versus self-hosted target handling.

## Platform Inputs Needed

The generic generator should receive:

- Platform ID, such as `github` or `gitlab`.
- Human platform name, such as `GitHub Actions`.
- Authenticator type.
- Optional authenticator subtype. SaaS plans may include it, while
  self-hosted/Enterprise plans omit it because their create-authenticator
  request body does not include `subtype`.
- Issuer.
- JWKS URI.
- Audience.
- Identity path.
- Token app property.
- Enforced claims.
- Workloads with full Conjur paths, relative host IDs, and annotations.
- Integration artifact writer or returned artifact list.
- Next-step content provider.

## Out Of Scope

- Changing GitHub CLI behavior.
- Adding a second platform.
- Reworking root command registration.
- Fully redesigning rollback metadata.

## Acceptance Criteria

- Done: `internal/conjur` no longer imports `internal/github`.
- Done: GitHub generation goes through compatibility glue that builds generic
  generation input.
- Done: Generated GitHub plans remain semantically equivalent and now include
  additional platform-neutral metadata needed by later slices.
- Done: The generic generator is unit-tested without GitHub discovery objects.

## Residual Risk

This is the main architectural extraction. Byte-for-byte output may change slightly if templates are reorganized, but command behavior and generated Conjur operations should stay equivalent.
