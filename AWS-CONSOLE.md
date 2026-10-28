# AWS Console deployment

This guide uses the AWS Management Console, EC2 Launch Templates, and GitHub User Data. Record your actual resource IDs and endpoints. Inspect and reuse existing resources instead of creating duplicates.

## 1. Prerequisites

Use an active AWS account or Learner Lab in one supported region, such as `us-east-1`. You need EC2, Auto Scaling, ALB, VPC, RDS, S3, Secrets Manager, CloudWatch Logs, and Session Manager permissions. In Learner Lab, use the existing `LabInstanceProfile`/`LabRole` where permitted. Never store access keys in User Data or Git.

Use Git, AWS CLI v2, Python 3, and Docker locally when migrating existing data. Confirm the current Lab budget and supported instance types: ALB, NAT, RDS Multi-AZ, EC2, and storage incur charges.

## 2. Architecture

```text
Users -> Internet-facing ALB -> HTTP:80 target group (/ready)
                    |                            |
             Private app subnet A        Private app subnet B
             EC2: Nginx + Go API          EC2: Nginx + Go API
                    +-------------+--------------+
                                  |
                        RDS PostgreSQL Multi-AZ
                         Primary <-> Standby
                                  |
                        Private worker EC2
                       FFmpeg + scratch disk
                                  |
                        Private S3 media bucket
                    source / HLS / covers / avatars

Private app subnets -> same-AZ NAT -> Internet Gateway
Private app subnets -> S3 Gateway Endpoint -> S3
```

Nginx serves React and proxies API/auth/media requests internally. Use one ALB target group on port 80, no public API port 8080, and no sticky sessions. The worker does not receive ALB traffic.

## 3. Network and security groups

Create `rachata-vpc` with CIDR `10.42.0.0/16`; enable DNS resolution and hostnames. Create six subnets across two Availability Zones:

| Subnet | CIDR | AZ |
| --- | --- | --- |
| public-a | 10.42.0.0/24 | A |
| public-b | 10.42.1.0/24 | B |
| app-a | 10.42.10.0/24 | A |
| app-b | 10.42.11.0/24 | B |
| db-a | 10.42.20.0/24 | A |
| db-b | 10.42.21.0/24 | B |

Attach an Internet Gateway. Give the public subnets a route table with `0.0.0.0/0 -> Internet Gateway`. Create a NAT gateway with an Elastic IP in each public subnet. Give each app subnet a separate route table with its default route pointing to the same-AZ NAT. DB subnets need only the local VPC route. Disable automatic public IP assignment on app and DB subnets.

Create an S3 Gateway Endpoint and associate it with the app route tables; its policy must allow the media and migration buckets.

Create these security groups in this VPC:

| Group | Inbound | Outbound |
| --- | --- | --- |
| alb-sg | TCP 80 for HTTP; 80/443 for HTTPS, from internet | TCP 80 to app-sg |
| app-sg | TCP 80 from alb-sg only | Default outbound for bootstrap/AWS access |
| worker-sg | None | Default outbound for bootstrap/AWS access |
| db-sg | TCP 5432 from app-sg and worker-sg | Default outbound |

Use security group IDs for internal sources. Do not open database, SSH, or API port 8080 to the internet. Session Manager uses outbound connectivity.

## 4. S3

Create a uniquely named private media bucket and a private artifact bucket for migration backups. Enable all Block Public Access settings, Bucket owner enforced ownership, SSE-S3 encryption, and versioning.

For media CORS, replace the origin below with the exact website origin selected in step 7:

```json
[
  {
    "AllowedOrigins": ["http://YOUR_ALB_DNS"],
    "AllowedMethods": ["PUT"],
    "AllowedHeaders": ["*"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 600
  }
]
```

Use `https://YOUR_HOSTNAME` for HTTPS. Multipart uploads need ETag exposed. Add a lifecycle rule to abort incomplete multipart uploads after two days under `media/`; do not automatically delete completed media without a retention plan.

For each bucket, add a policy denying `s3:*` when `aws:SecureTransport` is `false`, covering both the bucket ARN and its `/*` object ARN. Presigned S3 requests still use HTTPS during the HTTP website demo. Do not enable public-read.

## 5. RDS and Secrets Manager

Create a DB subnet group containing db-a and db-b. In RDS, choose Standard create:

| Setting | Value |
| --- | --- |
| Engine | PostgreSQL 17.x supported in this Lab/region |
| Availability | Multi-AZ DB instance, primary + standby |
| Identifier / DB name | rachata-db / rachata_plus |
| Master username | rachata |
| Class | db.t3.small if supported and permitted |
| Storage | 20 GiB supported SSD storage; review capacity/cost |
| Network | This VPC, private DB subnet group, db-sg only |
| Public access | No |
| Port | 5432 |
| Encryption / deletion protection | Enabled |
| Automated backups | 7 days, subject to account permissions |

Choose a strong generated password. Avoid optional monitoring requiring unavailable Lab roles. Select the two-instance primary/standby option, not the three-instance cluster. Wait for Available and record the writer endpoint.

In Secrets Manager, create an Other type of secret named `rachata/database` with keys `username`, `password`, and `dbname`, matching RDS. Record the ARN; never paste the password into User Data.

## 6. CloudWatch and role permissions

Create a log group such as `/rachata/application` with seven-day retention. The EC2 profile needs SSM managed-instance access, GetSecretValue on the DB secret, media bucket location/list access, object read/write/delete and multipart list/abort, migration artifact read access, and CloudWatch log stream creation/event publishing.

Resolve AccessDenied using the account's supported permissions and the instructor. Do not change organizational restrictions or make S3 public to bypass an error.

## 7. ALB and website origin

Create an instance target group: HTTP port 80, this VPC, health check `/ready`, success 200. Suggested settings: interval 15 seconds, timeout 5 seconds, healthy threshold 2, unhealthy threshold 8, deregistration delay 120 seconds, stickiness off.

Create an internet-facing Application Load Balancer in public-a/public-b using alb-sg. For a classroom HTTP demo, forward listener HTTP:80 to the target group. Record its generated DNS name and use `http://ACTUAL_ALB_DNS` as the origin, without a trailing slash. You cannot select the complete AWS-generated hostname.

For HTTPS, request and validate an ACM certificate in the ALB region for a domain you control. Add HTTPS:443 with that certificate and redirect HTTP:80 to HTTPS. Point your hostname at the ALB and use `https://YOUR_HOSTNAME` as origin. The AWS-owned ALB hostname cannot be used as your own ACM certificate domain.

Set an initial ALB idle timeout of 120 seconds. Keep User Data origin and S3 CORS consistent.

## 8. Source commit and Launch Templates

Get a full source SHA:

```powershell
git ls-remote https://github.com/Tanachart62/rachata-plus.git refs/heads/main
```

Copy [user-data-github.sh](deploy/aws/user-data-github.sh) and fill:

| Variable | Value |
| --- | --- |
| ROLE | web or worker |
| REGION | Selected supported region |
| GIT_COMMIT | Full 40-character Git SHA |
| DB_SECRET_ARN | DB secret ARN |
| DB_HOST | RDS endpoint, no port or scheme |
| MEDIA_BUCKET | Bucket name, no s3:// prefix |
| ORIGIN | Exact website origin from step 7 |
| HTTP_DEMO | true for HTTP ALB demo; false for HTTPS |
| LOG_GROUP | Existing log group name |

In EC2 > Launch Templates, create web and worker templates:

| Setting | Web | Worker |
| --- | --- | --- |
| AMI | Amazon Linux 2023 x86_64 with AWS CLI v2 | Same AMI |
| Instance type | t3.medium | t3.large |
| Root volume | Encrypted gp3, 30 GiB | Encrypted gp3, 80 GiB |
| Security group | app-sg | worker-sg |
| Instance profile | LabInstanceProfile or permitted app profile | Same |
| Public IP | Disabled | Disabled |
| Metadata | IMDSv2 required, hop limit 2 | Same |
| User Data | Filled script, ROLE=web | Filled script, ROLE=worker |

Leave subnet unset for the ASG. If configuring a network interface, use device index 0, disable public IPs, and specify the SG there rather than twice. Paste User Data as plain text; do not select already base64 encoded.

The script installs tools, fetches the pinned commit, builds Docker images, reads the DB secret, and starts the selected role. Initial builds can take several minutes. No migrations run at boot. Do not start the ASGs until database preparation is complete.

## 9. Prepare the database

Create a temporary migration EC2 outside the ASGs in app-a with worker-sg, the same instance profile, no public IP, IMDSv2/hop2, and sufficient disk. Its User Data should only install Docker/Git/Python/unzip and enable Docker; it must not start a worker.

Connect through Session Manager and use a root shell because runtime directories are private:

```bash
sudo -i
cloud-init status --wait
GIT_COMMIT='YOUR_FULL_40_CHARACTER_COMMIT'
curl --fail --location --retry 3 \
  "https://raw.githubusercontent.com/Tanachart62/rachata-plus/$GIT_COMMIT/deploy/aws/prepare-github.sh" \
  -o /tmp/prepare-github.sh
bash /tmp/prepare-github.sh "$GIT_COMMIT"
cd /opt/rachata-plus
python3 deploy/aws/bootstrap.py --image-source local --role worker --region us-east-1 \
  --secret YOUR_SECRET_ARN --db-host YOUR_RDS_ENDPOINT \
  --bucket YOUR_MEDIA_BUCKET --origin http://YOUR_ALB_DNS \
  --api-image "rachata-plus-api:$GIT_COMMIT" \
  --web-image "rachata-plus-web:$GIT_COMMIT" \
  --worker-image "rachata-plus-worker:$GIT_COMMIT" \
  --log-group /rachata/application --allow-http-demo --prepare-only
```

For HTTPS, change the origin and omit `--allow-http-demo`. Preparation writes private credentials and the RDS CA bundle without starting services.

For a new empty database:

```bash
python3 deploy/aws/database.py migrate
python3 deploy/aws/database.py counts
```

Registration creates ordinary users only. After web startup, register the intended owner and have the DB operator promote that specific active account to `role='admin', admin_level='owner'` using a verified ID and a controlled SQL transaction. Restored databases retain their owners. Never commit passwords.

For an existing local installation, stop local writers and run both backup scripts from README. Verify backups, upload the DB dump/media archive and metadata into the private artifact bucket under `migration/`, then download them to `/opt/rachata-import` and compare SHA256 before importing.

On an empty target database only:

```bash
python3 deploy/aws/database.py restore --file /opt/rachata-import/local.dump
python3 deploy/aws/database.py adopt-014
python3 deploy/aws/database.py migrate
python3 deploy/aws/database.py counts
```

Use `adopt-014` only for a restored local schema matching migrations 001-014; it verifies the schema before recording the baseline. Inspect any different/existing target first. Never rerun restore over an active database.

Extract the verified media archive under `/opt/rachata-import/storage`. Set extracted directories to 0750 and files to 0640, owned by UID/GID 10001 so the import container can read them. Keep the private parent root-owned and retain local source files.

```bash
docker run --rm --network host \
  --env-file /opt/rachata-plus/secrets/runtime.env \
  --mount type=bind,source=/opt/rachata-plus/secrets/rds-ca.pem,target=/run/secrets/rds-ca.pem,readonly \
  --mount type=bind,source=/opt/rachata-import/storage,target=/import,readonly \
  -e MEDIA_IMPORT_DIR=/import \
  "rachata-plus-api:$GIT_COMMIT" import-media
```

Review the dry-run counts, then repeat with `import-media --apply`. Do not start the ASGs until DB migration and media import succeed.

## 10. Auto Scaling Groups

Create the web ASG with its template, app-a/app-b, and the existing target group. Start with min 2, desired 2, max 3. Enable ELB health checks. Allow a long initial grace period, such as 1800 seconds, for first-boot builds and measure actual startup time before reducing it. Add CPU target tracking, for example 60%, with warmup based on measured startup time.

Create a separate worker ASG with its template, private app subnets, min/desired/max 1, EC2 health checks, and no ALB attachment. A database advisory lock controls the worker; adding instances does not automatically parallelize encoding. Avoid stopping an active job.

Wait for both web targets to become healthy. EC2 InService alone does not confirm application health.

## 11. Verify and operate

Verify `/ready`, catalog/covers, login/logout, account settings, playback, S3 uploads, HLS completion, and membership limits across instances. Test controlled scale-out, replacement, RDS failover, and backup restoration separately.

Diagnostics on a web EC2 through Session Manager:

```bash
sudo cloud-init status
sudo systemctl status rachata-plus --no-pager
sudo journalctl -u rachata-plus -n 100 --no-pager
sudo docker ps
curl --fail http://127.0.0.1/ready
```

The worker has no listener on port 80. Inspect container health and CloudWatch logs instead. Do not display runtime secrets or full container inspection output in screenshots.

For updates, publish a reviewed commit, create new Launch Template versions pinned to it, and select those versions in the ASGs. Apply required migrations separately. Changing a template does not update running instances. Use a permitted rolling replacement procedure and retain healthy capacity; coordinate worker updates with an empty queue. Some Learner Labs restrict Instance Refresh.

To pause applications, deliberately set ASG minimum/desired to zero after checking uploads and jobs. Stopping ASG instances individually can trigger replacements. ALB, NAT, RDS, EIPs, and storage can still incur charges with no app instances. Back up database/media before removing resources.

## AWS references

- [Create a Launch Template in the Console](https://docs.aws.amazon.com/autoscaling/ec2/userguide/create-launch-template.html)
- [ALB target health checks](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/target-group-health-checks.html)
