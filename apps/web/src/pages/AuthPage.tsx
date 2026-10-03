import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Icon } from '../components/UI'
import type { Profile } from '../types'

type Field = 'name' | 'email' | 'password' | 'confirm'
type Errors = Partial<Record<Field, string>>

interface AuthPageProps {
  mode: 'login' | 'register'
  initialEmail: string
  onRegisterPreview: (name: string, email: string) => void
  onLoginPreview: (email: string) => void
}

function PasswordField({ id, label, value, onChange, error, hint, autoComplete, disabled }: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  error?: string
  hint?: string
  autoComplete: 'current-password' | 'new-password'
  disabled: boolean
}) {
  const [visible, setVisible] = useState(false)

  return (
    <div className="rp-field">
      <label htmlFor={id}>{label}</label>
      <div className="auth-password">
        <input
          id={id}
          name={id}
          type={visible ? 'text' : 'password'}
          value={value}
          onChange={event => onChange(event.target.value)}
          autoComplete={autoComplete}
          required
          disabled={disabled}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
        />
        <button
          type="button"
          className="auth-reveal"
          onClick={() => setVisible(!visible)}
          aria-label={`${visible ? 'ซ่อน' : 'แสดง'}${label}`}
          aria-pressed={visible}
          disabled={disabled}
        >
          {visible ? 'ซ่อน' : 'แสดง'}
        </button>
      </div>
      {hint && <small id={`${id}-hint`}>{hint}</small>}
      {error && <span className="auth-error" id={`${id}-error`}>{error}</span>}
    </div>
  )
}

export function AuthPage({ mode, initialEmail, onRegisterPreview, onLoginPreview }: AuthPageProps) {
  const register = mode === 'register'
  const [name, setName] = useState('')
  const [email, setEmail] = useState(initialEmail)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [errors, setErrors] = useState<Errors>({})
  const [pending, setPending] = useState(false)
  const [complete, setComplete] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  const submitting = useRef(false)
  const successHeading = useRef<HTMLHeadingElement>(null)

  useEffect(() => () => window.clearTimeout(timer.current), [])
  useEffect(() => { if (complete) successHeading.current?.focus() }, [complete])

  function clearError(field: Field) {
    setErrors(current => ({ ...current, [field]: undefined }))
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting.current) return

    const next: Errors = {}
    const normalizedEmail = email.trim().toLowerCase()
    if (register && (!name.trim() || [...name.trim()].length > 100)) {
      next.name = 'ใส่ชื่อที่แสดง 1–100 ตัวอักษร'
    }
    if (!normalizedEmail || normalizedEmail.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(normalizedEmail)) {
      next.email = 'ใส่อีเมลให้ถูกต้อง เช่น name@example.com'
    }
    // Match the existing Go registration handler's Unicode character limits.
    if (!password) next.password = 'กรุณาใส่รหัสผ่าน'
    else if (register && ([...password].length < 15 || [...password].length > 128)) {
      next.password = 'รหัสผ่านต้องมี 15–128 ตัวอักษร'
    }
    if (register && (!confirm || password !== confirm)) next.confirm = 'ยืนยันรหัสผ่านให้ตรงกัน'
    setErrors(next)
    const firstError = (['name', 'email', 'password', 'confirm'] as const).find(field => next[field])
    if (firstError) {
      document.getElementById(`auth-${firstError}`)?.focus()
      return
    }

    submitting.current = true
    setPending(true)
    // Demo only: discard passwords before simulating a response. No requests or storage.
    setPassword('')
    setConfirm('')
    timer.current = window.setTimeout(() => {
      if (register) {
        onRegisterPreview(name.trim(), normalizedEmail)
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
        <div className="rp-eyebrow">Your next story starts here</div>
        <h2>เรื่องราวดี ๆ<br />เริ่มที่นี่<span>.</span></h2>
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
            <div className="rp-eyebrow">Ready for the next step</div>
            <h1 id="auth-heading" ref={successHeading} tabIndex={-1}>ลองสมัครเสร็จแล้ว</h1>
            <p>ข้อมูลของ <strong>{name.trim()}</strong> ผ่านการตรวจฟอร์มแล้ว</p>
            <p className="auth-email">{email.trim().toLowerCase()}</p>
            <p className="auth-demo">นี่คือตัวอย่างหลังสมัคร ยังไม่ได้สร้างบัญชีจริงหรือส่งอีเมลยืนยัน</p>
            <a className="rp-mainbutton rp-linkbutton auth-submit" href="#/login">ไปหน้าเข้าสู่ระบบ</a>
            <a className="auth-back" href="#/">กลับหน้าแรก</a>
          </div>
        ) : (
          <>
            <div className="rp-eyebrow">{register ? 'Join Rachata Plus' : 'Welcome back'}</div>
            <h1 id="auth-heading">{register ? 'สร้างบัญชี' : 'เข้าสู่ระบบ'}</h1>
            <p className="auth-subtitle">{register ? 'เริ่มต้นบัญชีของคุณในไม่กี่ขั้นตอน' : 'กลับมารับชมเรื่องราวที่คุณชอบ'}</p>
            <p className="auth-demo">โหมดตัวอย่าง · ยังไม่เชื่อมระบบบัญชี<br />ใช้ข้อมูลสมมุติ รหัสผ่านจะไม่ถูกส่งหรือบันทึก</p>

            <form noValidate onSubmit={submit} aria-busy={pending}>
              {register && (
                <div className="rp-field">
                  <label htmlFor="auth-name">ชื่อที่แสดง</label>
                  <input id="auth-name" name="name" autoComplete="nickname" value={name} onChange={event => { setName(event.target.value); clearError('name') }} required disabled={pending} aria-invalid={Boolean(errors.name)} aria-describedby={errors.name ? 'auth-name-error' : undefined} />
                  {errors.name && <span className="auth-error" id="auth-name-error">{errors.name}</span>}
                </div>
              )}
              <div className="rp-field">
                <label htmlFor="auth-email">อีเมล</label>
                <input id="auth-email" name="email" type="email" autoComplete="username" inputMode="email" autoCapitalize="none" spellCheck={false} value={email} onChange={event => { setEmail(event.target.value); clearError('email') }} required disabled={pending} aria-invalid={Boolean(errors.email)} aria-describedby={errors.email ? 'auth-email-error' : undefined} placeholder="name@example.com" />
                {errors.email && <span className="auth-error" id="auth-email-error">{errors.email}</span>}
              </div>
              <PasswordField id="auth-password" label="รหัสผ่าน" value={password} onChange={value => { setPassword(value); clearError('password'); clearError('confirm') }} error={errors.password} hint={register ? '15–128 ตัวอักษร ใช้เป็นประโยคที่จำง่ายได้' : undefined} autoComplete={register ? 'new-password' : 'current-password'} disabled={pending} />
              {register && <PasswordField id="auth-confirm" label="ยืนยันรหัสผ่าน" value={confirm} onChange={value => { setConfirm(value); clearError('confirm') }} error={errors.confirm} autoComplete="new-password" disabled={pending} />}
              {Object.values(errors).some(Boolean) && <p className="rp-sr" role="alert">กรุณาตรวจช่องกรอกที่มีข้อความแจ้งเตือน</p>}
              <button type="submit" className="rp-mainbutton auth-submit" disabled={pending}>
                {pending && <span className="auth-spinner" aria-hidden="true" />}
                {pending ? 'กำลังจำลอง…' : register ? 'ทดลองสร้างบัญชี' : 'ทดลองเข้าสู่ระบบ'}
              </button>
              <span className="rp-sr" role="status">{pending ? 'กำลังจำลอง กรุณารอสักครู่' : ''}</span>
            </form>

            <p className="auth-switch">{register ? 'มีบัญชีอยู่แล้ว?' : 'ยังไม่มีบัญชี?'} <a href={register ? '#/login' : '#/register'}>{register ? 'เข้าสู่ระบบ' : 'สร้างบัญชี'}</a></p>
            {register && <p className="auth-footnote">การสร้างบัญชีไม่ได้เปิดสิทธิ์สมาชิกแบบชำระเงินโดยอัตโนมัติ</p>}
          </>
        )}
      </section>
    </main>
  )
}

// Profile is shared through App; passwords never leave this component.
export type PreviewAccount = Pick<Profile, 'name' | 'email'>
