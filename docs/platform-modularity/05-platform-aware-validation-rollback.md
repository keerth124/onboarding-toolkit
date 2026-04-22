# Slice 5: Platform-Aware Validation And Rollback

Status: Implemented.

Implemented primarily in:

- `internal/core/plan.go`
- `internal/core/validate.go`
- `internal/core/rollback.go`
- `internal/conjur/generate.go`
- `internal/core/validate_test.go`
- `internal/core/rollback_test.go`

## Goal

Remove remaining platform assumptions from `internal/core` while keeping validation and rollback behavior safe for GitHub and future platforms.

## Motivation

`internal/core` should be the platform-neutral executor. Before this slice, it contained a GitHub-specific subtype check and rollback logic that depended on operation ID conventions. Those were manageable for one platform but became fragile as more adapters generated plans.

## Scope

- Remove GitHub-specific validation logic from `internal/core`.
- Represent expected authenticator properties in the plan or validation metadata.
- Keep generic Conjur reachability validation in core.
- Make rollback mappings driven by explicit operation metadata where practical.
- Preserve GitHub rollback behavior.
- Add tests for generic and GitHub-specific validation paths.

## Validation Direction

Core validation should remain responsible for:

- Ensuring referenced body files are readable.
- Checking Conjur endpoint reachability.
- Checking authentication errors.
- Loading and logging validation results.
- Comparing generic expected authenticator fields supplied by the plan.

Platform adapters or generated plan metadata should supply:

- Expected authenticator type.
- Expected authenticator subtype, if applicable.
- Expected identity path.
- Any platform-specific compatibility checks.

## Rollback Direction

Rollback should avoid relying only on operation ID strings where explicit metadata would be clearer.

Useful metadata may include:

- `rollback_kind`
- `authenticator_name`
- `workload_ids`
- `workload_id`
- `group_id`
- `member_kind`
- `rollback_behavior`

Core can still provide built-in rollback handlers for common Conjur operation kinds.

## Out Of Scope

- Building a full plugin runtime.
- Supporting arbitrary rollback scripts.
- Adding a new platform.
- Redesigning apply logs beyond what is needed for compatibility.

## Acceptance Criteria

- Done: `internal/core` has no hardcoded GitHub platform checks.
- Done: GitHub validation still catches authenticator subtype and identity path
  conflicts using `authenticator_subtype` in `api/plan.json`.
- Done: Self-hosted/Enterprise plans omit `authenticator_subtype` and use the
  account-scoped authenticator endpoint shape for create and rollback.
- Done: GitHub rollback tests still pass.
- Done: Generic validation tests prove a non-GitHub plan does not require
  GitHub subtype behavior.
- Done: Existing apply log compatibility is preserved through operation ID
  fallback.
- Done: New generated plans include rollback metadata such as
  `rollback_kind`, `workload_ids`, `workload_id`, and `member_kind`.

## Residual Risk

Rollback is safety-sensitive. Keep backward compatibility with existing operation IDs while adding clearer metadata for future plans.
