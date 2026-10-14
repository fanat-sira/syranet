# Security review of the replacement

This is a code/test review record, not a penetration test or a production certification.

| Area | Result |
| --- | --- |
| Telegram authentication | All user commands require a matching private chat. All admin dispatch requires private chat plus allowlisted ID. Callback updates are ignored; there is no administrative callback execution path. |
| Credential delivery | Bot debug logging disabled; secret messages use protect_content. Admin info/search and notification messages omit passwords/client links and reduce personal fields. |
| Logging | Telegram transport errors are sanitized because request URLs contain bot tokens. sing-box stdout/stderr is not propagated to Telegram, agent responses or application logs. |
| Random secrets | crypto/rand, independent 32-byte secrets per protocol/rotation. Historical passwords are preserved only for migration; rotate them. |
| Hysteria2 | `name`/`password` fields, password-only URI userinfo, percent encoding, IPv6 authorities and matching SNI. Server/URI/DB pairing and real network auth are tested. |
| TLS | No insecure setting. Certificate/key match, validity and hostname checked before apply. HTTPS clients validate trust/SNI; redirect following is disabled to avoid leaking bearer tokens. |
| SOCKS | Disabled by default, private-listener validation, explicit public opt-in, separate credentials. Empty lists omit inbound rather than creating anonymous access. No encryption claim. |
| Database | No published PostgreSQL port in deployment. Parameterized SQL; row locks, transactionally coupled audits and replay deduplication. Isolated real PostgreSQL tests exercise rollback on audit failure. |
| File/command boundaries | API has bounded typed JSON, fixed certificate paths, no command/path execution. Process uses exec arguments without a shell. Atomic writes sync files/directories and use mode 0600. |
| Containers | UID 10001, dropped capabilities, read-only root filesystem and no host/Docker socket/systemd mounts. Management mounts public CA material separately from node private keys. |
| Concurrency | In-process mutex plus DB advisory lock before snapshot. A single statement captures users/revision; the node serializes check/write/restart/rollback. Independent-manager reconciliation is tested. |
| Recovery | Failed checks leave active config untouched; failed restart attempts old config restoration. Desired DB changes persist and retry. Offline node/manager limitations are disclosed. |
| Public repository | Ignore/build allowlists protect local secrets. Scan history and all proposed files before release; ignore rules do not remove previously tracked secrets. |
| Updates | Embedded transactional/checksummed migrations. Backup script validates archive readability; restore targets only a new DB. No global volume deletion or automated schema rollback. |

Residual operational risks: administrator/host compromise, plaintext credentials in the DB/node state/backups, container image supply chain, public SOCKS misconfiguration, unreachable-node revocation delay and restart-related session interruption. These must be assessed in the actual deployment. Remote management endpoints must have verified HTTPS and external network restrictions. Docker/ACME/live Telegram checks are listed separately in VERIFICATION.md.
