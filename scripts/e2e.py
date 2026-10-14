#!/usr/bin/env python3
"""Isolated disposable Compose test project. Never loads .env or production volumes."""
import os, pathlib, secrets, shutil, subprocess, tempfile
root=pathlib.Path(__file__).resolve().parent.parent
project='syranet-e2e-'+secrets.token_hex(6)
if not shutil.which('docker') or not shutil.which('openssl'): raise SystemExit('Docker Compose and openssl required')
subprocess.run(['docker','info'],check=True,stdout=subprocess.DEVNULL)
with tempfile.TemporaryDirectory(prefix=project+'-') as temp:
    temp=pathlib.Path(temp);certs=temp/'certs';certs.mkdir(mode=0o755)
    subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-subj','/CN=vpn.example.com','-addext','subjectAltName=DNS:vpn.example.com,DNS:upstream.example.net,DNS:upstream','-keyout',str(certs/'privkey.pem'),'-out',str(certs/'fullchain.pem')],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    shutil.copy(certs/'fullchain.pem',certs/'ca.pem')
    # Synthetic one-day keys only: node runs as 10001 and must read the fixture.
    for f in certs.iterdir(): f.chmod(0o644)
    values={'DB_HOST':'postgres','DB_PORT':'5432','DB_NAME':'syranet_test','DB_USER':'syranet_test','DB_PASSWORD':secrets.token_hex(32),'TEST_DB_PASSWORD':'','USER_BOT_TOKEN':'1:synthetic_fixture','ADMIN_BOT_TOKEN':'2:synthetic_fixture','ADMIN_IDS':'1','ENTRY_AGENT_TOKEN':secrets.token_hex(32),'UPSTREAM_AGENT_TOKEN':secrets.token_hex(32),'UPSTREAM_TRANSIT_PASSWORD':secrets.token_hex(32)}
    values['TEST_DB_PASSWORD']=values['DB_PASSWORD']
    envfile=temp/'test.env';envfile.write_text(''.join(k+'='+v+'\n' for k,v in values.items()));envfile.chmod(0o600)
    cfg=(root/'configs/multi-node.yaml').read_text().replace('public_address: vpn.example.com','public_address: node').replace('public_address: upstream.example.net','public_address: upstream').replace('https://upstream.example.net:8443','https://upstream:8443\n      ca_path: /certs/ca.pem').replace('server_name: upstream.example.net','server_name: upstream.example.net\n      ca_path: /certs/ca.pem')
    (temp/'config.yaml').write_text(cfg)
    env=os.environ.copy()
    # Inherited credentials/Compose options cannot redirect this test to another deployment.
    for key in list(env):
        if key.startswith(('COMPOSE_','DB_','USER_BOT','ADMIN_BOT','ADMIN_IDS','ENTRY_AGENT','UPSTREAM_','TEST_DB','SYRANET_')): env.pop(key)
    env.update(values);env['SYRANET_E2E_DIR']=str(temp)
    compose=['docker','compose','--project-name',project,'--env-file',str(envfile),'-f',str(root/'tests/e2e/compose.yml')]
    def run(*args): subprocess.run(compose+list(args),env=env,cwd=root,check=True)
    try:
        run('build')
        run('up','-d','--wait','postgres','node','upstream')
        run('run','--rm','--use-aliases','tester','test','./internal/database','-count=1','-v')
        run('run','--rm','--use-aliases','tester','test','./internal/service','-count=1','-v')
        run('run','--rm','--use-aliases','tester','test','./tests/e2e','-run','TestAccessPipeline','-count=1','-v')
        run('restart','postgres','node','upstream')
        run('up','-d','--wait','postgres')
        run('run','--rm','--use-aliases','tester','test','./tests/e2e','-run','TestDatabasePersistence','-count=1','-v')
        print('Isolated database, migrations, TCP/UDP, TLS, upstream, revocation, reconciliation and restart checks passed.')
    finally:
        # Exact random test project only; never a global prune or production down.
        subprocess.run(compose+['down','--volumes','--remove-orphans'],env=env,cwd=root,check=True)
