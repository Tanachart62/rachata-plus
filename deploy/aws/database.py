#!/usr/bin/env python3
"""Explicit migration/restore commands; never run from an Auto Scaling boot."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import uuid

root = Path(__file__).resolve().parents[2]
parser = argparse.ArgumentParser()
parser.add_argument('action', choices=['migrate', 'adopt-014', 'restore', 'backup', 'counts'])
parser.add_argument('--file', type=Path)
parser.add_argument('--network', choices=['host', 'bridge'], default='host', help='bridge is for isolated Docker Desktop verification')
args = parser.parse_args()
env = root / 'secrets/runtime.env'
ca = root / 'secrets/rds-ca.pem'
base = ['docker', 'run', '--rm', '-i', '--network', args.network, '--env-file', str(env),
        '--mount', f'type=bind,source={ca},target=/run/secrets/rds-ca.pem,readonly']
setup = 'export PGHOST="$DB_HOST" PGPORT="$POSTGRES_PORT" PGUSER="$POSTGRES_USER" PGPASSWORD="$POSTGRES_PASSWORD" PGDATABASE="$POSTGRES_DB" PGSSLMODE="$DB_SSLMODE" PGSSLROOTCERT="$DB_SSLROOTCERT"; '

def sql(text):
    subprocess.run(base + ['postgres:17', 'sh', '-ec', setup + 'exec psql -X -v ON_ERROR_STOP=1'],
                   input=text, text=True, encoding='utf-8', check=True)

def content(path):
    return re.sub(r'^\s*(BEGIN|COMMIT);\s*$', '', path.read_text(encoding='utf-8-sig'), flags=re.M)

if args.action in ['backup', 'restore']:
    if not args.file:
        parser.error('--file is required')
    file = args.file.resolve()
    if args.action == 'backup':
        with file.open('xb') as output:
            subprocess.run(base + ['postgres:17', 'sh', '-ec', setup + 'exec pg_dump --format=custom --no-owner --no-acl'], stdout=output, check=True)
    else:
        sql("DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public') THEN RAISE EXCEPTION 'Restore requires an empty public schema'; END IF; END $$;")
        with file.open('rb') as input_file:
            subprocess.run(base + ['postgres:17', 'sh', '-ec', setup + 'exec pg_restore --dbname="$POSTGRES_DB" --no-owner --no-acl --single-transaction --exit-on-error'], stdin=input_file, check=True)
    raise SystemExit(0)
if args.action == 'counts':
    sql("SELECT 'users' AS table_name,count(*) FROM users UNION ALL SELECT 'videos',count(*) FROM videos UNION ALL SELECT 'subscriptions',count(*) FROM subscriptions; SELECT status,count(*) FROM videos WHERE deleted_at IS NULL GROUP BY status;")
    raise SystemExit(0)
order = [(number, root / 'db/migrations' / name) for number, name in
         (line.split() for line in (root / 'deploy/aws/migration-order.txt').read_text().splitlines() if line.strip())]
checksums = {number: hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest() for number, path in order}
script = "SELECT pg_advisory_lock(724286010);\n"
if args.action == 'migrate':
    script += """DO $$ BEGIN
      IF to_regclass('public.app_migrations') IS NULL AND EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public') THEN
       RAISE EXCEPTION 'Existing schema: restore verified local 014 backup then use adopt-014; do not rerun old migrations';
      END IF;
      IF to_regclass('public.app_migrations') IS NOT NULL THEN
       IF NOT EXISTS(SELECT 1 FROM public.app_migrations) AND EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public' AND tablename<>'app_migrations') THEN
        RAISE EXCEPTION 'Empty migration ledger with existing tables: baseline verification must succeed before migrate';
       END IF;
      END IF;
    END $$;
    """
script += "CREATE TABLE IF NOT EXISTS public.app_migrations(version TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());\n"
if args.action == 'adopt-014':
    reference = 'verify_' + uuid.uuid4().hex
    script += f'BEGIN; CREATE SCHEMA {reference}; SET LOCAL search_path={reference};\n'
    script += '\n'.join(content(path) for number, path in order if number <= '014')
    # Compare complete columns, constraints and indexes against a fresh 014 schema.
    script += f"""
    DO $$ DECLARE expected jsonb; actual jsonb; ns text; signature jsonb;
    BEGIN
      FOREACH ns IN ARRAY ARRAY['{reference}','public'] LOOP
       SELECT jsonb_agg(item ORDER BY item::text) INTO signature FROM (
        SELECT jsonb_build_array('column',c.relname,a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,a.attidentity,
          replace(replace(COALESCE(pg_get_expr(d.adbin,d.adrelid),''),'{reference}.',''),'public.','')) AS item
        FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
        LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
        WHERE n.nspname=ns AND c.relkind='r' AND c.relname<>'app_migrations'
        UNION ALL
        SELECT jsonb_build_array('constraint',c.relname,k.conname,replace(replace(pg_get_constraintdef(k.oid),'{reference}.',''),'public.',''))
        FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=ns AND c.relname<>'app_migrations'
        UNION ALL
        SELECT jsonb_build_array('index',t.relname,i.relname,replace(replace(pg_get_indexdef(i.oid),'{reference}.',''),'public.',''))
        FROM pg_index x JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_class t ON t.oid=x.indrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname=ns AND t.relname<>'app_migrations'
       ) records;
       IF ns='{reference}' THEN expected:=signature; ELSE actual:=signature; END IF;
      END LOOP;
      IF expected IS DISTINCT FROM actual THEN RAISE EXCEPTION 'Schema differs from migrations 001-014: stop and inspect; no baseline adopted'; END IF;
    END $$;
    SET LOCAL search_path=public;
    DROP SCHEMA {reference} CASCADE;
    """
    for number, path in order:
        if number <= '014':
            script += f"INSERT INTO public.app_migrations(version,checksum) VALUES('{number}','{checksums[number]}') ON CONFLICT(version) DO NOTHING;\n"
    script += 'COMMIT;\n'
else:
    for number, path in order:
        digest = checksums[number]
        script += fr"""SELECT EXISTS(SELECT 1 FROM public.app_migrations WHERE version='{number}') AS already \gset
\if :already
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM public.app_migrations WHERE version='{number}' AND checksum='{digest}') THEN RAISE EXCEPTION 'Migration {number} checksum mismatch'; END IF; END $$;
\else
BEGIN;
{content(path)}
INSERT INTO public.app_migrations(version,checksum) VALUES('{number}','{digest}');
COMMIT;
\endif
"""
script += 'SELECT pg_advisory_unlock(724286010);\n'
sql(script)
