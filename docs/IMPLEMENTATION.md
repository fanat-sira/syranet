# Implementation decisions and audit

The original service assumed a specific two-server topology, host systemd/D-Bus access, unchecked plaintext SOCKS, insecure TLS, a nonstandard Hysteria2 user/URI pairing and best-effort config reload. User updates and audits were not transactional; group chats could receive credentials; invalid environment values could silently preserve defaults; reconciliation and upgrades were missing.

The replacement uses:

- A management Go process, PostgreSQL and two private-chat Telegram bots.
- A small Go node supervisor in the same container as a pinned sing-box child process. No host process, Docker socket or host configuration access.
- Typed node configuration; a node routes directly or through one named upstream. Cycles and disabled/missing targets are rejected. Users select an entry node.
- A bearer-authenticated restricted apply/status API. Local Compose traffic remains on an internal network; remote management requires verified HTTPS. The API accepts generated configs, not commands or filesystem paths.
- Validation before atomic replacement, process restart, readiness check and rollback. Restart deliberately terminates existing sessions when access changes.
- PostgreSQL desired revisions, persisted per-node applied hashes and periodic reconciliation. A database advisory lock serializes snapshot/application across management processes; row locks serialize business transitions.
- Existing certificates, renewed externally by Certbot; no deprecated embedded ACME and no insecure TLS fallback.

Compatibility: backup first. This release changes configuration and issued Hysteria2 links, removes legacy shared fallback credentials and host-managed sing-box integration. Never point it at the old host configuration. Preserve old installations separately until the new isolated deployment is verified. Migration 002 maps `approved` to `active`, adds `revoked`, node selection and sync state. Existing passwords are preserved, but links must be reissued using password-only authentication. Expired subscriptions require explicit extension; revocation requires explicit rotation before activation.

Verification evidence is recorded in VERIFICATION.md. Docker and live Telegram availability must not be inferred from a successful Go build.
