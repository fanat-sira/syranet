# TLS certificates and renewal

Hysteria2 traffic and remote node management use certificate verification. Clients use the configured `server_name` as SNI, even when the transport address is an IP. A hostname certificate, its full chain and its matching private key are required in the default scenario.

## Certbot on a fresh Ubuntu VPS

Create DNS A/AAAA records pointing directly to the server. Keep TCP 80 available to the standalone HTTP-01 challenge and allow it at the provider firewall:

```sh
sudo apt-get update
sudo apt-get install -y certbot
sudo systemctl enable --now certbot.timer
sudo certbot certonly --standalone -d vpn.example.com
sudo scripts/certificates.sh vpn.example.com
sudo certbot renew --dry-run
```

Use your own domain. Certificate requests go to your chosen ACME provider; the application does not perform ACME issuance. For an occupied TCP 80 listener, use an existing webroot/DNS authenticator following the [official Certbot guide](https://eff-certbot.readthedocs.io/en/stable/using.html). Do not stop a production webserver implicitly.

For a custom `CERTS_DIR`, pass it as the second argument to `scripts/certificates.sh <domain> <directory>` and use the same path in the renewal hook.

The copy script checks hostname/expiry and certificate-key public-key equality, installs the key as mode 0600 owned by container UID 10001 and the chain as mode 0644. It replaces files in the directory mounted by the node. A temporary two-file mismatch causes validation failure/retry, never an insecure fallback.

## Renewal deployment hook

Record an absolute repository path and your domain in a root-owned executable hook. Example (replace `/opt/syranet` with your real checkout):

```sh
sudo install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
sudo tee /etc/letsencrypt/renewal-hooks/deploy/syranet >/dev/null <<'SH'
#!/bin/sh
set -eu
# Only deploy the intended certificate lineage.
[ "$RENEWED_LINEAGE" = /etc/letsencrypt/live/vpn.example.com ] || exit 0
/opt/syranet/scripts/certificates.sh vpn.example.com
SH
sudo chmod 700 /etc/letsencrypt/renewal-hooks/deploy/syranet
sudo certbot renew --dry-run --run-deploy-hooks
systemctl status certbot.timer
```

Check `/nodes` and node health after a real renewal. Management's periodic status request sees the certificate fingerprint change and reapplies the same config, restarting sing-box. The remote HTTPS server loads the current pair for new handshakes. If management is offline, copied Hysteria2 certificates are not applied until it returns; alert on expiry and stale reconciliation.

Do not give root a renewal hook in a checkout writable by untrusted users. The hook grants its referenced script root execution; restrict the checkout/parent permissions or install a reviewed root-owned copy of the certificate deployment script.

## Existing certificates / private CA

Provide `fullchain.pem` and `privkey.pem` inside `CERTS_DIR`, readable by UID 10001; keep the key mode 0600. `tls.server_name` must match the leaf's SAN. The node validates key pairing, hostname and validity before applying a config; clients validate chain trust.

For a private CA upstream, mount the issuing CA chain at `/certs/ca.pem` on every entry that dials it and configure the upstream node's `tls.ca_path: /certs/ca.pem`. For its remote management API, also place the public CA in management’s `TRUST_DIR` (default `trust/ca.pem`, mounted at `/certs/ca.pem`) and set `agent.ca_path` to that management-side PEM. End-user clients must import the private CA through their own supported client configuration: a plain URI does not distribute a CA file. Publicly trusted certificates are the simple Quick Start path.

Never put a private key or real certificate material in tracked fixtures, Docker build context, bot messages or CI artifacts. E2E creates ephemeral one-day self-signed certificates, explicitly trusts them, and keeps TLS verification enabled.
