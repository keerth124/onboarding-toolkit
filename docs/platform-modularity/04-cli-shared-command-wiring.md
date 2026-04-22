# Slice 4: CLI Shared Command Wiring

Status: Implemented.

Implemented primarily in:

- `cmd/shared/flags.go`
- `cmd/shared/conjur.go`
- `cmd/shared/commands.go`
- `cmd/shared/flags_test.go`
- `cmd/github/github.go`
- `cmd/root.go`

## Goal

Generalize CLI wiring so future platform commands can reuse common apply, validate, rollback, Conjur connection, and work directory behavior.

## Motivation

Before this slice, root command registration and shared Conjur flags lived in GitHub-specific command code. A second platform would either have duplicated that code or needed to refactor it under pressure. This slice made the CLI shape ready for more adapters.

## Scope

- Move shared Conjur connection flags out of `cmd/github`.
- Provide shared builders or helpers for apply, validate, and rollback commands.
- Make default work directory naming platform-aware instead of GitHub-only.
- Keep platform command registration explicit.
- Update root help text to describe only implemented platforms.
- Keep existing GitHub CLI behavior compatible.

## Proposed Structure

The exact package names can change, but a likely split is:

- `cmd/shared`: common command helpers and shared flags.
- `cmd/github`: GitHub-specific discover, inspect, generate, express, and adapter registration.
- `cmd/root.go`: root command and platform registration.

## Shared CLI Pieces

The reusable command helpers should cover:

- `--tenant`
- `--conjur-url`
- `--account`
- `--username`
- `--insecure-skip-tls-verify`
- `CONJUR_API_KEY` loading
- Conjur client construction
- Plan loading
- Apply command behavior
- Validate command behavior
- Rollback command behavior
- Consistent output summaries where platform-independent

## Out Of Scope

- Adding another platform command.
- Rewriting GitHub discovery or inspect.
- Changing the `core.Plan` schema.
- Changing generated artifacts.

## Acceptance Criteria

- Done: `conjur-onboard github apply` behavior remains compatible.
- Done: `conjur-onboard github validate` behavior remains compatible.
- Done: `conjur-onboard github rollback` behavior remains compatible.
- Done: Shared command code can be reused by a future `cmd/gitlab` or
  `cmd/jenkins` package.
- Done: Shared Conjur connection flags include local-test TLS verification
  bypass via `--insecure-skip-tls-verify`.
- Done: Root help lists only the implemented `github` platform.
- Done: `--work-dir` now defaults dynamically to
  `conjur-onboard-<platform>-<timestamp>` when omitted.

## Residual Risk

CLI refactors can accidentally alter flag defaults or help text. Keep tests or manual checks focused on command construction and required flag behavior.
