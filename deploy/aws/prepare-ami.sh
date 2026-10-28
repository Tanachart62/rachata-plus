#!/bin/bash
# Run on a new Amazon Linux 2023 x86_64 AMI builder. No app secrets belong in AMI.
set -euo pipefail
sudo dnf install -y docker python3 unzip
command -v curl >/dev/null || { echo 'curl (or curl-minimal) is required.' >&2; exit 1; }
sudo systemctl enable --now docker
command -v aws >/dev/null || { echo 'Install AWS CLI v2 from the official AWS instructions first.' >&2; exit 1; }
version=v2.39.4
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl --fail --location --silent --show-error "https://github.com/docker/compose/releases/download/$version/docker-compose-linux-x86_64" -o "$work/docker-compose-linux-x86_64"
curl --fail --location --silent --show-error "https://github.com/docker/compose/releases/download/$version/checksums.txt" -o "$work/checksums.txt"
(cd "$work"; grep ' docker-compose-linux-x86_64$' checksums.txt | sha256sum --check --strict -)
sudo install -d /usr/local/lib/docker/cli-plugins
sudo install -m 0755 "$work/docker-compose-linux-x86_64" /usr/local/lib/docker/cli-plugins/docker-compose
sudo docker compose version
echo 'Create the AMI before bootstrapping the application or fetching any secrets.'
