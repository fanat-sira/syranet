#!/usr/bin/env python3
"""Download the pinned official binary, verify the release asset digest, extract it."""
import hashlib, json, os, pathlib, platform, tarfile, tempfile, urllib.request
version = '1.14.3'
arch = {'x86_64':'amd64','aarch64':'arm64'}.get(platform.machine())
if not arch: raise SystemExit('Unsupported architecture')
release=json.load(urllib.request.urlopen('https://api.github.com/repos/SagerNet/sing-box/releases/tags/v'+version,timeout=30))
asset=next(a for a in release['assets'] if a['name']==f'sing-box-{version}-linux-{arch}.tar.gz')
digest=asset.get('digest','')
if not digest.startswith('sha256:'): raise SystemExit('Official release asset digest unavailable; refusing unverified binary')
target=pathlib.Path(os.environ.get('SINGBOX_DOWNLOAD_DIR','/tmp/syranet-tools'));target.mkdir(parents=True,exist_ok=True)
with tempfile.TemporaryDirectory() as tmp:
    archive=pathlib.Path(tmp)/'singbox.tar.gz'
    urllib.request.urlretrieve(asset['browser_download_url'],archive)
    if hashlib.sha256(archive.read_bytes()).hexdigest()!=digest.split(':',1)[1]: raise SystemExit('Release checksum mismatch')
    with tarfile.open(archive) as tar:
        member=next(m for m in tar.getmembers() if m.isfile() and m.name.endswith('/sing-box'))
        (target/'sing-box').write_bytes(tar.extractfile(member).read())
    (target/'sing-box').chmod(0o755)
print(target/'sing-box')
