import { useCallback, useEffect, useRef, useState } from 'react'
import { accountError } from '../lib/auth'
import type { ApiUser } from '../lib/auth'
import { findMembership, listMembershipAccounts, manageMembership } from '../lib/membership'
import type { MembershipAction, MembershipEvent } from '../lib/membership'
import { AccountSessions } from '../components/AccountSessions'
import { membershipPlans } from '../data/plans'

const planName = (id: string) => membershipPlans.find(plan => plan.id === id)?.name ?? 'Free'
const actionNames: Record<MembershipAction, string> = { grant: 'เปิด / ต่ออายุสมาชิก', revoke: 'ยกเลิกสิทธิ์สมาชิก', suspend: 'ระงับบัญชี', reactivate: 'เปิดใช้งานบัญชีคืน', logout_all: 'ออกจากระบบทุกอุปกรณ์', logout_session: 'ตัดการเชื่อมต่อเซสชัน' }
const date = (value: number) => value ? new Date(value).toLocaleString('th-TH') : '—'

export function AdminMembersPage({ actorId, onChanged }: { actorId: number; onChanged: () => Promise<void> }) {
  const [email, setEmail] = useState('')
  const [query, setQuery] = useState('')
  const [accounts, setAccounts] = useState<ApiUser[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [listBusy, setListBusy] = useState(true)
  const [listError, setListError] = useState('')
  const listController = useRef<AbortController | null>(null)
  const [target, setTarget] = useState<ApiUser | null>(null)
  const [selectedEmail, setSelectedEmail] = useState<string | null>(null)
  const listElement = useRef<HTMLUListElement>(null)
  const [events, setEvents] = useState<MembershipEvent[]>([])
  const [plan, setPlan] = useState('basic')
  const [days, setDays] = useState(30)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [review, setReview] = useState<MembershipAction | null>(null)
  const controller = useRef<AbortController | null>(null)
  const reviewHeading = useRef<HTMLHeadingElement>(null)
  const resultHeading = useRef<HTMLHeadingElement>(null)
  const grantButton = useRef<HTMLButtonElement>(null)
  const controlButtons = useRef<Partial<Record<MembershipAction, HTMLButtonElement | null>>>({})
  const revokeButton = useRef<HTMLButtonElement>(null)
  useEffect(() => () => controller.current?.abort(), [])
  useEffect(() => { if (review) reviewHeading.current?.focus() }, [review])

  const loadList = useCallback(async (cursor = '') => {
    listController.current?.abort()
    const request = new AbortController()
    listController.current = request
    await Promise.resolve()
    if (request.signal.aborted) return
    setListBusy(true); setListError('')
    if (!cursor) { setAccounts([]); setNextCursor('') }
    try {
      const result = await listMembershipAccounts(actorId, query, cursor, request.signal)
      if (request.signal.aborted) return
      setAccounts(previous => cursor ? [...previous, ...result.users.filter(user => !previous.some(item => item.id === user.id))] : result.users)
      setNextCursor(result.nextCursor)
    } catch (failure) {
      if (!request.signal.aborted) setListError(accountError(failure))
    } finally { if (!request.signal.aborted) setListBusy(false) }
  }, [actorId, query])
  useEffect(() => {
    let cancelled = false
    void Promise.resolve().then(() => { if (!cancelled) void loadList() })
    return () => { cancelled = true; listController.current?.abort() }
  }, [loadList])

  async function selectAccount(address: string) {
    if (busy) return
    const request = new AbortController()
    controller.current = request
    setSelectedEmail(address)
    setBusy(true); setError(''); setNotice(''); setTarget(null); setEvents([]); setReview(null)
    try {
      const result = await findMembership(actorId, address, request.signal)
      if (request.signal.aborted) return
      setTarget(result.user); setEvents(result.events); setPlan(result.user.plan || 'basic')
      requestAnimationFrame(() => { if (!request.signal.aborted) resultHeading.current?.focus() })
    } catch (failure) {
      if (!request.signal.aborted) setError(accountError(failure))
    } finally { if (!request.signal.aborted) setBusy(false) }
  }
  async function confirm() {
    if (busy || !target || !review) return
    const request = new AbortController()
    controller.current = request
    setBusy(true); setError(''); setNotice('')
    try {
      const next = await manageMembership(actorId, target, review, plan, days, request.signal)
      if (request.signal.aborted) return
      setTarget(next); setReview(null)
      setAccounts(previous => previous.map(user => user.id === next.id ? next : user))
      setNotice(review === 'grant' ? `เปิดแพ็กเกจ ${planName(next.plan)} แล้ว หมดอายุ ${date(next.expiresAt)}` : `${actionNames[review]}แล้ว`)
      requestAnimationFrame(() => { if (!request.signal.aborted) resultHeading.current?.focus() })
      // The write has succeeded. A subsequent history failure must not invite repeating it.
      try {
        if (review === 'logout_all' && next.id === actorId) { await onChanged(); return }
        const latest = await findMembership(actorId, next.email, request.signal)
        if (!request.signal.aborted) { setTarget(latest.user); setEvents(latest.events) }
      } catch {
        if (!request.signal.aborted) { setEvents([]); setNotice('บันทึกสิทธิ์แล้ว แต่โหลดประวัติล่าสุดไม่ได้ กรุณาค้นหาบัญชีอีกครั้ง') }
      }
      if (!request.signal.aborted) await onChanged()
    } catch (failure) {
      if (!request.signal.aborted) { setError(accountError(failure)); setReview(null) }
    } finally { if (!request.signal.aborted) setBusy(false) }
  }

  return <main className="rp-page">
    <div className="rp-page-heading"><h1>จัดการสมาชิก</h1><p>เปิด ต่ออายุ หรือยกเลิกสิทธิ์ในฐานข้อมูล ไม่มีการเรียกเก็บเงินผ่านเว็บไซต์</p></div>
    {selectedEmail === null ? <>
    <form className="rp-surface membership-admin-form" onSubmit={event => {
      event.preventDefault()
      if (query === email.trim()) void loadList()
      else setQuery(email.trim())
    }} aria-busy={listBusy}>
      <label className="rp-field" htmlFor="membership-email">ค้นหาชื่อ ชื่อผู้ใช้ หรืออีเมล
        <input id="membership-email" type="search" maxLength={254} autoComplete="off" value={email} disabled={busy} onChange={event => setEmail(event.target.value)} />
      </label>
      <div className="rp-actions"><button className="rp-mainbutton" disabled={busy || listBusy}>ค้นหาบัญชี</button>
        <button type="button" className="rp-softbutton" disabled={busy || listBusy} onClick={() => { setEmail(''); if (query) setQuery(''); else void loadList() }}>แสดงทั้งหมด</button></div>
    </form>
    <section className="rp-surface membership-admin-form" aria-labelledby="membership-list-heading" aria-busy={listBusy}>
      <h2 id="membership-list-heading">รายชื่อบัญชี{query ? ` · ผลค้นหา “${query}”` : ''}</h2>
      <p role="status">{listBusy ? 'กำลังโหลดรายชื่อ…' : `${accounts.length} บัญชีที่แสดง${nextCursor ? ' · ยังมีรายการเพิ่มเติม' : ''}`}</p>
      {listError && <div role="alert"><p>{listError}</p><button className="rp-softbutton" disabled={listBusy || busy} onClick={() => void loadList(nextCursor)}>ลองโหลดรายชื่อใหม่</button></div>}
      {!listBusy && !listError && !accounts.length && <p>ไม่พบบัญชีที่ตรงกับการค้นหา</p>}
      <ul ref={listElement} className="membership-account-list">{accounts.map(user => <li key={user.id}>
        <div><strong>{user.name} · @{user.username}</strong><p>{user.email}</p><small>{user.role === 'admin' ? 'ผู้ดูแลระบบ' : planName(user.plan)}{user.expiresAt ? ` · หมดอายุ ${date(user.expiresAt)}` : ''}{user.accountStatus === 'suspended' ? ' · ระงับบัญชี' : ' · ใช้งานได้'}</small></div>
        <button className="rp-softbutton" data-account-email={user.email} disabled={busy || listBusy} aria-label={`จัดการสมาชิก ${user.email}`} onClick={() => void selectAccount(user.email)}>จัดการ</button>
      </li>)}</ul>
      {nextCursor && <button className="rp-softbutton" disabled={listBusy || busy} onClick={() => void loadList(nextCursor)}>โหลดรายชื่อเพิ่มเติม</button>}
    </section>
    </> : <>
      <button type="button" className="rp-softbutton" disabled={busy} onClick={() => {
        const previousEmail = selectedEmail
        setSelectedEmail(null); setTarget(null); setReview(null); setEvents([]); setError(''); setNotice('')
        requestAnimationFrame(() => {
          const buttons = listElement.current?.querySelectorAll<HTMLButtonElement>('button[data-account-email]')
          Array.from(buttons ?? []).find(button => button.dataset.accountEmail === previousEmail)?.focus()
        })
      }}>กลับไปรายชื่อ</button>
      {busy && !target && <p role="status">กำลังโหลดบัญชีที่เลือก…</p>}
    </>}
    <p role="alert">{error}</p><p role="status">{notice}</p>
    {target && <section className="rp-surface membership-admin-form" aria-busy={busy}>
      <h2 ref={resultHeading} tabIndex={-1}>{target.name} · @{target.username}</h2>
      <p>{target.email}{target.role === 'admin' && ' · ผู้ดูแลระบบมีสิทธิ์รับชมโดยไม่ต้องมีแพ็กเกจ'}</p>
      <dl className="rp-info-grid"><div><dt>สถานะบัญชี</dt><dd>{target.accountStatus === "suspended" ? "ระงับบัญชี" : "ใช้งานได้"}</dd></div><div><dt>แพ็กเกจปัจจุบัน</dt><dd>{planName(target.plan)}</dd></div><div><dt>วันหมดอายุ</dt><dd>{date(target.expiresAt)}</dd></div></dl>
      {review ? <section aria-labelledby="membership-review-heading">
        <h3 id="membership-review-heading" ref={reviewHeading} tabIndex={-1}>ยืนยัน{actionNames[review]}</h3>
        <p>{review === 'grant' ? `เปิด ${planName(plan)} ให้ ${target.email} เพิ่ม ${days} วัน${target.member ? ' ต่อจากวันหมดอายุเดิม เปลี่ยนแพ็กเกจมีผลทันที' : ' เริ่มทันที'}` : review === 'revoke' ? `ยกเลิกสิทธิ์สมาชิกของ ${target.email} ทันที` : review === 'suspend' ? `ระงับ ${target.email} และออกจากระบบทุกอุปกรณ์ บัญชีจะเข้าสู่ระบบและรับชมวิดีโอเต็มไม่ได้ ข้อมูลและวันหมดอายุสมาชิกเดิมยังคงอยู่` : review === 'reactivate' ? `เปิดใช้งาน ${target.email} คืน ผู้ใช้ต้องเข้าสู่ระบบใหม่ วันหมดอายุสมาชิกไม่เปลี่ยน` : `ออกจากระบบทุกอุปกรณ์ของ ${target.email} ผู้ใช้ยังเข้าสู่ระบบใหม่ได้${target.id === actorId ? ' รวมถึงเซสชันแอดมินที่คุณกำลังใช้' : ''}`}</p>
        <div className="rp-actions">
          <button className="rp-softbutton" disabled={busy} onClick={() => { const previous = review; setReview(null); requestAnimationFrame(() => (previous === 'grant' ? grantButton.current : previous === 'revoke' ? revokeButton.current : controlButtons.current[previous])?.focus()) }}>กลับไปแก้ไข</button>
          <button className="rp-mainbutton" disabled={busy} onClick={() => void confirm()}>{busy ? 'กำลังบันทึก…' : 'ยืนยัน'}</button>
        </div>
      </section> : <form onSubmit={event => { event.preventDefault(); setError(''); setReview('grant') }}>
        <div className="membership-admin-fields">
          <div className="rp-field"><label htmlFor="membership-plan">แพ็กเกจ</label><select id="membership-plan" value={plan} disabled={busy} onChange={event => setPlan(event.target.value)}>{membershipPlans.filter(item => !item.free).map(item => <option key={item.id} value={item.id}>{item.name} · {item.price}</option>)}</select></div>
          <label className="rp-field" htmlFor="membership-days">จำนวนวันที่เพิ่ม (1–365)<input id="membership-days" type="number" min={1} max={365} step={1} required value={days} disabled={busy} onChange={event => setDays(Number(event.target.value))} /></label>
        </div>
        <p>หากยังมีสมาชิกอยู่ จะเพิ่มวันจากวันหมดอายุเดิม หากหมดอายุแล้วจะเริ่มจากวันนี้ คุณภาพวิดีโอปัจจุบันสูงสุด 720p ทุกแพ็กเกจ</p>
        <div className="rp-actions"><button ref={grantButton} className="rp-mainbutton" disabled={busy || target.accountStatus === "suspended"}>เปิด / ต่ออายุสมาชิก</button><button ref={revokeButton} type="button" className="rp-softbutton" disabled={busy || !target.member} onClick={() => { setError(''); setReview('revoke') }}>ยกเลิกสิทธิ์สมาชิก</button></div>
      </form>}
      {!review && <section aria-labelledby="account-controls-heading">
        <h3 id="account-controls-heading">บัญชีและความปลอดภัย</h3>
        <p>การระงับเก็บข้อมูลเดิมไว้ และไม่หยุดนับวันสมาชิก การเปิดบัญชีคืนไม่คืนเซสชันเดิม</p>
        <div className="rp-actions">
          {target.accountStatus === 'suspended'
            ? <button type="button" className="rp-softbutton" ref={button => { controlButtons.current.reactivate = button }} disabled={busy} onClick={() => { setError(''); setReview('reactivate') }}>เปิดใช้งานบัญชีคืน</button>
            : <button type="button" className="rp-softbutton" ref={button => { controlButtons.current.suspend = button }} disabled={busy || target.id === actorId} onClick={() => { setError(''); setReview('suspend') }}>ระงับบัญชี</button>}
          <button type="button" className="rp-softbutton" ref={button => { controlButtons.current.logout_all = button }} disabled={busy} onClick={() => { setError(''); setReview('logout_all') }}>ออกจากระบบทุกอุปกรณ์</button>
        </div>
        {target.id === actorId && <p>ไม่สามารถระงับบัญชีแอดมินที่กำลังใช้งานอยู่</p>}
      </section>}
      {!review && <AccountSessions actorId={actorId} user={target} disabled={busy} onBusy={setBusy} onRevoked={async (next, current, signal) => {
        if (signal.aborted) return
        setTarget(next); setAccounts(previous => previous.map(user => user.id === next.id ? next : user)); setNotice('ตัดการเชื่อมต่อเซสชันแล้ว')
        if (current) { await onChanged(); return }
        try {
          const latest = await findMembership(actorId, next.email, signal)
          if (signal.aborted) return
          setTarget(latest.user); setEvents(latest.events)
        } catch { if (!signal.aborted) { setEvents([]); setNotice('ตัดการเชื่อมต่อแล้ว แต่โหลดประวัติไม่ได้ กรุณาเปิดบัญชีนี้ใหม่') } }
      }} />}
      <h3>ประวัติการจัดการล่าสุด (สูงสุด 20 รายการ)</h3>
      {events.length ? <ul>{events.map((event, index) => <li key={index}>{date(event.createdAt)} · {event.actor} · {event.action === 'grant' ? `เปิด / ต่ออายุ ${planName(event.plan)} ถึง ${date(event.expiresAt)}` : `${actionNames[event.action]}${event.sessionId ? " #" + event.sessionId : ""}`}</li>)}</ul> : <p>ยังไม่มีประวัติการจัดการที่แสดงได้</p>}
    </section>}
  </main>
}
