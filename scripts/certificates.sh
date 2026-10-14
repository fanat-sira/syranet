#!/bin/sh
# Copies renewed Certbot files into the mounted directory; reconciliation reloads them.
set -eu
cd "$(dirname "$0")/.."
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then echo "Usage: sudo scripts/certificates.sh <domain> [certificate-directory]" >&2; exit 1; fi
target_dir="${2:-certs}"
case "$1" in ''|*[!a-zA-Z0-9.-]*|.*) echo "Invalid domain" >&2; exit 1;; esac
source_dir="/etc/letsencrypt/live/$1"
openssl x509 -in "$source_dir/fullchain.pem" -noout -checkhost "$1"
openssl x509 -in "$source_dir/fullchain.pem" -noout -checkend 86400
mkdir -p -- "$target_dir"
chmod 755 -- "$target_dir"
install -m 644 -- "$source_dir/fullchain.pem" "$target_dir/fullchain.pem.next"
install -m 600 -- "$source_dir/privkey.pem" "$target_dir/privkey.pem.next"
chown 10001:10001 -- "$target_dir/fullchain.pem.next" "$target_dir/privkey.pem.next"
# Confirm certificate and private key match before replacing either file.
cert_pub="$(openssl x509 -in "$target_dir/fullchain.pem.next" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum)"
key_pub="$(openssl pkey -in "$target_dir/privkey.pem.next" -pubout -outform DER | sha256sum)"
if [ "$cert_pub" != "$key_pub" ]; then rm -f -- "$target_dir/fullchain.pem.next" "$target_dir/privkey.pem.next"; echo 'TLS key mismatch' >&2; exit 1; fi
mv -- "$target_dir/privkey.pem.next" "$target_dir/privkey.pem"
mv -- "$target_dir/fullchain.pem.next" "$target_dir/fullchain.pem"
echo 'TLS files installed. Reconciliation checks and restarts the node when the certificate changes.'
