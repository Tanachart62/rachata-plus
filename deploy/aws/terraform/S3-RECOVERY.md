# กู้ต่อเมื่อ Learner Lab ปิดการอ่าน S3 Object Lock

อาการ: apply สร้าง bucket แล้ว แต่ AWS provider อ่าน `GetBucketObjectLockConfiguration` ไม่ได้เพราะมี explicit deny จาก SCP ของ Lab ขณะที่ RDS/ALB/VPC สร้างสำเร็จแล้ว การเพิ่ม Allow ใน IAM ไม่สามารถหักล้าง SCP explicit deny นี้ได้

ไม่ลบ bucket, ไม่ destroy โครงสร้าง, ไม่แก้ SCP และไม่รัน saved plan เก่าซ้ำ Bucket resource แบบทั่วไปอ่าน Object Lock ทุกครั้ง แม้เราไม่ได้เปิดใช้ จึงเปลี่ยนเฉพาะการอ้างอิงตัว bucket เป็น `data.aws_s3_bucket` ที่ตรวจ bucket/location โดยไม่เรียก Object Lock ส่วน Block Public Access, ownership, encryption, versioning, CORS, lifecycle และ bucket policies ยังเป็น Terraform resources ตามเดิม

## สำหรับชุดที่ apply ค้างอยู่

1. เก็บ state เดิมและสำรองก่อนเปลี่ยน config รอบนี้มีสำเนาไว้ใน `backups/terraform-before-s3-recovery-*.tfstate` แล้ว ไฟล์มีรหัส DB ห้ามอัปขึ้น Git
2. ใช้ชื่อ bucket เดิมใน `terraform.tfvars`: `media_bucket_name` และ `artifacts_bucket_name` รอบนี้กรอกชื่อจริงให้แล้ว ไม่สร้างใหม่
3. โค้ดใช้ `removed` พร้อม `lifecycle.destroy=false` สำหรับ addresses เดิมสองตัว เมื่อ apply จะย้ายออกจากการจัดการตัว bucket ใน state โดย **เก็บ bucket และ objects จริงทั้งหมด** ไม่ใช้ raw state editing
4. `terraform validate` และ mock plans 4 กรณีผ่าน พร้อม live recovery plan: **16 to add, 0 to change, 0 to destroy** และ forget ตัว bucket สองรายการโดยไม่ลบ
5. รัน recovery saved plan จาก root โปรเจกต์:

```powershell
$env:AWS_PROFILE = 'learner'
$env:AWS_REGION = 'us-east-1'
terraform -chdir=deploy/aws/terraform apply 's3-recovery.tfplan'
```

ถ้าใช้ `$tf` เป็นตำแหน่ง executable ให้ใช้ `& $tf` แทน `terraform` อย่าใช้ `http-demo-first.tfplan` หรือ `first.tfplan` เก่าเพราะ state เปลี่ยนหลัง apply ครั้งแรกแล้ว

เมื่อจบให้ตรวจ Outputs และขั้นย้ายข้อมูลตาม README เว็บ/Worker ยังเป็น desired=0 ตามที่ตั้งใจ ยังไม่ใช่เว็บพร้อมรับชม

ถ้าต้องสร้างแผนใหม่หลัง credentials หมดอายุหรือ config เปลี่ยน:

```powershell
terraform -chdir=deploy/aws/terraform plan '-out=s3-recovery.tfplan'
```

ตรวจว่ารายการแก้ไขไม่มี delete/replacement ของ bucket/RDS/ข้อมูลเดิมก่อน apply หากเห็น unexpected delete ให้หยุดตรวจ ไม่ใช้ `-replace`, `taint`, `destroy` หรือ `-refresh=false` เพื่อข้ามปัญหา

## สำหรับเริ่ม deployment ใหม่ใน Lab

เตรียม private buckets ผ่าน `ensure-buckets.py --create` ก่อน Terraform ตาม README แล้วกรอกชื่อ buckets ที่มีอยู่ ไม่ใช้ general bucket resource ที่เรียก Object Lock API ใน Lab นี้

อ้างอิง: [Terraform removed block](https://developer.hashicorp.com/terraform/language/block/removed), [AWS provider bucket data source](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/s3_bucket)
