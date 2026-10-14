# Verification record

Updated: 2026-10-14. The current GitHub Actions workflow passes with Go 1.27.2. CI and local checks are recorded separately from deployment validation.

## Local implementation checks

| Check | Evidence / scope |
| --- | --- |
| Go test with race detection | `SINGBOX_TEST_BINARY=/tmp/syranet-tools/sing-box go test -race -count=1 ./...`; unit/config/auth/lifecycle/supervisor and real local networking suites passed. Docker tests explicitly skip without their disposable flags. |
| Real sing-box configuration validation | Official 1.14.3 release binary, downloaded with verified release asset SHA-256; generated direct and upstream configurations accepted by `sing-box check`. |
| Real Hysteria2 traffic | Locally launched pinned server/client binaries with a trusted synthetic certificate: TCP and UDP passed, hostnames resolved, multi-node traffic exited from upstream source address `127.0.0.2`, revoked access failed and activation recovered. |
| Real SOCKS authorization | Local authenticated SOCKS inbound passed TCP with correct credentials and rejected an incorrect password. |
| TLS/API and apply failures | HTTPS trust rejection/acceptance, missing certificate failure, check failure, failed restart/rollback/retry, concurrent identical applies, certificate-change detection and canonical HTTP config hashes tested. Infrastructure failure tests use controlled engines; they do not pretend to be a VPS outage. |
| PostgreSQL 16.15 lifecycle | A native packaged PostgreSQL 16.15 binary ran in a fresh temporary directory bound only to localhost; the archive checksum was verified. `TestPostgresLifecycle` passed migrations/re-run, registration/approval idempotence, command replay deduplication, questionnaire protection, real transaction rollback on audit failure, revocation, rotation and expiry. |
| PostgreSQL reconciliation | `TestPostgresReconciliation` passed against the same fresh DB: independent reconcilers serialize, rejected apply persists desired/applied mismatch, token recovery reconciles and disabled nodes are drained. These agent endpoints use controlled supervisors in the test process. |
| Static/build checks | `go vet ./...`, `go build -trimpath`, `go mod verify`, gofmt and `git diff --check` passed. |
| Secret scan | Gitleaks 8.30.1 reported no leaks in the checked Git history and publishable files. Ignored deployment secrets were excluded. |
| Compose and scripts | Production, remote-node and disposable-E2E Compose models parsed with generated fixture variables; Python compilation and shell syntax checks passed. No privileged containers or Docker socket mounts are configured. |
| Image availability | Registry manifests for Go 1.27.2-bookworm, PostgreSQL 16.15-alpine and sing-box 1.14.3 resolved for amd64/arm64; their multi-platform digests are pinned. This verifies image availability, not execution. |

## Confirmed on GitHub Actions

[CI run 38567519723](https://github.com/fanat-sira/syranet/actions/runs/38567519723) completed successfully on 2026-10-14 for commit [`0b9ef3a8f0d43737a8f57852ead18da14c149f39`](https://github.com/fanat-sira/syranet/commit/0b9ef3a8f0d43737a8f57852ead18da14c149f39). All three job conclusions were verified through the GitHub API.

| Job / check | Confirmed result |
| --- | --- |
| [Docker infrastructure E2E](https://github.com/fanat-sira/syranet/actions/runs/38567519723/job/115863897161) | **Passed.** The disposable Compose pipeline covers migrations, lifecycle transitions, real TCP/UDP/SOCKS traffic, verified synthetic TLS, direct/upstream egress, suspension/revocation/rotation, expiry/extension, retry, port-collision rollback and PostgreSQL/node restart persistence. |
| [Secret scan](https://github.com/fanat-sira/syranet/actions/runs/38567519723/job/115863896572) | **Passed.** GitHub Gitleaks job completed successfully. |
| [Go checks](https://github.com/fanat-sira/syranet/actions/runs/38567519723/job/115863896736) | **Passed.** Formatting, module verification, vet, real-binary/race tests, build, vulnerability scan and script syntax checks succeeded with Go 1.27.2. |
| Overall workflow | **Passed.** |

## Go toolchain

CI, the module minimum and the Docker builder are updated to Go 1.27.2. The builder uses the verified multi-platform digest `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.

Executed locally with the checksum-verified official Go 1.27.2 linux/amd64 toolchain:

- Formatting and `go mod verify`: passed.
- `go vet ./...`: passed.
- `SINGBOX_TEST_BINARY=/tmp/syranet-tools/sing-box go test -race -count=1 ./...`: passed, including real local Hysteria2 TCP/UDP and SOCKS authentication. Docker-only tests skip locally without their disposable environment.
- `go build -trimpath`: passed.
- The exact CI command `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`: **No vulnerabilities found**.
- Python/shell syntax: passed.

## Still unverified in deployment

- Fresh Ubuntu VPS Quick Start, provider firewall behavior, public ACME issuance/automatic renewal and real two-VPS networking.
- Real Telegram BotFather tokens, long polling and private message delivery with both bots. Tests use synthetic transport; no production token was used.
- Backup/restore scripts against deployment data: `pg_dump`/`pg_restore` and recovery cutover were not exercised by the Docker E2E harness.
- ARM64 runtime: image availability was checked, but runtime tests used amd64.

The local environment still has no accessible Docker daemon. This limits local reruns; it does **not** undo the confirmed GitHub Docker E2E result.

## Deployment validation

For a new deployment, exercise the Quick Start with test bots and real certificates, two-server egress, key revocation, certificate renewal, backup/restore and temporary node/DB outages. Legacy migrations require credential rotation and a verified backup before cutover; see [OPERATIONS.md](OPERATIONS.md) and [IMPLEMENTATION.md](IMPLEMENTATION.md).

Disabled/revoked/expired access is enforced after a successful node apply. Unreachable nodes retain their last applied configuration, and an offline manager cannot enforce new subscription expirations.
