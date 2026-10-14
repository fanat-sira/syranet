# Operations

## Readiness and sync

```sh
docker compose ps
docker compose logs --tail=100 manager node
docker compose exec manager /usr/local/bin/syranet health
docker compose exec node /usr/local/bin/syranet health
```

Node readiness confirms a running child, a valid currently loaded certificate and no pending supervisor error. Management health confirms a recent successful reconciliation; `/nodes` displays desired/applied hashes, the last applied timestamp and pending errors. A child running with matching config is not proof that a remote upstream or the public network is reachable; test a real client.

Every eligible subscription is checked on each reconciliation tick (default 15s). Expiry removes credentials from generated config. Changed config restarts the process and drops existing sessions. Transient failures remain visible in PostgreSQL and retry without another administrative command. A saved user change can temporarily differ from running access during a node outage. Never describe such a pending change as enforced revocation.

## Backup verification and restore

```sh
scripts/backup.sh
sha256sum -c backups/syranet-<timestamp>-<pid>.dump.sha256
scripts/restore.sh backups/syranet-<timestamp>-<pid>.dump syranet_restore_check
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d syranet_restore_check -c "SELECT status,count(*) FROM users GROUP BY status;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d syranet_restore_check -c "SELECT name,applied_at FROM schema_migrations ORDER BY name;"'
```

Use a new database name. Restore never uses DROP or `--clean`; `createdb` fails if a name exists, and `pg_restore` uses one transaction. An unsuccessful restore can leave a new empty database; it does not overwrite the configured database. Verify user counts, statuses, migration checksums and required grants before cutover. Avoid querying passwords in support sessions/logs.

For an intentional recovery cutover, stop management, update its `DB_NAME` to the verified restored name while keeping the existing database role/password, then recreate management. Do not stop/remove the PostgreSQL volume. Verify `/nodes` and a client. The old database remains available for a separately planned rollback. Preserve `.env`, topology, public CA material and node certificate/key files in encrypted off-host backup; a database dump alone is not a complete deployment backup.

## Updates and legacy migration

Migration SQL is embedded and checksum-verified. First startup on a legacy database executes the baseline idempotent schema creation, then the universal-node migration. `approved` maps to `active`; passwords are preserved; user/node and sync state are added. Unsupported/custom legacy statuses block the constraint migration and roll back the transaction; resolve them explicitly after backup. Already applied SQL files must not be edited.

The old host-managed deployment and old `username:password` links are incompatible with this containerized topology/password-only link format. Keep the old service/config and database backup separately until the replacement is verified. Do not mount the old `/etc/sing-box` into the new containers. Remove shared legacy/fallback access deliberately; it is not imported into the new node configs. Reissue links through the new bot. Old 64-bit generated passwords are preserved for compatibility; rotate them for the new 256-bit credential format.

Setup never overwrites your existing `.env`. For a deliberate legacy conversion, archive it with mode 0600 outside version control, compare needed values against `.env.example`, create the new topology/secrets explicitly and preserve a verified database backup. Do not reuse historical infrastructure addresses blindly. The old deployment volume may have a different Compose project/volume name; attaching it or restoring a dump is an explicit migration decision, not an automatic setup action.

There are no automatic down migrations or data rollback. A config restart failure attempts node-local file/process rollback; a schema/application rollback can need a separate restored database and the matching old binary.

## Frequent failures

| Symptom | Checks / recovery |
| --- | --- |
| Invalid TLS certificate | Check SAN/SNI, expiry, full chain, key match, UID 10001 readability and client CA trust. Run `openssl x509 -in certs/fullchain.pem -noout -dates -ext subjectAltName`. Never add insecure TLS. |
| Port already in use | Inspect `sudo ss -lntup`; the public mapping and YAML listener must match. Do not stop a host VPN without an explicit migration plan. |
| Invalid Telegram token | Use two different BotFather tokens; update `.env`, recreate management, and check outbound HTTPS/DNS. Clear an old webhook using BotFather/Telegram's API if polling reports a conflict. Do not paste tokens into public logs or URLs in shell history. |
| Database unavailable | Confirm PostgreSQL health and the retained role/database. Changing environment does not change an existing role's password. Do not delete the volume. |
| sing-box config rejected | Confirm the pinned version, supported YAML options and readable certificate files. Unknown/unsupported generated JSON is rejected before replacement. Inspect `/nodes`; errors are intentionally secret-free. |
| Upstream unreachable | Check upstream process/TLS, UDP port/firewall, transit secret and SNI. The HTTPS control port working does not establish UDP reachability. |
| Key generated but client fails | Check approval/expiry, selected entry, applied hash, DNS/SNI, UDP provider firewall, client Hysteria2 support and certificate trust. The auth component is the exact password, not a name/password alias. |
| Config sync failed | Check agent token, management address/HTTPS trust, certificate availability, state directory ownership/free space and child process. Reconciliation retries; DB changes remain pending until confirmed. |
| Subscription expired | `/extend <id> <days>` explicitly extends/reactivates expired access. `/activate` cannot silently extend it. |
| Revoked user cannot activate | `/rotate <id>`, extend if necessary, then `/activate <id>` for an unexpired suspended subscription. Old credentials must remain invalid. |
| Notifications missing | The allowlisted administrator must start the private admin bot chat. Reminder/alert deduplication is in memory and resets on manager restart. |
| Node disabled but still reachable | Keep it configured as disabled until its empty config is applied; confirm health/hash and a denied old key before removing its definition. Unreachable agents cannot enforce new changes. |

## Monitoring limits

Legacy Clash connection/device counting and automatic disconnection have been removed. Process/TLS/sync status and subscription counts are available; no count of distinct client devices is claimed. `last_success` is the most recent confirmed config application/start, not measured public uptime. Success does not establish bandwidth, availability or a service SLA.
