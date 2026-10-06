// Local integration check. Uses only Node built-ins and the running Compose stack.
// Stops/restarts OUR worker: run when no other uploads are pending/processing.
import { execFileSync } from 'node:child_process'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import assert from 'node:assert/strict'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const base = process.env.RUNTIME_TEST_URL ?? 'http://127.0.0.1:8088'
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(base)) throw Error('This check allows only local HTTP')
const project = process.env.RUNTIME_TEST_PROJECT ?? 'rachata-plus-local'
if (!/^rachata-plus-[a-z0-9-]+$/.test(project)) throw Error('Unexpected Compose project name')
const flags = ['compose', '--env-file', '.env.local', '-p', project, '-f', 'compose.yaml', '-f', 'compose.runtime.yaml']
function docker(args, timeout = 90000) { return execFileSync('docker', [...flags, ...args], { cwd: root, encoding: 'utf8', timeout, windowsHide: true, maxBuffer: 8 * 1024 * 1024 }) }
function sql(query) { return docker(['exec', '-T', 'db', 'psql', '-U', 'rachata', '-d', 'rachata_plus', '-At', '-v', 'ON_ERROR_STOP=1', '-c', query]).trim() }
const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
async function waitFor(check, label, milliseconds = 90000) {
  const deadline = Date.now() + milliseconds
  while (Date.now() < deadline) { if (await check()) return; await pause(300) }
  throw Error('Timed out: ' + label)
}
async function request(path, cookie, options = {}) {
  return fetch(base + path, { ...options, signal: AbortSignal.timeout(15000), headers: { Origin: base, ...(cookie ? { Cookie: cookie } : {}), ...options.headers } })
}
async function login(email) {
  const response = await request('/auth/login', null, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password: process.env.RUNTIME_TEST_PASSWORD ?? 'RachataDemo!2026' }) })
  assert.equal(response.status, 200, 'seeded test login: ' + email)
  const cookies = response.headers.getSetCookie().filter(value => /^rachata_session=[^;]/.test(value))
  assert.equal(cookies.length, 1)
  return cookies[0].split(';')[0]
}
const ids = []
let admin, stopped = false
const tag = 'Runtime check ' + Date.now()
async function video(id) {
  const response = await request('/api/videos/' + id, admin)
  assert.equal(response.status, 200)
  return (await response.json()).video
}
async function awaitReady(id) {
  await waitFor(async () => {
    const current = await video(id)
    assert.notEqual(current.processingStatus, 'failed', current.processingError)
    return current.processingStatus === 'ready'
  }, 'video ready')
}
async function upload(bytes, suffix) {
  const form = new FormData()
  form.append('title', tag + ' ' + suffix)
  form.append('description', 'Temporary local integration fixture')
  form.append('category', 'ความรู้')
  form.append('file', new Blob([bytes], { type: 'video/mp4' }), 'fixture.mp4')
  const response = await request('/api/admin/videos', admin, { method: 'POST', body: form })
  assert.equal(response.status, 202)
  const record = (await response.json()).video
  assert(Number.isSafeInteger(record.id) && record.id > 0)
  ids.push(record.id)
  return record.id
}
async function publish(id, status) {
  const response = await request('/api/admin/videos/' + id, admin, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ publicationStatus: status }) })
  assert.equal(response.status, 200)
}
try {
  assert.equal((await request('/ready')).status, 200)
  assert.equal(sql("SELECT count(*) FROM videos WHERE status IN ('pending','processing') AND deleted_at IS NULL"), '0', 'finish other uploads before this check')
  assert.equal(docker(['exec', '-T', 'api', 'sh', '-c', 'if command -v ffmpeg; then exit 1; fi; id -u']).trim(), '10001')
  admin = await login('admin@example.com')
  const member = await login('member@example.com')
  const free = await login('free@example.com')
  // Real 1080p source makes interruption/cancellation meaningful on the limited worker.
  docker(['exec', '-T', 'worker', 'ffmpeg', '-nostdin', '-v', 'error', '-y', '-f', 'lavfi', '-i', 'testsrc2=size=1920x1080:rate=24', '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=48000', '-t', '40', '-c:v', 'libx264', '-threads', '2', '-preset', 'ultrafast', '-crf', '38', '-pix_fmt', 'yuv420p', '-c:a', 'aac', '/tmp/runtime-fixture.mp4'])
  // Stream binary bytes out of tmpfs; Docker's archive/cp does not read this mount.
  const bytes = execFileSync('docker', [...flags, 'exec', '-T', 'worker', 'cat', '/tmp/runtime-fixture.mp4'], { cwd: root, windowsHide: true, maxBuffer: 32 * 1024 * 1024 })
  docker(['exec', '-T', 'worker', 'rm', '/tmp/runtime-fixture.mp4'])
  docker(['stop', '-t', '10', 'worker']); stopped = true
  const first = await upload(bytes, 'playback')
  await pause(1500)
  assert.equal((await video(first)).processingStatus, 'pending')
  assert.equal((await request('/ready')).status, 200)
  console.log('PASS API accepts uploads without FFmpeg and without the worker')
  docker(['start', 'worker']); stopped = false
  await awaitReady(first)
  await publish(first, 'published')
  const full = `/media/${first}/full/index.m3u8`, preview = `/media/${first}/preview/index.m3u8`, segment = `/media/${first}/full/segment_000000.ts`
  for (const path of [full, segment]) {
    assert.equal((await request(path)).status, 401)
    assert.equal((await request(path, free)).status, 403)
    assert.equal((await request(path, null, { headers: { 'X-Accel-Redirect': '/_hls/forged' } })).status, 401)
  }
  const playlist = await request(full, member)
  assert.equal(playlist.status, 200)
  assert.equal(playlist.headers.get('x-accel-redirect'), null)
  assert.match(playlist.headers.get('content-type'), /mpegurl/)
  assert.match(playlist.headers.get('cache-control'), /no-store/)
  assert.match(await playlist.text(), /#EXT-X-ENDLIST/)
  const trailer = await request(preview)
  assert.equal(trailer.status, 200)
  const durations = [...(await trailer.text()).matchAll(/#EXTINF:([\d.]+)/g)].map(item => Number(item[1]))
  assert(durations.reduce((sum, value) => sum + value, 0) >= 19 && durations.reduce((sum, value) => sum + value, 0) <= 21)
  assert.equal((await request(segment, member, { method: 'HEAD' })).status, 200)
  const range = await request(segment, member, { headers: { Range: 'bytes=0-31' } })
  assert.equal(range.status, 206); assert.equal((await range.arrayBuffer()).byteLength, 32)
  const key = sql(`SELECT storage_key FROM videos WHERE id=${first}`)
  assert.match(key, /^[a-f0-9]{32}$/)
  for (const cookie of [null, free, member, admin]) assert.equal((await request(`/_hls/${key}/full/index.m3u8`, cookie)).status, 404)
  for (const path of [`/media/${first}/full/source`, `/storage/${key}/source`, `/_hls/${key}/source`, '/.env']) assert.equal((await request(path, admin)).status, 404)
  console.log('PASS Nginx sends protected HLS, HEAD/range; direct/internal/raw paths are blocked')
  const restarted = await upload(bytes, 'restart')
  await waitFor(async () => (await video(restarted)).processingStatus === 'processing', 'worker begins job')
  docker(['stop', '-t', '10', 'worker']); stopped = true
  assert.equal((await video(restarted)).processingStatus, 'processing')
  assert.equal((await request(segment, member)).status, 200)
  docker(['start', 'worker']); stopped = false
  await awaitReady(restarted)
  console.log('PASS interrupted processing recovers; playback works while worker is stopped')
  const cancelled = await upload(bytes, 'cancel')
  await waitFor(async () => (await video(cancelled)).processingStatus === 'processing', 'worker begins cancellable job')
  assert.equal((await request('/api/admin/videos/' + cancelled, admin, { method: 'DELETE' })).status, 204)
  await waitFor(() => sql(`SELECT storage_key IS NULL FROM videos WHERE id=${cancelled}`) === 't', 'separate worker cancels and removes deleted files')
  console.log('PASS delete cancels work in the separate process and removes its files')
  const simultaneous = await Promise.all(Array.from({ length: 10 }, () => request(segment, member, { headers: { Range: 'bytes=0-31' } })))
  for (const response of simultaneous) { assert.equal(response.status, 206); await response.arrayBuffer() }
  console.log('PASS 10 parallel authorized requests (functional check, not a capacity benchmark)')
  await publish(first, 'hidden')
  for (const path of [full, preview, segment]) assert.equal((await request(path, member)).status, 404)
  console.log('PASS hiding blocks subsequent playlist and segment requests')
} finally {
  if (stopped) docker(['start', 'worker'])
  for (const id of ids) {
    try { await request('/api/admin/videos/' + id, admin, { method: 'DELETE' }) } catch {}
  }
  if (ids.length) {
    await waitFor(() => sql(`SELECT count(*) FROM videos WHERE id IN (${ids.join(',')}) AND storage_key IS NOT NULL`) === '0', 'fixture file cleanup')
    console.log('PASS removed all test files; existing users and videos preserved')
  }
}
