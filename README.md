# Rachata Plus

คู่มือขึ้น AWS แบบทำตาม AWS Console ทีละขั้น พร้อม User Data สำหรับ Launch Template: [AWS-WORKSHOP.md](AWS-WORKSHOP.md)

เว็บวิดีโอสำหรับงานส่งอาจารย์ ใช้ React + TypeScript + Vite, Go + Gin + pgx, PostgreSQL และ FFmpeg/ffprobe แปลง HLS หลายความละเอียด

ชุด source นี้เก็บโค้ดแอป ไฟล์ build และไฟล์รันระบบ ส่วน tests และเครื่องมือ lint ถูกนำออกตามขอบเขตส่งงาน ไม่รวมการเก็บเงินจริง

## โครงสร้างที่ใช้ build และรัน

- `apps/web/src/` — หน้าเว็บ เครื่องเล่น และการเชื่อม API
- `apps/web/package.json`, `package-lock.json` และ `tsconfig*.json` — dependency และการ build frontend
- `apps/web/Dockerfile`, `apps/web/nginx/nginx.conf` — build และส่งเว็บ/HLS
- `apps/api/` — API, worker, Go modules และ Dockerfile
- `db/migrations/` — migrations 001–014 ที่ Compose อ้างอิง
- `compose.yaml`, `compose.runtime.yaml` — PostgreSQL, API, worker และ web สำหรับ local
- `scripts/backup-local-db.mjs`, `scripts/backup-local-media.mjs` — backup/ตรวจ restore ของชุด local

React, hls.js, TypeScript และ Vite ยังจำเป็นต่อแอปหรือขั้นตอน build แม้ production จะส่งเฉพาะไฟล์ static ผ่าน Nginx ส่วน FFmpeg อยู่ใน worker image

## รันชุด local ด้วย Docker

สร้าง `.env.local` จาก `.env.local.example` แล้วเปลี่ยนรหัสผ่านฐานข้อมูลก่อนเริ่ม อย่านำตัวอย่างรหัสผ่านไปใช้บน AWS

```sh
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml up -d --build
```

ค่าเริ่มต้นเปิดที่ `http://127.0.0.1:8088/` ตรวจสถานะด้วย `docker compose` โดยใช้ env/project/ไฟล์ Compose ชุดเดียวกันตามคำสั่งข้างต้น

Docker build ติดตั้ง dependency ให้เอง ไม่ต้องแนบ `node_modules` หรือ `dist` ไปกับ source หากจะ build frontend บน host ให้เข้า `apps/web` แล้วรัน `npm ci` และ `npm run build`

## ข้อกำหนดก่อน deploy AWS

อ่าน [AWS-DEPLOY.md](AWS-DEPLOY.md) สำหรับขั้นตอนละเอียดและข้อจำกัด Learner Lab แผน AWS ใช้ ALB + EC2 Auto Scaling สอง AZ, Worker แยกหนึ่งเครื่อง, RDS PostgreSQL Multi-AZ และ S3 private โดยไม่มี CloudFront

- `compose.aws.yaml` เป็น standalone แยกจาก Compose local ไม่รัน PostgreSQL บน EC2 และไม่ให้เครื่องเว็บรอ Worker ในเครื่องเดียวกัน
- `deploy/aws/stack.json` เตรียม VPC/subnets, NAT สองตัว, S3 endpoint, ALB, RDS, Secrets Manager, CloudWatch และ Launch Templates/ASGs โดยใช้ LabInstanceProfile เดิม ยังไม่ได้สร้างทรัพยากรจริง
- production ต้อง `MEDIA_BACKEND=s3`, `MEDIA_DELIVERY=direct`, `DB_SSLMODE=verify-full`, HTTPS และ secure cookies ไฟล์ HLS ยังผ่าน API ตรวจสิทธิ์ก่อนส่งจาก S3
- Migration 015 ใช้สำหรับ AWS เพิ่ม object keys, direct multipart uploads และ shared rate limits แยกจาก local 001–014 สคริปต์ migration มี schema baseline/checksum ledger และไม่รันจาก Auto Scaling boot
- รูปภาพถูก normalize ก่อนอัปโหลดและมีงบ decoder/scratch ตามเครื่อง การรับไฟล์ใหญ่ไม่ใช่การรับประกันว่าทรัพยากรไม่จำกัด
- ต้องมีโดเมน/subdomain ที่ควบคุม DNS ได้และ ACM certificate ก่อนสร้างชุด HTTPS ตรวจงบจริงก่อนสร้าง ALB/NAT/RDS เพราะแม้ StartApps=false ก็ยังมีค่าใช้จ่ายของ infrastructure
- ต้องตรวจ IAM, quota, capacity, scaling, RDS failover, backup/restore และ flows บน AWS จริงก่อนส่งอาจารย์ ผลทดสอบ local/emulator ไม่แทนผล AWS

Demo checkout เปิดเฉพาะ local เมื่อเป็น production ให้แอดมินจัดสิทธิ์สมาชิกแก่บัญชีเดโม่ ไม่มีระบบรับเงินจริง Basic สูงสุด 1080p/2 จอ, Standard 1440p/2 จอ, Premium 2160p/4 จอ ตามต้นฉบับและไม่ upscale Dashboard แอดมินดึงจำนวนบัญชี สมาชิก สถานะวิดีโอ คิวแปลง และกิจกรรมจาก PostgreSQL จริง อัปเดตทุก 30 วินาทีขณะเปิดหน้า ไม่แสดงกราฟยอดชม/CPU/RAM จำลอง และยังไม่ได้เชื่อม CloudWatch

## ฐานข้อมูลและการสำรอง

Compose เรียก migrations 001–014 อัตโนมัติเฉพาะ volume ว่าง ฐานข้อมูลเดิมต้องตรวจ schema และใช้เฉพาะ migration ที่ขาด ห้ามลบ volume เพื่ออัปเดต schema และห้ามรัน migration ซ้ำโดยไม่ตรวจ

สคริปต์ backup ปัจจุบันใช้ได้กับ Compose local เท่านั้น ก่อน backup ให้หยุด API/worker/web ด้วยพารามิเตอร์ Compose ชุดเดียวกับที่เปิดระบบ แล้วรันจาก root:

```sh
node scripts/backup-local-db.mjs
node scripts/backup-local-media.mjs
```

หยุดตัวเขียนตลอดการสำรองทั้งคู่ เปิดกลับเมื่อสำเร็จ สคริปต์ DB ใช้ฐานข้อมูลชั่วคราวตรวจ restore; media ตรวจ checksum และเก็บโฟลเดอร์ตรวจ restore ไว้ ต้องปรับวิธีสำรองให้ตรงกับเครื่อง AWS และเก็บสำเนาแยกเครื่อง

## จัดชุด source ส่งงาน

เก็บไฟล์ source, migrations, Docker/Compose, lockfiles และตัวอย่าง env ไม่แนบ `.env`, `.env.local`, `.git`, `backups`, `node_modules`, `dist`, `test-results` หรือ `.archify` ส่วน `storage` ส่งเฉพาะสื่อที่ตั้งใจแนบและมีสิทธิ์เผยแพร่ ข้อมูลบัญชีและ avatar อยู่ในฐานข้อมูล ไม่ได้รวมใน source

ยังไม่ได้ยืนยันการ deploy AWS หรือ production readiness การผ่าน build ไม่แทนการตรวจ runtime, capacity หรือ WCAG ทั้งหมด

## เจ้าของระบบและแอดมิน

Migration 014 (`db/migrations/add_admin_controls_and_video_edits.sql`) เพิ่มระดับสิทธิ์โดยคง role เดิมเป็น `user`/`admin` และเพิ่ม `admin_level` เป็น `owner`/`admin` สำหรับแอดมิน บัญชี admin เดิมจะเป็นเจ้าของระบบ ตรวจ schema ก่อนใช้ migration นี้เพียงครั้งเดียว สำรอง/ตรวจ restore และหยุด API/worker รุ่นเก่าก่อนลง ไม่ลบ volume

- เจ้าของระบบแต่งตั้งหรือถอดแอดมิน/เจ้าของระบบ จัดการสมาชิก วิดีโอ และลบบัญชีได้ เปลี่ยน role หรือลบบัญชีตัวเองผ่านคำสั่งแอดมินไม่ได้ จึงไม่สามารถทำให้เจ้าของระบบคนสุดท้ายหายไปผ่านหน้าเว็บ
- แอดมินจัดการวิดีโอและบัญชีผู้ใช้ทั่วไปได้ แต่แก้สิทธิ์ ลบ หรือจัดการบัญชีแอดมินคนอื่นไม่ได้ รีเซ็ตรหัสผ่านให้ผู้ใช้ทั่วไปได้
- เปลี่ยน role/รีเซ็ตรหัสผ่าน/ลบบัญชีต้องยืนยันรหัสผ่านผู้ดำเนินการ และตรวจ revision ก่อนบันทึก มี audit ใน `membership_events` ไม่เก็บรหัสผ่านใน audit ทุกคำสั่งนี้ยกเลิก session/สิทธิ์รับชมเดิมของเป้าหมาย
- รีเซ็ตรหัสผ่านกำหนดค่าใหม่ 8–128 ตัวอักษร แจ้งเจ้าของบัญชีผ่านช่องทางส่วนตัว ให้ผู้ใช้เปลี่ยนอีกครั้งในโปรไฟล์ ระบบยังไม่มีการส่งอีเมลรีเซ็ตหรือบังคับเปลี่ยนครั้งแรก
- ลบบัญชีต้องพิมพ์อีเมลยืนยัน ล้างชื่อ อีเมล รูป รหัสผ่าน ประวัติและดูภายหลัง พร้อมยกเลิกสมาชิก/เซสชัน บัญชีจะถูกซ่อนและเปิดคืนผ่านเว็บไม่ได้ เก็บแถวที่ไม่ระบุตัวตนเพื่อให้ audit เดิมอ้างอิงได้ ไม่ใช่การลบประวัติ audit หรือสำเนา backup ที่เคยสร้างไว้
- คลังวิดีโอมีปุ่มแก้ชื่อ คำอธิบาย หมวดหมู่ และภาพปก โดยไม่เปลี่ยนไฟล์ต้นฉบับ/สถานะเผยแพร่หรือแปลง HLS ใหม่ ตรวจ metadata revision เพื่อกันการบันทึกทับจากหน้าเก่า
- ภาพปกที่เลือกถูกย่ออัตโนมัติและ normalize เป็น JPEG ใน local เก็บภาพแก้ไขใน PostgreSQL และภาพอัปโหลดเดิมใน storage ส่วนโหมด AWS เก็บใน S3 โดยมีงบ decoder ป้องกัน RAM เต็ม การสำรอง DB และ media/object versions ยังต้องเก็บเป็นคู่

Dashboard ใช้ GET /auth/admin/dashboard เฉพาะแอดมิน ไม่ cache และอ่านข้อมูลใน transaction เดียว สมาชิกนับบัญชีปกติที่มี subscription active และอยู่ในช่วงเวลาใช้งาน ไม่นับบัญชี/วิดีโอที่ลบแล้ว กิจกรรมบัญชีมาจาก membership_events ส่วนวิดีโอแสดงการอัปเดตล่าสุดของแต่ละรายการ ไม่ใช่ audit ประวัติการแก้ไขทุกครั้ง เมื่อ API ล้มเหลวจะแสดงข้อผิดพลาด ไม่มีข้อมูลจำลองทดแทน

local เอาเพดานวิดีโอ 100 MB และข้อจำกัดภาพต้นฉบับเดิมออกแล้ว Nginx local ไม่กำหนด client_max_body_size แต่ API ยังคงงบพื้นที่ MEDIA_JOB_BYTES/พื้นที่รวม/พื้นที่ว่างของเครื่อง (local 1 GiB ต่อ job, 10 GiB รวม) และ Cloudflare อาจจำกัด request ก่อนถึงแอป โหมด AWS ส่งต้นฉบับตรง S3 แบบแบ่งส่วน ส่วน API/Nginx รับ metadata กับภาพที่ย่อแล้ว ไม่ได้เปิด request body ทุกชนิดแบบไร้ขอบเขต
