# Syranet: Self-hosted VPN/Proxy Access Management

Suggested GitHub description:

> Self-hosted Hysteria2 access management with Go, Telegram bots, PostgreSQL, configurable node routing and reconciled sing-box configuration.

Suggested topics: `go`, `telegram-bot`, `self-hosted`, `hysteria2`, `sing-box`, `postgresql`, `docker-compose`, `access-management`, `proxy`.

Syranet grew from a personal infrastructure tool into a configurable self-hosted access management project. A Go management service handles registration and timed subscriptions through two Telegram bots. PostgreSQL stores user state, transactional audit records and desired/applied configuration status. Small authenticated node supervisors validate and atomically install generated sing-box configurations, restart their owned process, attempt rollback and reconcile pending changes. Entry nodes can exit directly or route through a configured Hysteria2 upstream.

The project demonstrates secure credential provisioning, private-chat authorization, node topology validation, lifecycle state transitions, migrations, reproducible deployment and failure recovery. It is a proxy access management tool; client operating-system traffic capture depends on the chosen Hysteria2 client. It has no payment integration, commercial service claims or SLA.

Only publish measured results from VERIFICATION.md. Docker E2E and the secret scan are confirmed on GitHub; the full original workflow failed on Go 1.27.1 vulnerabilities, and the Go 1.27.2 correction still needs a new remote run. Repository metadata remains proposed here; it has not been modified remotely in this session.

## Apply repository metadata

The description/topics above can be applied by an authorized maintainer without committing or pushing code:

```sh
gh auth login
gh repo edit fanat-sira/syranet \
  --description 'Self-hosted Hysteria2 access management with Go, Telegram bots, PostgreSQL, configurable node routing and reconciled sing-box configuration.' \
  --add-topic go \
  --add-topic telegram-bot \
  --add-topic self-hosted \
  --add-topic hysteria2 \
  --add-topic sing-box \
  --add-topic postgresql \
  --add-topic docker-compose \
  --add-topic access-management \
  --add-topic proxy
gh api repos/fanat-sira/syranet --jq '{description, topics}'
```

This adds the proposed topics without deleting other existing topics. The current session has no authorized CLI/API channel for repository administration and no connected browser; these commands are prepared, not reported as executed.

## Release candidate

Release notes are prepared in [RELEASE_NOTES.md](RELEASE_NOTES.md). After the Go 1.27.2 correction is explicitly committed/published by the owner and its complete CI passes, select that exact verified commit:

```sh
gh run list --repo fanat-sira/syranet --workflow CI --branch main --limit 5
# Set this to the full SHA from the reviewed, successful Go 1.27.2 CI run.
release_sha='<verified-green-commit-sha>'
gh release create v0.1.0-rc.1 \
  --repo fanat-sira/syranet \
  --target "$release_sha" \
  --draft --prerelease \
  --title 'Syranet v0.1.0-rc.1' \
  --notes-file docs/RELEASE_NOTES.md
```

Review the draft before publishing. No release/tag was created by the agent; the old failing Go 1.27.1 commit is not used as a release target. Version `v0.1.0-rc.1` is a proposed release-candidate name, not an existing tag or a production-readiness claim.
