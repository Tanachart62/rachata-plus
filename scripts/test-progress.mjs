// Regression tests for the actual TypeScript queue, using existing TypeScript.
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import vm from 'node:vm'
const require = createRequire(new URL('../apps/web/package.json', import.meta.url))
const ts = require('typescript')
function load(relative, dependencies = {}) {
  const source = readFileSync(new URL('../apps/web/src/lib/' + relative, import.meta.url), 'utf8')
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const module = { exports: {} }
  vm.runInNewContext(
    code,
    {
      module,
      exports: module.exports,
      require: (name) => {
        if (!(name in dependencies)) throw Error('Unexpected dependency ' + name)
        return dependencies[name]
      },
      Promise,
      Map,
      Error,
      AbortController,
      Number,
    },
    { filename: fileURLToPath(new URL(relative, import.meta.url)) },
  )
  return module.exports
}
const { ApiError } = load('http.ts')
const { ProgressQueue } = load('progress-queue.ts', { './http': { ApiError } })
const deferred = () => {
  let resolve, reject
  const promise = new Promise((a, b) => {
    resolve = a
    reject = b
  })
  return { promise, resolve, reject }
}
const entry = (position, revision) => ({ videoId: 1, position, revision, watchedAt: Date.now() })
const until = async (check) => {
  for (let i = 0; i < 50; i++) {
    if (check()) return
    await new Promise((resolve) => setImmediate(resolve))
  }
  throw Error('Queue did not settle')
}
{
  const calls = [],
    holds = []
  let active = 0,
    maximum = 0
  const q = new ProgressQueue(
    {
      read: async () => entry(0, 0),
      save: async (id, position, revision, flush) => {
        active++
        maximum = Math.max(maximum, active)
        calls.push({ position, revision, flush })
        const hold = deferred()
        holds.push(hold)
        await hold.promise
        active--
        return entry(position, revision + 1)
      },
    },
    () => true,
    () => {},
  )
  const first = q.enqueue(1, 50)
  await until(() => calls.length === 1)
  const second = q.enqueue(1, 100)
  const third = q.enqueue(1, 40, true)
  holds[0].resolve()
  await until(() => calls.length === 2)
  assert.deepEqual(
    calls.map((c) => c.position),
    [50, 40],
  )
  assert.equal(calls[1].revision, 1)
  assert.equal(calls[1].flush, true)
  holds[1].resolve()
  await Promise.all([first, second, third])
  assert.equal(maximum, 1)
  q.stop()
  console.log('PASS serialized writes, latest seek wins, ACK revision')
}
{
  const calls = []
  let fail = true
  const q = new ProgressQueue(
    {
      read: async () => entry(0, 0),
      save: async (id, position, revision, flush) => {
        calls.push({ position, flush })
        if (fail) {
          fail = false
          throw new ApiError('service_unavailable')
        }
        return entry(position, revision + 1)
      },
    },
    () => true,
    () => {},
  )
  await assert.rejects(q.enqueue(1, 60), /service_unavailable/)
  await q.enqueue(1, 60, true)
  assert.equal(calls.length, 2)
  assert.equal(calls[1].flush, true)
  q.stop()
  console.log('PASS failed same-position save can be flushed again')
}
{
  let reads = 0
  const calls = []
  const hold = deferred()
  const q = new ProgressQueue(
    {
      read: async () => entry(0, reads++ ? 7 : 0),
      save: async (id, position, revision) => {
        calls.push({ position, revision })
        if (calls.length === 1) {
          await hold.promise
          throw new ApiError('progress_conflict')
        }
        return entry(position, revision + 1)
      },
    },
    () => true,
    () => {},
  )
  const old = q.enqueue(1, 100)
  await until(() => calls.length === 1)
  const latest = q.enqueue(1, 40)
  hold.resolve()
  await Promise.all([old, latest])
  assert.deepEqual(calls, [
    { position: 100, revision: 0 },
    { position: 40, revision: 7 },
  ])
  q.stop()
  console.log('PASS conflict refresh never retries a queued obsolete position')
}
{
  let current = true,
    sends = 0
  const hold = deferred()
  const q = new ProgressQueue(
    {
      read: () => hold.promise,
      save: async () => {
        sends++
        return entry(50, 1)
      },
    },
    () => current,
    () => {},
  )
  const pending = q.enqueue(1, 50)
  current = false
  q.stop()
  hold.resolve(entry(0, 0))
  await assert.rejects(pending, /account_changed/)
  assert.equal(sends, 0)
  console.log('PASS stale account queue cannot send after owner changes')
}
