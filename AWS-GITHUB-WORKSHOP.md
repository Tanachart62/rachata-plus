# Workshop: Rachata Plus จาก GitHub → EC2 Launch Template + Auto Scaling

คู่มือนี้ทำผ่าน AWS Console ทีละขั้นตามแนวทาง Workshop ที่ส่งมา แต่ค่าทั้งหมดอิงโค้ด Rachata Plus: React/Vite + Nginx, Go/Gin, FFmpeg, PostgreSQL และ S3 ใช้ source ที่ `C:\Users\User\Desktop\rachata-plus`

**คู่มือนี้เป็นเส้นทาง GitHub ไม่ใช้ ECR หรือ S3 release สำหรับโค้ด เลือกทำคู่มือนี้เพียงทางเดียว ไม่สร้าง CloudFormation ซ้ำ** คู่มือนี้สร้างทรัพยากรด้วย Console ไม่ต้องสร้าง `stack.json` ซ้ำอีกชุด สคริปต์ตรวจใน local แล้ว แต่ยังไม่ได้สร้างบริการหรือทดสอบ deployment บน AWS จริง

## 0 — เตรียมก่อนเริ่ม

สิ่งที่ใช้: AWS Academy Learner Lab, Docker Desktop, AWS CLI v2, Python 3, Session Manager และ hostname ที่ควบคุม DNS ได้

- เลือก region **US East (N. Virginia) — us-east-1** ตลอดคู่มือ
- คุณยังไม่มีโดเมน: ขอ subdomain จากอาจารย์/สถาบัน หรือเตรียมโดเมนที่คุณมีสิทธิ์แก้ DNS ก่อนขั้น ACM ไม่จำเป็นต้องซื้อโดเมนผ่าน AWS
- งบที่แจ้งเหลือประมาณ **$26.90**; ALB, NAT สองตัว, เว็บสองเครื่อง, Worker และ RDS Multi-AZ มีค่าใช้จ่าย ควรเตรียมไฟล์และ hostname ให้ครบก่อนสร้างส่วนที่คิดเงิน และกำหนดช่วงให้อาจารย์ตรวจ
- Start Lab → AWS Details → ตั้ง temporary credentials รวม session token ใน AWS CLI profile `learner` บนเครื่องคุณ ห้ามใส่ credentials ใน Git, User Data หรือส่งในแชต
- ตรวจว่ามี instance profile `LabInstanceProfile` ซึ่งใช้ `LabRole` เดิมของ Lab ไม่สร้าง IAM role ใหม่
- ตรวจรายการบริการเดิมก่อนสร้าง ใช้ prefix `rachata-` ให้แยกจากงานอื่น ไม่กด Reset Lab

ใน PowerShell บนเครื่องคุณ:

```powershell
Set-Location C:\Users\User\Desktop\rachata-plus
$env:AWS_PROFILE = 'learner'
$env:AWS_REGION = 'us-east-1'
aws --version
python --version
docker version
aws sts get-caller-identity
$taskAccount = aws sts get-caller-identity --query Account --output text
$taskArtifacts = "rachata-artifacts-$taskAccount-us-east-1"
$taskMedia = "rachata-media-$taskAccount-us-east-1"
```

จดชื่อ buckets ไว้จนทำจบ ถ้าเปิด PowerShell ใหม่ให้ตั้งค่าตัวแปรเดิมกลับ

## 1 — โครงสร้างที่จะได้

```text
Users → DNS hostname → Internet Gateway → Public ALB HTTPS :443
                                          │
                                 Target Group HTTP :80
                                    /ready health check
                         ┌────────────────┴────────────────┐
                         │                                 │
                    Private App A                     Private App B
                    EC2 Web/API                       EC2 Web/API
                    Docker + Nginx                    Docker + Nginx
                    React → Go :8080                  React → Go :8080
                         └──────────────┬──────────────────┘
                                        │
                       RDS PostgreSQL :5432, private
                         Primary AZ-A ⇄ Standby AZ-B

        Private Worker EC2 (หนึ่งเครื่อง, แยก ASG) → RDS job queue
                         │ FFmpeg / EBS scratch
                         ▼
              S3 private: source / HLS / covers / avatars

Private App A → NAT-A (Public A) → Internet Gateway
Private App B → NAT-B (Public B) → Internet Gateway
Private App A/B → S3 Gateway Endpoint → S3

Browser อัปโหลด video parts → S3 ด้วย URL ชั่วคราวจาก API
Browser ดู HLS → ALB → API ตรวจสิทธิ์ → S3
```

เว็บใช้ Target Group เดียวที่พอร์ต 80 โดย Nginx แยก `/api/` ภายใน ไม่ต้องทำ TG frontend:3000 และไม่ต้องเปิด Sticky Session เพราะ session เก็บใน RDS ร่วมกัน Worker ไม่รับ traffic จาก ALB

## 2 — ใช้ source จาก GitHub และล็อก commit ที่จะ deploy

Repository: https://github.com/Tanachart62/rachata-plus เป็น Public จึงไม่ต้องใส่ GitHub token ใน EC2

ใน PowerShell บนเครื่องคุณ:

```powershell
$taskCommit = (git ls-remote https://github.com/Tanachart62/rachata-plus.git refs/heads/main).Split()[0]
$taskCommit
```

จด SHA เต็ม 40 ตัวนี้ไว้ ใช้ **ค่าเดียวกันกับทุกเครื่องและ migration host** ตรวจว่า commit นี้มี `deploy/aws/prepare-github.sh`, `deploy/aws/user-data-github.sh` และ bootstrap ที่รองรับ `--image-source local` แล้ว

User Data จะดึง commit นี้จาก GitHub แล้ว build Docker images ทั้งสามตัวบน EC2 และใช้ tag เป็น commit เดียวกัน ไม่ต้อง build/push ECR จากเครื่องคุณ ไม่ใช้ `git pull main` ทุกครั้งเพราะอาจทำให้เครื่องใน ASG ได้โค้ดคนละรุ่น

เครื่องใหม่ต้องดาวน์โหลด base images และ dependencies ผ่าน NAT ตอน build ครั้งแรกอาจใช้เวลาหลายนาทีถึงหลายสิบนาทีตาม CPU/เครือข่าย ต้องวัดจริงและปรับ grace period ให้ยาวกว่าระยะ build ไม่ใช่ขยายเครื่องแล้วรับ traffic ได้ทันที

## 3 — S3 สอง buckets สำหรับ media และข้อมูลย้ายระบบ

S3 → **Create bucket** สองครั้ง ใช้ชื่อจาก `$taskArtifacts` กับ `$taskMedia`:

| Field | Artifact bucket | Media bucket |
|---|---|---|
| Region | us-east-1 | us-east-1 |
| Object Ownership | Bucket owner enforced | Bucket owner enforced |
| Block all public access | เปิดทั้งหมด | เปิดทั้งหมด |
| Default encryption | SSE-S3 | SSE-S3 |
| Bucket Versioning | Enable | Enable |

Artifact เก็บ backup สำหรับย้ายข้อมูล ส่วน Media เก็บหนัง/ภาพ ไม่ตั้ง public-read หรือ public bucket policy

Media bucket → **Permissions → CORS → Edit** ใส่ hostname จริง:

```json
[
  {
    "AllowedOrigins": ["https://YOUR_HOSTNAME"],
    "AllowedMethods": ["PUT"],
    "AllowedHeaders": ["*"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 600
  }
]
```

Media → Management → Lifecycle rules → Create rule → prefix `media/` → เลือกเฉพาะ **Delete incomplete multipart uploads หลัง 2 วัน** ไม่ตั้งลบวิดีโอที่สมบูรณ์หรือเวอร์ชันเก่าโดยยังไม่ตกลง retention

ทั้งสอง bucket → Permissions → Bucket policy เพิ่ม deny HTTP โดยแทน `YOUR_BUCKET` ของแต่ละตัว:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Sid": "DenyInsecureTransport",
    "Effect": "Deny",
    "Principal": "*",
    "Action": "s3:*",
    "Resource": ["arn:aws:s3:::YOUR_BUCKET", "arn:aws:s3:::YOUR_BUCKET/*"],
    "Condition": {"Bool": {"aws:SecureTransport": "false"}}
  }]
}
```

จดชื่อ bucket ทั้งสองไว้ โค้ดมาจาก GitHub จึง **ไม่ต้องสร้างหรืออัปโหลด release.tar.gz** ส่วน backup จะอัปโหลด prefix `migration/` ในขั้น 12 เก็บเป็น private ทั้งหมด

## 4 — VPC และ Subnets

VPC → **Your VPCs → Create VPC → VPC only**:

| Field | Value |
|---|---|
| Name | `rachata-vpc` |
| IPv4 CIDR | `10.42.0.0/16` |
| Tenancy | Default |

เลือก VPC → Actions → Edit VPC settings → เปิด DNS resolution และ DNS hostnames

VPC → Subnets → Create subnet → เลือก `rachata-vpc` สร้าง:

| Name | AZ | CIDR | ใช้ทำอะไร |
|---|---|---|---|
| `rachata-public-a` | us-east-1a | `10.42.0.0/24` | ALB + NAT-A |
| `rachata-public-b` | us-east-1b | `10.42.1.0/24` | ALB + NAT-B |
| `rachata-app-a` | us-east-1a | `10.42.10.0/24` | Web/API/Worker |
| `rachata-app-b` | us-east-1b | `10.42.11.0/24` | Web/API/Worker |
| `rachata-db-a` | us-east-1a | `10.42.20.0/24` | RDS |
| `rachata-db-b` | us-east-1b | `10.42.21.0/24` | RDS |

คง **Auto-assign public IPv4 ปิด** ทุก subnet เครื่องแอปต้องไม่มี Public IP ส่วน ALB/NAT จัดการ public address ของบริการเอง

ใช้ CIDR ตามนี้ เพราะ `deploy/aws/nginx.conf` เชื่อถือ forwarded headers จากสอง public subnets นี้ ถ้าเปลี่ยนต้องแก้ Nginx, commit/push แล้วใช้ Git commit ใหม่ทั้งชุด

## 5 — Internet Gateway, NAT และ Route Tables

1. VPC → Internet gateways → Create → `rachata-igw` → Attach to `rachata-vpc`
2. Route tables → Create → `rachata-rt-public` ใน VPC นี้ → Routes → เพิ่ม `0.0.0.0/0 → rachata-igw`
3. Subnet associations → Associate `rachata-public-a` และ `rachata-public-b`
4. Elastic IP addresses → Allocate สอง IP สำหรับ NAT-A และ NAT-B
5. NAT gateways → Create **Public NAT แบบอยู่ใน AZ** ชื่อ `rachata-nat-a` ใน public-a เลือก EIP แรก; สร้าง `rachata-nat-b` ใน public-b เลือก EIP ที่สอง รอ Available
6. สร้าง route tables ตามตาราง แล้วผูก subnet ให้ตรง:

| Route table | Subnet associations | Default route |
|---|---|---|
| `rachata-rt-public` | public-a, public-b | `0.0.0.0/0 → IGW` |
| `rachata-rt-app-a` | app-a | `0.0.0.0/0 → NAT-A` |
| `rachata-rt-app-b` | app-b | `0.0.0.0/0 → NAT-B` |
| `rachata-rt-db` | db-a, db-b | ไม่มี default route; เก็บ local route |

ทุกตารางมี local route `10.42.0.0/16` อยู่แล้ว ไม่ลบทิ้ง Network ACL ใช้ค่า default ของ VPC ก่อน

VPC → **Endpoints → Create endpoint** → AWS services → เลือก `com.amazonaws.us-east-1.s3` ชนิด **Gateway** → VPC นี้ → เลือก route tables `rachata-rt-app-a` และ `rachata-rt-app-b` → สร้าง ใช้ endpoint policy ที่อนุญาต bucket ของเราและไม่ขัด LabRole ค่า Full access ของ endpoint ไม่ได้ทำให้ S3 เป็น public และยังต้องผ่าน IAM/bucket policy

ผลที่ควรได้: app subnets ออกไป Secrets Manager/CloudWatch/GitHub/Docker registry ผ่าน NAT และเข้า S3 ผ่าน Gateway Endpoint; DB ไม่มีทางออกตรงสู่อินเทอร์เน็ต

## 6 — Security Groups

EC2 → Security Groups → Create ใน `rachata-vpc` สร้างสี่กลุ่มก่อน แล้วแก้ inbound/outbound ให้ใช้ SG ID จริง:

| SG | Inbound | Outbound |
|---|---|---|
| `rachata-alb-sg` | TCP 80,443 จาก `0.0.0.0/0` | TCP 80 ไป `rachata-app-sg` |
| `rachata-app-sg` | TCP 80 จาก `rachata-alb-sg` | All traffic ตามค่าเริ่มต้น |
| `rachata-worker-sg` | ไม่มี | All traffic ตามค่าเริ่มต้น |
| `rachata-db-sg` | TCP 5432 จาก app-sg และ worker-sg สอง rules | ค่าเริ่มต้น |

เลือกชนิด source/destination เป็น **Security group** สำหรับ rule ระหว่างกลุ่ม ไม่ใช้ `0.0.0.0/0` เปิดพอร์ต DB ไม่เปิด 8080 หรือ SSH จากอินเทอร์เน็ต

## 7 — RDS PostgreSQL Multi-AZ

RDS → **Subnet groups → Create DB subnet group**:

| Field | Value |
|---|---|
| Name | `rachata-db-subnets` |
| VPC | rachata-vpc |
| AZ / Subnets | db-a และ db-b เท่านั้น |

RDS → Databases → **Create database → Standard create**:

| Field | Value |
|---|---|
| Engine | PostgreSQL |
| Engine version | **17.x ที่ Lab/region มีให้เลือกจริง** ตรง major กับ backup local |
| Template | Production แล้วตรวจค่าด้านล่างเอง |
| Availability | Multi-AZ DB instance deployment: Primary + Standby **สอง instances** |
| Identifier | `rachata-db` |
| Master username | `rachata` |
| Credentials | Self managed; สร้างรหัสผ่านสุ่มและเก็บใน password manager |
| Class | db.t3.small ถ้า Lab อนุญาตและมีให้ใช้กับ PG17.x |
| Storage | gp2, 20 GiB; ปิด storage autoscaling สำหรับเพดาน Lab นี้ |
| VPC / Subnet group | rachata-vpc / rachata-db-subnets |
| Public access | No |
| Security groups | เฉพาะ rachata-db-sg |
| Port | 5432 |
| Initial database name | `rachata_plus` |
| Encryption | Enable, AWS managed key ที่ Lab อนุญาต |
| Backup retention | 7 days |
| Enhanced Monitoring | **ปิด** |
| Database Insights | ไม่เปิด Advanced/ตัวเลือกที่ต้อง role เพิ่ม; ตรวจค่าใช้จ่ายก่อน |
| Log exports | PostgreSQL log ถ้า Lab อนุญาต |
| Deletion protection | Enable |

เลือก Primary+Standby ไม่ใช่ Multi-AZ DB cluster สามเครื่อง ถ้าตัวเลือก PG17.x/class/Multi-AZ ไม่มีให้ใช้ หยุดตรวจข้อจำกัด Lab ก่อนกดสร้าง ไม่เปลี่ยน major ฐานข้อมูลเอง

รอ Available → Connectivity & security → จด **Endpoint ของ DB** ไม่ใช้ IP และไม่ต่อ Standby โดยตรง

Secrets Manager → **Store a new secret → Other type of secret → Key/value**:

| Key | Value |
|---|---|
| `username` | `rachata` |
| `password` | รหัสเดียวกับ RDS ที่ตั้งเมื่อกี้ |
| `dbname` | `rachata_plus` |

Encryption key ใช้ค่า AWS managed ที่อนุญาต → Name `rachata/database` → ยังไม่เปิด automatic rotation → Store → จด Secret ARN อย่าใส่ค่ารหัสผ่านใน User Data; bootstrap อ่าน secret ผ่าน instance role เอง ถ้าเปลี่ยนรหัส RDS ต้องปรับ secret ให้ตรงกันด้วย

## 8 — CloudWatch และสิทธิ์ instance role

CloudWatch → Logs → Log groups → **Create log group** ` /rachata-plus/application ` (ไม่รวมช่องว่าง) → ตั้ง retention 7 days

ตรวจ LabRole เดิมว่ามีสิทธิ์สำหรับบริการต่อไปนี้ ไม่ต้องสร้าง access key ให้ EC2:

| บริการ | สิทธิ์ที่ต้องใช้งาน |
|---|---|
| S3 artifact | List bucket / GetObject สำหรับ backup ที่ใช้ย้าย |
| S3 media | ListBucket, GetBucketLocation, Get/Put/DeleteObject, multipart list/abort |
| Secrets Manager | GetSecretValue ของ secret นี้ |
| CloudWatch Logs | CreateLogStream, PutLogEvents และอ่านข้อมูล stream ที่ driver ต้องใช้ |
| Systems Manager | สิทธิ์ SSM managed instance ตาม role ของ Lab |

หาก AccessDenied ให้ตรวจ resource ARN, region และข้อจำกัด Lab กับอาจารย์ ไม่เปลี่ยน S3 เป็น public เพื่อหลบปัญหา IAM

## 9 — Domain และ ACM สำหรับ HTTPS

1. เตรียม hostname ที่ควบคุม DNS ได้ เช่น `video.YOUR_DOMAIN`
2. ACM → us-east-1 → Request certificate → Public certificate → ใส่ hostname → DNS validation
3. ไปผู้ให้บริการ DNS ของ hostname แล้วเพิ่ม CNAME validation ตาม ACM
4. รอ certificate เป็น **Issued** และจด ARN

ตอนนี้คุณยังไม่มีโดเมน ขั้นนี้จึงต้องจัดหา hostname ก่อนเดินต่อไปเปิดแอป DNS ของ ALB ใช้ดูปลายทางได้ แต่เราไม่สามารถขอ ACM certificate สำหรับชื่อ `*.elb.amazonaws.com` ของ AWS มาแทนได้ ระบบ production ที่เตรียมไว้ต้องใช้ HTTPS และ secure cookies

## 10 — Target Group และ ALB

EC2 → **Target Groups → Create target group**:

| Field | Value |
|---|---|
| Target type | Instances |
| Name | `rachata-web-tg` |
| Protocol / Port | HTTP / 80 |
| VPC | rachata-vpc |
| Health check | HTTP, path `/ready`, success code 200 |
| Healthy / Unhealthy threshold | 2 / 8 |
| Interval / Timeout | 15 / 5 seconds |

ไม่ต้อง register เครื่องเอง → Create → Attributes → Stickiness **Off**, Deregistration delay **120 seconds**

EC2 → **Load Balancers → Create → Application Load Balancer**:

| Field | Value |
|---|---|
| Name | `rachata-alb` |
| Scheme / IP type | Internet-facing / IPv4 |
| VPC | rachata-vpc |
| Network mapping | public-a และ public-b |
| Security group | rachata-alb-sg เท่านั้น |
| HTTPS :443 | Forward ไป rachata-web-tg |
| Certificate | ACM ที่ Issued แล้วใน us-east-1 |
| TLS security policy | เลือก policy ปัจจุบันที่รองรับ TLS 1.2/1.3 |
| HTTP :80 | Redirect ไป HTTPS :443, status HTTP_301 |

ถ้า wizard ให้ตั้ง redirect หลังสร้าง ให้แก้ listener 80 ทันที ตรวจ Attributes → Idle timeout ใช้ 120 seconds เป็นค่าเริ่มต้นสำหรับทดสอบ จด ALB DNS name ตอนนี้ TG ว่างเป็นเรื่องปกติ ยังไม่เปิดแอปจนย้ายข้อมูลครบ

## 11 — User Data และ Launch Templates

เปิด **`deploy/aws/user-data-github.sh`** แล้วคัดลอกไฟล์เพื่อกรอกค่าบนเครื่องคุณ ไม่แก้รหัสผ่านลง source

| Variable | ค่าที่ต้องใส่ |
|---|---|
| ROLE | `web` สำหรับเว็บ, `worker` สำหรับ Worker |
| REGION | `us-east-1` |
| GIT_COMMIT | SHA เต็ม 40 ตัวที่จดในขั้น 2 |
| DB_SECRET_ARN | ARN ของ secret `rachata/database` |
| DB_HOST | RDS endpoint ไม่ใส่พอร์ตหรือ scheme |
| MEDIA_BUCKET | ชื่อ bucket media ไม่ใส่ `s3://` |
| ORIGIN | `https://HOSTNAMEจริง` ไม่มี path หรือ `/` ท้าย |
| LOG_GROUP | `/rachata-plus/application` |

**Script ที่ใช้คือ `user-data-github.sh` ไม่ใช่ `user-data.sh` รุ่น S3/ECR** ไม่ใส่ GitHub token, AWS keys หรือรหัสผ่าน RDS ใน script

EC2 → **Launch Templates → Create launch template** สร้างสองชุด:

| Field | Web | Worker |
|---|---|---|
| Name | `rachata-web-lt` | `rachata-worker-lt` |
| Auto Scaling guidance | เปิด | เปิด |
| AMI | Amazon Linux 2023 64-bit **x86** มี AWS CLI v2 | AMI ID เดียวกัน |
| Instance type | t3.medium | t3.large |
| Key pair | ไม่จำเป็นเมื่อใช้ SSM | เหมือนกัน |
| Subnet | ไม่ระบุ ให้ ASG เลือก | เหมือนกัน |
| Security group | rachata-app-sg | rachata-worker-sg |
| Root EBS | gp3 30 GiB encrypted | gp3 80 GiB encrypted |
| IAM instance profile | **LabInstanceProfile** | **LabInstanceProfile** |
| Metadata endpoint | Enabled | Enabled |
| Metadata version | V2 only / Required | เหมือนกัน |
| Metadata response hop limit | **2** | **2** |
| User Data | script ที่กรอกแล้ว ROLE=web | script เดียวกันแต่ ROLE=worker |

Network settings → Advanced network configuration หากเพิ่ม interface ให้ device index 0, **Auto-assign public IP Disable**, ไม่ล็อก subnet และใช้ SG ของเครื่องชนิดนั้น หากกำหนด SG ใน interface แล้วไม่ต้องเลือกซ้ำอีกตำแหน่ง

Advanced details → User data → วาง script ตั้งแต่ `#!/bin/bash` เป็นข้อความธรรมดา **ไม่ติ๊ก already base64 encoded** ช่อง IAM instance profile เลือก `LabInstanceProfile` ซึ่งใช้ role `LabRole` เดิมของ Lab

Script ติดตั้ง Docker/Git → ดาวน์โหลด helper จาก GitHub commit ที่ระบุ → fetch source commit เดียวกัน → build API/Worker/Web ตาม Dockerfiles → คัดลอกเฉพาะ runtime config/SQL ไป `/opt/rachata-plus` → อ่าน DB secret → เริ่ม profile ที่เลือก มี systemd service เรียก bootstrap หลัง reboot และใช้ images ที่ build สำเร็จแล้วซ้ำ

ไม่รัน migrations หรือ restore ใน User Data ต้องย้ายข้อมูลด้วยเครื่องชั่วคราวก่อนเปิด ASG และใช้ commit เดียวกันทั้งชุด

## 12 — สำรองและย้ายข้อมูลก่อนเปิด ASG

รอบนี้เป็น cutover ให้หยุดการเขียน local ชั่วคราวและสำรองใหม่ ถ้าจะยังพัฒนาต่อให้แยกช่วงย้ายจริง อย่าใช้ backup เก่าโดยไม่เทียบข้อมูลล่าสุด

PowerShell เครื่องคุณ:

```powershell
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml stop web api worker
node scripts/backup-local-db.mjs
node scripts/backup-local-media.mjs
```

ต้องได้ PASS จาก restore verification และ checksum จดไฟล์ `.dump`, `.tar` จริง แล้วอัปโหลดคู่เดียวกันพร้อม metadata/checksum ไป private artifact bucket prefix `migration/`:

```powershell
aws s3 cp backups/YOUR_DB.dump "s3://$taskArtifacts/migration/YOUR_DB.dump"
aws s3 cp backups/YOUR_MEDIA.tar "s3://$taskArtifacts/migration/YOUR_MEDIA.tar"
Get-FileHash backups/YOUR_DB.dump -Algorithm SHA256
Get-FileHash backups/YOUR_MEDIA.tar -Algorithm SHA256
```

### 12.1 เครื่อง migration ชั่วคราว

EC2 → Launch instance → Amazon Linux 2023 x86_64, t3.large, VPC นี้, **app-a**, Public IP Disable, worker-sg, LabInstanceProfile, EBS gp3 80 GiB encrypted, IMDSv2 Required/hop2 ใช้ Session Manager เครื่องนี้อยู่นอก ASG ไม่เปิด inbound SSH

User Data ของ migration host ใช้แค่เตรียมเครื่องมือ **ไม่เริ่ม Worker**:

```bash
#!/bin/bash
set -euo pipefail
dnf install -y docker git python3 unzip
systemctl enable --now docker
```

Connect → Session Manager → รอ `sudo cloud-init status --wait` แล้วรัน โดยใส่ commit เดียวกับ Launch Templates:

```bash
GIT_COMMIT='YOUR_FULL_40_CHARACTER_COMMIT'
[[ "$GIT_COMMIT" =~ ^[a-f0-9]{40}$ ]]
curl --fail --location --retry 3 "https://raw.githubusercontent.com/Tanachart62/rachata-plus/$GIT_COMMIT/deploy/aws/prepare-github.sh" -o /tmp/prepare-github.sh
sudo bash /tmp/prepare-github.sh "$GIT_COMMIT"
cd /opt/rachata-plus
sudo python3 deploy/aws/bootstrap.py --image-source local --role worker --region us-east-1 \
  --secret YOUR_DATABASE_SECRET_ARN --db-host YOUR_RDS_ENDPOINT \
  --bucket YOUR_MEDIA_BUCKET --origin https://YOUR_HOSTNAME \
  --api-image "rachata-plus-api:$GIT_COMMIT" --web-image "rachata-plus-web:$GIT_COMMIT" \
  --worker-image "rachata-plus-worker:$GIT_COMMIT" --log-group /rachata-plus/application --prepare-only
sudo install -d -m 0700 /opt/rachata-import
```

`--prepare-only` เขียน config/CA และตรวจ local images แต่ไม่เริ่ม API/Worker อย่าแสดงไฟล์ secrets/runtime.env ลงจอหรือแชต ข้อความท้าย helper เดิมเรื่องสร้าง AMI ไม่ต้องทำ เพราะเส้นทางนี้ใช้ stock AMI + User Data

หาก build ล้มเหลวให้แก้ปัญหาแล้วรัน helper commit เดิมซ้ำ images ที่สำเร็จแล้วจะถูกใช้ซ้ำ ห้ามข้าม build error ไปทำ migrations

### 12.2 ย้ายฐานข้อมูล

```bash
sudo aws s3 cp s3://YOUR_ARTIFACT_BUCKET/migration/YOUR_DB.dump /opt/rachata-import/local.dump --region us-east-1
sudo aws s3 cp s3://YOUR_ARTIFACT_BUCKET/migration/YOUR_MEDIA.tar /opt/rachata-import/media.tar --region us-east-1
sudo sha256sum /opt/rachata-import/local.dump /opt/rachata-import/media.tar
sudo tar -tf /opt/rachata-import/media.tar
```

เทียบ SHA256 กับเครื่องคุณ และตรวจรายชื่อไฟล์ใน tar ก่อนแตกว่าเป็น storage ของโปรเจกต์ตามที่สำรอง จากนั้น:

```bash
sudo install -d -m 0700 /opt/rachata-import/storage
sudo tar -xf /opt/rachata-import/media.tar -C /opt/rachata-import/storage
cd /opt/rachata-plus
sudo python3 deploy/aws/database.py restore --file /opt/rachata-import/local.dump
sudo python3 deploy/aws/database.py adopt-014
sudo python3 deploy/aws/database.py migrate
sudo python3 deploy/aws/database.py counts
```

ตรวจว่าโครงสร้างหลังแตกเป็น `/opt/rachata-import/storage/<storage_key>/...` ตาม media backup ไม่ซ้อน `storage/storage/` หาก layout ต่างให้ปรับ mount ในขั้นถัดไปให้ชี้ directory ที่มี storage keys โดยตรง

ใช้ชุด restore → adopt-014 นี้สำหรับ backup local schema 001–014 ที่ตรวจในรอบเตรียม ถ้า schema เปลี่ยนไปแล้วต้องตรวจใหม่ โปรแกรมจะปฏิเสธ schema ที่ไม่ตรง ห้ามข้าม guard หรือรัน migrations เก่าทับข้อมูล

ถ้าเลือกฐานข้อมูลใหม่ว่างจริง ให้ใช้ `database.py migrate` อย่างเดียว แต่ต้องเตรียมเจ้าของระบบและข้อมูลใหม่แยก สำหรับโปรเจกต์นี้แนะนำย้ายข้อมูลเดิมพร้อมบัญชีเดิม

### 12.3 ย้ายวิดีโอ/ภาพไป S3

```bash
cd /opt/rachata-plus
sudo docker run --rm --network host --env-file secrets/runtime.env \
  --mount type=bind,source=/opt/rachata-plus/secrets/rds-ca.pem,target=/run/secrets/rds-ca.pem,readonly \
  --mount type=bind,source=/opt/rachata-import/storage,target=/import,readonly \
  -e MEDIA_IMPORT_DIR=/import "rachata-plus-api:$GIT_COMMIT" import-media
```

ตรวจ dry run ไม่มี error จากนั้นรันคำสั่งเดิมเพิ่ม `--apply` ต่อท้าย `import-media` จึงอัปโหลดจริงและบันทึก object keys ใน RDS ตรวจ counts อีกครั้ง พร้อมตรวจ S3 prefix `media/` มี source/HLS/covers/avatars ที่คาดไว้ โปรแกรมไม่ลบไฟล์ local

อย่าเปิด ASG จน schema 015 และ import เสร็จ ไม่แสดง `secrets/runtime.env` ลงแชตหรือ log เพราะมีรหัสผ่าน

## 13 — สร้าง Auto Scaling Groups

EC2 → **Auto Scaling Groups → Create Auto Scaling group**:

| Field | เว็บ | Worker |
|---|---|---|
| Name | `rachata-web-asg` | `rachata-worker-asg` |
| Launch template | rachata-web-lt, ระบุเลข version ที่ทดสอบ | rachata-worker-lt, ระบุเลข version |
| VPC / Subnets | rachata-vpc, app-a + app-b | เหมือนกัน |
| Load balancing | Attach existing rachata-web-tg | ไม่ attach |
| Health checks | EC2 + Elastic Load Balancing | EC2 |
| Health check grace period | 3600 seconds เริ่มต้นสำหรับ build ครั้งแรก | 3600 seconds |
| Desired / Min / Max | **2 / 2 / 3** | **1 / 1 / 1** |
| Default instance warmup | 300 seconds เริ่มต้น แล้วปรับตามเวลาจริง | 300 seconds |
| Scaling policy | Target tracking: Average CPU 60% | ไม่เพิ่ม policy |
| Tags propagated to instances | Name=rachata-web | Name=rachata-worker |

Worker ASG วางในสอง subnet แต่สร้าง **รวมหนึ่งเครื่อง** ใช้ PostgreSQL advisory lock คุมคิว ไม่ใช่หนึ่ง Worker ต่อ AZ และไม่เพิ่ม Worker ตาม CPU ของเว็บ

ช่วง grace period 3600 วินาทีทำให้ ASG รอ build ได้นานขึ้น แต่ทำให้การแทนเครื่องที่เสียช่วงเริ่มต้นช้าลงด้วย หลังวัดเวลา startup จริงควรปรับให้เหมาะสม

รอ EC2 เริ่ม → Target Group → Targets ต้องได้เว็บสองเครื่อง **Healthy** ควรเห็นกระจายคนละ AZ → ASG Activity ไม่มีการสร้าง/ลบเครื่องวน ถ้า unhealthy ให้เปิด log ก่อน ไม่ลด health checks เพื่อกลบปัญหา

User Data รันตอนเปิดเครื่องใหม่ ส่วน systemd ที่ script สร้างจะเรียก bootstrap หลัง reboot; การเปลี่ยน Launch Template อย่างเดียวไม่เปลี่ยนเครื่องที่รันอยู่ ต้องทำ Instance Refresh เมื่ออัปเดตรุ่น

## 14 — ชี้ DNS และตรวจเว็บ

ผู้ให้บริการ DNS → เพิ่ม CNAME `video` ไป ALB DNS หรือ Alias หากรองรับ ไม่ชี้ไป private IP ของ EC2 รอ DNS แล้วเข้า `https://HOSTNAMEจริง`

ตรวจตามลำดับ:

1. `https://HOSTNAME/ready` ได้ 200 และ HTTPS certificate ถูกต้อง
2. Login ด้วยบัญชีเดิม → รูปโปรไฟล์/ประวัติ/รายการสมาชิกถูกต้อง
3. ภาพปกและ HLS เดิมอ่านได้จริง ไม่ใช่ดูเพียง counts
4. อัปโหลดหนังใหม่เกิน 100 MB → Network เห็น PUT ไป S3 → คิวแปลง → ready → ดูได้
5. บัญชีฟรี/สมาชิกได้สิทธิ์ตรงแพ็กเกจ จำกัดจอ/คุณภาพจริง ดูจบขึ้นประวัติ
6. Dashboard แสดงข้อมูลจริงจาก RDS และ Worker
7. Production ปิด demo checkout; ใช้เจ้าของระบบ/แอดมินจัดแพ็กเกจบัญชีสาธิต ไม่มีเก็บเงินจริง

ต้องทดสอบบน AWS จริงทั้งหมดก่อนรายงานว่าส่งงานพร้อม ผลทดสอบ local ก่อนหน้านี้ยังไม่ยืนยัน IAM, browser CORS, ASG หรือ RDS failover ของชุดนี้

## 15 — สาธิต Auto Scaling และ High Availability

เลือกช่วงไม่มีอัปโหลดสำคัญ และจดจำนวนเครื่องก่อนทดสอบ:

1. เพิ่ม Web desired จาก 2 → 3 รอเครื่องใหม่ Healthy เพื่อยืนยัน User Data ทำงานบนเครื่องใหม่
2. ลดกลับ 2 และตรวจ scaling activities/Target Group
3. ทดสอบแทนเครื่องเว็บหนึ่งตัวผ่านขั้นตอนที่ตกลงกันแล้วใน EC2/ASG ระหว่างนั้นลองเว็บและ login อีกเครื่อง บัญชีต้องไม่หลุดเพียงเพราะเปลี่ยน target อาจมี request ที่ต้อง retry ระหว่าง connection เดิมถูกตัด
4. ถ้าจะสาธิต CPU policy ต้องสร้าง workload ที่ควบคุมได้แล้วดู metric และ scaling activity การเปลี่ยน desired เองพิสูจน์ bootstrap แต่ยังไม่พิสูจน์ target tracking
5. ทดสอบ RDS Reboot with failover ในช่วง maintenance แล้วตรวจแอปกลับมาต่อ writer endpoint เดิม ต้องเผื่อช่วง reconnect ไม่รับรองว่าไม่มีสะดุด

อย่าใช้การหยุด Worker ระหว่างแปลงงานจริงเป็นการทดสอบรอบแรก Worker มี health check ใน Docker แต่ ASG แบบ EC2 health check ไม่ได้ตรวจ business health ของคิว ต้องดู CloudWatch/คิวด้วย

## 16 — เปิด log เมื่อมีปัญหา

Session Manager บนเครื่องที่มีปัญหา:

```bash
sudo cloud-init status
sudo tail -n 100 /var/log/cloud-init-output.log
sudo systemctl status rachata-plus --no-pager
sudo journalctl -u rachata-plus -n 100 --no-pager
sudo docker ps
curl -i http://127.0.0.1/ready
```

คำสั่ง curl ใช้กับเครื่องเว็บ เครื่อง Worker ไม่มีเว็บพอร์ต 80 อ่าน application logs ใน CloudWatch → `/rachata-plus/application` หลีกเลี่ยง `docker inspect` ทั้งก้อน, `env`, หรือ `cat secrets/runtime.env` ซึ่งอาจแสดงค่าลับ

| อาการ | จุดตรวจ |
|---|---|
| SSM ไม่ online | instance profile, SSM agent, NAT route, outbound HTTPS |
| User Data ไม่จบ | cloud-init-output, x86_64, AWS CLI, NAT/GitHub/registry, Docker build logs |
| Bootstrap AccessDenied | role, secret ARN, S3 bucket owner, STS/CloudWatch region |
| Target Unhealthy | SG ALB→app80, /ready, CA/DB5432, migration015, image health |
| Upload CORS error | S3 AllowedOrigins ต้องตรง HTTPS hostname และ ExposeHeaders ETag |
| Login มีปัญหา | ORIGIN, HTTPS, secure cookie, Nginx trusted public CIDRs, DB |
| วิดีโอไม่แปลง | Worker logs, RDS lock/queue, scratch เหลือ, S3 access, job budget |
| รูป/หนังเก่าหาย | import layout, media prefix/keys, counts และ import logs |

## 17 — อัปเดตและพักระบบหลังส่ง

อัปเดตรุ่น: commit/push โค้ดใหม่ → จด Git commit SHA เต็ม → Create new version ของ LT และแก้ GIT_COMMIT ใน User Data → ASG เลือก version ใหม่ → Instance Refresh ของเว็บโดยคง healthy capacity → เปลี่ยน Worker เมื่อคิวว่าง ต้องสำรองและทำ migration แยกก่อนหากรุ่นใหม่เปลี่ยน schema

การพักแอป: ASG ทั้งสองกลุ่มตั้ง Min=0, Desired=0 และพัก scaling policies/กำหนด Max=0 ชั่วคราวเพื่อไม่ให้ policy ขยายกลับ ตรวจว่าไม่มี instances ค้าง ไม่กด Stop EC2 ใน ASG อย่างเดียวเพราะอาจถูกสร้างทดแทน เมื่อกลับมาเปิดคืนค่า Min/Desired/Max และ policy ตามขั้น 13

**ALB, NAT/EIP, RDS, EBS/snapshots และ S3 ยังอาจมีค่าใช้จ่ายแม้แอปเป็นศูนย์เครื่อง** ตรวจ Lab budget และกำหนดแผนเก็บ/รื้อหลังอาจารย์ตรวจ เก็บ backup ที่ตรวจ restore แล้วก่อนตกลงลบข้อมูล ไม่มีขั้นลบ bucket/DB อัตโนมัติในคู่มือนี้

## แหล่งอ้างอิง

- รูปแบบขั้นตอนจาก [Workshop ที่คุณส่งมา](https://app.notion.com/p/Workshop-AWS-Production-Deployment-315d519d5aa6809793b5fa3864a6682b) ปรับเทคโนโลยี/network/storage ให้ตรงโปรเจกต์นี้
- [EC2 User Data](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html): Console รับ script ธรรมดาได้ ขนาดก่อน base64 ไม่เกิน 16 KB
- [Launch Template สำหรับ Auto Scaling](https://docs.aws.amazon.com/autoscaling/ec2/userguide/create-launch-template.html)
- [Amazon Linux 2023 มี AWS CLI v2](https://docs.aws.amazon.com/linux/al2023/ug/awscli2.html)
- [RDS ใน VPC และ DB subnet groups](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_VPC.WorkingWithRDSInstanceinaVPC.html)
- [HTTPS Listener ของ ALB](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/create-https-listener.html)
- คู่มือทางเลือก S3/ECR: [AWS-WORKSHOP.md](AWS-WORKSHOP.md), CloudFormation: [AWS-DEPLOY.md](AWS-DEPLOY.md); ผลตรวจ local: [VERIFICATION.md](deploy/aws/VERIFICATION.md)
