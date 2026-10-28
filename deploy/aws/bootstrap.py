#!/usr/bin/env python3
"""Run on a prepared Amazon Linux 2023 x86_64 AMI, as root; never print secrets."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import urllib.request

def command(args, **kwargs):
    try:
        return subprocess.run(args, check=True, text=True, capture_output=True, **kwargs).stdout.strip()
    except subprocess.CalledProcessError as error:
        # Never include captured stdout: GetSecretValue returns a password there.
        raise RuntimeError(f'{args[0]} {args[1]} failed: {error.stderr.strip()}') from None

parser = argparse.ArgumentParser()
parser.add_argument('--role', choices=['web', 'worker'], required=True)
parser.add_argument('--region', required=True)
parser.add_argument('--secret', required=True)
parser.add_argument('--db-host', required=True)
parser.add_argument('--bucket', required=True)
parser.add_argument('--origin', required=True)
parser.add_argument('--api-image', required=True)
parser.add_argument('--web-image', required=True)
parser.add_argument('--worker-image', required=True)
parser.add_argument('--log-group', required=True)
parser.add_argument('--prepare-only', action='store_true', help='Migration host: fetch config/images without starting services')
args = parser.parse_args()
root = Path('/opt/rachata-plus')
os.chdir(root)
os.umask(0o077)
if not re.fullmatch(r'https://[A-Za-z0-9.-]+', args.origin):
    raise SystemExit('An HTTPS origin without path is required')
images = [args.api_image, args.web_image, args.worker_image]
for image in images:
    if not re.fullmatch(r'[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com/[a-z0-9_./-]+@sha256:[a-f0-9]{64}', image):
        raise SystemExit('Use immutable private ECR image digests')
secret = json.loads(command(['aws', 'secretsmanager', 'get-secret-value', '--secret-id', args.secret,
                            '--region', args.region, '--query', 'SecretString', '--output', 'text']))
secrets = root / 'secrets'
secrets.mkdir(mode=0o700, exist_ok=True)
values = {
    'APP_ENV': 'production', 'AUTH_ORIGIN': args.origin, 'COOKIE_SECURE': 'true',
    'HTTP_ADDR': '0.0.0.0:8080', 'DB_HOST': args.db_host, 'POSTGRES_PORT': '5432',
    'POSTGRES_DB': secret['dbname'], 'POSTGRES_USER': secret['username'], 'POSTGRES_PASSWORD': secret['password'],
    'DB_SSLMODE': 'verify-full', 'DB_SSLROOTCERT': '/run/secrets/rds-ca.pem', 'DB_MAX_CONNS': '10',
    'AWS_REGION': args.region, 'MEDIA_BACKEND': 's3', 'S3_BUCKET': args.bucket, 'S3_PREFIX': 'media',
    'LOCAL_VIDEO_ENABLED': 'true', 'VIDEO_WORKER_MODE': 'external', 'MEDIA_DELIVERY': 'direct',
    'MEDIA_DIR': '/srv/media', 'MEDIA_JOB_BYTES': str(20 << 30), 'MEDIA_TOTAL_BYTES': str(40 << 30),
    'MEDIA_MIN_FREE_BYTES': str(2 << 30), 'FFMPEG_THREADS': '2',
    'VIDEO_JOB_TIMEOUT': '6h',
    'TRUSTED_PROXY_CIDRS': '172.30.84.0/24', 'GIN_MODE': 'release',
}
if any(not isinstance(v, str) or any(c in v for c in '\r\n\x00') for v in values.values()):
    raise SystemExit('Invalid environment values')
(secrets / 'runtime.env').write_text(''.join(f'{k}={v}\n' for k, v in values.items()), encoding='utf-8')
os.chmod(secrets / 'runtime.env', 0o600)
ca = secrets / 'rds-ca.pem'
with urllib.request.urlopen('https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem', timeout=30) as response:
    ca.write_bytes(response.read())
os.chmod(ca, 0o644)
(root / '.env').write_text(f'API_IMAGE={args.api_image}\nWEB_IMAGE={args.web_image}\nWORKER_IMAGE={args.worker_image}\nAWS_REGION={args.region}\nAWS_LOG_GROUP={args.log_group}\n', encoding='utf-8')
scratch = Path('/srv/rachata-scratch')
scratch.mkdir(mode=0o700, exist_ok=True)
os.chown(scratch, 10001, 10001)
registry = args.api_image.split('/')[0]
if any(image.split('/')[0] != registry for image in images):
    raise SystemExit('All images must use the same ECR registry and region')
command(['aws', 's3api', 'head-bucket', '--bucket', args.bucket, '--region', args.region,
         '--expected-bucket-owner', registry.split('.')[0]])
password = command(['aws', 'ecr', 'get-login-password', '--region', args.region])
command(['docker', 'login', '--username', 'AWS', '--password-stdin', registry], input=password)
compose = ['docker', 'compose', '-p', 'rachata-plus-aws', '-f', 'compose.aws.yaml', '--profile', args.role]
command(compose + ['pull'])
command(['docker', 'logout', registry])
if args.prepare_only:
    print('Migration host prepared; no application containers started.')
    raise SystemExit(0)
# The boot service reruns after reboot, refreshing secrets and ECR credentials.
command(compose + ['up', '-d', '--no-build', '--remove-orphans'])
print(f'Rachata Plus {args.role} containers started; check /ready and application logs.')
