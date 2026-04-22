# Slice 5: Platform-Aware Validation And Rollback

## Goal

Remove remaining platform assumptions from `internal/core` while keeping validation and rollback behavior safe for GitHub and future platforms.

## Motivation

`internal/core` should be the platform-neutral executor. It currently contains a GitHub-specific subtype check and rollback logic that depends on operation ID conventions. These are manageable for one platform but become fragile as more adapters generate plans.

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

- `internal/core` has no hardcoded GitHub platform checks.
- GitHub validation still catches authenticator subtype and identity path conflicts.
- GitHub rollback tests still pass.
- Generic validation tests prove a non-GitHub plan does not require GitHub subtype behavior.
- Existing apply log compatibility is preserved.

## Residual Risk

Rollback is safety-sensitive. Keep backward compatibility with existing operation IDs while adding clearer metadata for future plans.
