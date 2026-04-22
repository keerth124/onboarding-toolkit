# Slice 4: CLI Shared Command Wiring

## Goal

Generalize CLI wiring so future platform commands can reuse common apply, validate, rollback, Conjur connection, and work directory behavior.

## Motivation

Right now root command registration and shared Conjur flags live in GitHub-specific command code. A second platform would either duplicate that code or need to refactor it under pressure. This slice makes the CLI shape ready for more adapters.

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

- `conjur-onboard github apply` behavior remains compatible.
- `conjur-onboard github validate` behavior remains compatible.
- `conjur-onboard github rollback` behavior remains compatible.
- Shared command code can be reused by a future `cmd/gitlab` or `cmd/jenkins` package.
- Root help does not imply unsupported platforms are implemented.

## Residual Risk

CLI refactors can accidentally alter flag defaults or help text. Keep tests or manual checks focused on command construction and required flag behavior.
