// Local integration check. Uses only Node built-ins and the running Compose stack.
// Stops/restarts OUR worker: run when no other uploads are pending/processing.
import { execFileSync } from 'node:child_process'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { mkdirSync, writeFileSync } from 'node:fs'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const base = process.env.RUNTIME_TEST_URL ?? 'http://127.0.0.1:8088'
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(base)) throw Error('This check allows only local HTTP')
const project = process.env.RUNTIME_TEST_PROJECT ?? 'rachata-plus-local'
if (!/^rachata-plus-[a-z0-9-]+$/.test(project)) throw Error('Unexpected Compose project name')
const flags = ['compose', '--env-file', '.env.local', '-p', project, '-f', 'compose.yaml', '-f', 'compose.runtime.yaml']
function docker(args, timeout = 90000) { return execFileSync('docker', [...flags, ...args], { cwd: root, encoding: 'utf8', timeout, windowsHide: true, maxBuffer: 8 * 1024 * 1024 }) }
function sql(query) { return docker(['exec', '-T', 'db', 'sh', '-c', 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -X -At -v ON_ERROR_STOP=1 -c "$1"', 'sh', query]).trim() }
const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
async function waitFor(check, label, milliseconds = 300000) {
  const deadline = Date.now() + milliseconds
  while (Date.now() < deadline) { if (await check()) return; await pause(300) }
  throw Error('Timed out: ' + label)
}
async function request(path, cookie, options = {}) {
  const full = path.match(/^\/media\/([1-9][0-9]*)\/full\//)
  const lease = full && permits.get(cookie + ':' + full[1])
  if (lease) path += '?playback=' + encodeURIComponent(lease.token)
  return fetch(base + path, { ...options, signal: AbortSignal.timeout(15000), headers: { Origin: base, ...(cookie ? { Cookie: cookie } : {}), ...options.headers } })
}
const permits = new Map()
let renewal
async function openPlayer(cookie, videoId) {
  const me = await request('/auth/me', cookie)
  assert.equal(me.status, 200)
  const expectedUserId = (await me.json()).user.id
  const endpoint = '/api/playback/' + videoId
  const response = await request(endpoint, cookie, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expectedUserId }) })
  assert.equal(response.status, 200)
  permits.set(cookie + ':' + videoId, { cookie, endpoint, expectedUserId, token: (await response.json()).token })
}
async function login(email) {
  const response = await request('/auth/login', null, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) })
  assert.equal(response.status, 200, 'temporary test login')
  const cookies = response.headers.getSetCookie().filter(value => /^rachata_session=[^;]/.test(value))
  assert.equal(cookies.length, 1)
  return cookies[0].split(';')[0]
}
const ids = []
const accounts = []
const password = randomBytes(24).toString('base64url')
const fixturePrefix = 'runtime_' + Date.now()
async function createAccount(role) {
  const username = fixturePrefix + '_' + role
  const email = username + '@example.invalid'
  const response = await request('/auth/register', null, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, name: 'Temporary runtime fixture', email, password }) })
  assert.equal(response.status, 201)
  const id = (await response.json()).user.id
  assert(Number.isSafeInteger(id) && id > 0)
  accounts.push(id)
  if (role === 'admin') sql(`UPDATE users SET role='admin' WHERE id=${id}`)
  const cookie = await login(email)
  return { id, cookie }
}
async function upgrade(account, plan) {
  const me = await request('/auth/me', account.cookie)
  assert.equal(me.status, 200)
  const user = (await me.json()).user
  const response = await request('/auth/membership/demo-checkout', account.cookie, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expectedUserId: account.id, expectedRevision: user.membershipRevision, plan }) })
  assert.equal(response.status, 200)
}
let admin, stopped = false
const tag = 'Runtime check ' + Date.now()
const measurements = []
async function measureSegments(path, cookie, concurrency) {
  const deadline = Date.now() + 5000
  const times = []
  let bytes = 0
  await Promise.all(Array.from({ length: concurrency }, async () => {
    while (Date.now() < deadline) {
      const start = performance.now()
      const response = await request(path, cookie)
      assert.equal(response.status, 200)
      const length = (await response.arrayBuffer()).byteLength
      bytes += length
      times.push(performance.now() - start)
    }
  }))
  times.sort((a, b) => a - b)
  const result = { concurrency, requests: times.length, bytes, p50ms: Math.round(times[Math.floor(times.length * .5)]), p95ms: Math.round(times[Math.floor(times.length * .95)]), elapsedMs: Math.round(Date.now() - deadline + 5000) }
  measurements.push(result)
  console.log('PASS bounded local full-segment HTTP load:', JSON.stringify(result))
}
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
  assert.equal(docker(['exec', '-T', 'api', 'printenv', 'APP_ENV']).trim(), 'local', 'fixture creation is local only')
  admin = (await createAccount('admin')).cookie
  const memberAccount = await createAccount('member')
  await upgrade(memberAccount, 'basic')
  const member = memberAccount.cookie
  const free = (await createAccount('free')).cookie
  // Real 1080p source makes interruption/cancellation meaningful on the limited worker.
  docker(['exec', '-T', 'worker', 'ffmpeg', '-nostdin', '-v', 'error', '-y', '-f', 'lavfi', '-i', 'testsrc2=size=1920x1080:rate=24', '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=48000', '-t', '24', '-c:v', 'libx264', '-threads', '2', '-preset', 'ultrafast', '-crf', '38', '-pix_fmt', 'yuv420p', '-c:a', 'aac', '/tmp/runtime-fixture.mp4'])
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
  await openPlayer(member, first)
  let renewing = false
  renewal = setInterval(async () => {
    if (renewing) return
    renewing = true
    try {
      for (const lease of permits.values()) {
        const response = await request(lease.endpoint, lease.cookie, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expectedUserId: lease.expectedUserId, token: lease.token }) })
        if (!response.ok) console.error('Playback renewal failed:', response.status)
      }
    } catch (error) { console.error('Playback renewal failed:', error.message) }
    finally { renewing = false }
  }, 20000)
  const full = `/media/${first}/full/index.m3u8`, preview = `/media/${first}/preview/index.m3u8`, segment = `/media/${first}/full/segment_360p_000000.ts`
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
  assert.match(await playlist.text(), /#EXT-X-STREAM-INF/)
  for (const height of [360,480,720,1080]) {
    const variant = await request(`/media/${first}/full/${height}p.m3u8`, member)
    assert.equal(variant.status,200); assert.match(await variant.text(), /#EXT-X-ENDLIST/)
    assert.equal((await request(`/media/${first}/full/${height}p.m3u8`,free)).status,403)
  }
  const trailer = await request(preview)
  assert.equal(trailer.status, 200)
  assert.match(await trailer.text(), /#EXT-X-STREAM-INF/)
  const previewVariant = await request(`/media/${first}/preview/360p.m3u8`)
  assert.equal(previewVariant.status,200)
  const durations = [...(await previewVariant.text()).matchAll(/#EXTINF:([\d.]+)/g)].map(item => Number(item[1]))
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
  for (const concurrency of [1, 5, 10]) await measureSegments(segment, member, concurrency)
  await publish(first, 'hidden')
  for (const path of [full, preview, segment]) assert.equal((await request(path, member)).status, 404)
  console.log('PASS hiding blocks subsequent playlist and segment requests')
  const previous = permits.get(member + ':' + first)
  assert.equal((await request(previous.endpoint, member, { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expectedUserId: previous.expectedUserId, token: previous.token }) })).status, 204)
  permits.delete(member + ':' + first)
  // Short 4K fixture verifies the real container codec and memory limit.
  docker(['exec', '-T', 'worker', 'ffmpeg', '-nostdin', '-v', 'error', '-y', '-f', 'lavfi', '-i', 'color=c=navy:size=3840x2160:rate=24', '-t', '2', '-c:v', 'libx264', '-threads', '2', '-preset', 'ultrafast', '-pix_fmt', 'yuv420p', '/tmp/runtime-4k.mp4'])
  const highBytes = execFileSync('docker', [...flags, 'exec', '-T', 'worker', 'cat', '/tmp/runtime-4k.mp4'], { cwd: root, windowsHide: true, maxBuffer: 32 * 1024 * 1024 })
  docker(['exec', '-T', 'worker', 'rm', '/tmp/runtime-4k.mp4'])
  const high = await upload(highBytes, '4k')
  await awaitReady(high)
  await publish(high, 'published')
  const highKey = sql(`SELECT storage_key FROM videos WHERE id=${high}`)
  const generation = sql(`SELECT hls_generation FROM videos WHERE id=${high}`)
  assert.match(highKey, /^[a-f0-9]{32}$/)
  assert.match(generation, /^[1-9][0-9]*$/)
  for (const height of [360, 480, 720, 1080, 1440, 2160]) {
    const path = `/srv/media/${highKey}/jobs/g${generation}/full/segment_${height}p_000000.ts`
    assert.equal(docker(['exec', '-T', 'worker', 'ffprobe', '-v', 'error', '-select_streams', 'v:0', '-show_entries', 'stream=height', '-of', 'csv=p=0', path]).trim().split('\n')[0], String(height))
  }
  console.log('PASS actual Docker FFmpeg produces 360/480/720/1080/1440/2160p')
  for (const [plan, height, screens] of [['basic', 1080, 2], ['standard', 1440, 2], ['premium', 2160, 4]]) {
    if (plan !== 'basic') await upgrade(memberAccount, plan)
    const payload = { expectedUserId: memberAccount.id }
    const options = body => ({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    const endpoint = '/api/playback/' + high
    const opened = []
    try {
      const responses = await Promise.all(Array.from({ length: screens + 1 }, () => request(endpoint, member, options(payload))))
      for (const response of responses) {
        if (response.status === 200) opened.push((await response.json()).token)
        else { assert.equal(response.status, 409); assert.equal((await response.json()).error, 'stream_limit_reached') }
      }
      assert.equal(opened.length, screens)
      const suffix = '?playback=' + encodeURIComponent(opened[0])
      const master = await request(`/media/${high}/full/index.m3u8` + suffix, member)
      assert.equal(master.status, 200)
      const heights = [...(await master.text()).matchAll(/RESOLUTION=\d+x(\d+)/g)].map(match => Number(match[1]))
      assert.equal(Math.max(...heights), height)
      if (height < 2160) assert.equal((await request(`/media/${high}/full/2160p.m3u8` + suffix, member)).status, 403)
      const token = opened.pop()
      assert.equal((await request(endpoint, member, { ...options({ ...payload, token }), method: 'DELETE' })).status, 204)
      const retry = await request(endpoint, member, options(payload))
      assert.equal(retry.status, 200)
      opened.push((await retry.json()).token)
      console.log(`PASS ${plan}: max ${height}p; ${screens} players; reject excess; release/reopen`)
    } finally {
      for (const token of opened) assert.equal((await request(endpoint, member, { ...options({ ...payload, token }), method: 'DELETE' })).status, 204)
    }
  }
  const reports = resolve(root, 'test-results')
  mkdirSync(reports, { recursive: true })
  writeFileSync(resolve(reports, 'runtime-load.json'), JSON.stringify({ measuredAt: new Date().toISOString(), scope: 'Local HTTP full-segment requests for one permitted player; not unique viewers or EC2 capacity', measurements }, null, 2) + '\n')
} finally {
  try {
    clearInterval(renewal)
    for (const lease of permits.values()) {
      try { await request(lease.endpoint, lease.cookie, { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expectedUserId: lease.expectedUserId, token: lease.token }) }) } catch {}
    }
    if (stopped) docker(['start', 'worker'])
    for (const id of ids) {
      try { await request('/api/admin/videos/' + id, admin, { method: 'DELETE' }) } catch {}
    }
    if (ids.length) {
      await waitFor(() => sql(`SELECT count(*) FROM videos WHERE id IN (${ids.join(',')}) AND storage_key IS NOT NULL`) === '0', 'fixture file cleanup')
      sql(`DELETE FROM videos WHERE id IN (${ids.join(',')}) AND deleted_at IS NOT NULL AND storage_key IS NULL`)
      console.log('PASS removed all test files; existing users and videos preserved')
    }
  } finally {
    // Even failed file cleanup must not leave behind our privileged account.
    if (accounts.length) {
      const list = accounts.join(',')
      sql(`BEGIN; DELETE FROM membership_events WHERE user_id IN (${list}) OR actor_id IN (${list}); DELETE FROM subscriptions WHERE user_id IN (${list}); DELETE FROM users WHERE id IN (${list}); COMMIT;`)
      console.log('PASS removed temporary test accounts; original accounts unchanged')
    }
  }
}
