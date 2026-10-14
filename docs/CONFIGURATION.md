# Configuration

Precedence is deliberately narrow: YAML owns policy/topology; environment owns database/bot secrets and the secrets named by each node. YAML secret values are rejected as unknown fields. Compose deployment parameters control its mounts and port mappings; they are not another topology format.

## Platform YAML

| Setting | Meaning / allowed values |
| --- | --- |
| `app.log_level` | `info`, `warn`, `error` |
| `app.reconcile_interval` | Duration, minimum `1s`; default `15s` |
| `app.shutdown_timeout` | Duration, minimum `1s`; default `15s` |
| `policy.subscription_days` | Initial approval duration, 1–3650 days |
| `policy.reminder_threshold` | Positive duration before expiry, default `72h` |
| `policy.default_node` | ID of an enabled entry |
| `nodes[].id` | Unique lowercase identifier, 1–32 characters |
| `nodes[].name` | Display name, 1–64 characters without newlines/tabs |
| `enabled`, `entry` | Whether selectable/usable; whether it accepts end users |
| `public_address`, `listen_address`, `port` | Hostname/IP without scheme/port, listener IP, UDP port 1–65535 |
| `tls.server_name` | Certificate identity/SNI, independent of public IP |
| `tls.certificate_path`, `tls.key_path` | Fixed node paths `/certs/fullchain.pem`, `/certs/privkey.pem` |
| `tls.ca_path` | Optional `/certs/ca.pem` used by other nodes when dialing this upstream |
| `agent.url` | Unique per node; verified HTTPS remotely; `http://node:8080` only for local private Compose control |
| `agent.token_env` | Environment key containing a random independent agent token, minimum 32 characters |
| `agent.ca_path` | Optional management-side trusted CA PEM for the HTTPS API |
| `transit_password_env` | Random Hysteria2 secret for nodes used as upstreams, minimum 32 characters |
| `outbound` | `direct` or an enabled node ID with transit credentials; cycles rejected |
| `socks.enabled` | Default false; omit SOCKS inbound when no eligible users exist |
| `socks.listen_address`, `socks.port` | Listener IP and TCP port |
| `socks.allow_public` | Explicit permission for non-private listeners; this does not encrypt SOCKS |

Each node controls one sing-box process. Do not reuse the same agent URL for several node IDs. A user credential is installed only on the selected enabled entry, or the default entry when no selection exists. Upstream-only nodes accept transit credentials, not end-user credentials.

To disable a node, leave it configured with `enabled: false`, reconcile and verify `/nodes`: it is actively drained with no user/transit access. Only then remove its definition/stop its deployment. Deleting a node definition cannot contact the forgotten endpoint to revoke old access. Nodes routing through a disabled upstream must first be rerouted or disabled.

## Secrets and deployment

All checked-in secret values are empty. `setup.py` generates random database/local-agent values. Additional node tokens/transit passwords are generated per deployment and added to private `.env`; manager `env_file` passes custom secret names for any number of nodes. The explicit Compose environment overrides its fixed internal database host/port and the standard keys.

| Environment | Purpose |
| --- | --- |
| `USER_BOT_TOKEN`, `ADMIN_BOT_TOKEN` | Different BotFather tokens, never debug-logged |
| `ADMIN_IDS` | Comma-separated positive numeric Telegram user IDs |
| `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` | Database connection; Compose fixes host/port to its service |
| `DB_SSLMODE` | `disable` on internal Compose network; `verify-full` for external DB |
| `BOT_POLLING_TIMEOUT` | Integer 1–60 seconds |
| `CONFIG_PATH` | Direct binary YAML path; Compose explicitly uses `/app/config.yaml` |
| `COMPOSE_PROJECT_NAME` | Compose deployment namespace; keep it stable across updates |
| `PLATFORM_CONFIG` | Host YAML bind-mount source; default `./configs/local.yaml` |
| `CERTS_DIR` | Host certificate directory; default `./certs` |
| `TRUST_DIR` | Public CA material mounted on management at `/certs`; default `./trust` |
| `HYSTERIA_PORT` | Host/container UDP mapping; match `nodes[].port` |
| `ENTRY_AGENT_TOKEN` | Default local node secret |
| `UPSTREAM_AGENT_TOKEN`, `UPSTREAM_TRANSIT_PASSWORD` | Example remote node secrets; optional in single-node mode |

The standard database pool uses max 10, min 1 connection and a 30-minute connection lifetime. Unsupported legacy tuning and monitoring variables are removed; do not assume obsolete keys still configure anything.

Node-only environment is documented by `deploy/node.env.example`: `NODE_TOKEN`, `NODE_LISTEN`, state/binary paths, HTTPS certificate/key paths and health URL/CA. `NODE_ALLOW_PLAINTEXT=true` is restricted operationally to the unpublished local control network. No insecure TLS option exists.

## SOCKS opt-in

Enabling private SOCKS in YAML makes it listen **inside the node container**, not on the host loopback. To expose it only to the host, set the node listener to `0.0.0.0`, explicitly set `allow_public: true` (the container listener is non-private) and add a Compose override:

```yaml
services:
  node:
    ports:
      - '127.0.0.1:1080:1080/tcp'
```

Keep host publication private. `allow_public` is a deliberate acknowledgement of an unencrypted listener; it is not a recommendation to publish it to the internet. Use separate credentials and a trusted private transport/network. The default deployment and Telegram key flow use Hysteria2.

## Protocol compatibility

Pinned source and official specifications:

- [sing-box Hysteria2 inbound](https://sing-box.sagernet.org/configuration/inbound/hysteria2/): `users[].name` and `users[].password`.
- [Hysteria2 URI scheme](https://v2.hysteria.network/docs/developers/URI-Scheme/): the password is percent-encoded as URI userinfo. Syranet does not use a separate `username:password` userpass alias.
- [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/hysteria2/): the same transit password, configured server port and verified TLS/SNI.
- [Pinned sing-box source](https://github.com/SagerNet/sing-box/tree/v1.14.3).

A synthetic stored password, generated server user and issued URI are tested together. IPv6 public endpoints use bracketed URI authorities. No JSON text substitution or shell command is used to generate a config.
