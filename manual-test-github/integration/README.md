# GitHub Actions Integration

This directory contains a starter workflow for a GitHub Actions workload using the generated Conjur Cloud JWT authenticator.

Generated values:

- Tenant URL: `https://my-tenant.secretsmgr.cyberark.cloud`
- Authenticator service ID: `github-keerth124`
- Example workload host ID: `data/github-apps/keerth124/keerth124/onboarding-toolkit`
- Example repository: `onboarding-toolkit`

Before using the workflow, replace `data/vault/example/safe/test-secret` with a real variable path and grant the authenticator apps group access to the required safe.

The workflow must keep:

- `permissions: id-token: write`
- `permissions: contents: read`
- `cyberark/conjur-action@v2`
