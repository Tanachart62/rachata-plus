#!/bin/bash
# Run as root on Amazon Linux 2023 x86_64; no credentials are stored in source.
set -euo pipefail
umask 077
commit="${1:?Pass a full 40-character Git commit}"
[[ "$commit" =~ ^[a-f0-9]{40}$ ]]
[[ "$(uname -m)" == x86_64 ]]
repo='https://github.com/Tanachart62/rachata-plus.git'
source_dir="/opt/rachata-build/$commit"
install -d -m 0700 "$source_dir"
if [[ ! -d "$source_dir/.git" ]]; then
  git -C "$source_dir" init
  git -C "$source_dir" remote add origin "$repo"
  GIT_TERMINAL_PROMPT=0 git -C "$source_dir" fetch --depth=1 origin "$commit"
  git -C "$source_dir" checkout --detach FETCH_HEAD
fi
[[ "$(git -C "$source_dir" rev-parse HEAD)" == "$commit" ]]
[[ "$(git -C "$source_dir" remote get-url origin)" == "$repo" ]]
git -C "$source_dir" diff --quiet HEAD --
bash "$source_dir/deploy/aws/prepare-ami.sh"
cd "$source_dir"
# Build sequentially. A successful image is reused on reboot; never prune here.
if ! docker image inspect "rachata-plus-api:$commit" >/dev/null 2>&1; then
  docker build --platform linux/amd64 --target api -t "rachata-plus-api:$commit" apps/api
fi
if ! docker image inspect "rachata-plus-worker:$commit" >/dev/null 2>&1; then
  docker build --platform linux/amd64 --target worker -t "rachata-plus-worker:$commit" apps/api
fi
if ! docker image inspect "rachata-plus-web:$commit" >/dev/null 2>&1; then
  docker build --platform linux/amd64 -t "rachata-plus-web:$commit" apps/web
fi
install -d -m 0700 /opt/rachata-plus /var/lib/rachata-plus-install
# release.py copies only deployment files/SQL; source/build contexts have no secrets.
bundle_dir=$(mktemp -d /var/lib/rachata-plus-install/bundle.XXXXXX)
python3 deploy/aws/release.py --output "$bundle_dir/runtime.tar.gz"
tar -xzf "$bundle_dir/runtime.tar.gz" -C /opt/rachata-plus
echo "Prepared Git commit $commit. Database migrations are a separate step."
