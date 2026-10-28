# Rachata Plus

A video streaming web application developed for an academic project. It supports guest browsing, registered accounts, membership plans, and administration. FFmpeg converts uploaded videos into adaptive HLS streams.

## Technology stack

| Component | Technology |
| --- | --- |
| Frontend | React, TypeScript, Vite, hls.js |
| Backend | Go, Gin, pgx |
| Database | PostgreSQL |
| Video processing | FFmpeg and ffprobe |
| Runtime | Docker Compose and Nginx |
| AWS | EC2 Auto Scaling, ALB, RDS, S3, Secrets Manager, CloudWatch |

## Features

- Registration, login, logout, Argon2id password hashing, and cookie-based sessions.
- Account settings, password changes, profile pictures, and session management.
- Video catalog, search, details, watch history, continue watching, completed videos, and watchlist.
- HLS quality selection and recommendations after playback.
- Membership quality limits and concurrent playback leases shared through PostgreSQL.
- Video upload, metadata/cover editing, publishing, hiding, deletion, and retranscoding.
- Owner and administrator permissions, membership management, account suspension, password resets, and session revocation.
- A database-backed administration dashboard for accounts, memberships, video status, processing queues, and recent activity.

| Plan | Price | Maximum quality | Concurrent screens |
| --- | --- | --- | --- |
| Basic | THB 219 | 1080p | 2 |
| Standard | THB 419 | 1440p | 2 |
| Premium | THB 789 | 2160p | 4 |

Available qualities depend on the source video; the worker does not upscale. Payment confirmation is a local demonstration only. No real payments are collected. Administrators assign demonstration memberships on AWS.

## Repository layout

```text
apps/web/                 React application, Dockerfile, and Nginx configuration
apps/api/                 Go API, background worker, and Dockerfile
db/migrations/            PostgreSQL schema migrations
deploy/aws/               User Data, bootstrap, database, and runtime helpers
scripts/                  Local database and media backup utilities
compose.yaml              Local PostgreSQL configuration
compose.runtime.yaml      Local API, worker, and web services
compose.aws.yaml          Standalone EC2 runtime using RDS and S3
AWS-CONSOLE.md            AWS Console deployment instructions
```

## Run locally

Install Docker Desktop with Docker Compose. From the repository root, copy the environment template and choose a local database password:

```powershell
Copy-Item .env.local.example .env.local
notepad .env.local
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml up -d --build
```

Open [http://127.0.0.1:8088/](http://127.0.0.1:8088/), unless `WEB_PORT` was changed. Check the actual runtime with:

```powershell
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml ps
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml logs --tail 100 api worker web
```

Docker builds install their own dependencies. For frontend development on the host:

```powershell
cd apps/web
npm ci
npm run dev
```

Use the origin/port configured in `vite.config.ts` and the API environment. Run `npm run build` from `apps/web` to create a frontend production build.

## Deploy using the AWS Console

Follow [AWS-CONSOLE.md](AWS-CONSOLE.md). This setup uses a public ALB, private EC2 application subnets, private RDS subnets, and a private S3 media bucket. Launch Templates use [user-data-github.sh](deploy/aws/user-data-github.sh) to fetch a pinned GitHub commit and build Docker images on EC2.

The web Auto Scaling Group runs Nginx and the API. A separate worker processes the PostgreSQL job queue. Sessions and playback leases live in RDS; source videos, HLS, covers, and uploaded avatars use S3. EC2 disk provides temporary processing space. Sticky sessions are unnecessary.

HTTPS requires a DNS name you control and an ACM certificate. A classroom-only HTTP option supports the exact AWS ALB DNS origin with `HTTP_DEMO='true'` in User Data. Use test credentials for this unencrypted demonstration. The standard HTTPS configuration uses secure cookies.

Database migrations run separately before application Auto Scaling Groups start. They never run automatically during EC2 boot. The Console guide also covers migration of existing local data.

## Database and backups

Local initialization applies migrations 001-014 only to an empty database volume. AWS additionally uses migration 015 for S3 object references, multipart uploads, and shared rate limits. Inspect existing schemas and apply only missing migrations. Never delete a database volume to update its schema.

For a consistent local backup, stop local writers and run both tools from the repository root:

```powershell
docker compose --env-file .env.local -p rachata-plus-local -f compose.yaml -f compose.runtime.yaml stop web api worker
node scripts/backup-local-db.mjs
node scripts/backup-local-media.mjs
```

The database tool verifies restoration into a temporary database. The media tool verifies checksums and retains verification files. Keep writers stopped until both backups finish. Restart local writers only if this installation is still intended to accept changes. AWS backups need a separate RDS/S3 retention and restore procedure.

## Submission contents

Include tracked source files, SQL migrations, Dockerfiles, Compose files, lockfiles, environment examples, and documentation. Dependencies/build outputs are generated with `npm ci` and `npm run build` and are not included.

When creating a ZIP manually, exclude `.git`, `.env`, `.env.local`, `secrets`, `storage`, `backups`, `node_modules`, `dist`, and generated caches. Database dumps may contain personal data and password hashes. Existing media and accounts are separate from the source submission; keep their original local copies outside the ZIP.

## Project scope

This is a classroom project. Build and health checks do not prove every user flow, capacity, accessibility requirement, or failover scenario. Validate login, playback, uploads, package limits across instances, backup restoration, and scaling on the deployed system before demonstration.

The submission does not include an automated test suite or lint tooling. CloudWatch infrastructure metrics are separate from the administration dashboard. The admin profile avatar currently uses a temporary preview; ordinary account avatars use the persisted upload path.
