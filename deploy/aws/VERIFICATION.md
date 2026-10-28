# ผลตรวจรอบเตรียม AWS — 28 ตุลาคม 2026

ยังไม่มีการสร้างหรือแก้ทรัพยากรใน AWS และยังไม่ใช่การรับรอง production readiness ผลด้านล่างทำบนเครื่อง local, PostgreSQL 17 แยกสำหรับทดสอบ, FFmpeg จริง และ S3 simulator (Moto) ใช้ credentials ทดสอบ ไม่ใช่ AWS credentials จริง

## สิ่งที่ตรวจผ่าน

- Go build และ `go vet`; การเรียก `go test ./...` ใน source ไม่มี test files เพราะถูกนำออกตามขอบเขตก่อนหน้า จึงไม่ใช่หลักฐาน integration ด้วยตัวเอง
- Integration suite แยกนอก repository: API สองตัวอ่าน DB/S3 เดียวกัน แต่ใช้ scratch คนละ directory
  - guest/user ใช้ endpoint อัปโหลดแอดมินไม่ได้
  - multipart upload ข้าม API node และผูกกับ session เจ้าของ
  - Complete ซ้ำคืนวิดีโอเดิม ไม่สร้างแถวซ้ำ
  - Worker ดาวน์โหลดจาก S3 แปลง FFmpeg จริง และส่ง HLS กลับก่อน ready
  - อีก API อ่านภาพปกและ HLS ได้โดยไม่มีไฟล์ local ของวิดีโอ
  - ตรวจสิทธิ์ HLS, HEAD, byte range และ private/no-store
  - แก้ metadata/ตำแหน่งภาพปก S3 ผ่าน transaction
  - shared limiter รับ 10/20 คำขอพร้อมกันจาก API สองตัวตามเพดานรวมเดียว
  - production ปฏิเสธ DB SSL ที่ไม่ verify และ custom S3 test endpoint
- Frontend TypeScript/Vite build และ Nginx configuration ทั้ง local/AWS ตรวจผ่าน มี warning hls.js chunk >500 kB เดิม ซึ่งโหลดแยกเมื่อใช้เครื่องเล่น
- Docker images API, Worker และ Web build ได้ด้วย base image digests ที่ pin ไว้
- Compose AWS profiles web/worker ผ่านการตรวจ configuration โดยไม่อ่าน secrets จริง
- CloudFormation template ผ่าน cfn-lint; User Data ของสอง Launch Templates และ prepare-ami.sh ผ่าน bash syntax check
- Release packager ตรวจ archive allowlist ไม่มี env จริง, credentials, media หรือ DB dumps ใน deployment bundle

## สำรองและซ้อมย้ายข้อมูล

- DB backup local-2026-10-28T14-48-07-470Z.dump ตรวจ restore ในฐานข้อมูลชั่วคราว เทียบ counts/schema ผ่าน
- media-2026-10-28T14-48-17-465Z.tar ตรวจ SHA256 ของไฟล์สื่อ 202 ไฟล์ผ่าน
- Restore backup เดิมในฐานข้อมูลทดสอบแยก เทียบ schema กับ migrations 001–014 ก่อน adopt baseline ผ่าน
- ลง migration 015 แล้วเรียก migrate ซ้ำ ตรวจ checksum ledger โดยไม่รัน migrations เดิมซ้ำ ผ่าน
- ย้ายวิดีโอที่ยังไม่ลบ 4 รายการและรูปโปรไฟล์จากฐานข้อมูลทดสอบไป S3 simulator โดยไม่ลบ source local ผ่าน
- ตรวจ import ซ้ำ และเพิ่ม regression สำหรับภาพปกแก้ไขใน DB ไม่ให้ภาพเก่าบนดิสก์เขียนทับภาพที่ย้ายแล้ว ผ่าน

ฐานข้อมูล local ที่ใช้งานจริงยังอยู่ที่ schema เดิม 001–014 ไม่ได้ลง migration 015 ลงบนข้อมูลใช้งานจริงในรอบนี้ การทดสอบ restore/import ทำกับสำเนาแยก

## สิ่งที่ต้องตรวจบน AWS จริง

- สิทธิ์ LabRole, quota, RDS engine/class availability และการสร้าง service-linked roles
- ACM/domain DNS, HTTPS/cookies, trusted proxy/IP และ CORS ของ S3 ผ่าน browser จริง
- SigV4, temporary role credential refresh, S3 checksums และ multipart retry บน AWS S3 จริง Simulator ไม่ยืนยันสิทธิ์หรือพฤติกรรมบริการจริงทั้งหมด
- bootstrap จาก AMI ที่เลือก, ECR pull, CloudWatch logs, health checks, scale out/in และ Instance Refresh
- RDS Multi-AZ failover/reconnect และ network interruption
- CPU/RAM/disk/bandwidth และเวลาแปลงของไฟล์จริงบน EC2 รวมถึงการกู้คืน job เมื่อ Worker ถูกแทนที่
- backup/restore RDS + S3 versions ใน AWS และค่าใช้จ่ายจริงก่อนเปิดให้อาจารย์ตรวจ

ยังไม่ได้ตรวจ AWS end-to-end, load/capacity เต็มรูปแบบ หรือ accessibility audit ครบทุก flow
