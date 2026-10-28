# Rachata Plus: deploy บน AWS Academy Learner Lab

ถ้าต้องการทำผ่าน AWS Console แบบ Workshop และวาง User Data ใน Launch Template เอง ใช้ [AWS-WORKSHOP.md](AWS-WORKSHOP.md) แทนเส้นทาง CloudFormation ในเอกสารนี้ อย่าสร้างทั้งสองชุดซ้ำกัน

เอกสารนี้ใช้กับ source บน `C:\Users\User\Desktop\rachata-plus` และแผนที่ตกลง: ALB → EC2 Auto Scaling สอง AZ, Worker แยกหนึ่งเครื่อง, RDS PostgreSQL Multi-AZ Primary/Standby และ S3 private ไม่ใช้ CloudFront

เป็นชุดเตรียม deployment ไม่ใช่หลักฐานว่า deploy สำเร็จหรือผ่าน production audit แล้ว ยังต้องตรวจ IAM, quota, engine/class ที่มีให้เลือก และทดสอบบนบัญชีจริง

## 1. อ่านจุดที่ต้องเตรียมก่อนสร้างบริการ

- งบที่แจ้งคือ Used $23.10 of $50 จึงเหลือประมาณ **$26.90** ตัวเลขของ Lab อาจอัปเดตล่าช้า
- แบบนี้มีค่าใช้จ่ายทั้ง ALB, NAT สองตัว, EC2 เว็บอย่างน้อยสองตัว, Worker หนึ่งตัว, RDS Multi-AZ, EBS, S3 และการส่งข้อมูล อย่าเปิดทิ้งหลายวัน ประเมินราคาจริงตาม region/class ใน AWS Pricing Calculator ก่อนสร้าง
- `StartApps=false` หยุดเฉพาะการสร้าง EC2 ของแอป **ไม่ได้หยุดค่า ALB/NAT/RDS** สร้าง stack เมื่อ artifacts และการย้ายข้อมูลพร้อมเท่านั้น
- เลือก `us-east-1` สำหรับคู่มือนี้ Lab ยังอนุญาต `us-west-2` แต่ทุก resource/AMI/certificate/image ต้องอยู่ region ที่เลือกให้ตรงกัน
- ยังไม่มีโดเมน: ต้องได้โดเมนหรือ subdomain ที่แก้ DNS ได้ก่อน เพื่อขอ ACM certificate และเปิด HTTPS ที่ ALB ชื่อ DNS ของ ALB ไม่ใช่โดเมนที่เราออกใบรับรองเองได้
- ทางเลือก: ขอ subdomain จากอาจารย์/สถาบัน หรือใช้โดเมนที่คุณมีสิทธิ์ควบคุม DNS การซื้อหรือสมัครบริการภายนอกต้องตกลงแยกต่างหาก อย่าแก้เป็น HTTP หรือปิด secure cookies เพื่อข้ามขั้นนี้
- อย่ากด Reset Lab และอย่าลบ resource เดิมที่ไม่เกี่ยวกับโปรเจกต์ ให้ตรวจรายการเดิมทั้งสอง region ก่อน เพราะนับรวม quota และงบด้วย

## 2. สิ่งที่เตรียมในโค้ดแล้ว

| ส่วน | พฤติกรรมในโหมด AWS |
|---|---|
| อัปโหลด | API ตรวจแอดมินแล้วออก URL สำหรับ S3 UploadPart ทีละส่วน ผูกงานกับบัญชีและ session เดิม ส่วนที่ส่งล้มเหลวลองใหม่ได้ ขณะหน้ายังเปิด |
| ยืนยันไฟล์ | API ตรวจจำนวน/ขนาด parts จาก S3 เอง แล้วจึงสร้างวิดีโอและคิวใน RDS Complete ซ้ำได้โดยไม่สร้างวิดีโอซ้ำ |
| Worker | ล็อกเจ้าของงานใน PostgreSQL ดาวน์โหลด source ลง scratch แปลง FFmpeg แล้วอัปโหลด generation ใหม่ครบก่อนประกาศ ready |
| เล่น | ทุก playlist/segment ผ่าน API ตรวจ cookie, สมาชิก, lease, generation และคุณภาพก่อนอ่าน S3 รองรับ HEAD และ single byte range |
| รูป | หน้าเว็บย่อภาพปกก่อนส่ง API, API normalize JPEG และเก็บใน S3 รูปโปรไฟล์ถูกเก็บใน S3 ในโหมด AWS |
| หลาย EC2 | Session/ประวัติ/Playback leases ใช้ RDS ร่วมกัน พร้อม rate limit การเขียนบัญชีที่นับร่วมใน PostgreSQL ไม่ต้องเปิด Sticky Session |
| production | ต้องใช้ HTTPS, secure cookies, RDS `verify-full`, external worker และ S3 ไม่อนุญาต test endpoint |
| เริ่มเครื่อง | User Data ดึง release ที่ตรวจ SHA256 แล้วใช้ instance role อ่าน Secrets Manager/ECR โดยไม่ฝังคีย์หรือรหัสผ่านใน template |

การเลือกไฟล์ภาพต้นฉบับไม่มีเพดาน 5 MB/4096 px เดิม แต่มีการย่ออัตโนมัติและงบหน่วยความจำสำหรับ decoder ที่ API ไม่ใช่การรับภาพได้ไม่จำกัดทรัพยากร การอัปโหลดวิดีโอถูกจำกัดด้วยงบงานของ Worker ค่า AWS ตัวอย่างคือ source+HLS รวม 20 GiB, scratch ต้องมีพื้นที่เหลืออย่างน้อย 2 GiB และ job timeout 6 ชั่วโมง ต้องทดสอบกับไฟล์จริงและขนาด EC2 ที่เลือก

Worker เริ่มหนึ่งเครื่อง active ระบบ advisory lock/generation fencing รองรับการกู้คืนงานหลังเสียการเชื่อมต่อ ไม่ได้เพิ่มการแปลงหลายงานขนาน และ Worker กลุ่มนี้ไม่ได้ขยายตาม CPU ของเว็บ

HLS generations เก่าบน S3 ยังเก็บไว้ ไม่ได้ล้างอัตโนมัติทุกครั้งที่แปลงใหม่ ต้องติดตามพื้นที่และวาง retention ให้สัมพันธ์กับ backup ก่อนล้าง ส่วนการลบวิดีโอ/บัญชีผ่านระบบจะซ่อนสิทธิ์ทันทีและให้ Worker ทำ cleanup; หากเปิด S3 Versioning เวอร์ชันเก่ายังคงอยู่ตามนโยบายเก็บข้อมูล

## 3. เตรียมเครื่องคุณและ credentials ชั่วคราว

ใช้ PowerShell ที่ root โปรเจกต์ ติดตั้ง AWS CLI v2, Python 3 และเปิด Docker Desktop ตรวจ:

```powershell
Set-Location C:\Users\User\Desktop\rachata-plus
aws --version
python --version
docker version
docker compose version
```

กด Start Lab แล้วไป AWS Details นำ temporary credentials ไปตั้ง AWS CLI profile ชื่อ `learner` ในเครื่องคุณ รวม **session token** ด้วย Credentials จะหมดอายุตาม Lab ต้องอัปเดตเมื่อต่อ session ใหม่ ห้ามส่ง credentials ในแชตหรือใส่ใน repo/AMI/User Data

ตั้งค่าตัวแปรที่ไม่ใช่ความลับ:

```powershell
$env:AWS_PROFILE = 'learner'
$env:AWS_REGION = 'us-east-1'
aws sts get-caller-identity
$taskAccount = aws sts get-caller-identity --query Account --output text
$taskRegistry = "$taskAccount.dkr.ecr.$env:AWS_REGION.amazonaws.com"
$taskRelease = Get-Date -Format 'yyyyMMdd-HHmmss'
$taskArtifacts = "rachata-artifacts-$taskAccount-$env:AWS_REGION"
```

ตรวจ Console ว่ามี `LabRole` และ `LabInstanceProfile` อย่าสร้าง IAM role/user ใหม่เพื่อหลีกเลี่ยงข้อจำกัด Lab ถ้าคำสั่งถูกปฏิเสธให้ตรวจสิทธิ์กับคู่มือ Lab/อาจารย์ก่อน

## 4. สำรองข้อมูลก่อนย้ายจริง

รอบเตรียมโค้ดได้สำรองและตรวจ restore ใน local แล้ว แต่ถ้ามีอัปโหลดหรือแก้ข้อมูลต่อ ต้องสำรองใหม่ก่อน cutover:

```powershell
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml stop web api worker
node scripts/backup-local-db.mjs
node scripts/backup-local-media.mjs
```

ต้องได้ PASS ทั้ง DB restore/counts/schema และ checksum สื่อ เก็บชื่อ `.dump`, `.tar`, metadata/checksum คู่กัน ในช่วง cutover ให้ local อยู่ใน maintenance จนย้ายเสร็จ ไม่เช่นนั้นข้อมูลหลัง backup จะไม่ตามไปด้วย

การเปิด local กลับหลังสำรองเพื่อพัฒนาต่อ ให้ใช้ค่ารัน Origin/cookie/port ของ local หรือ Tunnel ที่ใช้อยู่ อย่าเปิดกลับโดยใช้ production env โดยไม่ตั้งใจ ข้อมูลสำรองมีบัญชีและ session จึงเก็บเป็น private และอย่าแนบไปส่งอาจารย์พร้อม source

## 5. สร้าง ECR และ build images รุ่นเดียวกัน

ขั้นต่อจากนี้สร้างทรัพยากรและมีค่าใช้จ่าย ให้ตรวจงบก่อน เริ่มจาก ECR สาม repositories ถ้ามีชื่อเดิมอยู่แล้วให้ตรวจว่าเป็นของโปรเจกต์นี้และใช้ตัวเดิม:

```powershell
aws ecr create-repository --repository-name rachata-api --image-tag-mutability IMMUTABLE
aws ecr create-repository --repository-name rachata-web --image-tag-mutability IMMUTABLE
aws ecr create-repository --repository-name rachata-worker --image-tag-mutability IMMUTABLE
aws ecr get-login-password | docker login --username AWS --password-stdin $taskRegistry

docker build --platform linux/amd64 --target api -t "$taskRegistry/rachata-api`:$taskRelease" apps/api
docker build --platform linux/amd64 --target worker -t "$taskRegistry/rachata-worker`:$taskRelease" apps/api
docker build --platform linux/amd64 -t "$taskRegistry/rachata-web`:$taskRelease" apps/web
docker push "$taskRegistry/rachata-api`:$taskRelease"
docker push "$taskRegistry/rachata-worker`:$taskRelease"
docker push "$taskRegistry/rachata-web`:$taskRelease"

aws ecr describe-images --repository-name rachata-api --image-ids "imageTag=$taskRelease" --query 'imageDetails[0].imageDigest' --output text
aws ecr describe-images --repository-name rachata-web --image-ids "imageTag=$taskRelease" --query 'imageDetails[0].imageDigest' --output text
aws ecr describe-images --repository-name rachata-worker --image-ids "imageTag=$taskRelease" --query 'imageDetails[0].imageDigest' --output text
```

หยุดหาก build/push ใดล้มเหลว ต้องใช้ URI **`repository@sha256:...`** ที่ ECR คืนมาจริงในการตั้ง stack ไม่ใช้ `latest` และไม่ใช้ digest ตัวอย่างในเอกสาร

## 6. เตรียม private artifact bucket และ release

คำสั่ง create-bucket นี้สำหรับ us-east-1 เท่านั้น ถ้า bucket มีอยู่แล้วให้ตรวจเจ้าของ/region ก่อน ไม่ต้องสร้างซ้ำ:

```powershell
aws s3api create-bucket --bucket $taskArtifacts --region us-east-1
aws s3api put-public-access-block --bucket $taskArtifacts --public-access-block-configuration file://deploy/aws/artifact-public-access.json
aws s3api put-bucket-encryption --bucket $taskArtifacts --server-side-encryption-configuration file://deploy/aws/artifact-encryption.json
python deploy/aws/release.py --output "backups/aws-release-$taskRelease.tar.gz"
aws s3 cp "backups/aws-release-$taskRelease.tar.gz" "s3://$taskArtifacts/releases/$taskRelease.tar.gz"
aws s3 cp deploy/aws/prepare-ami.sh "s3://$taskArtifacts/tools/prepare-ami.sh"
```

สคริปต์ release รวมเฉพาะ Compose, bootstrap, migration tool และ SQL ไม่รวม `.env`, source media, DB backup, credentials หรือ dependencies จด SHA256 ที่สคริปต์แสดงไว้ใช้กับ `ReleaseSha256`

อัปโหลด DB/media backup สำหรับ cutover ไป private artifact bucket ใน prefix `migration/` แยกจาก release และเก็บ checksum ห้ามเปิด bucket หรือ objects เป็น public

## 7. เตรียม AMI สำหรับ Launch Template

1. ใน EC2 Console region เดียวกัน สร้าง **เครื่องเตรียม AMI ชั่วคราว** จาก Amazon Linux 2023, x86_64 เช่น t3.small, EBS 20 GiB
2. ใช้ public subnet ที่มี route ไป Internet Gateway และ Public IPv4 เพื่อดาวน์โหลดของตอนเตรียมเครื่อง ติด `LabInstanceProfile` ไม่ต้องเปิด inbound SSH ใช้ Systems Manager Session Manager
3. ใน Session Manager ตรวจ `aws --version`, `python3 --version` ถ้า AWS CLI ยังไม่มี ติดตั้งตามเอกสาร AWS CLI v2 ทางการก่อน
4. รันโดยแทน bucket ให้ตรง:

```sh
aws s3 cp s3://YOUR_ARTIFACT_BUCKET/tools/prepare-ami.sh /tmp/prepare-ami.sh --region us-east-1
sudo bash /tmp/prepare-ami.sh
sudo docker compose version
```

5. สร้าง AMI จากเครื่องนี้ **ก่อนดึง secrets หรือ bootstrap แอป** จด AMI ID และรอ Available
6. เครื่อง ASG จริงจะใช้ AMI นี้, private subnets, ไม่มี Public IP, IMDSv2 required และ hop limit 2 เพื่อให้ SDK ภายใน Docker ใช้ instance role ได้
7. หยุดเครื่องเตรียม AMI หลังตรวจว่าไม่ต้องใช้แล้ว เครื่องนี้อยู่นอก stack จึงต้องดูแลค่าใช้จ่ายแยกด้วย AMI snapshot ยังมีค่าเก็บข้อมูล

## 8. เตรียมโดเมนและ ACM

1. เลือก hostname เช่น `video.your-domain.example` ที่คุณแก้ DNS ได้
2. ACM ใน **us-east-1** → Request public certificate → ใส่ hostname → DNS validation
3. เพิ่ม CNAME validation ตามที่ ACM ระบุในผู้ให้บริการ DNS รอ Issued แล้วจด certificate ARN
4. ยังไม่ต้องชี้ hostname มาที่เว็บ จนกว่าข้อมูลและ health checks ผ่าน

หากยังไม่มี hostname ที่ใช้ได้ ให้หยุดก่อนสร้าง stack ไม่ต้องลดระบบบัญชีลงไปใช้ HTTP และอย่าใช้ certificate ของโดเมนที่ไม่ได้ควบคุม

## 9. ตรวจ template และสร้าง infrastructure โดยยังไม่เปิดแอป

ใช้ `deploy/aws/stack.json` ซึ่งสร้าง public subnets สอง, private app subnets สอง, private DB subnets สอง, NAT แยกสอง AZ, S3 Gateway Endpoint, Security Groups, ALB, CloudWatch logs/alarms, RDS Multi-AZ และ Launch Templates/ASGs โดยใช้ role เดิมของ Lab

1. คัดลอก `deploy/aws/parameters.example.json` ไปไฟล์ local เช่น `secrets/aws-parameters.json` เติม AMI, certificate ARN, Origin, artifact bucket/key/SHA และ ECR image digests จริง
2. คง `StartApps=false` ตรวจว่า WebMin ≤ WebDesired ≤ WebMax และทุก class มีให้ใช้ใน Lab/region ปัจจุบัน โดย RDS เลือก Burstable class, PostgreSQL, gp2, Enhanced Monitoring ปิด
   เติม `DBEngineVersion` เป็น PostgreSQL **17.x ที่มีให้ใช้จริง** เพื่อให้ตรง major ของ backup และการตรวจ schema ไม่ใช้ค่า `17.REPLACE...` จากตัวอย่าง ตรวจตัวเลือกก่อนสร้าง:

```powershell
aws rds describe-db-engine-versions --engine postgres --query "DBEngineVersions[?contains(EngineVersion, '17.')].EngineVersion" --output table
aws rds describe-orderable-db-instance-options --engine postgres --engine-version 17.YOUR_MINOR --db-instance-class db.t3.small --query 'OrderableDBInstanceOptions[].MultiAZCapable' --output table
```

   ถ้าไม่มีตัวเลือกหรือ MultiAZCapable ไม่รองรับ ให้เลือก minor/class ที่ Lab อนุญาตก่อนสร้าง ไม่ปล่อยให้ RDS เลือก major ใหม่อัตโนมัติแทน source 17
3. ถ้าไม่ใช้ network CIDR ใน template ต้องแก้ trusted ALB subnet CIDRs ใน `deploy/aws/nginx.conf` ด้วย และสร้าง release ใหม่
4. ตรวจ template:

```powershell
aws cloudformation validate-template --template-body file://deploy/aws/stack.json
```

5. ใช้ CloudFormation Console → Create stack → Upload `stack.json` → กรอก parameters ตามไฟล์ เลือกใช้ `LabRole` ถ้าต้องกำหนด service role ตรวจ resource list/ค่าใช้จ่ายก่อนกด Create
6. หรือ CLI หลังตรวจค่าแล้ว:

```powershell
aws cloudformation create-stack --stack-name rachata-plus --template-body file://deploy/aws/stack.json --parameters file://secrets/aws-parameters.json --role-arn "arn:aws:iam::$taskAccount`:role/LabRole"
```

7. รอ `CREATE_COMPLETE` หากล้มเหลวอ่าน Events ไม่กดสร้างซ้ำหลายชุด อย่าเพิ่มสิทธิ์ข้ามข้อจำกัด Lab เพื่อแก้ error
8. ใน Outputs จด MediaBucket, DatabaseEndpoint, DatabaseSecretArn, VpcId, AppSubnetA/B, LoadBalancerDNS และชื่อ ASGs ในช่วงนี้ Target Group ยังไม่มีเว็บ healthy เพราะตั้ง StartApps=false ไว้โดยตั้งใจ

RDS ตั้ง Deletion Protection และเก็บ snapshot เมื่อถูกลบผ่าน stack, bucket/secret/log group มี retention เพื่อป้องกันข้อมูลหาย จึงต้องวางแผน cleanup แยก ไม่ใช่ Delete stack แล้วค่าใช้จ่ายทุกอย่างหายทันที

## 10. ย้ายฐานข้อมูลและสื่อผ่านเครื่อง migration ชั่วคราว

1. สร้าง EC2 ชั่วคราวจาก AMI ที่เตรียมใน **AppSubnetA ของ VPC ใหม่**, ไม่มี Public IP, ใช้ WorkerSG และ `LabInstanceProfile`, EBS 80 GiB เข้าผ่าน Session Manager เครื่องนี้มีทางออกผ่าน NAT และเข้าฐานข้อมูลได้ผ่าน DB-SG
2. ดาวน์โหลด release ตาม SHA256 เดียวกับ stack:

```sh
sudo install -d -m 0700 /opt/rachata-plus
sudo aws s3 cp s3://YOUR_ARTIFACT_BUCKET/releases/YOUR_RELEASE.tar.gz /opt/rachata-release.tar.gz --region us-east-1
echo 'YOUR_RELEASE_SHA256  /opt/rachata-release.tar.gz' | sudo sha256sum --check --strict -
sudo tar -xzf /opt/rachata-release.tar.gz -C /opt/rachata-plus
```

3. รัน bootstrap แบบเตรียม config อย่างเดียว แทนทุกค่าจาก stack Outputs และ image digests จริง:

```sh
cd /opt/rachata-plus
sudo python3 deploy/aws/bootstrap.py --role worker --region us-east-1 \
  --secret YOUR_DATABASE_SECRET_ARN --db-host YOUR_RDS_ENDPOINT \
  --bucket YOUR_MEDIA_BUCKET --origin https://YOUR_HOSTNAME \
  --api-image YOUR_API_IMAGE_AT_DIGEST --web-image YOUR_WEB_IMAGE_AT_DIGEST \
  --worker-image YOUR_WORKER_IMAGE_AT_DIGEST --log-group YOUR_STACK_LOG_GROUP --prepare-only
```

ใช้ Output `ApplicationLogGroup` หรือ stack Resources หา ApplicationLogs physical name ค่า secret ถูกเขียนใน `secrets/runtime.env` ที่สิทธิ์ 0600 ไม่ต้องแสดงไฟล์นี้หรือส่งกลับแชต

4. ดาวน์โหลด `.dump` และ `.tar` คู่ที่ตรวจแล้วจาก private artifact bucket ไป `/opt/rachata-import/` และตรวจ SHA256 ให้ตรงกับ local ก่อนใช้ `tar -tf` ตรวจรายการให้เป็น storage keys ที่คาดไว้ แล้วแตก media ไป `/opt/rachata-import/storage/` รักษาโครงสร้าง `<storage_key>/source`, `jobs/gN/full/`, `cover.jpg` เดิม
5. เลือกฐานข้อมูลเพียงทางเดียว:

**ย้ายข้อมูล local เดิม (เหมาะกับโปรเจกต์นี้):**

```sh
sudo python3 deploy/aws/database.py restore --file /opt/rachata-import/local.dump
sudo python3 deploy/aws/database.py adopt-014
sudo python3 deploy/aws/database.py migrate
sudo python3 deploy/aws/database.py counts
```

`restore` ยอมทำเฉพาะ public schema ว่าง, `adopt-014` สร้าง schema อ้างอิงชั่วคราวแล้วเทียบ columns/constraints/indexes ทั้งหมดก่อนบันทึก baseline โดยไม่รัน migration เก่าบนข้อมูลจริง จากนั้น `migrate` ใช้ advisory lock และ checksum ledger รันเฉพาะ migration ที่ขาด ถ้าเทียบไม่ผ่านให้หยุดตรวจ ห้ามข้าม guard หรือรัน SQL เดาซ้ำ

**ฐานข้อมูลใหม่ว่าง ไม่มีข้อมูล local:** รันแค่ `database.py migrate` ไม่ต้อง restore/adopt-014 และต้องเตรียมบัญชีเจ้าของระบบอย่างมีขั้นตอนแยก ไม่ใส่บัญชี/รหัสผ่าน admin เริ่มต้นใน AMI

6. ย้ายไฟล์ไป S3 ด้วยคำสั่ง dry run ก่อน (`sudo` ใช้ .env ของ host ที่ bootstrap เตรียมให้):

```sh
cd /opt/rachata-plus
sudo docker run --rm --network host --env-file secrets/runtime.env \
  --mount type=bind,source=/opt/rachata-plus/secrets/rds-ca.pem,target=/run/secrets/rds-ca.pem,readonly \
  --mount type=bind,source=/opt/rachata-import/storage,target=/import,readonly \
  -e MEDIA_IMPORT_DIR=/import YOUR_API_IMAGE_AT_DIGEST import-media
```

7. ตรวจจำนวนวิดีโอ/objects และหากไม่มี error ใช้คำสั่งเดิมเพิ่ม `--apply` ต่อท้าย `import-media` โปรแกรมอัปโหลดเฉพาะ directory ของวิดีโอที่ยังไม่ลบ ตรวจ SHA256 metadata/ขนาด และย้ายภาพปกที่เคยอยู่ใน DB กับ avatar ไป S3 อัปเดตตำแหน่งใน RDS หลังส่งสำเร็จ ไม่มีการลบไฟล์ local
8. รัน `import-media --apply` ซ้ำได้ โดยข้าม objects ที่ SHA256/ขนาดตรงแล้ว ถ้าคำสั่งล้มเหลวหยุดแก้สาเหตุก่อนเปิดแอป ตรวจ MediaBucket เป็น private และ prefix `media/` มี source/HLS/ภาพครบ
9. ตรวจจำนวนบัญชี สมาชิก วิดีโอ เทียบกับ backup เดิม ช่องรูปใน DB จะเปลี่ยนเป็น object keys ตามที่ตั้งใจ ตรวจ sample ภาพและวิดีโอจริงหลังเปิดแอปด้วย ไม่ใช้จำนวนแถวเพียงอย่างเดียวยืนยันว่ารับชมได้

## 11. เปิด EC2 Auto Scaling และชี้ DNS

1. Update stack โดยใช้ parameters เดิมทุกตัว เปลี่ยนเฉพาะ `StartApps=true` อย่าสั่งเปลี่ยน template/เปลี่ยน Origin/ย้าย bucket ไปพร้อมกัน
2. ควรได้เว็บสองเครื่องคนละ AZ และ Worker หนึ่งเครื่อง แต่ละเครื่องโหลด release เดียวกันและเริ่มจาก systemd/Docker อัตโนมัติ
3. รอ Target Group เว็บสองเครื่อง Healthy เส้นทาง `/ready` ตรวจ API/DB; `/health` ของ Nginx อย่างเดียวไม่ยืนยันว่าฐานข้อมูลพร้อม
4. ถ้า Unhealthy ใช้ SSM ตรวจ `sudo systemctl status rachata-plus`, `sudo journalctl -u rachata-plus -n 80` และ CloudWatch log group ห้ามพิมพ์ env/secrets ลง log ตรวจ CA, DB-SG, S3/IAM, ECR pull และ migration 015 ก่อน
5. เพิ่ม DNS CNAME ของ hostname ไป LoadBalancerDNS หรือ Alias หากผู้ให้บริการรองรับ รอ DNS แล้วเข้า `https://YOUR_HOSTNAME` ไม่เปิดเว็บผ่าน IP ของ EC2
6. Worker มี health check ภายใน Docker แต่ ASG Worker ใช้ EC2 health check เท่านั้น หาก process ทำงานผิดปกติ Docker จะ restart แต่ application failure ไม่ได้ทำให้ ASG แทนเครื่องอัตโนมัติทุกกรณี ต้องดู health/logs
7. เครื่อง migration/AMI builder ที่ไม่ใช้แล้วอยู่นอก stack ให้หยุดหลังตรวจครบ เพื่อลดงบ แต่คง backup และ AMI ที่จำเป็นไว้

## 12. ตรวจรับก่อนส่งอาจารย์

- สมัคร เข้าสู่ระบบ ออกจากระบบ แก้โปรไฟล์ และรูป avatar
- อัปโหลดเกิน 100 MB ผ่าน S3 multipart โดยไม่ผ่าน body ของ ALB/API ดู Network ว่า PUT ไป S3 ตรวจว่ากดอัปโหลดซ้ำไม่สร้างงานซ้ำโดยไม่ตั้งใจ
- ตัดเน็ตสั้น ๆ ระหว่างส่งหนึ่ง part แล้วต่อใหม่ ส่วนที่ล้มเหลวต้อง retry หากปิดหน้า/refresh ยังไม่มีระบบ resume งานเดิมข้ามหน้า ต้องเริ่มใหม่ งานค้างถูก cleanup ตาม TTL
- Worker แปลงจริง ภาพปกจริง ไม่มีข้อมูลจำลองใน Dashboard
- Basic/Standard/Premium จำกัดคุณภาพและจำนวนจอจริง ดูจบแล้วอยู่ประวัติ และเลือกเรื่องแนะนำต่อได้
- เปิดสองเครื่องเว็บ: สลับ/หยุดหนึ่ง target ผ่านขั้นตอนทดสอบที่ตกลงแล้ว บัญชีและการเล่นต้องยังทำงานบนอีกเครื่องโดยไม่ต้อง login ใหม่
- ทดสอบ ASG เพิ่ม desired จาก 2 เป็น 3 ก่อน แล้วลดกลับ 2 หลังทดสอบ เพื่อยืนยัน bootstrap/health checks ไม่ใช่แค่มี policy ใน Console
- ทดสอบ CPU target tracking ด้วย workload ที่ควบคุมได้และดู scaling activities ค่าเริ่มต้น 60% เป็นจุดเริ่มต้นสำหรับ Lab ไม่ใช่ capacity ที่รับรองแล้ว วิดีโออาจติด bandwidth ก่อน CPU ต้องตรวจเครื่องจริง
- นัดช่วงทดสอบ RDS failover แล้วใช้ Console Reboot with failover ตรวจว่า connection ใหม่กลับมาที่ writer endpoint และผู้ใช้ retry ได้ ห้ามทำระหว่างอัปโหลดที่ยังต้องการรักษา
- ตรวจ restart และการแทนเครื่อง Worker ว่างานถูกกู้คืนและ generation เก่าไม่ประกาศ ready ทับงานใหม่
- ตรวจ CloudWatch logs/alarms ตั้ง notification actions เองหากต้องการรับแจ้ง เพราะ template ยังไม่ได้ผูก SNS/email ให้ใคร
- ทดสอบ backup และ restore ของ RDS คู่กับ S3 object versions ในชุดใหม่แยกก่อนลบหรือแก้ข้อมูลจริง Multi-AZ/Versioning ไม่ใช่สำเนาสำรองแยกทั้งหมด

Production ปิด demo checkout อยู่แล้ว ใช้เจ้าของระบบ/แอดมินจัดแพ็กเกจให้บัญชีเดโม่ ไม่มีการเก็บเงินจริง ไม่ใช้รหัสผ่านบัญชีจริงของคุณในการสาธิต

## 13. อัปเดตรุ่นและ rollback

1. Build/push tag ใหม่ หา digests ใหม่ ทำ release tar ใหม่ที่มี SHA ใหม่ ห้ามเขียนทับ tag/release ที่ใช้แล้ว
2. สำรองก่อน migration ใหม่และรัน migration แยก หาก schema ไม่ backward-compatible ต้องวาง maintenance/rollback ก่อน ไม่หวังแก้ด้วยการย้อน image อย่างเดียว
3. Update stack ให้ Launch Template ชี้ image/release ใหม่ จากนั้นทำ Instance Refresh ของ **WebASG** ใน Console กำหนดให้คง healthy capacity ระหว่างเปลี่ยนเครื่อง ติดตามจนสองเครื่องใช้รุ่นใหม่และ healthy
4. Worker เปลี่ยนรุ่นในช่วงว่างของคิว หรือจัดช่วง maintenance งานที่ถูกตัดต้องถูกกู้คืนโดย worker ใหม่
5. Rollback: ชี้ digests/release เดิมและ refresh อีกครั้ง เฉพาะเมื่อ schema ยังรองรับ ห้ามลบ volume/bucket หรือ reset ฐานข้อมูลเพื่อ rollback

## 14. งบและการจบ Lab

- คู่มือ Lab ระบุว่า EC2 อาจหยุดเมื่อ session จบและเริ่มใหม่เมื่อ Start Lab แต่ **อย่าใช้สิ่งนี้รับประกันพฤติกรรมของ Auto Scaling** เพราะ ASG อาจต้องการแทนเครื่องที่หยุด ต้องตรวจจำนวนเครื่องและ scaling activities จริง
- Stop EC2 ที่อยู่ใน ASG อาจทำให้ ASG สร้างตัวแทน หากต้องพัก ให้ตกลงช่วง maintenance แล้ว Update `StartApps=false` เพื่อลด desired/min เป็น 0 และตรวจว่าไม่มีเครื่องแอปค้าง
- ALB/NAT/EIP, RDS, storage, AMI snapshots และ retained resources ยังมีค่าใช้จ่าย แม้ ASG เป็น 0 ไม่ใช่ปุ่มหยุดค่าใช้จ่ายทั้งหมด
- RDS อาจไม่หยุดตาม session และถ้าหยุดครบ 7 วัน AWS อาจเปิดกลับ ต้องตรวจตามคู่มือ Lab
- หากต้องรื้อชุดเดโม่ ให้สำรอง/ทดสอบกู้คืนและตกลงรายการที่จะลบก่อน RDS มี Deletion Protection; S3/secret/log group ถูก Retain จึงไม่หายตาม stack อัตโนมัติ อย่าลบทรัพยากรเดิมของงานอื่น
- ก่อนส่งลิงก์ให้อาจารย์ ต้องตกลงว่าจะเปิดตรวจช่วงไหน Lab ไม่ใช่โฮสต์ที่รับรองเปิดได้ตลอด และงบนี้ต้องเผื่อช่วงอาจารย์ตรวจด้วย

## แหล่งอ้างอิง

- คู่มือ Learner Lab ที่ผู้ใช้ส่งมา: จำกัด region, instance classes, IAM, RDS storage/monitoring และการจบ session
- AWS SDK Go v2 S3: https://github.com/awsdocs/aws-doc-sdk-examples/tree/main/gov2/s3
- Multipart upload: https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpuoverview.html
- RDS PostgreSQL TLS: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/PostgreSQL.Concepts.General.SSL.html
- ALB forwarding headers: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/x-forwarded-headers.html
- Instance Refresh: https://docs.aws.amazon.com/autoscaling/ec2/userguide/instance-refresh-overview.html
- Docker Compose release ที่ pin สำหรับ AMI: https://github.com/docker/compose/releases/tag/v2.39.4
- AWS CLI installation: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
- Pricing Calculator: https://calculator.aws/
