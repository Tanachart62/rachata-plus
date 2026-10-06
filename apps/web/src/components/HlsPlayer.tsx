import { useEffect, useRef, useState } from 'react'
import type Hls from 'hls.js'

export function HlsPlayer({
  src,
  title,
  initialPosition = 0,
  onProgress,
}: {
  src: string
  title: string
  initialPosition?: number
  onProgress?: (seconds: number, flush?: boolean) => Promise<void>
}) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const progressRef = useRef(onProgress)
  const resume = useRef({ src, position: initialPosition })
  const acknowledged = useRef(-1)
  const requestSequence = useRef(0)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    progressRef.current = onProgress
  }, [onProgress])
  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    if (resume.current.src !== src) {
      resume.current = { src, position: initialPosition }
      acknowledged.current = -1
    }
    let hls: Hls | undefined
    let disposed = false
    let played = false
    let broken = false
    let lastReport = 0
    let lastRequested = -1
    const metadata = () => {
      const position = resume.current.position
      if (position > 0 && position < video.duration) video.currentTime = position
    }
    const failed = () => {
      broken = true
      if (!disposed)
        setError('เล่นวิดีโอไม่ได้ กรุณาตรวจสิทธิ์สมาชิกหรือเข้าสู่ระบบใหม่ แล้วลองอีกครั้ง')
    }
    const report = (flush = false, ended = false) => {
      if (!played || !Number.isFinite(video.currentTime)) return
      if (!broken) resume.current.position = ended ? 0 : video.currentTime
      const seconds = ended
        ? Math.ceil(video.duration)
        : Math.floor(broken ? resume.current.position : video.currentTime)
      const now = Date.now()
      if (
        !Number.isFinite(seconds) ||
        seconds === acknowledged.current ||
        (!flush && now - lastReport < 10000)
      )
        return
      const send = progressRef.current
      if (!send) return
      lastReport = now
      lastRequested = seconds
      const sequence = ++requestSequence.current
      void send(seconds, flush)
        .then(() => {
          // A coalesced newer request may resolve multiple waiters. Only the
          // latest reported position can become this player's saved position.
          if (sequence === requestSequence.current) acknowledged.current = lastRequested
        })
        .catch(() => {
          /* Account hook retains the pending value and shows retry. */
        })
    }
    const playing = () => {
      played = true
      report()
    }
    const progress = () => report()
    const flush = () => report(true)
    const ended = () => report(true, true)
    const hidden = () => {
      if (document.visibilityState === 'hidden') flush()
    }
    video.addEventListener('loadedmetadata', metadata)
    video.addEventListener('error', failed)
    video.addEventListener('timeupdate', progress)
    video.addEventListener('ended', ended)
    video.addEventListener('playing', playing)
    video.addEventListener('pause', flush)
    video.addEventListener('seeked', flush)
    window.addEventListener('pagehide', flush)
    document.addEventListener('visibilitychange', hidden)
    if (video.canPlayType('application/vnd.apple.mpegurl')) video.src = src
    else
      void import('hls.js')
        .then(({ default: Hls }) => {
          if (disposed) return
          if (!Hls.isSupported()) {
            failed()
            return
          }
          hls = new Hls({ maxBufferLength: 20 })
          hls.on(Hls.Events.ERROR, (_event, data) => {
            if (data.fatal) {
              failed()
              hls?.destroy()
              hls = undefined
            }
          })
          hls.loadSource(src)
          hls.attachMedia(video)
        })
        .catch(failed)
    return () => {
      flush()
      disposed = true
      video.removeEventListener('loadedmetadata', metadata)
      video.removeEventListener('error', failed)
      video.removeEventListener('timeupdate', progress)
      video.removeEventListener('ended', ended)
      video.removeEventListener('playing', playing)
      video.removeEventListener('pause', flush)
      video.removeEventListener('seeked', flush)
      window.removeEventListener('pagehide', flush)
      document.removeEventListener('visibilitychange', hidden)
      hls?.destroy()
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
    // Initial position is a seed; changing history props must not restart HLS.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [src, attempt])
  return (
    <div style={{ width: '100%' }}>
      <video
        ref={videoRef}
        controls
        playsInline
        preload="metadata"
        aria-label={title}
        style={{ width: '100%', aspectRatio: '16 / 9', display: 'block', background: '#000' }}
      />
      <p className={error ? 'auth-error' : 'rp-sr'} role="status">
        {error}
      </p>
      {error && (
        <button
          type="button"
          className="rp-softbutton"
          onClick={() => {
            setError('')
            setAttempt((v) => v + 1)
          }}
        >
          ลองเล่นอีกครั้ง
        </button>
      )}
    </div>
  )
}
