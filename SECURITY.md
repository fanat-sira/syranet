# Security

Syranet manages network credentials. Do not send tokens, private keys, database dumps or full client links in public issues.

Report a vulnerability through GitHub **Security → Report a vulnerability** when private reporting is enabled: https://github.com/fanat-sira/syranet/security/advisories/new . Include the affected revision, a minimal reproduction using synthetic data, expected behavior and impact. If private reporting is unavailable, open an issue asking the maintainer for a private contact channel without exploit details. Do not publish working credentials or private deployment addresses.

The release process must enable private vulnerability reporting before public deployment is encouraged. No response SLA is promised.

Supported security boundaries:

- Telegram credentials are issued only in a matching private user chat. Administrator actions require an allowlisted positive Telegram ID and a private chat. No administrative callback payload is accepted.
- Node agents run without root, host mounts, Docker socket or host process control. Their API accepts only a bounded generated config with fixed certificate paths and supported protocols.
- Remote node APIs require certificate verification and bearer authentication. Local plaintext API traffic stays on an unpublished Compose control network; operators must not publish that port.
- Hysteria2 uses verified TLS. SOCKS5 is optional and unencrypted; never treat it as a protected tunnel.
- Database mutations and audit entries share a transaction. Replayed Telegram administrative messages are deduplicated persistently.
- Configuration changes restart the owned sing-box process, terminating existing sessions. Failed restarts attempt to restore the previous configuration. Pending changes retry periodically.

Operational limits: a node unreachable by management can retain its last applied config until reconciliation recovers. A stopped manager cannot enforce newly expired subscriptions. Monitor health and do not rely on offline revocation. Root on a VPS and compromise of the management service/agent token are outside the protection boundary; management necessarily has access to provisioned credentials. PostgreSQL, backups and node state contain plaintext credentials and need restricted access and encrypted off-host backup.

Run tests and vulnerability checks before release. Passing local tests does not establish the security of a real deployment or of repository history.
