import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { accountError } from '../lib/auth'
import type { ApiUser } from '../lib/auth'
import { controlAccount } from '../lib/membership'
import type { AccountControlAction } from '../lib/membership'
import { PasswordField } from './PasswordField'

const labels: Record<AccountControlAction, string> = {
  set_role: 'เปลี่ยนระดับสิทธิ์', reset_password: 'รีเซ็ตรหัสผ่าน', delete_account: 'ลบบัญชี',
}

export function AdminAccountControls({ actor, target, disabled, onBusy, onSaved, onDeleted }: {
  actor: ApiUser; target: ApiUser; disabled: boolean; onBusy: (busy: boolean) => void;
  onSaved: (next: ApiUser) => Promise<void>; onDeleted: () => void;
}) {
  const [action, setAction] = useState<AccountControlAction | null>(null)
  const [level, setLevel] = useState(target.adminLevel || 'user')
  const [password, setPassword] = useState('')
  const [nextPassword, setNextPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [confirmEmail, setConfirmEmail] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const heading = useRef<HTMLHeadingElement>(null)
  const submitting = useRef(false)
  const mounted = useRef(true)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => { if (action) heading.current?.focus() }, [action])
  const owner = actor.adminLevel === 'owner'
  if (actor.id === target.id) return <p>เปลี่ยนรหัสผ่านของตัวเองได้ในหน้าโปรไฟล์ ไม่สามารถเปลี่ยนสิทธิ์หรือลบบัญชีตัวเองที่นี่</p>
  if (!owner && target.role === 'admin') return null

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!action || disabled || submitting.current) return
    if (action === 'reset_password' && nextPassword !== repeat) { setError('ยืนยันรหัสผ่านใหม่ให้ตรงกัน'); return }
    if (action === 'reset_password' && ([...nextPassword].length < 8 || [...nextPassword].length > 128)) { setError('รหัสผ่านต้องมี 8–128 ตัวอักษร'); return }
    submitting.current = true; onBusy(true); setError(''); setNotice('')
    try {
      const next = await controlAccount(actor.id, target, action, level, password, nextPassword, confirmEmail)
      if (!mounted.current) return
      setAction(null)
      if (!next) { onDeleted(); return }
      setNotice(`${labels[action]}แล้ว เซสชันเดิมของบัญชีนี้ถูกยกเลิก ผู้ใช้ต้องเข้าสู่ระบบใหม่`)
      try { await onSaved(next) } catch { if (mounted.current) setNotice('บันทึกสำเร็จแล้ว แต่โหลดรายละเอียดล่าสุดไม่ได้ กรุณาเลือกบัญชีนี้ใหม่') }
    } catch (failure) { if (mounted.current) setError(accountError(failure)) }
    finally {
      submitting.current = false
      if (mounted.current) { setPassword(''); setNextPassword(''); setRepeat('') }
      onBusy(false)
    }
  }
  return <section aria-label="สิทธิ์และคำสั่งบัญชี">
    <h3>สิทธิ์และคำสั่งบัญชี</h3>
    <p>ระดับปัจจุบัน: {target.adminLevel === 'owner' ? 'เจ้าของระบบ' : target.role === 'admin' ? 'แอดมิน' : 'ผู้ใช้ทั่วไป'}</p>
    <p role="alert">{error}</p><p role="status">{notice}</p>
    {action ? <form onSubmit={event => void submit(event)}>
      <h4 ref={heading} tabIndex={-1}>ยืนยัน{labels[action]} · {target.email}</h4>
      {action === 'set_role' && <label className="rp-field">ระดับสิทธิ์ใหม่<select value={level} disabled={disabled} onChange={event => setLevel(event.target.value)}>
        <option value="user">ผู้ใช้ทั่วไป — ถอดสิทธิ์แอดมิน</option><option value="admin">แอดมิน — จัดการสมาชิกและวิดีโอ</option><option value="owner">เจ้าของระบบ — จัดการทุกสิทธิ์และลบบัญชี</option>
      </select></label>}
      {action === 'reset_password' && <>
        <p>กำหนดรหัสผ่านใหม่ให้ผู้ใช้ ส่งให้เจ้าของบัญชีผ่านช่องทางส่วนตัว และให้เปลี่ยนรหัสผ่านในโปรไฟล์หลังเข้าสู่ระบบ</p>
        <PasswordField id="admin-new-password" label="รหัสผ่านใหม่ของผู้ใช้" value={nextPassword} onChange={setNextPassword} autoComplete="new-password" disabled={disabled} />
        <PasswordField id="admin-repeat-password" label="ยืนยันรหัสผ่านใหม่" value={repeat} onChange={setRepeat} autoComplete="new-password" disabled={disabled} />
      </>}
      {action === 'delete_account' && <>
        <p>บัญชีนี้จะใช้งานไม่ได้ ข้อมูลโปรไฟล์ รูป ประวัติและดูภายหลังถูกล้าง สมาชิกและเซสชันถูกยกเลิก เก็บเฉพาะบัญชีที่ไม่ระบุตัวตนเพื่ออ้างอิงบันทึกการดูแลระบบ การลบย้อนกลับผ่านเว็บไม่ได้</p>
        <label className="rp-field">พิมพ์อีเมลของบัญชีที่จะลบ<input type="email" autoComplete="off" required value={confirmEmail} disabled={disabled} onChange={event => setConfirmEmail(event.target.value)} /></label>
      </>}
      <PasswordField id="admin-confirm-password" label="รหัสผ่านของคุณเพื่อยืนยันคำสั่ง" value={password} onChange={setPassword} autoComplete="current-password" disabled={disabled} />
      <div className="rp-actions"><button className="rp-mainbutton" disabled={disabled || !password}>{disabled ? 'กำลังบันทึก…' : `ยืนยัน${labels[action]}`}</button>
        <button type="button" className="rp-softbutton" disabled={disabled} onClick={() => { setAction(null); setPassword(''); setNextPassword(''); setRepeat(''); setError('') }}>ยกเลิก</button></div>
    </form> : <div className="rp-actions">
      {owner && <button type="button" className="rp-softbutton" disabled={disabled} onClick={() => { setLevel(target.adminLevel || 'user'); setError(''); setAction('set_role') }}>เปลี่ยนระดับสิทธิ์</button>}
      <button type="button" className="rp-softbutton" disabled={disabled} onClick={() => { setError(''); setAction('reset_password') }}>รีเซ็ตรหัสผ่าน</button>
      {owner && <button type="button" className="rp-softbutton" disabled={disabled} onClick={() => { setConfirmEmail(''); setError(''); setAction('delete_account') }}>ลบบัญชี</button>}
    </div>}
  </section>
}
