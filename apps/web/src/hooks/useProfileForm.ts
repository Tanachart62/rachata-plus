import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import type { Profile } from '../types'
import { accountError, apiAuthEnabled, AuthError } from '../lib/auth'

export function useProfileForm(
  profile: Profile,
  onSave: (next: Profile, password: string, signal?: AbortSignal) => Promise<void>,
) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(profile)
  const [tab, setTab] = useState<'account' | 'membership'>('account')
  const [message, setMessage] = useState('')
  const [failure, setFailure] = useState('')
  const [pending, setPending] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [errors, setErrors] = useState<{ name?: string; email?: string; password?: string }>({})
  const abort = useRef(new AbortController())
  useEffect(() => {
    abort.current = new AbortController()
    return () => abort.current.abort()
  }, [])
  const editButton = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (editing) document.getElementById('profile-name')?.focus()
  }, [editing])
  function closeEditor() {
    setEditing(false)
    setErrors({})
    setCurrentPassword('')
    requestAnimationFrame(() => editButton.current?.focus())
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    setFailure('')
    const next = {
      ...profile,
      name: draft.name.trim(),
      email: draft.email.trim().toLowerCase(),
      emailPending:
        !apiAuthEnabled &&
        (profile.emailPending || draft.email.trim().toLowerCase() !== profile.email),
    }
    const issues: typeof errors = {}
    if (!next.name || [...next.name].length > 100) issues.name = 'ชื่อ–นามสกุลต้องมี 1–100 ตัวอักษร'
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(next.email))
      issues.email = 'กรุณาใส่อีเมล เช่น name@example.com'
    setErrors(issues)
    if (Object.keys(issues).length) {
      requestAnimationFrame(() =>
        document.getElementById(issues.name ? 'profile-name' : 'profile-email')?.focus(),
      )
      return
    }
    if (apiAuthEnabled && next.email !== profile.email && !currentPassword) {
      setErrors({ password: 'กรุณาใส่รหัสผ่านปัจจุบันเพื่อเปลี่ยนอีเมล' })
      document.getElementById('profile-password')?.focus()
      return
    }
    abort.current?.abort()
    abort.current = new AbortController()
    setPending(true)
    try {
      if (apiAuthEnabled) {
        await onSave(next, currentPassword, abort.current.signal)
      } else await onSave(next, currentPassword)
      if (abort.current.signal.aborted) return
      closeEditor()
      setMessage(
        apiAuthEnabled
          ? 'บันทึกข้อมูลแล้ว'
          : 'บันทึกในตัวอย่างแล้ว ข้อมูลจะกลับค่าเริ่มต้นเมื่อรีเฟรช',
      )
    } catch (error) {
      if (abort.current.signal.aborted) return
      if (error instanceof AuthError && error.code === 'email_already_exists') {
        setErrors({ email: accountError(error) })
        requestAnimationFrame(() => document.getElementById('profile-email')?.focus())
      } else if (error instanceof AuthError && error.code === 'invalid_current_password') {
        setErrors({ password: accountError(error) })
        requestAnimationFrame(() => document.getElementById('profile-password')?.focus())
      } else setFailure(accountError(error))
    } finally {
      if (!abort.current.signal.aborted) setPending(false)
    }
  }
  return {
    editing,
    setEditing,
    draft,
    setDraft,
    tab,
    setTab,
    message,
    setMessage,
    failure,
    setFailure,
    pending,
    currentPassword,
    setCurrentPassword,
    errors,
    editButton,
    closeEditor,
    submit,
  }
}
