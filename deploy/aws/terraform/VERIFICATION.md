# ผลตรวจ Terraform ชุดนี้

ตรวจบน Windows วันที่ 28 ตุลาคม 2026 โดย Terraform 1.17.0, AWS provider 6.70.0 และ Random provider 3.9.1

- Terraform CLI และ provider packages เทียบ SHA256 กับไฟล์ checksum จาก releases.hashicorp.com ตรงกัน การตรวจใช้ mirror ใน workspace เพราะดาวน์โหลดไฟล์ใหญ่ผ่านเน็ตขัดข้อง โดยไม่ได้ตั้ง mirror ใน configuration ที่ส่งให้ผู้ใช้
- `terraform init -backend=false` ผ่านจาก local mirror; `terraform validate` ผ่าน
- `terraform fmt -check` ผ่าน และตรวจ JSON/ลิงก์ในคู่มือแล้ว
- ใช้ test configuration แยกนอก repository และ mock providers ทั้ง AWS/Random: infrastructure before migration, apps after migration และ rejected unmigrated start ผ่าน 3 กรณี ไม่มี AWS API provisioning หรือค่าใช้จ่ายจากการทดสอบนี้
- ตรวจ User Data หลังแทนค่าทั้ง web/worker ด้วย `bash -n` ผ่าน ขนาดตัวอย่างประมาณ 2.9 KB ต่ำกว่าเพดาน raw 16 KB
- ตรวจ 6 subnets, NAT สองตัว, DB private/Multi-AZ, media private, EC2 ไม่มี Public IP, IMDSv2 hop2, TG เดียว และไม่มี IAM role resource

ยังไม่ได้ตรวจ apply กับบัญชีจริง, สิทธิ์สร้างทรัพยากร/quotas ทั้งหมดของ Lab, cloud-init/build บน EC2, migration บน RDS, browser CORS, playback ข้ามเครื่อง, Auto Scaling หรือ failover ของ infrastructure ชุดนี้ ต้องทำตามคู่มือและตรวจ runtime ก่อนส่งงาน

## เพิ่มโหมด HTTP วันที่ 29 ตุลาคม 2026

- HTTP เป็น opt-in `http_demo=true`; Terraform ใช้ ALB DNS จริงเป็น origin และ CORS ไม่สร้าง listener 443 และไม่ต้องมี hostname/ACM ส่วนโหมด HTTPS เดิมยังรองรับ
- `terraform validate` ผ่าน และ mock plans ผ่าน 4 กรณี รวม HTTP แบบไม่มีโดเมน/certificate
- External Go tests ตรวจ HTTP runtime/cookie attributes/Origin guard, configuration ที่ต้องถูกปฏิเสธ 8 กรณี, HTTPS production และ localhost HTTP ผ่าน พร้อม `go vet ./...` ผ่าน ไม่เพิ่ม test files ลงชุด source ส่งงาน
- Bootstrap mock checks ตรวจ HTTP/HTTPS env และ prepare-only ผ่าน; User Data ผ่าน `bash -n`
- ตรวจ RDS orderable options จริงใน us-east-1: db.t3.small, gp2, Multi-AZ รองรับ PostgreSQL 17.5–17.11 ณ เวลาตรวจ เลือก 17.11 ใน local tfvars และพบ LabInstanceProfile เดิม
- `terraform plan` กับ credentials ของ Learner Lab ผ่าน: **68 to add, 0 to change, 0 to destroy**; ตรวจแผนว่า HTTP listener forward, ไม่มี HTTPS listener, เว็บ/Worker desired=0 และ DB/private EC2 ตามที่ตั้งใจ
- Saved plan และ local tfvars ไม่เข้า Git; ยังไม่ได้รัน `apply` หรือสร้างบริการจากชุดนี้ ผล plan ไม่รับประกันสิทธิ์ Create APIs หรือ runtime หลังสร้าง

## กู้ apply หลังพบ S3 SCP deny วันที่ 29 ตุลาคม 2026

- apply ครั้งแรกสร้าง VPC/NAT/ALB/RDS Multi-AZ/migration host จริงแล้ว แต่ general aws_s3_bucket Read ล้มเหลวเมื่ออ่าน Object Lock ที่ SCP ของ Lab explicit deny ทำให้สอง bucket ถูก mark tainted ใน state โดย bucket จริงสร้างแล้วและ Block Public Access ครบทั้งสี่ค่าตรวจแล้ว
- สำรอง state ลง backups ก่อนแก้ แทนการอ่าน bucket metadata ทั่วไปด้วย data.aws_s3_bucket และ removed destroy=false สำหรับ addresses เดิม ไม่แก้ IAM/SCP และไม่ลบ bucket/objects
- Recovery live plan ผ่าน: **16 to add, 0 to change, 0 to destroy** มี state forget สองรายการที่เก็บ buckets จริงไว้ ตรวจ machine-readable plan ว่า RDS/no-op และไม่มี update/delete
- terraform validate และ mock plans 4 กรณีผ่านอีกครั้ง; ensure-buckets.py syntax และ mock cases existing/missing-check/missing-create/forbidden/wrong-region ผ่าน โดย AccessDenied ไม่สร้าง bucket และไม่เรียก Object Lock API
- ยังไม่ได้ apply recovery plan หรือยืนยันแอปบน AWS หลัง recovery

## แก้ Compose checksum วันที่ 29 ตุลาคม 2026

- พบจาก migration EC2 ว่า checksums.txt ของ Compose v2.39.4 ใช้ binary marker ` *docker-compose-linux-x86_64`; grep เดิมเลือกไม่เจอจึงหยุดก่อน build
- ตรวจไฟล์ checksum ทางการและแก้ prepare-ami.sh กับ User Data รุ่น S3/ECR ให้รับทั้ง binary/text checksum markers โดยยังเลือกชื่อ binary ตรงตัว ไม่เลือก provenance/SBOM
- ทดสอบ parser/checksum ด้วย fixtures ทั้งสองรูปแบบผ่าน และไฟล์ที่แก้ไขถูกปฏิเสธ; bash syntax ผ่าน
- Pin Git source รุ่นแก้ไขใน Terraform/example/local configuration เพื่อไม่ให้ ASG เครื่องใหม่ดึง script รุ่นที่ล้มเหลว
- ยังต้องรัน helper รุ่นใหม่บน EC2 ให้ build ผ่าน แล้ว update Launch Template/ASG ที่ยังมี desired=0 ก่อนเปิดแอป
