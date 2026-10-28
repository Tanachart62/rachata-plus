#!/usr/bin/env python3
"""Prepare ordinary private S3 buckets without Object Lock APIs; never delete buckets."""
import argparse
import json
import re
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--media-bucket', required=True)
parser.add_argument('--artifacts-bucket', required=True)
parser.add_argument('--region', choices=['us-east-1', 'us-west-2'], default='us-east-1')
parser.add_argument('--profile', default='learner')
parser.add_argument('--create', action='store_true', help='Allow creation of missing buckets; default only checks existing buckets')
args = parser.parse_args()
names = [args.media_bucket, args.artifacts_bucket]
if len(set(names)) != 2 or any(not re.fullmatch(r'[a-z0-9][a-z0-9-]{1,61}[a-z0-9]', name) for name in names):
    raise SystemExit('Use two distinct lowercase bucket names without dots')

def call(command, *, allow_failure=False):
    result = subprocess.run(['aws', '--profile', args.profile, '--region', args.region] + command,
                            capture_output=True, text=True, encoding='utf-8', errors='replace')
    if result.returncode and not allow_failure:
        raise SystemExit(result.stderr.strip())
    return result

account = call(['sts', 'get-caller-identity', '--query', 'Account', '--output', 'text']).stdout.strip()
for name in names:
    head = call(['s3api', 'head-bucket', '--bucket', name, '--expected-bucket-owner', account], allow_failure=True)
    if head.returncode:
        # A 403 is not proof that the bucket is absent. Never create on AccessDenied.
        missing = '(404)' in head.stderr or '(NoSuchBucket)' in head.stderr
        if not missing:
            raise SystemExit(f'{name}: cannot confirm ownership/access; no creation attempted.\n{head.stderr}')
        if not args.create:
            raise SystemExit(f'{name}: missing. Review the name and rerun with --create to create a private bucket.')
        command = ['s3api', 'create-bucket', '--bucket', name, '--object-ownership', 'BucketOwnerEnforced']
        if args.region != 'us-east-1':
            command += ['--create-bucket-configuration', f'LocationConstraint={args.region}']
        call(command)
        call(['s3api', 'put-public-access-block', '--bucket', name, '--expected-bucket-owner', account,
              '--public-access-block-configuration', json.dumps({
                  'BlockPublicAcls': True, 'IgnorePublicAcls': True,
                  'BlockPublicPolicy': True, 'RestrictPublicBuckets': True})])
        print(f'Created private bucket: {name}')
    location = call(['s3api', 'get-bucket-location', '--bucket', name, '--expected-bucket-owner', account,
                     '--output', 'json'])
    region = json.loads(location.stdout).get('LocationConstraint') or 'us-east-1'
    if region != args.region:
        raise SystemExit(f'{name}: expected region {args.region}, found {region}; no bucket changes attempted')
    public = json.loads(call(['s3api', 'get-public-access-block', '--bucket', name,
                             '--expected-bucket-owner', account, '--output', 'json']).stdout)
    required = ['BlockPublicAcls', 'IgnorePublicAcls', 'BlockPublicPolicy', 'RestrictPublicBuckets']
    if not all(public.get('PublicAccessBlockConfiguration', {}).get(key) is True for key in required):
        raise SystemExit(f'{name}: all four Block Public Access settings must be true before continuing')
    print(f'PASS owned private bucket in {region}: {name}')
print('Ready. Set media_bucket_name and artifacts_bucket_name in terraform.tfvars; Terraform manages their configurations.')
