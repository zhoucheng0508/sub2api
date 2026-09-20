"""Run real host/plugin contract tests with local fixtures and an ephemeral test signer.

Builds the vendored plugin for this machine. No real accounts, deployment config,
publisher trust settings or upstream traffic are used. Requires Go and OpenSSL.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
HOST = ROOT.parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--openssl', default=shutil.which('openssl') or 'openssl')
    args = parser.parse_args()
    goos = {'Windows': 'windows', 'Linux': 'linux', 'Darwin': 'darwin'}[platform.system()]
    arch = {'amd64': 'amd64', 'x86_64': 'amd64', 'arm64': 'arm64', 'aarch64': 'arm64'}[platform.machine().lower()]
    target = f'{goos}-{arch}'
    with tempfile.TemporaryDirectory(prefix='state-kit-contract-') as temp:
        out = Path(temp)
        binary = out / ('state-kit.exe' if goos == 'windows' else 'state-kit')
        subprocess.run(['go', 'build', '-buildvcs=false', '-o', str(binary), './cmd/state-kit'], cwd=ROOT / 'plugin', check=True)
        runtime_path = f'runtimes/{target}/{binary.name}'
        files = {runtime_path: binary.read_bytes()}
        for file in (ROOT / 'plugin/ui').rglob('*'):
            if file.is_file():
                files[file.relative_to(ROOT / 'plugin').as_posix()] = file.read_bytes()
        manifest = {
            'schema_version': 1, 'id': 'io.github.wangyunjeff.sub2api-state-kit',
            'name': 'STATE Kit local contract fixture', 'version': '0.3.4',
            'description': 'Ephemeral locally signed integration-test fixture; not a release.',
            'author': 'Local test fixture',
            'requires': {'sub2api': '>=0.2.7 <0.3.0', 'recommended_sub2api_version': '0.2.7',
                         'tested_sub2api_versions': ['0.2.7'], 'plugin_protocol': 1, 'transport_api': 1, 'ui_bridge': 1},
            'capabilities': [{'id': 'openai.oauth.outbound_transport.v1', 'platform': 'openai', 'account_type': 'oauth'}],
            'runtimes': {target: {'path': runtime_path}}, 'ui': {'entrypoint': 'ui/index.html'},
            'files': {name: hashlib.sha256(data).hexdigest() for name, data in sorted(files.items())},
        }
        manifest_bytes = (json.dumps(manifest, ensure_ascii=False, indent=2) + '\n').encode()
        manifest_path = out / 'manifest.json'
        manifest_path.write_bytes(manifest_bytes)
        key = out / 'test-only-private.pem'
        subprocess.run([args.openssl, 'genpkey', '-algorithm', 'ED25519', '-out', str(key)], check=True)
        public_der = subprocess.check_output([args.openssl, 'pkey', '-in', str(key), '-pubout', '-outform', 'DER'])
        signature = out / 'signature.bin'
        subprocess.run([args.openssl, 'pkeyutl', '-sign', '-inkey', str(key), '-rawin', '-in', str(manifest_path), '-out', str(signature)], check=True)
        files['manifest.json'] = manifest_bytes
        files['signature.json'] = json.dumps({'algorithm': 'ed25519', 'key_id': 'state-kit-release-v1',
                                             'signature': base64.b64encode(signature.read_bytes()).decode()}).encode()
        package = out / 'local-test-only.s2plugin'
        with zipfile.ZipFile(package, 'w', zipfile.ZIP_DEFLATED) as archive:
            for name, data in sorted(files.items()):
                info = zipfile.ZipInfo(name)
                info.create_system = 3
                info.external_attr = (0o100755 if name == runtime_path else 0o100644) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, data)
        env = {**os.environ, 'STATE_KIT_TEST_PACKAGE': str(package),
               'STATE_KIT_TEST_PUBLIC_KEY': base64.b64encode(public_der[-32:]).decode()}
        result = subprocess.run(['go', 'test', '-json', '-tags=statekit_host_contract', './internal/service',
                                 '-run', '^TestStateKitOfficialHost', '-count=1', '-timeout=4m'], cwd=HOST / 'backend', env=env)
        return result.returncode


if __name__ == '__main__':
    raise SystemExit(main())
