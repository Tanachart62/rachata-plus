#!/usr/bin/env python3
"""Package only deployment code and migrations, never .env/media/backups/keys."""
import argparse
import hashlib
import json
from pathlib import Path
import tarfile

root = Path(__file__).resolve().parents[2]
parser = argparse.ArgumentParser()
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
target = args.output.resolve()
if target.exists():
    raise SystemExit('Output already exists; choose a new release filename')
target.parent.mkdir(parents=True, exist_ok=True)
files = ['compose.aws.yaml', 'deploy/aws/nginx.conf', 'deploy/aws/bootstrap.py',
         'deploy/aws/database.py', 'deploy/aws/migration-order.txt']
files += ['db/migrations/' + line.split()[1] for line in (root / 'deploy/aws/migration-order.txt').read_text().splitlines() if line.strip()]
with tarfile.open(target, 'w:gz', format=tarfile.PAX_FORMAT) as archive:
    for name in files:
        path = root / name
        if not path.is_file() or path.is_symlink():
            raise SystemExit(f'Missing/unsafe release input: {name}')
        archive.add(path, arcname=name, recursive=False)
digest = hashlib.sha256(target.read_bytes()).hexdigest()
manifest = {'file': target.name, 'sha256': digest, 'files': files}
target.with_suffix(target.suffix + '.manifest.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'release': str(target), 'sha256': digest, 'fileCount': len(files)}, indent=2))
