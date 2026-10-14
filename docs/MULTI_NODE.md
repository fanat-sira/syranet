# Two nodes and additional entry servers

First deploy the single-node management/entry VPS using README. Every remote node uses the same repository and the isolated node supervisor image. It needs no PostgreSQL or Telegram tokens. Management is the only source of user state.

## Prepare the upstream VPS

Install Docker Engine/Compose, Python 3, Git, OpenSSL and Certbot as in README. Use a domain pointing directly to this second VPS:

```sh
git clone https://github.com/fanat-sira/syranet.git
cd syranet
sudo certbot certonly --standalone -d upstream.example.net
sudo scripts/certificates.sh upstream.example.net
cp deploy/node.env.example .node.env
chmod 600 .node.env
```

Generate an agent token into the private file without exposing it in shell history:

```sh
python3 - <<'PY'
from pathlib import Path
import secrets
p = Path('.node.env')
s = p.read_text()
if '\nNODE_TOKEN=\n' not in s:
    raise SystemExit('Existing token preserved; edit explicitly to rotate it')
p.write_text(s.replace('\nNODE_TOKEN=\n', '\nNODE_TOKEN=' + secrets.token_hex(32) + '\n'))
p.chmod(0o600)
PY
```

Edit `.node.env`: set `NODE_DOMAIN`, `HYSTERIA_PORT` and `MANAGEMENT_BIND_IP` to a private interface accessible to the management VPS. The default binds management only to localhost and is intentionally unreachable remotely. If no private network is available, bind the public interface and **restrict TCP 8443 to the management VPS address** using the provider firewall or Docker-aware host firewall. Docker forwarding can bypass ordinary UFW INPUT rules; verify the provider/network policy from another host.

Allow upstream **443/UDP** only from intended entry servers, **8443/TCP** only from management, **80/TCP** for certificate renewal and the SSH port for administration. Never open PostgreSQL or grant Docker socket access. Set up the certificate renewal hook from TLS.md on this VPS too.

Start the supervisor:

```sh
docker compose --env-file .node.env -f deploy/node.compose.yml up --build -d
```

A new node is not healthy until management applies a valid config. Do not use `--wait` for this bootstrap step. After management connects, run the same command with `--wait --wait-timeout 180` or inspect its health.

## Configure management

On the management VPS:

```sh
cp configs/multi-node.yaml configs/local.yaml
```

Edit the YAML with **both** real node identities, addresses, SNI and ports. The local entry's `agent.url` remains `http://node:8080`. The remote `agent.url` points to the reachable verified HTTPS endpoint, e.g. `https://upstream.example.net:8443`. If the management address differs from the public Hysteria2 hostname, its certificate must cover the management hostname too, or use an HTTPS reverse proxy with a suitable certificate. HTTPS verification is never disabled.

Transfer `NODE_TOKEN` from the remote node into `UPSTREAM_AGENT_TOKEN` in the management `.env` over a secure channel. Do not send it through a public issue, group chat or command line. Generate the transit secret **on management**:

```sh
python3 - <<'PY'
from pathlib import Path
import secrets
p = Path('.env')
s = p.read_text()
if '\nUPSTREAM_TRANSIT_PASSWORD=\n' not in s:
    raise SystemExit('Existing transit password preserved')
p.write_text(s.replace('\nUPSTREAM_TRANSIT_PASSWORD=\n', '\nUPSTREAM_TRANSIT_PASSWORD=' + secrets.token_hex(32) + '\n'))
p.chmod(0o600)
PY
./deploy.sh
```

Management applies the upstream's direct-egress config with its transit password, then the entry's Hysteria2 outbound with the **same** password and verified SNI. No manually prepared upstream JSON or master key is required. Check `/nodes` for both nodes, then confirm an actual client's traffic exits from the upstream. Process/config health alone does not prove upstream reachability; the disposable E2E pipeline verifies source addresses to test the routing.

## Multiple selectable entries

Deploy another remote node the same way. In `configs/local.yaml`, add a unique `id`, display `name`, `entry: true`, `enabled: true`, public endpoint/TLS, unique HTTPS agent URL and an independent `token_env`, such as `SECOND_ENTRY_AGENT_TOKEN`. Put its token in management `.env`; Compose forwards custom env names. Give it `outbound: direct` or an upstream ID. Multiple entries may use the same upstream.

Recreate management after changes. Users run `/servers` then `/select <node_id>`. Their credential moves from the old entry's desired config to the new one. Sync errors are reported and retried. Each node must be contacted to remove old access; unreachable nodes can retain an old config until recovery.

Disable a node and confirm its drained state before deleting its definition. There is no distributed atomic swap across several servers; brief partial application during failures is reported, not hidden. Cycles and missing/disabled upstream references are rejected. Geographic labels are optional display names with no routing semantics.
