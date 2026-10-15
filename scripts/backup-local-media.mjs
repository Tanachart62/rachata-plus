// Local media backup. Quiesce uploads/worker before taking a DB + media pair.
import { execFileSync } from 'node:child_process'
import { createReadStream, mkdirSync, mkdtempSync, readdirSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import assert from 'node:assert/strict'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const flags = ['compose', '--env-file', '.env.local', '-p', 'rachata-plus-local', '-f', 'compose.yaml', '-f', 'compose.runtime.yaml']
const writers = execFileSync('docker', [...flags, 'ps', '--status', 'running', '--services'], { cwd: root, encoding: 'utf8', windowsHide: true, timeout: 30000 }).trim().split(/\s+/)
assert(!writers.some(name => ['api', 'worker'].includes(name)), 'Stop API/worker before backing up media')
const source = resolve(root, 'storage')
const backups = resolve(root, 'backups')
mkdirSync(backups, { recursive: true })
const archive = resolve(backups, 'media-' + new Date().toISOString().replaceAll(/[:.]/g, '-') + '.tar')
async function fingerprint(directory) {
  const entries = {}
  async function visit(folder) {
    for (const item of readdirSync(folder, { withFileTypes: true })) {
      assert(!item.isSymbolicLink(), 'Media backup rejects symbolic links')
      const path = join(folder, item.name)
      if (item.isDirectory()) await visit(path)
      else {
        assert(item.isFile(), 'Unexpected special file in media storage')
        const hash = createHash('sha256')
        for await (const chunk of createReadStream(path)) hash.update(chunk)
        entries[relative(directory, path)] = hash.digest('hex')
      }
    }
  }
  await visit(directory)
  return entries
}
const original = await fingerprint(source)
execFileSync('tar', ['-cf', archive, '-C', source, '.'], { windowsHide: true, timeout: 180000 })
const restored = mkdtempSync(resolve(backups, 'restore-media-'))
execFileSync('tar', ['-xf', archive, '-C', restored], { windowsHide: true, timeout: 180000 })
assert.deepEqual(await fingerprint(restored), original, 'Restored media file paths and SHA256 must match')
const archiveHash = createHash('sha256')
for await (const chunk of createReadStream(archive)) archiveHash.update(chunk)
writeFileSync(archive + '.json', JSON.stringify({ sha256: archiveHash.digest('hex'), files: Object.keys(original).length, restoreVerified: true, restoredDirectory: restored }, null, 2) + '\n', { mode: 0o600 })
console.log('PASS restored all media files; every SHA256 matches:', Object.keys(original).length)
console.log('Backup:', archive)
console.log('Verified restore directory:', restored)
