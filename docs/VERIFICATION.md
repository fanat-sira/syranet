# Verification record

Updated: 2026-10-14. Local implementation checks and confirmed GitHub Actions results are recorded separately below. Neither is a production VPS rollout or a live Telegram validation. No author's VPN servers, production credentials or deployment volumes were used; the existing private `.env` was preserved.

## Original local implementation checks

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
| Original Go dependency scan | The initial local scan reported no findings at that time. This result was superseded by the GitHub Go 1.27.1 scan below; it must not be presented as current security clearance. The scanner does not cover OS/container packages or the separate sing-box binary. |
| Secret scan | Gitleaks 8.30.1 reported no leaks in the accessible one-commit Git history and an export of all proposed publishable files, including untracked additions. Local ignored `.env` was excluded from publication. No remote repository mutation/history rewrite was performed. |
| Compose and scripts | Production, remote-node and disposable-E2E Compose models parsed with generated fixture variables; Python compilation and shell syntax checks passed. No privileged containers or Docker socket mounts are configured. |
| Image availability | Registry manifests for Go 1.27.1-bookworm, PostgreSQL 16.15-alpine and sing-box 1.14.3 resolved for amd64/arm64; their multi-platform digests are pinned. This verifies image availability, not execution. |

## Confirmed on GitHub Actions

[CI run 38564446222](https://github.com/fanat-sira/syranet/actions/runs/38564446222) tested commit [`06e33ffb6feadfbd26ebecab19b75bb1c81802a3`](https://github.com/fanat-sira/syranet/commit/06e33ffb6feadfbd26ebecab19b75bb1c81802a3) on 2026-10-14. Job conclusions and test logs were inspected directly.

| Job / check | Confirmed result |
| --- | --- |
| [Docker infrastructure E2E](https://github.com/fanat-sira/syranet/actions/runs/38564446222/job/115853425087) | **Passed.** Images built and the disposable Compose environment started. `TestPostgresLifecycle`, generated-config and real TCP/UDP/SOCKS suites, `TestPostgresReconciliation`, `TestAccessPipeline` and `TestDatabasePersistence` passed. The pipeline covered verified synthetic TLS, direct/upstream egress, suspension/revocation/rotation, expiry/extension, retry, real port-collision rollback and PostgreSQL/node restart persistence. |
| [Secret scan](https://github.com/fanat-sira/syranet/actions/runs/38564446222/job/115853424334) | **Passed.** GitHub Gitleaks job completed successfully. |
| [Go checks](https://github.com/fanat-sira/syranet/actions/runs/38564446222/job/115853424713) | Formatting, module verification, vet, real-binary/race tests and build **passed**. `govulncheck` **failed** on nine reachable Go standard-library findings with Go 1.27.1; the report identifies Go 1.27.2 as fixed. The later script-syntax step was skipped. |
| Overall workflow | **Failed** because of the Go vulnerability step. Passing Docker E2E is confirmed independently; it is not a claim that the entire workflow passed. |

## Go 1.27.2 correction

CI, the module minimum and the Docker builder are updated to Go 1.27.2. The builder uses the verified multi-platform digest `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`. Documentation uses the same version. No application refactor or vulnerability-check suppression is involved.

Executed locally with the checksum-verified official Go 1.27.2 linux/amd64 toolchain:

- Formatting and `go mod verify`: passed.
- `go vet ./...`: passed.
- `SINGBOX_TEST_BINARY=/tmp/syranet-tools/sing-box go test -race -count=1 ./...`: passed, including real local Hysteria2 TCP/UDP and SOCKS authentication. Docker-only tests skip locally without their disposable environment.
- `go build -trimpath`: passed.
- The exact CI command `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`: **No vulnerabilities found**.
- Python/shell syntax: checked locally; it had been skipped after the vulnerability failure in the original GitHub Go job.

A GitHub run on the corrected commit is still required: the confirmed run above used the older compiler. These local edits are not a remote CI result, and no commit or push was performed by the agent.

## Still unverified in deployment

- Fresh Ubuntu VPS Quick Start, provider firewall behavior, public ACME issuance/automatic renewal and real two-VPS networking.
- Real Telegram BotFather tokens, long polling and private message delivery with both bots. Tests use synthetic transport; no production token was used.
- Backup/restore scripts against deployment data: `pg_dump`/`pg_restore` and recovery cutover were not exercised by the Docker E2E harness.
- GitHub Actions on the Go 1.27.2 correction and actual private vulnerability-reporting settings.
- ARM64 runtime: image availability was checked, but runtime tests used amd64.

The local environment still has no accessible Docker daemon. This limits local reruns; it does **not** undo the confirmed GitHub Docker E2E result.

## Before publishing and public use

1. Review the local diff and licensing/notices. Commit/push only by explicit owner action; this task did neither. Preserve deployment secrets outside tracked files.
2. Publish the reviewed Go 1.27.2 correction explicitly and confirm a new complete CI pass. Rerun Docker E2E when changing deployment/network behavior; its existing GitHub success is already recorded above.
3. Run a fresh VPS Quick Start with test bots and real certificates, verify two-server egress and key revocation, exercise certificate renewal, backup/restore and temporary node/DB outages.
4. Enable private vulnerability reporting on GitHub and apply the description/topics from PORTFOLIO.md through an authorized account. Publish a release only from the reviewed, CI-green corrected commit; the earlier failed workflow is not a release validation.
5. Rotate weak historical credentials during a deliberate legacy migration; reissue password-only Hysteria2 links and keep the verified old deployment backup until cutover succeeds.

The release is not labeled production-ready. Disabled/revoked/expired access is enforced after a successful node apply; unreachable nodes and an offline manager remain an explicit operational limitation.
