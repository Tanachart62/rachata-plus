import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { accountError, AuthError } from '../lib/auth'
import defaultAvatar from '../assets/default-avatar.png'

const maxBytes = 2 * 1024 * 1024

async function avatarRequest(userId: number, signal: AbortSignal, file?: File) {
  const body = file ? new FormData() : undefined
  if (body && file) {
    body.append('expectedUserId', String(userId))
    body.append('file', file)
  }
  const response = await fetch(`/auth/avatar?expectedUserId=${userId}`, {
    method: file ? 'POST' : 'GET', body, signal: AbortSignal.any([signal, AbortSignal.timeout(20000)]),
    credentials: 'same-origin',
  })
  if (!file && response.status === 404) return null
  if (!response.ok) {
    const data = await response.json()
    throw new AuthError(data.error ?? 'service_unavailable')
  }
  return file ? null : response.blob()
}

export function ProfileAvatar({ userId, name }: { userId: number; name: string }) {
  const [url, setUrl] = useState('')
  const pictureUrl = useRef('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const picker = useRef<HTMLInputElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const upload = useRef<AbortController | null>(null)
  const initialLoad = useRef<AbortController | null>(null)
  const hintId = useId()
  const errorId = useId()
  const setPicture = useCallback((picture: Blob | null) => {
    if (pictureUrl.current) URL.revokeObjectURL(pictureUrl.current)
    pictureUrl.current = picture ? URL.createObjectURL(picture) : ''
    setUrl(pictureUrl.current)
  }, [])
  useEffect(() => {
    const request = new AbortController()
    initialLoad.current = request
    if (userId) void avatarRequest(userId, request.signal).then(picture => {
      if (!request.signal.aborted) setPicture(picture)
    }).catch(cause => {
      if (!request.signal.aborted) setError(accountError(cause))
    })
    return () => {
      request.abort(); upload.current?.abort()
      if (pictureUrl.current) URL.revokeObjectURL(pictureUrl.current)
    }
  }, [userId, setPicture])
  async function choose(file?: File) {
    if (!file || pending) return
    setError('')
    setNotice('')
    if (!['image/jpeg', 'image/png'].includes(file.type) || file.size > maxBytes || !file.size) {
      setError('เลือกรูป JPEG หรือ PNG ขนาดไม่เกิน 2 MB')
      if (picker.current) picker.current.value = ''
      button.current?.focus()
      return
    }
    const request = new AbortController()
    initialLoad.current?.abort()
    upload.current = request
    setPending(true)
    try {
      if (userId) {
        await avatarRequest(userId, request.signal, file)
        const picture = await avatarRequest(userId, request.signal)
        if (request.signal.aborted) return
        setPicture(picture)
      } else setPicture(file)
      window.dispatchEvent(new CustomEvent('rachata-avatar-changed', { detail: { userId, picture: userId ? undefined : file } }))
      setNotice(userId ? 'อัปโหลดรูปโปรไฟล์แล้ว' : 'รูปตัวอย่างอยู่เฉพาะรอบนี้')
    } catch (cause) {
      if (!request.signal.aborted) setError(accountError(cause))
    } finally {
      if (!request.signal.aborted) {
        setPending(false)
        if (picker.current) picker.current.value = ''
        requestAnimationFrame(() => button.current?.focus())
      }
    }
  }
  return <div className="rp-profile-avatar" aria-busy={pending}>
    <img className="rp-avatar" src={url || defaultAvatar} alt={`รูปโปรไฟล์ของ ${name}`} />
    <button ref={button} className="rp-softbutton" type="button" disabled={pending}
      aria-describedby={`${hintId} ${errorId}`} onClick={() => picker.current?.click()}>
      {pending ? 'กำลังอัปโหลด…' : 'อัปโหลดรูปโปรไฟล์'}
    </button>
    <input ref={picker} hidden type="file" disabled={pending} accept="image/jpeg,image/png" aria-label="เลือกไฟล์รูปโปรไฟล์"
      onChange={event => void choose(event.target.files?.[0])} />
    <p id={hintId} className="rp-demo-note">JPEG / PNG ไม่เกิน 2 MB<br />รูปจะครอบตรงกลางเป็นวงกลม</p>
    <p id={errorId} className={error ? 'auth-error' : 'rp-sr'} role="alert">{error}</p>
    <p className={notice ? 'rp-demo-note' : 'rp-sr'} role="status">{notice}</p>
  </div>
}

export function AccountAvatar({ userId }: { userId: number }) {
  const [url, setUrl] = useState('')
  useEffect(() => {
    let pictureUrl = ''
    let request: AbortController | undefined
    const display = (picture: Blob | null) => {
      if (pictureUrl) URL.revokeObjectURL(pictureUrl)
      pictureUrl = picture ? URL.createObjectURL(picture) : ''
      setUrl(pictureUrl)
    }
    const refresh = async () => {
      if (!userId) return
      request?.abort()
      const current = new AbortController()
      request = current
      try {
        const picture = await avatarRequest(userId, current.signal)
        if (!current.signal.aborted) display(picture)
      } catch { /* Keep the current avatar if the request fails. */ }
    }
    const changed = (event: Event) => {
      const detail = (event as CustomEvent<{ userId: number; picture?: Blob }>).detail
      if (detail.userId !== userId) return
      if (userId) void refresh()
      else if (detail.picture) display(detail.picture)
    }
    void refresh()
    window.addEventListener('rachata-avatar-changed', changed)
    return () => {
      request?.abort()
      window.removeEventListener('rachata-avatar-changed', changed)
      if (pictureUrl) URL.revokeObjectURL(pictureUrl)
    }
  }, [userId])
  return <img className="rp-accountdot" src={url || defaultAvatar} alt="" />
}
