import { useCallback, useEffect, useRef, useState } from 'react'
import { accountError } from '../lib/auth'
import type { ApiUser } from '../lib/auth'
import { listDeviceSessions, manageMembership, revokeOwnDeviceSession } from '../lib/membership'
import type { DeviceSession } from '../lib/membership'

const date = (value: number) => new Date(value).toLocaleString('th-TH')
export function AccountSessions({ actorId, user, disabled, onBusy, onRevoked, own = false }: {
  actorId: number; user: ApiUser; disabled: boolean; own?: boolean
  onBusy: (busy: boolean) => void
  onRevoked: (user: ApiUser, current: boolean, signal: AbortSignal) => Promise<void>
}) {
  const [sessions, setSessions] = useState<DeviceSession[]>([])
  const [cursor, setCursor] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [pending, setPending] = useState<DeviceSession | null>(null)
  const fetchController = useRef<AbortController | null>(null)
  const actionController = useRef<AbortController | null>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const confirmHeading = useRef<HTMLHeadingElement>(null)
  const list = useRef<HTMLUListElement>(null)
  const load = useCallback(async (next = '') => {
    fetchController.current?.abort()
    const controller = new AbortController()
    fetchController.current = controller
    await Promise.resolve()
    if (controller.signal.aborted) return
    setLoading(true); setError('')
    if (!next) { setSessions([]); setCursor('') }
    try {
      const data = await listDeviceSessions(actorId, user.id, next, controller.signal, own)
      if (!controller.signal.aborted) {
        setSessions(previous => next ? [...previous, ...data.sessions.filter(session => !previous.some(item => item.id === session.id))] : data.sessions)
        setCursor(data.nextCursor)
      }
    } catch (failure) { if (!controller.signal.aborted) setError(accountError(failure)) }
    finally { if (!controller.signal.aborted) setLoading(false) }
  }, [actorId, user.id, own])
  useEffect(() => {
    let cancelled = false
    void Promise.resolve().then(() => { if (!cancelled) void load() })
    return () => { cancelled = true; fetchController.current?.abort() }
  }, [load, user.membershipRevision])
  useEffect(() => () => actionController.current?.abort(), [])
  useEffect(() => { if (pending) confirmHeading.current?.focus() }, [pending])
  async function revoke() {
    if (!pending || disabled) return
    const controller = new AbortController()
    actionController.current = controller
    onBusy(true); setError('')
    try {
      const next = own ? await revokeOwnDeviceSession(user.id, pending.id, controller.signal)
        : await manageMembership(actorId, user, 'logout_session', '', 0, controller.signal, pending.id)
      if (controller.signal.aborted) return
      setSessions(previous => previous.filter(session => session.id !== pending.id)); setPending(null)
      requestAnimationFrame(() => { if (!controller.signal.aborted) heading.current?.focus() })
      await onRevoked(next, pending.current, controller.signal)
    } catch (failure) { if (!controller.signal.aborted) setError(accountError(failure)) }
    finally { if (!controller.signal.aborted) onBusy(false) }
  }
  return <section aria-labelledby="account-sessions-heading" aria-busy={loading}>
    <h3 id="account-sessions-heading" ref={heading} tabIndex={-1}>{own ? "อุปกรณ์และเซสชันของฉัน" : "เซสชันที่ยังใช้งานได้"}</h3>
    <p>ข้อมูลเบราว์เซอร์และระบบปฏิบัติการมาจากเบราว์เซอร์ ไม่ยืนยันรุ่นเครื่องหรือว่าออนไลน์อยู่ขณะนี้ เซสชันเก่าอาจไม่ทราบอุปกรณ์</p>
    <p role="alert">{error}</p>
    {pending ? <div>
      <h4 ref={confirmHeading} tabIndex={-1}>ยืนยันตัดการเชื่อมต่อ</h4>
      <p>{pending.device} · เซสชัน #{pending.id} · เข้าสู่ระบบ {date(pending.createdAt)}{pending.current ? own ? ' · อุปกรณ์ที่คุณกำลังใช้จะออกจากระบบด้วย ต้องเข้าสู่ระบบใหม่' : ' · รวมเซสชันแอดมินที่คุณกำลังใช้ ต้องเข้าสู่ระบบใหม่' : ''}</p>
      <div className="rp-actions"><button className="rp-softbutton" disabled={disabled} onClick={() => {
        const id = pending.id; setPending(null)
        requestAnimationFrame(() => Array.from(list.current?.querySelectorAll<HTMLButtonElement>('button[data-session-id]') ?? []).find(button => button.dataset.sessionId === id)?.focus())
      }}>ยกเลิก</button><button className="rp-mainbutton" disabled={disabled} onClick={() => void revoke()}>ยืนยันตัดการเชื่อมต่อ</button></div>
    </div> : <>
      <button className="rp-softbutton" disabled={disabled || loading} onClick={() => void load()}>โหลดเซสชันใหม่</button>
      <p role="status">{loading ? 'กำลังโหลดเซสชัน…' : `${sessions.length} เซสชันที่แสดง${cursor ? ' · มีรายการเพิ่มเติม' : ''}`}</p>
      {!loading && !error && !sessions.length && <p>ไม่มีเซสชันที่ยังใช้งานได้</p>}
      <ul ref={list} className="membership-account-list">{sessions.map(session => <li key={session.id}>
        <div><strong>{session.device}{session.current ? ' · เซสชันนี้' : ''}</strong><p>เซสชัน #{session.id} · เข้าสู่ระบบ {date(session.createdAt)}</p><small>หมดอายุ {date(session.expiresAt)}</small></div>
        <button className="rp-softbutton" data-session-id={session.id} aria-label={`ตัดการเชื่อมต่อเซสชัน ${session.id}`} disabled={disabled || loading} onClick={() => { setError(''); setPending(session) }}>ตัดการเชื่อมต่อ</button>
      </li>)}</ul>
      {cursor && <button className="rp-softbutton" disabled={disabled || loading} onClick={() => void load(cursor)}>โหลดเซสชันเพิ่มเติม</button>}
    </>}
  </section>
}
