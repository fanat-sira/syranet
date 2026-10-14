# Syranet: Self-hosted VPN/Proxy Access Management

Suggested GitHub description:

> Self-hosted Hysteria2 access management with Go, Telegram bots, PostgreSQL, configurable node routing and reconciled sing-box configuration.

Suggested topics: `go`, `telegram-bot`, `self-hosted`, `hysteria2`, `sing-box`, `postgresql`, `docker-compose`, `access-management`, `proxy`.

Syranet grew from a personal infrastructure tool into a configurable self-hosted access management project. A Go management service handles registration and timed subscriptions through two Telegram bots. PostgreSQL stores user state, transactional audit records and desired/applied configuration status. Small authenticated node supervisors validate and atomically install generated sing-box configurations, restart their owned process, attempt rollback and reconcile pending changes. Entry nodes can exit directly or route through a configured Hysteria2 upstream.

The project demonstrates secure credential provisioning, private-chat authorization, node topology validation, lifecycle state transitions, migrations, reproducible deployment and failure recovery. It is a proxy access management tool; client operating-system traffic capture depends on the chosen Hysteria2 client. It has no payment integration, commercial service claims or SLA.

Only publish measured results from VERIFICATION.md. Do not imply that a prepared CI workflow or Docker test harness has already passed on GitHub. Repository metadata is proposed here, not modified remotely.
