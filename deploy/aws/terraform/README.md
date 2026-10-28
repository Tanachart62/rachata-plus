# ลง Rachata Plus บน AWS ด้วย Terraform

ชุดนี้เป็น **Terraform native JSON (`.tf.json`)** ซึ่ง Terraform รันได้โดยตรง ไม่ได้เรียก CloudFormation สร้าง VPC สอง AZ, NAT สองตัว, RDS PostgreSQL Multi-AZ, Secrets Manager, ALB HTTP สำหรับเดโม่ (รองรับ HTTPS เป็นทางเลือก), Launch Templates, Web/Worker ASGs, CloudWatch logs/alarms และเครื่อง migration ชั่วคราว พร้อมจัดการ configurations ของ S3 private สอง buckets ที่เตรียมไว้ โดยใช้ `LabInstanceProfile` เดิม ไม่สร้าง IAM role ใหม่

ถ้า apply ค้างจาก S3 Object Lock explicit deny ให้ใช้ [ขั้นตอนกู้ต่อ](S3-RECOVERY.md) รักษา bucket/state เดิม ไม่เริ่ม deployment ใหม่ รอบวันที่ 29 ตุลาคม 2026 พบข้อจำกัดนี้จริงใน Lab จึงใช้ bucket data sources และให้เตรียมตัว bucket ผ่าน AWS CLI แทน API metadata ที่ provider เรียกไม่ได้

โค้ดแอปถูกดึงจาก GitHub commit `b35233dbdbc39cd6f8c68fec293750e49e74bfe7` และ build Docker บน EC2 ไม่ต้องใช้ ECR หรืออัปโหลด source release ไป S3 ไฟล์ User Data ถูกกรอกค่า resource IDs ให้อัตโนมัติ

`git_commit` ใช้เป็นรุ่นหลัก/Worker ส่วน `web_git_commit` ใช้ pin รุ่นเฉพาะเครื่องเว็บที่รันทั้ง Nginx และ API ได้ หากเป็นค่าว่างจะใช้ `git_commit` เดียวกัน ตัวอย่างล่าสุดแยกรุ่นเว็บที่เพิ่ม favicon เพื่อไม่ต้องเปลี่ยน Worker เมื่ออัปเดตเฉพาะ frontend

สถานะวันที่ 29 ตุลาคม 2026: กู้ปัญหา S3 metadata แล้ว ย้าย DB/media สำเร็จ และเปิด Web ASG สองเครื่องกับ Worker หนึ่งเครื่องจริง ตรวจ ALB targets/container health, `/ready`, หน้าเว็บ/JS/CSS, catalog และภาพปกผ่านแล้ว ยังต้องตรวจ login/playback/upload/scale/failover ตาม [ผลตรวจ](VERIFICATION.md) ให้เลือกใช้ชุดนี้เพียงทางเดียว ไม่สร้าง infrastructure จากคู่มือ Console/CloudFormation อีกชุดซ้ำ หากมีของเดิมที่สร้างไว้แล้วต้องตรวจ/import ให้เหมาะสมก่อน ห้ามรันสร้างทับโดยเดา resource IDs

## 1. ใช้ HTTP ผ่านชื่อ ALB ที่ AWS ให้

โหมดที่เลือกสำหรับส่งงานคือ `http_demo=true` เปิดด้วย `http://<ALB-DNS>` ไม่ต้องมีโดเมนส่วนตัวหรือ ACM certificate ชื่อ `rachata-alb-123456...` เป็นตัวอย่างเท่านั้น Terraform ใส่ชื่อจริงจาก resource ให้ทุก EC2 และ S3 CORS อัตโนมัติ URL จริงอยู่ใน output `website_url`

```hcl
http_demo = true
hostname = ""
certificate_arn = ""
```

ค่าเดียวที่ต้องเลือกเพิ่มเติมคือ `db_engine_version` เป็น PostgreSQL 17.x ที่รองรับ db.t3.small / Multi-AZ / gp2 ใน Lab ตรวจจริงวันที่ 29 ตุลาคม 2026 พบ 17.5 ถึง 17.11 และเตรียมค่าท้องถิ่นเป็น 17.11 ก่อน plan ยังต้องอ่าน plan/สิทธิ์และ quota ของบัญชีจริงก่อน apply

HTTP เดโมไม่เข้ารหัสข้อมูลระหว่างเบราว์เซอร์กับ ALB ใช้บัญชีและรหัสผ่านสำหรับสาธิต โหมดนี้ต้อง opt in ชัดเจนด้วย `AWS_HTTP_DEMO=true` และจำกัด origin เป็นชื่อ ALB ใน us-east-1/us-west-2 เท่านั้น ปิด Secure flag ของ session cookie แต่คง HttpOnly, SameSite=Strict และการตรวจ Origin ฝั่ง API RDS ยัง verify-full TLS, S3 ยัง private/HTTPS และ shared rate limits ยังทำงาน ไม่เปิด demo checkout

ถ้าจะใช้ HTTPS ภายหลัง ให้ตั้ง `http_demo=false` พร้อม hostname ที่ควบคุม DNS ได้และ ACM certificate Issued ใน region เดียวกัน โหมดปกติยังบังคับ HTTPS/Secure cookie ตามเดิม

งบที่แจ้งเหลือประมาณ $26.90 ชุด HA นี้คิดค่าบริการทั้ง NAT สองตัว, ALB, EC2, RDS Multi-AZ และ storage กำหนดช่วงเปิดตรวจและประเมินราคาก่อน apply แม้ตั้ง ASG เป็นศูนย์ ALB/NAT/RDS ยังคิดเงิน

## 2. ติดตั้งเครื่องมือและเข้า Learner Lab

เปิด Docker Desktop สำหรับทำ backup local ติดตั้ง [Terraform CLI](https://developer.hashicorp.com/terraform/install) รุ่น 1.9 ขึ้นไปและ AWS CLI v2 บนเครื่องคุณ

Start Lab → AWS Details → ตั้ง temporary credentials **รวม session token** ใน AWS CLI profile `learner` ห้ามใส่ credentials ใน `.tf`, `.tfvars`, Git หรือ User Data

เปิด PowerShell:

```powershell
Set-Location C:\Users\User\Desktop\rachata-plus
$env:AWS_PROFILE = 'learner'
$env:AWS_REGION = 'us-east-1'
terraform version
aws sts get-caller-identity
aws iam get-instance-profile --instance-profile-name LabInstanceProfile --query 'InstanceProfile.InstanceProfileName' --output text
aws rds describe-db-engine-versions --engine postgres --query "DBEngineVersions[?contains(EngineVersion, '17.')].EngineVersion" --output table
```

เลือก minor ที่มีจริงแล้วตรวจ class ตัวอย่าง แทน `17.YOUR_MINOR`:

```powershell
aws rds describe-orderable-db-instance-options --engine postgres --engine-version 17.YOUR_MINOR --db-instance-class db.t3.small --query 'OrderableDBInstanceOptions[].{MultiAZ:MultiAZCapable,Storage:StorageType}' --output table
```

ต้องรองรับ Multi-AZ และ gp2 ถ้า Lab ปฏิเสธ ให้แก้ค่าตามที่อาจารย์อนุญาตก่อนสร้าง ไม่เปลี่ยน PostgreSQL major ตามใจ เพราะ backup/schema เดิมใช้ 17

## 3. กรอก config แล้ว init/plan

**ถ้ามี buckets จาก apply ครั้งก่อนแล้ว ให้ใช้ชื่อเดิมและข้ามคำสั่งสร้าง buckets** สำหรับ deployment ใหม่เท่านั้น เลือกชื่อที่ไม่ซ้ำและเตรียม private buckets:

```powershell
$taskAccount = aws sts get-caller-identity --query Account --output text
$taskMedia = "rachata-media-$taskAccount-us-east-1"
$taskArtifacts = "rachata-artifacts-$taskAccount-us-east-1"
python deploy/aws/terraform/ensure-buckets.py --media-bucket $taskMedia --artifacts-bucket $taskArtifacts --region us-east-1 --profile learner --create
```

สคริปต์ตรวจเจ้าของ/region/Block Public Access ก่อนใช้ ไม่เรียก Object Lock ไม่ลบ buckets และไม่สร้างหากได้ AccessDenied; ไม่ใส่ `--create` จะตรวจอย่างเดียว Terraform จะจัดการ encryption, versioning, CORS และ policies ต่อเมื่อ apply

```powershell
if (-not (Test-Path deploy/aws/terraform/terraform.tfvars)) {
  Copy-Item deploy/aws/terraform/terraform.tfvars.example deploy/aws/terraform/terraform.tfvars
}
notepad deploy/aws/terraform/terraform.tfvars
```

ใช้ HTTP settings จากข้อ 1 เลือก DB minor ที่ตรวจได้จริง และคงค่าต่อไปนี้ในรอบแรก:

กรอก `media_bucket_name` และ `artifacts_bucket_name` ให้ตรง bucket ที่ตรวจแล้วด้วย หากไม่ได้สร้างใหม่ใช้ชื่อเดิมที่มีอยู่ ไม่เปลี่ยนชื่อเพื่อแก้ error

```hcl
start_apps = false
data_migration_complete = false
create_migration_host = true
```

จาก root โปรเจกต์:

```powershell
terraform -chdir=deploy/aws/terraform init
terraform -chdir=deploy/aws/terraform validate
terraform -chdir=deploy/aws/terraform plan '-out=first.tfplan'
```

ตรวจ plan ว่าเป็น resources ของโปรเจกต์นี้ ไม่ destroy ของเดิม ไม่มี IAM roles ใหม่ RDS ต้อง Multi-AZ/private, NAT สองตัว, app EC2 ไม่มี public IP, web/worker ASG ทั้งคู่ desired=0 และมี migration EC2 ชั่วคราวหนึ่งเครื่อง

ไฟล์ state/plan มีรหัส DB ที่ Terraform สุ่มและบันทึกลง Secrets Manager แม้ console จะแสดง `(sensitive)` ก็ยังเป็นข้อมูลลับ เก็บไฟล์ state ไว้ในเครื่องที่ปลอดภัยและสำรองแยก ห้ามอัปโหลดไป GitHub/ส่งพร้อมงาน ไฟล์เหล่านี้และ `.tfvars` ถูก ignore แล้ว ส่วน `.terraform.lock.hcl` ควรเก็บใน Git

Lock file ที่ให้มาทดสอบบน Windows amd64 ตามเครื่องคุณ หากไปรัน Terraform บน OS อื่น ให้เพิ่ม platform ด้วย `terraform providers lock -platform=linux_amd64` หรือ platform ที่ใช้จริงก่อน ไม่เกี่ยวกับ EC2 ที่รัน Linux เพราะ Terraform ชุดนี้ทำงานจาก Windows ของคุณ

## 4. Apply โครงสร้างโดยยังไม่เปิดแอป

หลังตรวจ plan/งบแล้วรัน:

```powershell
terraform -chdir=deploy/aws/terraform apply first.tfplan
terraform -chdir=deploy/aws/terraform output
```

RDS อาจใช้เวลาหลายสิบนาที ถ้า error ให้อ่านชื่อ resource และสาเหตุ อย่าลบ state หรือสร้างซ้ำหลาย directory แก้สิทธิ์/ค่าที่ผิดแล้ว `plan` ใหม่จาก directory เดิม

Outputs ไม่มีรหัสผ่าน จด `migration_instance_id`, `artifact_bucket`, `media_bucket`, `db_endpoint`, `db_secret_arn`, `application_log_group`, `alb_dns`, `git_commit` และ `ami_id` โดย AMI ถูกเลือกจาก public SSM parameter ของ Amazon Linux 2023 x86_64 หลัง apply รอบแรก ให้นำ output AMI มาใส่ `ami_id` ใน tfvars เพื่อ pin ก่อน plan รอบต่อไป ป้องกัน SSM latest เปลี่ยน AMI จนเสนอแทนเครื่อง migration ระหว่างย้ายข้อมูล

เครื่องแอปยังไม่เกิดและ ALB ยังไม่มี healthy target ตามที่ตั้งใจ RDS/S3/secret สร้างพร้อมแล้ว รหัสผ่านไม่ต้องคัดลอกไปใส่ User Data

## 5. สำรองข้อมูล local และย้ายขึ้น RDS/S3

หยุดการเขียน local ในช่วง cutover และสำรองใหม่ก่อนย้าย:

```powershell
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml stop web api worker
node scripts/backup-local-db.mjs
node scripts/backup-local-media.mjs
```

ต้อง PASS การตรวจ restore และ checksum จดไฟล์ `.dump`/`.tar` คู่ล่าสุดที่สำเร็จ แล้วส่งไป private artifact bucket จาก Terraform output:

```powershell
$taskArtifacts = terraform -chdir=deploy/aws/terraform output -raw artifact_bucket
aws s3 cp backups/YOUR_DB.dump "s3://$taskArtifacts/migration/YOUR_DB.dump"
aws s3 cp backups/YOUR_MEDIA.tar "s3://$taskArtifacts/migration/YOUR_MEDIA.tar"
Get-FileHash backups/YOUR_DB.dump -Algorithm SHA256
Get-FileHash backups/YOUR_MEDIA.tar -Algorithm SHA256
```

EC2 Console → instance ID จาก `migration_instance_id` → Connect → Session Manager → รอ:

```bash
sudo cloud-init status --wait
```

ตั้งค่าต่อไปนี้ด้วย Outputs จริง (ไม่มีรหัสผ่าน):

```bash
GIT_COMMIT='b35233dbdbc39cd6f8c68fec293750e49e74bfe7'
DB_SECRET_ARN='YOUR_TERRAFORM_DB_SECRET_ARN'
DB_HOST='YOUR_TERRAFORM_DB_ENDPOINT'
MEDIA_BUCKET='YOUR_TERRAFORM_MEDIA_BUCKET'
ARTIFACT_BUCKET='YOUR_TERRAFORM_ARTIFACT_BUCKET'
ORIGIN='YOUR_TERRAFORM_WEBSITE_URL'
LOG_GROUP='/rachata/application'
```

ใส่ ORIGIN ตาม output `website_url` ให้ตรงทุกตัว ถ้าใช้ HTTPS ให้เอา `--allow-http-demo` ออกจากคำสั่งต่อไปนี้ ถ้าเปลี่ยน `name` ให้ใช้ output `application_log_group` จริงแทน `/rachata/application` จากนั้น:

```bash
curl --fail --location --retry 3 "https://raw.githubusercontent.com/Tanachart62/rachata-plus/$GIT_COMMIT/deploy/aws/prepare-github.sh" -o /tmp/prepare-github.sh
sudo bash /tmp/prepare-github.sh "$GIT_COMMIT"
cd /opt/rachata-plus
sudo python3 deploy/aws/bootstrap.py --image-source local --role worker --region us-east-1 \
  --secret "$DB_SECRET_ARN" --db-host "$DB_HOST" --bucket "$MEDIA_BUCKET" --origin "$ORIGIN" \
  --api-image "rachata-plus-api:$GIT_COMMIT" --web-image "rachata-plus-web:$GIT_COMMIT" \
  --worker-image "rachata-plus-worker:$GIT_COMMIT" --log-group "$LOG_GROUP" --allow-http-demo --prepare-only
sudo install -d -m 0700 /opt/rachata-import/storage
sudo aws s3 cp "s3://$ARTIFACT_BUCKET/migration/YOUR_DB.dump" /opt/rachata-import/local.dump --region us-east-1
sudo aws s3 cp "s3://$ARTIFACT_BUCKET/migration/YOUR_MEDIA.tar" /opt/rachata-import/media.tar --region us-east-1
sudo sha256sum /opt/rachata-import/local.dump /opt/rachata-import/media.tar
sudo tar -tf /opt/rachata-import/media.tar
```

เทียบ SHA256 กับเครื่องคุณและตรวจรายการ tar ก่อนแตก จาก backup script ของโปรเจกต์นี้ archive เริ่มที่ storage keys โดยตรง:

```bash
sudo tar -xf /opt/rachata-import/media.tar -C /opt/rachata-import/storage
cd /opt/rachata-plus
sudo python3 deploy/aws/database.py restore --file /opt/rachata-import/local.dump
sudo python3 deploy/aws/database.py adopt-014
sudo python3 deploy/aws/database.py migrate
sudo python3 deploy/aws/database.py counts
```

ขั้นนี้ใช้ backup local schema 001–014 ที่ตรวจไว้ หากมี schema เปลี่ยนไปต้องตรวจใหม่ `adopt-014` เทียบ schema ก่อนรับ baseline ไม่ข้าม guard หรือรัน migrations เก่าทับข้อมูล ถ้าเลือก DB ใหม่ว่างให้รันแค่ `migrate` และต้องเตรียมบัญชีเจ้าของระบบแยก สำหรับข้อมูลเดิมใช้ restore ตามนี้

ย้าย media แบบ dry run:

```bash
sudo docker run --rm --network host --env-file secrets/runtime.env \
  --mount type=bind,source=/opt/rachata-plus/secrets/rds-ca.pem,target=/run/secrets/rds-ca.pem,readonly \
  --mount type=bind,source=/opt/rachata-import/storage,target=/import,readonly \
  -e MEDIA_IMPORT_DIR=/import "rachata-plus-api:$GIT_COMMIT" import-media
```

ตรวจไม่มี error แล้วรันคำสั่งเดิมเพิ่ม `--apply` ต่อท้าย `import-media` ตรวจ counts และ S3 `media/` ต้องมี source/HLS/ภาพที่คาดไว้ ขั้นนี้ไม่ลบไฟล์ local ไม่เปิดเผย `secrets/runtime.env` เพราะมีรหัสผ่าน

## 6. เปิดเว็บและ Worker

แก้ `terraform.tfvars` หลัง migration/import ผ่านจริง:

```hcl
start_apps = true
data_migration_complete = true
create_migration_host = true
```

```powershell
terraform -chdir=deploy/aws/terraform plan '-out=start.tfplan'
terraform -chdir=deploy/aws/terraform apply start.tfplan
```

Web ASG = min2/desired2/max3, Worker ASG = 1/1/1, CPU policy เว็บ 60% EC2 ใหม่ build จาก GitHub ผ่าน NAT ครั้งแรกอาจใช้หลายนาทีถึงหลายสิบนาที User Data มี log และ retry; grace period เริ่มต้น 3600 วินาที ต้องปรับหลังวัดจริง

**Apply สำเร็จยังไม่ยืนยันว่าเว็บพร้อม** ชุดนี้ไม่ block รอ target health ให้ตรวจ EC2 → Target groups → web TG ต้อง Healthy สองเครื่อง และ ASG Activity ไม่วนสร้างเครื่องทดแทน Worker ไม่อยู่ใน TG

เปิด URL จาก output `website_url` แล้วเติม `/ready` ต้องได้ 200 โหมด HTTP ไม่ต้องสร้าง DNS record หรือ certificate ส่วนกรณีเลือก HTTPS ให้ตั้ง DNS ของ hostname ไปที่ ALB ก่อน Nginx ส่งทั้งเว็บกับ `/api/` ผ่าน TG HTTP80 เดียว ไม่ต้องเปิด sticky session

## 7. ตรวจรับและดู log

ทดสอบ login/บัญชีเดิม รูป ประวัติ ดูจบ แพ็กเกจ จำกัดจอ/คุณภาพ อัปโหลดใหม่ >100 MB ผ่าน S3 multipart, Worker แปลง FFmpeg และเล่น HLS จริง ตรวจ Dashboard ข้อมูลจริง Production ปิด demo checkout ให้แอดมินจัดแพ็กเกจบัญชีสาธิต

ถ้าเข้าไม่ได้ ใช้ SSM บนเครื่องเว็บ:

```bash
sudo tail -n 100 /var/log/cloud-init-output.log
sudo systemctl status rachata-plus --no-pager
sudo journalctl -u rachata-plus -n 100 --no-pager
sudo docker ps
curl -i http://127.0.0.1/ready
```

Application logs อยู่ CloudWatch ตาม output `application_log_group` มี alarms ของ unhealthy target/DB storage แต่ยังไม่ผูกการแจ้งเตือน SNS/email

ทดสอบเพิ่มเว็บเป็น 3 แล้วลดกลับ 2 เพื่อยืนยันเครื่องใหม่ bootstrap ได้ จากนั้นทดสอบแทนเครื่องหนึ่งตัวและ RDS failover ในช่วง maintenance ผลเหล่านี้ยังต้องตรวจบน AWS จริง Terraform เก็บ desired=2 ไว้ หาก autoscale/manual เปลี่ยนจำนวนแล้ว plan อาจเสนอปรับกลับ ต้องอ่าน plan ก่อน apply ทุกครั้ง

ตรวจข้อมูลครบแล้วให้ตั้ง `create_migration_host=false` → plan จะเสนอ **ลบเครื่อง migration และ scratch ของเครื่องนั้น** ตรวจว่า backup ต้นฉบับ/S3 ครบและไม่มีข้อมูลที่ต้องเก็บบนเครื่องนี้ก่อน apply

## 8. อัปเดตรุ่น พักระบบ และเก็บ state

- อัปเดตรุ่น: commit/push ใหม่ → เปลี่ยน `git_commit` พร้อมตั้ง `web_git_commit=""` (หรือให้ทั้งสองเป็น commit เดียวกัน) สำหรับทั้งระบบ หรือเปลี่ยนเฉพาะ `web_git_commit` สำหรับเครื่องเว็บ → plan/apply อัปเดต Launch Template → ทำ Instance Refresh ของ Web ASG โดยคง healthy capacity ใช้ Worker รุ่นใหม่ในช่วงคิวว่าง การแก้ LT อย่างเดียวไม่แทนเครื่องที่กำลังรัน
- พักแอป: ตั้ง `start_apps=false` แล้ว plan/apply จะลดทั้ง ASGs เป็น 0 และถอด CPU policy ตรวจว่าทุกเครื่องแอปหยุดแล้ว เครื่อง migration ต้องจัดการแยกตามข้อ 7
- ALB/NAT/EIP/RDS/storage/snapshots ยังมีค่าใช้จ่ายแม้ ASG=0 ไม่กด Stop EC2 ใน ASG อย่างเดียวเพราะอาจถูกสร้างทดแทน
- DB, secret และ log group มี `prevent_destroy` และ RDS มี deletion protection ตัว S3 buckets ใช้ data source จึงไม่ถูก Terraform ลบ ส่วน configurations ของ bucket ยังอยู่ใน Terraform ไม่ใช้ `terraform destroy` เป็นขั้นตอนทดลอง หากจะรื้อหลังอาจารย์ตรวจต้องสำรอง/ตรวจ restore และตกลงรายการก่อน
- ไม่ลบ state เพื่อแก้ error ไม่อัปโหลด state/plan/tfvars ให้ GitHub และไม่แก้ resource IDs ใน state เอง ถ้าเปลี่ยนเครื่องทำงานต้องย้าย state แบบปลอดภัยด้วย ไม่ใช่ clone repo แล้ว apply จาก state ว่าง
- ถ้า temporary credentials หมดอายุ ให้อัปเดต profile `learner` จาก Lab แล้ว plan ใหม่ ไม่ reset Lab

## อ้างอิง

- [Terraform native JSON](https://developer.hashicorp.com/terraform/language/syntax/json)
- [ข้อมูลลับใน Terraform state/plan](https://developer.hashicorp.com/terraform/language/manage-sensitive-data)
- [AWS provider: Auto Scaling Group](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/autoscaling_group)
- [AWS provider: RDS instance](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/db_instance)
- คู่มือ Console ทางเลือก: [AWS-GITHUB-WORKSHOP.md](../../../AWS-GITHUB-WORKSHOP.md) ไม่ทำสองเส้นทางพร้อมกัน
