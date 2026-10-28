#!/bin/bash
# Paste as plain text into Amazon Linux 2023 x86_64 Launch Template User Data.
set -euo pipefail
umask 077

# Same commit/config for every EC2. Change ROLE to worker for the Worker LT.
ROLE='web'
REGION='us-east-1'
GIT_COMMIT='REPLACE_FULL_40_CHARACTER_COMMIT'
DB_SECRET_ARN='REPLACE_SECRET_ARN'
DB_HOST='REPLACE_RDS_ENDPOINT'
MEDIA_BUCKET='REPLACE_MEDIA_BUCKET'
ORIGIN='https://REPLACE_HOSTNAME'
HTTP_DEMO='false'
LOG_GROUP='/rachata-plus/application'

for value in "$ROLE" "$REGION" "$GIT_COMMIT" "$DB_SECRET_ARN" "$DB_HOST" "$MEDIA_BUCKET" "$ORIGIN"; do
  if [[ "$value" == *REPLACE* || -z "$value" ]]; then
    echo 'Fill every User Data placeholder before launching.' >&2; exit 1
  fi
done
[[ "$ROLE" == web || "$ROLE" == worker ]]
[[ "$HTTP_DEMO" == true || "$HTTP_DEMO" == false ]]
[[ "$GIT_COMMIT" =~ ^[a-f0-9]{40}$ ]]
[[ "$(uname -m)" == x86_64 ]]
dnf install -y docker git python3 unzip
command -v curl >/dev/null || { echo 'curl (or curl-minimal) is required.' >&2; exit 1; }
systemctl enable --now docker
command -v aws >/dev/null || { echo 'AWS CLI v2 is required.' >&2; exit 1; }
install -d -m 0700 /opt/rachata-plus /var/lib/rachata-plus-install
{
  echo '#!/bin/bash'
  echo 'set -euo pipefail'
  echo 'umask 077'
  for name in ROLE REGION GIT_COMMIT DB_SECRET_ARN DB_HOST MEDIA_BUCKET ORIGIN HTTP_DEMO LOG_GROUP; do
    printf '%s=%q\n' "$name" "${!name}"
  done
  cat <<'BOOT'
helper="/var/lib/rachata-plus-install/prepare-$GIT_COMMIT.sh"
curl --fail --location --silent --show-error --retry 3 \
  "https://raw.githubusercontent.com/Tanachart62/rachata-plus/$GIT_COMMIT/deploy/aws/prepare-github.sh" -o "$helper"
bash "$helper" "$GIT_COMMIT"
cd /opt/rachata-plus
demo_args=()
if [[ "$HTTP_DEMO" == true ]]; then demo_args+=(--allow-http-demo); fi
python3 deploy/aws/bootstrap.py --image-source local --role "$ROLE" --region "$REGION" \
  --secret "$DB_SECRET_ARN" --db-host "$DB_HOST" --bucket "$MEDIA_BUCKET" --origin "$ORIGIN" \
  --api-image "rachata-plus-api:$GIT_COMMIT" --web-image "rachata-plus-web:$GIT_COMMIT" \
  --worker-image "rachata-plus-worker:$GIT_COMMIT" --log-group "$LOG_GROUP" "${demo_args[@]}"
BOOT
} > /etc/rachata-plus-boot.sh
chmod 0700 /etc/rachata-plus-boot.sh
cat > /etc/systemd/system/rachata-plus.service <<'UNIT'
[Unit]
Description=Rachata Plus from pinned GitHub commit
Requires=docker.service
After=network-online.target docker.service
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/bash /etc/rachata-plus-boot.sh
Restart=on-failure
RestartSec=60
TimeoutStartSec=3600
TimeoutStopSec=75
WorkingDirectory=/opt/rachata-plus

[Install]
WantedBy=multi-user.target
UNIT
sed -i "/^WorkingDirectory=/a ExecStop=/usr/bin/docker compose -p rachata-plus-aws -f compose.aws.yaml --profile $ROLE stop" /etc/systemd/system/rachata-plus.service
systemctl daemon-reload
systemctl enable --now rachata-plus.service
echo 'Bootstrap finished; verify /ready and application logs.'
