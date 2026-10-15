// Back up the local Compose database and verify restoration in a NEW database.
// Stop API/worker writes first so the comparison uses a stable database.
import { execFileSync, spawnSync } from 'node:child_process'
import { openSync, closeSync, mkdirSync, renameSync, writeFileSync, createReadStream } from 'node:fs'
import { createHash, randomBytes } from 'node:crypto'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import assert from 'node:assert/strict'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const flags = ['compose', '--env-file', '.env.local', '-p', 'rachata-plus-local', '-f', 'compose.yaml', '-f', 'compose.runtime.yaml']
function command(args, options = {}) {
  return execFileSync('docker', args, { cwd: root, windowsHide: true, encoding: 'utf8', timeout: 30000, ...options })
}
const container = command([...flags, 'ps', '-q', 'db']).trim()
assert.match(container, /^[a-f0-9]{64}$/, 'Start the existing local database first')
const labels = JSON.parse(command(['inspect', container, '--format', '{{json .Config.Labels}}']))
assert.equal(labels['com.docker.compose.project'], 'rachata-plus-local')
assert.equal(labels['com.docker.compose.service'], 'db')
const activeWriters = command([...flags, 'ps', '--status', 'running', '--services']).trim().split(/\s+/)
assert(!activeWriters.some(name => ['api', 'worker'].includes(name)), 'Stop API and worker before backing up')

function sql(query, database) {
  const env = database ? ['-e', 'POSTGRES_DB=' + database] : []
  return command(['exec', '-i', ...env, container, 'sh', '-c', 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -X -At -v ON_ERROR_STOP=1'], { input: query }).trim()
}
function snapshot(database) {
  const tables = sql("SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename", database).split('\n').filter(Boolean)
  const counts = {}
  for (const table of tables) {
    const quoted = '"' + table.replaceAll('"', '""') + '"'
    counts[table] = sql('SELECT count(*) FROM public.' + quoted, database)
  }
  const columns = sql("SELECT table_name,column_name,data_type,is_nullable,column_default FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position", database)
  const constraints = sql("SELECT c.relname,k.conname,pg_get_constraintdef(k.oid) FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' ORDER BY c.relname,k.conname", database)
  return { counts, columns, constraints }
}
const stamp = new Date().toISOString().replaceAll(/[:.]/g, '-')
const folder = resolve(root, 'backups')
mkdirSync(folder, { recursive: true })
const target = resolve(folder, 'local-' + stamp + '.dump')
const partial = target + '.partial'
const before = snapshot()
const fd = openSync(partial, 'wx', 0o600)
try {
  const result = spawnSync('docker', ['exec', container, 'sh', '-c', 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc'], {
    cwd: root, windowsHide: true, timeout: 180000, stdio: ['ignore', fd, 'pipe'], encoding: 'utf8',
  })
  if (result.error || result.status !== 0) throw Error('Database backup failed; keep the .partial file for diagnosis')
} finally { closeSync(fd) }
renameSync(partial, target)

const restored = 'rachata_restore_' + randomBytes(8).toString('hex')
let created = false
try {
  sql('CREATE DATABASE ' + restored + ' TEMPLATE template0;')
  created = true
  const input = openSync(target, 'r')
  try {
    const result = spawnSync('docker', ['exec', '-i', '-e', 'POSTGRES_DB=' + restored, container, 'sh', '-c', 'exec pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-privileges --exit-on-error'], {
      cwd: root, windowsHide: true, timeout: 180000, stdio: [input, 'ignore', 'pipe'], encoding: 'utf8',
    })
    if (result.error || result.status !== 0) throw Error('Restore verification failed; the original database was not overwritten')
  } finally { closeSync(input) }
  assert.deepEqual(snapshot(restored), before, 'Restored table counts, columns and constraints must match')
} finally {
  if (created) sql('DROP DATABASE ' + restored + ';')
}
// The dump contains private account data. /backups is ignored by Git.
const hash = createHash('sha256')
for await (const chunk of createReadStream(target)) hash.update(chunk)
const sha256 = hash.digest('hex')
writeFileSync(target + '.json', JSON.stringify({ createdAt: stamp, sha256, tables: before.counts, restoreVerified: true }, null, 2) + '\n', { mode: 0o600 })
console.log('PASS backup restored into a separate temporary database; counts/schema match')
console.log('Backup:', target)
console.log('SHA256:', sha256)
