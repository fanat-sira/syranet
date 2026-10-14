# Syranet v0.1.0-rc.1

First release candidate of **Syranet: Self-hosted VPN/Proxy Access Management**.

## Included

- Go backend, PostgreSQL migrations and two private-chat Telegram bots.
- Single-node Hysteria2 access with verified TLS and configurable upstream routing.
- Multiple selectable entry nodes, independent credentials and timed subscriptions.
- Transactional administrative changes, persistent replay deduplication and reconciled desired/applied node state.
- Restricted node supervisors with validation, atomic configuration writes, restart and rollback.
- Compose deployment, interactive setup, certificate renewal guidance, backup/restore scripts and security documentation.
- Go 1.27.2 minimum and pinned builder image; standard-library security fixes required by the vulnerability check.

## Validation

[GitHub Actions CI](https://github.com/fanat-sira/syranet/actions/runs/38567519723) passed on commit `0b9ef3a8f0d43737a8f57852ead18da14c149f39` with Go 1.27.2. Go formatting, vet, tests/race, build, vulnerability scan, script syntax, Docker E2E and the secret scan all passed.

See [VERIFICATION.md](VERIFICATION.md) for test coverage and deployment validation scope.

## Deployment status and compatibility

Real VPS deployment, public ACME renewal, live Telegram delivery and deployment backup/restore remain unverified. This release candidate is not labeled production-ready. Config changes restart sing-box and disconnect current sessions; an unreachable node can retain its last applied access until reconciliation recovers.

Legacy host-managed installations require an explicit migration and verified backup. The new topology replaces geographic environment variables and host systemd integration; Hysteria2 links use password-only authentication. Read README.md and docs/IMPLEMENTATION.md before migrating.

Original Syranet source uses MIT. The separate sing-box executable retains GPL-3.0-or-later; see docs/THIRD_PARTY.md.
