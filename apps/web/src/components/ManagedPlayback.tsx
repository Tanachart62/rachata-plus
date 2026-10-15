import { useEffect, useRef, useState } from 'react'
import { ApiError, isObject, jsonRequest } from '../lib/http'
import { HlsPlayer } from './HlsPlayer'

type Lease = { token: string; url: string; maxHeight: number }
function message(error: unknown) {
  if (error instanceof ApiError) {
    if (error.code === 'stream_limit_reached') return 'เปิดรับชมครบจำนวนจอตามแพ็กเกจแล้ว กรุณาปิดเครื่องเล่นบนจออื่นแล้วลองใหม่'
    if (error.code === 'membership_required') return 'ไม่มีสิทธิ์สมาชิกสำหรับรับชมวิดีโอนี้'
    if (error.code === 'unauthenticated' || error.code === 'account_changed') return 'บัญชีหรือเซสชันเปลี่ยนแล้ว กรุณาเข้าสู่ระบบใหม่'
    if (error.code === 'video_changed') return 'วิดีโอถูกแปลงใหม่ กรุณาลองเปิดเครื่องเล่นอีกครั้ง'
    if (error.code === 'entitlements_changed') return 'สิทธิ์แพ็กเกจเปลี่ยนแล้ว กรุณาเปิดเครื่องเล่นอีกครั้งเพื่อใช้คุณภาพล่าสุด'
  }
  return 'ตรวจสอบสิทธิ์รับชมไม่ได้ กรุณาลองอีกครั้ง'
}

// A slot belongs to one open player, not to an entire logged-in device.
// Release on navigation; crashed/offline clients expire after 90 seconds.
export function ManagedPlayback({ videoId, userId, src, title, initialPosition, onProgress }: {
  videoId: number; userId: number; src: string; title: string; initialPosition: number
  onProgress: (seconds: number, flush?: boolean) => Promise<void>
}) {
  const [lease, setLease] = useState<(Lease & { source: string; position: number }) | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  // Serialize StrictMode setup/cleanup, including a late acquisition response.
  const queue = useRef<Promise<void>>(Promise.resolve())
  const position = useRef(initialPosition)
  useEffect(() => {
    let disposed = false
    let active: Lease | null = null
    let heartbeat: number | undefined
    let renewing = false
    const endpoint = `/api/playback/${videoId}`
    const release = async () => {
      const current = active
      active = null
      if (current) await jsonRequest(endpoint, { method: 'DELETE', body: { expectedUserId: userId, token: current.token }, keepalive: true }).catch(() => {})
    }
    const stop = () => { disposed = true; window.clearInterval(heartbeat) }
    const request = (token?: string) => jsonRequest(endpoint, { body: { expectedUserId: userId, token } })
    queue.current = queue.current.then(async () => {
      if (disposed) return
      setLease(null)
      setError('')
      try {
        const result = await request()
        if (!isObject(result) || typeof result.token !== 'string' || typeof result.url !== 'string' || typeof result.maxHeight !== 'number') throw new Error('invalid playback response')
        const url = new URL(result.url, window.location.origin)
        if (url.origin !== window.location.origin || url.pathname !== `/media/${videoId}/full/index.m3u8`) throw new Error('invalid playback URL')
        active = { token: result.token, url: url.href, maxHeight: result.maxHeight }
        if (disposed) { await release(); return }
        setLease({ ...active, source: src, position: position.current })
        heartbeat = window.setInterval(() => {
          if (!active || disposed || renewing) return
          renewing = true
          void request(active.token).then(result => {
            if (disposed || !active) return
            if (!isObject(result) || typeof result.maxHeight !== 'number') throw new Error('invalid playback response')
            // Stop a player whose manifest still lists a previous package's
            // levels; reopening obtains the current filtered master playlist.
            if (result.maxHeight !== active.maxHeight) throw new ApiError('entitlements_changed')
          }).catch(async error => {
            if (disposed) return
            stop()
            setLease(null)
            setError(message(error))
            await release()
          }).finally(() => { renewing = false })
        }, 20000)
      } catch (error) { if (!disposed) setError(message(error)) }
    })
    const leaving = () => {
      stop()
      setLease(null)
      setError('การรับชมสิ้นสุดแล้ว กดเปิดเครื่องเล่นอีกครั้งเพื่อรับชมต่อ')
      queue.current = queue.current.then(release)
    }
    const returned = (event: PageTransitionEvent) => { if (event.persisted) setAttempt(value => value + 1) }
    window.addEventListener('pagehide', leaving)
    window.addEventListener('pageshow', returned)
    return () => {
      stop()
      window.removeEventListener('pagehide', leaving)
      window.removeEventListener('pageshow', returned)
      queue.current = queue.current.then(release)
    }
  }, [videoId, userId, src, attempt])
  return lease?.source === src ? (
    <HlsPlayer src={lease.url} title={title} initialPosition={lease.position} onProgress={(seconds, flush) => {
      position.current = seconds
      return onProgress(seconds, flush)
    }} />
  ) : (
    <div className="rp-player-center rp-playback-status" aria-busy={!error}>
      <p role="status">{error || 'กำลังตรวจสอบสิทธิ์รับชม…'}</p>
      {error && <button type="button" className="rp-softbutton" onClick={() => setAttempt(value => value + 1)}>เปิดเครื่องเล่นอีกครั้ง</button>}
    </div>
  )
}
