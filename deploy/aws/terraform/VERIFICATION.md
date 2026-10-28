# ผลตรวจ Terraform ชุดนี้

ตรวจบน Windows วันที่ 28 ตุลาคม 2026 โดย Terraform 1.17.0, AWS provider 6.70.0 และ Random provider 3.9.1

- Terraform CLI และ provider packages เทียบ SHA256 กับไฟล์ checksum จาก releases.hashicorp.com ตรงกัน การตรวจใช้ mirror ใน workspace เพราะดาวน์โหลดไฟล์ใหญ่ผ่านเน็ตขัดข้อง โดยไม่ได้ตั้ง mirror ใน configuration ที่ส่งให้ผู้ใช้
- `terraform init -backend=false` ผ่านจาก local mirror; `terraform validate` ผ่าน
- `terraform fmt -check` ผ่าน และตรวจ JSON/ลิงก์ในคู่มือแล้ว
- ใช้ test configuration แยกนอก repository และ mock providers ทั้ง AWS/Random: infrastructure before migration, apps after migration และ rejected unmigrated start ผ่าน 3 กรณี ไม่มี AWS API provisioning หรือค่าใช้จ่ายจากการทดสอบนี้
- ตรวจ User Data หลังแทนค่าทั้ง web/worker ด้วย `bash -n` ผ่าน ขนาดตัวอย่างประมาณ 2.9 KB ต่ำกว่าเพดาน raw 16 KB
- ตรวจ 6 subnets, NAT สองตัว, DB private/Multi-AZ, media private, EC2 ไม่มี Public IP, IMDSv2 hop2, TG เดียว และไม่มี IAM role resource

ยังไม่ได้ตรวจ apply/plan กับบัญชีจริง, Lab IAM/quota, RDS engine/class availability, certificate/DNS, cloud-init/build บน EC2, migration บน RDS, browser CORS, playback ข้ามเครื่อง, Auto Scaling หรือ failover ของ infrastructure ชุดนี้ ต้องทำตามคู่มือและตรวจ runtime ก่อนส่งงาน
