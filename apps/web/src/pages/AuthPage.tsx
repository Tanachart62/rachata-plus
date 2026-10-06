import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Icon } from '../components/UI'
import { PasswordField } from '../components/PasswordField'
import type { Profile } from '../types'
import { apiAuthEnabled, AuthError, loginAccount, registerAccount } from '../lib/auth'
import type { ApiUser } from '../lib/auth'

type Field = 'username' | 'name' | 'email' | 'password' | 'confirm'
type Errors = Partial<Record<Field, string>>

interface AuthPageProps {
  mode: 'login' | 'register'
  initialEmail: string
  onRegisterPreview: (username: string, name: string, email: string) => void
  existingAccounts: PreviewAccount[]
  onLoginPreview: (email: string) => void
  onAuthenticated: (user: ApiUser) => void
}

export function AuthPage({ mode, initialEmail, onRegisterPreview, onLoginPreview, onAuthenticated, existingAccounts }: AuthPageProps) {
  const register = mode === 'register'
  const [username, setUsername] = useState('')
  const [name, setName] = useState('')
  const [email, setEmail] = useState(initialEmail)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [errors, setErrors] = useState<Errors>({})
  const [pending, setPending] = useState(false)
  const [complete, setComplete] = useState(false)
  const [serverError, setServerError] = useState('')
  const serverErrorRef = useRef<HTMLParagraphElement>(null)
  const mounted = useRef(true)
  const timer = useRef<number | undefined>(undefined)
  const submitting = useRef(false)
  const successHeading = useRef<HTMLHeadingElement>(null)

  useEffect(() => () => window.clearTimeout(timer.current), [])
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => { if (serverError) serverErrorRef.current?.focus() }, [serverError])
  useEffect(() => { if (complete) successHeading.current?.focus() }, [complete])

  function clearError(field: Field) {
    setErrors(current => ({ ...current, [field]: undefined }))
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting.current) return
    setServerError('')

    const next: Errors = {}
    const normalizedEmail = email.trim().toLowerCase()
    const normalizedUsername = username.trim().toLowerCase()
    if (register && !/^[a-z0-9_]{3,30}$/.test(normalizedUsername)) next.username = 'ใช้ a–z, 0–9 หรือ _ จำนวน 3–30 ตัวอักษร'
    else if (register && !apiAuthEnabled && existingAccounts.some(account => account.username === normalizedUsername)) next.username = 'ชื่อผู้ใช้นี้ถูกใช้แล้วในเดโม่ กรุณาเปลี่ยนชื่อผู้ใช้'
    if (register && (!name.trim() || [...name.trim()].length > 100)) {
      next.name = 'ใส่ชื่อ–นามสกุล 1–100 ตัวอักษร'
    }
    if (!normalizedEmail || normalizedEmail.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(normalizedEmail)) {
      next.email = 'ใส่อีเมลให้ถูกต้อง เช่น name@example.com'
    }
    if (register && !apiAuthEnabled && existingAccounts.some(account => account.email === normalizedEmail)) next.email = 'อีเมลนี้ถูกใช้แล้วในเดโม่ กรุณาใช้อีเมลอื่น'
    // Match the existing Go registration handler's Unicode character limits.
    if (!password) next.password = 'กรุณาใส่รหัสผ่าน'
    else if (register && ([...password].length < 15 || [...password].length > 128)) {
      next.password = 'รหัสผ่านต้องมี 15–128 ตัวอักษร'
    }
    if (register && (!confirm || password !== confirm)) next.confirm = 'ยืนยันรหัสผ่านให้ตรงกัน'
    setErrors(next)
    const firstError = (['username', 'name', 'email', 'password', 'confirm'] as const).find(field => next[field])
    if (firstError) {
      requestAnimationFrame(() => document.getElementById(`auth-${firstError}`)?.focus())
      return
    }

    submitting.current = true
    setPending(true)
    if (apiAuthEnabled) {
      try {
        if (register) {
          await registerAccount(normalizedUsername, name.trim(), normalizedEmail, password)
          if (!mounted.current) return
          onRegisterPreview(normalizedUsername, name.trim(), normalizedEmail)
          setComplete(true)
        } else {
          const user = await loginAccount(normalizedEmail, password)
          // App owns authentication even if the user left the form during the request.
          onAuthenticated(user)
        }
      } catch (error) {
        if (!mounted.current) return
        const code = error instanceof AuthError ? error.code : ''
        const field: Field | undefined = code === 'username_already_exists' || code === 'invalid_username' ? 'username'
          : code === 'email_already_exists' ? 'email' : code === 'invalid_password_length' ? 'password' : undefined
        if (field) {
          const message = field === 'username' ? (code === 'invalid_username' ? 'ใช้ a–z, 0–9 หรือ _ จำนวน 3–30 ตัวอักษร' : 'ชื่อผู้ใช้นี้ถูกใช้แล้ว')
            : field === 'email' ? 'อีเมลนี้ถูกใช้แล้ว' : 'รหัสผ่านต้องมี 15–128 ตัวอักษร'
          setErrors({ [field]: message })
          requestAnimationFrame(() => document.getElementById(`auth-${field}`)?.focus())
        } else {
          setServerError(code === 'invalid_credentials' ? 'อีเมลหรือรหัสผ่านไม่ถูกต้อง'
            : code === 'account_suspended' ? 'บัญชีถูกระงับ กรุณาติดต่อผู้ดูแลระบบ'
            : code === 'too_many_requests' ? 'ส่งคำขอถี่เกินไป กรุณารอประมาณ 1 นาทีแล้วลองใหม่'
            : 'เชื่อมต่อระบบบัญชีไม่ได้ กรุณาลองใหม่อีกครั้ง')
        }
      } finally {
        submitting.current = false
        if (mounted.current) { setPending(false); setPassword(''); setConfirm('') }
      }
      return
    }
    // Demo only: discard passwords before simulating a response. No requests or storage.
    setPassword('')
    setConfirm('')
    timer.current = window.setTimeout(() => {
      if (register) {
        onRegisterPreview(normalizedUsername, name.trim(), normalizedEmail)
        setComplete(true)
      } else {
        onLoginPreview(normalizedEmail)
      }
      submitting.current = false
      setPending(false)
    }, 700)
  }

  return (
    <main className="auth-page">
      <section className="auth-intro" aria-label="เกี่ยวกับ Rachata Plus">
        <div className="rp-eyebrow" lang="en">Your next story starts here</div>
        <p className="auth-intro-title">เรื่องราวดี ๆ<br />เริ่มที่นี่<span>.</span></p>
        <p>พื้นที่ของเรื่องราวที่คุณชอบ<br />กลับมาดูต่อได้ในแบบของคุณ</p>
        <div className="auth-art" aria-label="พื้นที่ภาพประกอบที่ยังว่าง">
          <span>พื้นที่ภาพประกอบ</span>
          <div className="auth-art-caption">RACHATA<span>+</span><small>เรื่องราวดี ๆ อยู่ที่นี่</small></div>
        </div>
      </section>

      <section className="auth-card" aria-labelledby="auth-heading">
        {complete ? (
          <div className="auth-complete">
            <span className="auth-complete-icon"><Icon name="check" /></span>
            <div className="rp-eyebrow" lang="en">Ready for the next step</div>
            <h1 id="auth-heading" ref={successHeading} tabIndex={-1}>{apiAuthEnabled ? 'สร้างบัญชีสำเร็จแล้ว' : 'ลองสมัครเสร็จแล้ว'}</h1>
            <p>ข้อมูลของ <strong>{name.trim()}</strong> ผ่านการตรวจฟอร์มแล้ว</p>
            <p className="auth-email">{email.trim().toLowerCase()}</p>
            <p className="auth-demo">{apiAuthEnabled ? 'บัญชีถูกบันทึกแล้ว เข้าสู่ระบบเพื่อใช้งาน ยังไม่มีการส่งอีเมลยืนยัน' : 'นี่คือตัวอย่างหลังสมัคร ยังไม่ได้สร้างบัญชีจริงหรือส่งอีเมลยืนยัน'}</p>
            <a className="rp-mainbutton rp-linkbutton auth-submit" href="#/login">ไปหน้าเข้าสู่ระบบ</a>
            <a className="auth-back" href="#/">กลับหน้าแรก</a>
          </div>
        ) : (
          <>
            <div className="rp-eyebrow" lang="en">{register ? 'Join Rachata Plus' : 'Welcome back'}</div>
            <h1 id="auth-heading">{register ? 'สร้างบัญชี' : 'เข้าสู่ระบบ'}</h1>
            <p className="auth-subtitle">{register ? 'เริ่มต้นบัญชีของคุณในไม่กี่ขั้นตอน' : 'กลับมารับชมเรื่องราวที่คุณชอบ'}</p>
            <p className="auth-demo">{apiAuthEnabled ? 'สมัครและเข้าสู่ระบบเพื่อบันทึกประวัติและรายการดูภายหลัง' : <>โหมดตัวอย่าง · ยังไม่เชื่อมระบบบัญชี<br />ใช้ข้อมูลสมมุติ รหัสผ่านจะไม่ถูกส่งหรือบันทึก</>}</p>

            <p className="rp-demo-note">จำเป็นต้องกรอกทุกช่อง</p>
            <form noValidate onSubmit={submit} aria-busy={pending}>
              {register && <div className="rp-field"><label htmlFor="auth-username">ชื่อผู้ใช้</label><input id="auth-username" name="username" autoComplete="username" autoCapitalize="none" spellCheck={false} value={username} onChange={event => { setUsername(event.target.value); clearError('username') }} required disabled={pending} aria-invalid={Boolean(errors.username)} aria-describedby={errors.username ? 'auth-username-error' : 'auth-username-hint'} /><small id="auth-username-hint">a–z, 0–9 หรือ _ จำนวน 3–30 ตัวอักษร</small>{errors.username && <span className="auth-error" id="auth-username-error">{errors.username}</span>}</div>}
              {register && (
                <div className="rp-field">
                  <label htmlFor="auth-name">ชื่อ–นามสกุล</label>
                  <input id="auth-name" name="name" autoComplete="name" value={name} onChange={event => { setName(event.target.value); clearError('name') }} required disabled={pending} aria-invalid={Boolean(errors.name)} aria-describedby={errors.name ? 'auth-name-error' : undefined} />
                  {errors.name && <span className="auth-error" id="auth-name-error">{errors.name}</span>}
                </div>
              )}
              <div className="rp-field">
                <label htmlFor="auth-email">อีเมล</label>
                <input id="auth-email" name="email" type="email" autoComplete={register ? "email" : "username"} inputMode="email" autoCapitalize="none" spellCheck={false} value={email} onChange={event => { setEmail(event.target.value); clearError('email') }} required disabled={pending} aria-invalid={Boolean(errors.email)} aria-describedby={errors.email ? 'auth-email-error' : undefined} placeholder="name@example.com" />
                {errors.email && <span className="auth-error" id="auth-email-error">{errors.email}</span>}
              </div>
              <PasswordField id="auth-password" label="รหัสผ่าน" value={password} onChange={value => { setPassword(value); clearError('password'); clearError('confirm') }} error={errors.password} hint={register ? '15–128 ตัวอักษร ใช้เป็นประโยคที่จำง่ายได้' : undefined} autoComplete={register ? 'new-password' : 'current-password'} disabled={pending} />
              {register && <PasswordField id="auth-confirm" label="ยืนยันรหัสผ่าน" value={confirm} onChange={value => { setConfirm(value); clearError('confirm') }} error={errors.confirm} autoComplete="new-password" disabled={pending} />}
              {Object.values(errors).some(Boolean) && <p className="rp-sr" role="alert">กรุณาตรวจช่องกรอกที่มีข้อความแจ้งเตือน</p>}
              <button type="submit" className="rp-mainbutton auth-submit" disabled={pending}>
                {pending && <span className="auth-spinner" aria-hidden="true" />}
                {pending ? (apiAuthEnabled ? 'กำลังส่งข้อมูล…' : 'กำลังจำลอง…') : register ? (apiAuthEnabled ? 'สร้างบัญชี' : 'ทดลองสร้างบัญชี') : (apiAuthEnabled ? 'เข้าสู่ระบบ' : 'ทดลองเข้าสู่ระบบ')}
              </button>
            </form>
            <span className="rp-sr" role="status">{pending ? 'กำลังดำเนินการ กรุณารอสักครู่' : ''}</span>
            <p className={serverError ? 'auth-error' : 'rp-sr'} ref={serverErrorRef} tabIndex={-1} role="alert">{serverError}</p>

            <p className="auth-switch">{register ? 'มีบัญชีอยู่แล้ว?' : 'ยังไม่มีบัญชี?'} <a href={register ? '#/login' : '#/register'}>{register ? 'เข้าสู่ระบบ' : 'สร้างบัญชี'}</a></p>
            {register && <p className="auth-footnote">การสร้างบัญชีไม่ได้เปิดสิทธิ์สมาชิกแบบชำระเงินโดยอัตโนมัติ</p>}
          </>
        )}
      </section>
    </main>
  )
}

// Profile is shared through App; passwords never leave this component.
export type PreviewAccount = Pick<Profile, 'username' | 'name' | 'email'>
