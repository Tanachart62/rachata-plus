import { useEffect, useRef, useState } from 'react'
import { PasswordField } from './PasswordField'
import { accountError, apiAuthEnabled, AuthError } from '../lib/auth'

type Fields = 'current' | 'next' | 'confirm'

export function ChangePasswordForm({
  onSave,
}: {
  onSave: (password: string, next: string, signal?: AbortSignal) => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [values, setValues] = useState({ current: '', next: '', confirm: '' })
  const [errors, setErrors] = useState<Partial<Record<Fields, string>>>({})
  const [pending, setPending] = useState(false)
  const [message, setMessage] = useState('')
  const [failure, setFailure] = useState('')
  const timer = useRef<number | undefined>(undefined)
  const abort = useRef(new AbortController())
  useEffect(() => {
    abort.current = new AbortController()
    return () => abort.current.abort()
  }, [])
  const trigger = useRef<HTMLButtonElement>(null)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  useEffect(() => {
    if (editing) document.getElementById('change-current')?.focus()
  }, [editing])
  function close() {
    window.clearTimeout(timer.current)
    setValues({ current: '', next: '', confirm: '' })
    setErrors({})
    setPending(false)
    setEditing(false)
    window.requestAnimationFrame(() => trigger.current?.focus())
  }
  return (
    <section className="rp-surface password-section">
      <div className="rp-profile-head">
        <h2>รหัสผ่านและความปลอดภัย</h2>
        {!editing && (
          <button
            ref={trigger}
            type="button"
            className="rp-edit-button"
            onClick={() => {
              setEditing(true)
              setMessage('')
            }}
          >
            เปลี่ยนรหัสผ่าน
          </button>
        )}
      </div>
      <p className="password-help">
        {apiAuthEnabled
          ? 'ยืนยันด้วยรหัสผ่านปัจจุบัน หลังเปลี่ยนรหัสผ่าน อุปกรณ์อื่นจะออกจากระบบ'
          : 'โหมดเดโม่ตรวจรูปแบบเท่านั้น ยังไม่ได้เปลี่ยนรหัสผ่านจริง'}
      </p>
      <p className={message ? 'rp-success' : 'rp-sr'} role="status">
        {pending ? 'กำลังบันทึก…' : message}
      </p>
      <p className={failure ? 'auth-error' : 'rp-sr'} role="alert">
        {failure}
      </p>
      {editing && (
        <form
          noValidate
          aria-busy={pending}
          onSubmit={async (event) => {
            event.preventDefault()
            if (pending) return
            setFailure('')
            const nextErrors: Partial<Record<Fields, string>> = {}
            if (!values.current) nextErrors.current = 'กรุณาใส่รหัสผ่านปัจจุบัน'
            if ([...values.next].length < 15 || [...values.next].length > 128)
              nextErrors.next = 'รหัสผ่านใหม่ต้องมี 15–128 ตัวอักษร'
            else if (values.next === values.current)
              nextErrors.next = 'รหัสผ่านใหม่ต้องต่างจากรหัสผ่านปัจจุบัน'
            if (!values.confirm || values.confirm !== values.next)
              nextErrors.confirm = 'ยืนยันรหัสผ่านใหม่ให้ตรงกัน'
            setErrors(nextErrors)
            const first = (['current', 'next', 'confirm'] as const).find(
              (field) => nextErrors[field],
            )
            if (first) {
              requestAnimationFrame(() => document.getElementById(`change-${first}`)?.focus())
              return
            }
            setPending(true)
            if (apiAuthEnabled) {
              try {
                await onSave(values.current, values.next, abort.current.signal)
                if (abort.current.signal.aborted) return
                close()
                setMessage('เปลี่ยนรหัสผ่านแล้ว อุปกรณ์อื่นออกจากระบบแล้ว')
              } catch (error) {
                if (abort.current.signal.aborted) return
                setPending(false)
                if (error instanceof AuthError && error.code === 'invalid_current_password') {
                  setErrors({ current: accountError(error) })
                  requestAnimationFrame(() => document.getElementById('change-current')?.focus())
                } else setFailure(accountError(error))
              }
            } else
              timer.current = window.setTimeout(() => {
                close()
                setMessage('ตรวจฟอร์มผ่านแล้ว · ยังไม่ได้เปลี่ยนรหัสผ่านจริง')
              }, 700)
          }}
        >
          <p className="rp-demo-note">จำเป็นต้องกรอกทุกช่อง</p>
          {(['current', 'next', 'confirm'] as const).map((field) => (
            <PasswordField
              key={field}
              id={`change-${field}`}
              label={
                {
                  current: 'รหัสผ่านปัจจุบัน',
                  next: 'รหัสผ่านใหม่',
                  confirm: 'ยืนยันรหัสผ่านใหม่',
                }[field]
              }
              value={values[field]}
              onChange={(value) => {
                setValues((current) => ({ ...current, [field]: value }))
                setErrors({})
              }}
              error={errors[field]}
              hint={field === 'next' ? '15–128 ตัวอักษร' : undefined}
              autoComplete={field === 'current' ? 'current-password' : 'new-password'}
              disabled={pending}
            />
          ))}
          {Object.keys(errors).length > 0 && (
            <p className="rp-sr" role="alert">
              กรุณาตรวจช่องรหัสผ่านที่มีข้อความแจ้งเตือน
            </p>
          )}
          <div className="rp-formfooter">
            <button
              type="button"
              className="rp-softbutton"
              disabled={pending}
              onClick={() => {
                close()
                setFailure('')
              }}
            >
              ยกเลิก
            </button>
            <button type="submit" className="rp-mainbutton" disabled={pending}>
              {pending
                ? 'กำลังบันทึก…'
                : apiAuthEnabled
                  ? 'เปลี่ยนรหัสผ่าน'
                  : 'ทดลองเปลี่ยนรหัสผ่าน'}
            </button>
          </div>
        </form>
      )}
    </section>
  )
}
