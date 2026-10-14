#!/usr/bin/env python3
"""Interactive setup. Never overwrites credentials; --check is read-only."""
import argparse, getpass, ipaddress, os, pathlib, re, secrets, shutil, socket, subprocess, sys
root=pathlib.Path(__file__).resolve().parent.parent
parser=argparse.ArgumentParser();parser.add_argument('--check',action='store_true');args=parser.parse_args()
os.chdir(root)
def fail(s): raise SystemExit(s)
def dotenv(path):
    values={}
    for line in path.read_text().splitlines():
        if line and not line.startswith('#') and '=' in line:
            k,v=line.split('=',1);values[k]=v.strip().strip("'")
    return values
if not shutil.which('docker'): fail('Install Docker Engine with Compose first.')
if not shutil.which('openssl'): fail('Install openssl first.')
if subprocess.run(['docker','compose','version'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode: fail('Docker Compose is required.')
if args.check:
    if not (root/'.env').is_file(): fail('Missing .env. Run scripts/setup.py first.')
    values=dotenv(root/'.env')
    required=['DB_PASSWORD','USER_BOT_TOKEN','ADMIN_BOT_TOKEN','ADMIN_IDS','ENTRY_AGENT_TOKEN']
    if any(not values.get(k) for k in required): fail('Required secrets/admin IDs missing in .env.')
    if subprocess.run(['docker','compose','config','--quiet'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode: fail('Compose configuration invalid.')
    cfg=root/values.get('PLATFORM_CONFIG','configs/local.yaml')
    if not cfg.is_file(): fail('Missing platform YAML.')
    certs=root/values.get('CERTS_DIR','certs')
    if not all((certs/n).is_file() for n in ['fullchain.pem','privkey.pem']): fail('TLS PEM files missing; run scripts/certificates.sh.')
    if subprocess.run(['openssl','x509','-checkend','86400','-noout','-in',str(certs/'fullchain.pem')],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode: fail('TLS certificate expires within 24h or cannot be read.')
    if (root/'.env').stat().st_mode & 0o077: fail('.env must have mode 0600.')
    # No container changes: check port only when this deployment is not running.
    active=subprocess.run(['docker','compose','ps','-q','node'],capture_output=True,text=True)
    if not active.stdout.strip():
        s=socket.socket(socket.AF_INET6,socket.SOCK_DGRAM)
        try:s.bind(('::',int(values.get('HYSTERIA_PORT','443'))))
        except OSError: fail('Hysteria2 UDP port is occupied or not accessible.')
        finally:s.close()
    print('Setup files and TLS validity checked. Validate YAML with: docker compose run --rm --no-deps manager -check')
    sys.exit()
if (root/'.env').exists() or (root/'configs/local.yaml').exists(): fail('Existing deployment preserved. Use --check; edit configuration explicitly to update.')
domain=input('Server hostname (certificate must cover it): ').strip()
if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]',domain) or '..' in domain: fail('Invalid hostname.')
port_text=input('Hysteria2 UDP port [443]: ').strip() or '443'
if not port_text.isdecimal() or not 1<=int(port_text)<=65535: fail('Invalid port.')
user=getpass.getpass('User bot token (hidden): ').strip();admin=getpass.getpass('Admin bot token (hidden): ').strip();ids=input('Admin Telegram numeric IDs (comma separated): ').strip()
if not re.fullmatch(r'[0-9]+:[A-Za-z0-9_-]+',user) or not re.fullmatch(r'[0-9]+:[A-Za-z0-9_-]+',admin) or user==admin: fail('Two distinct valid BotFather tokens required.')
if not all(i.strip().isdigit() and int(i)>0 for i in ids.split(',')): fail('Invalid admin IDs.')
os.umask(0o077)
values=(root/'.env.example').read_text()
for key,value in {'DB_PASSWORD':secrets.token_hex(32),'ENTRY_AGENT_TOKEN':secrets.token_hex(32),'USER_BOT_TOKEN':user,'ADMIN_BOT_TOKEN':admin,'ADMIN_IDS':ids,'HYSTERIA_PORT':port_text}.items():
    values=re.sub(r'^'+key+r'=.*$',lambda m:key+'='+value,values,flags=re.M)
config=(root/'configs/single-node.yaml').read_text().replace('vpn.example.com',domain).replace('    port: 443','    port: '+port_text)
# Exclusive create prevents races and accidental secret replacement.
with (root/'.env').open('x') as f: f.write(values)
with (root/'configs/local.yaml').open('x') as f: f.write(config)
print('Created private .env and configs/local.yaml. Next: obtain TLS certificates (README), then ./deploy.sh.')
