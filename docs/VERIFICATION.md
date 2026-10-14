# Verification record

Date: 2026-10-14. This records local execution, not a GitHub CI run or a production rollout. No author's VPN servers, production credentials or deployment volumes were used. The existing private `.env` was preserved.

## Executed successfully

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
| Go dependency scan | `govulncheck` v1.9.0 reported no vulnerabilities in the scanned Go packages/dependencies. This does not scan OS/container packages. |
| Secret scan | Gitleaks 8.30.1 reported no leaks in the accessible one-commit Git history and an export of all proposed publishable files, including untracked additions. Local ignored `.env` was excluded from publication. No remote repository mutation/history rewrite was performed. |
| Compose and scripts | Production, remote-node and disposable-E2E Compose models parsed with generated fixture variables; Python compilation and shell syntax checks passed. No privileged containers or Docker socket mounts are configured. |
| Image availability | Registry manifests for Go 1.27.1-bookworm, PostgreSQL 16.15-alpine and sing-box 1.14.3 resolved for amd64/arm64; their multi-platform digests are pinned. This verifies image availability, not execution. |

## Prepared, not executed here

- `python3 scripts/e2e.py`: builds the production/node/test images in an isolated random Compose project, uses fresh random secrets/one-day test certificates, exercises the DB suite and real local TCP/UDP suite, then tests the full DB → reconciliation → node HTTPS/HTTP API → Hysteria2 pipeline, direct/upstream source addresses, suspension, revoke/rotation, expiry/extension, failure recovery, a real port-collision rollback and persistence after PostgreSQL/node restart. Only its disposable project/volumes are removed.
- Actual Docker image build/start, read-only-root/UID behavior and all-container full-pipeline/restart checks. The current environment has a Docker CLI/Compose but **no accessible Docker daemon/socket**. The Docker harness compiles and its Compose model parses; its runtime results remain unverified.
- Backup/restore scripts against a running Compose PostgreSQL service: syntax was checked, but `pg_dump`/`pg_restore` and recovery cutover were not executed here.
- Fresh Ubuntu VPS Quick Start, provider firewall behavior, public ACME issuance/automatic renewal and real two-VPS networking.
- Real Telegram BotFather tokens, long polling and private message delivery with both bots. Tests verify authorization and request payloads using synthetic Telegram transport; no production token was used.
- GitHub Actions execution and private security-reporting availability/settings. Workflows and reporting procedure are prepared locally; repository metadata/visibility were not changed.
- ARM64 runtime: image availability was checked, but runtime tests used amd64.

## Before publishing and public use

1. Review the local diff and licensing/notices. Commit/push only by explicit owner action; this task did neither. Preserve deployment secrets outside tracked files.
2. Run the Docker E2E harness from a host with Docker Engine; fix any image/runtime failures before calling deployment validated.
3. Run a fresh VPS Quick Start with test bots and real certificates, verify two-server egress and key revocation, exercise certificate renewal, backup/restore and temporary node/DB outages.
4. Enable private vulnerability reporting on GitHub and review the proposed description/topics in PORTFOLIO.md. Verify actual CI results after publishing; a workflow file is not a CI pass.
5. Rotate weak historical credentials during a deliberate legacy migration; reissue password-only Hysteria2 links and keep the verified old deployment backup until cutover succeeds.

The release is not labeled production-ready. Disabled/revoked/expired access is enforced after a successful node apply; unreachable nodes and an offline manager remain an explicit operational limitation.
