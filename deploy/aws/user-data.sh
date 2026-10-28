#!/bin/bash
# Amazon Linux 2023 x86_64. Paste as plain text into a Launch Template.
# Only resource identifiers belong here. Never paste credentials or passwords.
set -euo pipefail
umask 077

# Fill every REPLACE value. Use ROLE=worker in the Worker Launch Template.
ROLE='web'
REGION='us-east-1'
ARTIFACT_BUCKET='REPLACE_ARTIFACT_BUCKET'
RELEASE_KEY='releases/REPLACE_RELEASE.tar.gz'
RELEASE_SHA256='REPLACE_SHA256'
DB_SECRET_ARN='REPLACE_SECRET_ARN'
DB_HOST='REPLACE_RDS_ENDPOINT'
MEDIA_BUCKET='REPLACE_MEDIA_BUCKET'
ORIGIN='https://REPLACE_HOSTNAME'
API_IMAGE='REPLACE_ECR_API_URI_AT_SHA256'
WEB_IMAGE='REPLACE_ECR_WEB_URI_AT_SHA256'
WORKER_IMAGE='REPLACE_ECR_WORKER_URI_AT_SHA256'
LOG_GROUP='/rachata-plus/application'

for value in "$ROLE" "$REGION" "$ARTIFACT_BUCKET" "$RELEASE_KEY" "$RELEASE_SHA256" \
  "$DB_SECRET_ARN" "$DB_HOST" "$MEDIA_BUCKET" "$ORIGIN" "$API_IMAGE" "$WEB_IMAGE" "$WORKER_IMAGE"; do
  if [[ "$value" == *REPLACE* || -z "$value" ]]; then
    echo 'Fill all User Data resource identifiers before launching.' >&2
    exit 1
  fi
done
[[ "$ROLE" == web || "$ROLE" == worker ]]
[[ "$RELEASE_SHA256" =~ ^[a-f0-9]{64}$ ]]
[[ "$(uname -m)" == x86_64 ]]

dnf install -y docker python3 unzip
command -v curl >/dev/null || { echo 'curl (or curl-minimal) is required.' >&2; exit 1; }
systemctl enable --now docker
command -v aws >/dev/null || { echo 'This AMI must contain AWS CLI v2.' >&2; exit 1; }

version=v2.39.4
install -d -m 0700 /var/lib/rachata-plus-install
cd /var/lib/rachata-plus-install
curl --fail --location --silent --show-error --retry 3 \
  "https://github.com/docker/compose/releases/download/$version/docker-compose-linux-x86_64" -o docker-compose-linux-x86_64
curl --fail --location --silent --show-error --retry 3 \
  "https://github.com/docker/compose/releases/download/$version/checksums.txt" -o checksums.txt
grep ' docker-compose-linux-x86_64$' checksums.txt | sha256sum --check --strict -
install -d /usr/local/lib/docker/cli-plugins
install -m 0755 docker-compose-linux-x86_64 /usr/local/lib/docker/cli-plugins/docker-compose
docker compose version

# Persist non-secret identifiers so the boot service can refresh role credentials
# and retrieve the current database secret after a reboot.
{
  echo '#!/bin/bash'
  echo 'set -euo pipefail'
  echo 'umask 077'
  for name in ROLE REGION ARTIFACT_BUCKET RELEASE_KEY RELEASE_SHA256 DB_SECRET_ARN \
    DB_HOST MEDIA_BUCKET ORIGIN API_IMAGE WEB_IMAGE WORKER_IMAGE LOG_GROUP; do
    printf '%s=%q\n' "$name" "${!name}"
  done
  cat <<'BOOT'
install -d -m 0700 /opt/rachata-plus
archive=$(mktemp /var/lib/rachata-plus-install/release.XXXXXX)
trap 'rm -f -- "$archive"' EXIT
aws s3 cp "s3://$ARTIFACT_BUCKET/$RELEASE_KEY" "$archive" --region "$REGION" --only-show-errors
printf '%s  %s\n' "$RELEASE_SHA256" "$archive" | sha256sum --check --strict -
tar -xzf "$archive" -C /opt/rachata-plus
cd /opt/rachata-plus
exec_args=(python3 deploy/aws/bootstrap.py --role "$ROLE" --region "$REGION"
  --secret "$DB_SECRET_ARN" --db-host "$DB_HOST" --bucket "$MEDIA_BUCKET" --origin "$ORIGIN"
  --api-image "$API_IMAGE" --web-image "$WEB_IMAGE" --worker-image "$WORKER_IMAGE" --log-group "$LOG_GROUP")
"${exec_args[@]}"
BOOT
} > /etc/rachata-plus-boot.sh
chmod 0700 /etc/rachata-plus-boot.sh

cat > /etc/systemd/system/rachata-plus.service <<'UNIT'
[Unit]
Description=Rachata Plus Docker application
Requires=docker.service
After=network-online.target docker.service
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/bash /etc/rachata-plus-boot.sh
Restart=on-failure
RestartSec=30
TimeoutStartSec=900
TimeoutStopSec=75
WorkingDirectory=/opt/rachata-plus

[Install]
WantedBy=multi-user.target
UNIT
install -d -m 0700 /opt/rachata-plus
# ROLE is validated above. Stop containers gracefully during OS shutdown.
sed -i "/^WorkingDirectory=/a ExecStop=/usr/bin/docker compose -p rachata-plus-aws -f compose.aws.yaml --profile $ROLE stop" /etc/systemd/system/rachata-plus.service
systemctl daemon-reload
systemctl enable --now rachata-plus.service
echo 'Bootstrap completed. Verify target health and CloudWatch application logs.'
