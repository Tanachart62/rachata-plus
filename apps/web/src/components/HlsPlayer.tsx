import { useEffect, useId, useRef, useState } from 'react'
import type Hls from 'hls.js'
import { PlaybackControls } from './PlaybackControls'

function qualityLabel(height: number) {
  return height === 1440 ? '1440p (2K)' : height === 2160 ? '2160p (4K)' : height ? `${height}p` : 'คุณภาพต้นฉบับ'
}

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
  const qualityId = useId()
  const qualityHintId = useId()
  const playerRef = useRef<HTMLDivElement>(null)
  const hlsRef = useRef<Hls | null>(null)
  const [quality, setQuality] = useState({ source: '', levels: [] as { value: string; label: string }[], selected: '-1', native: false })
  const nativeResume = useRef<{ position: number; playing: boolean } | null>(null)
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
    const controller = new AbortController()
    let disposed = false
    let played = false
    let broken = false
    let lastReport = 0
    let lastRequested = -1
    const metadata = () => {
      const position = nativeResume.current?.position ?? resume.current.position
      if (position > 0 && position < video.duration) video.currentTime = position
      if (nativeResume.current?.playing) void video.play().catch(() => {})
      nativeResume.current = null
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
    const nativePlayback = async () => {
      if (!video.canPlayType('application/vnd.apple.mpegurl')) { failed(); return }
      video.src = src
      let levels: {value:string;label:string}[] = []
      try {
        const response = await fetch(src, { signal: controller.signal })
        if (!response.ok) throw new Error('manifest unavailable')
        const master = await response.text()
        levels = [...master.matchAll(/^([0-9]{1,4})p\.m3u8(?:\?[^\s]*)?\s*$/gm)].map(match => ({ value: match[0].trim(), label: qualityLabel(Number(match[1])) }))
      } catch { /* Native playback can still handle a legacy media playlist. */ }
      if (!disposed) setQuality({source:src,levels,selected:'-1',native:true})
    }
    void import('hls.js').then(({ default: Hls }) => {
      if (disposed) return
      if (!Hls.isSupported()) { void nativePlayback(); return }
      hls = new Hls({ maxBufferLength: 20 })
      hlsRef.current = hls
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (disposed || !hls) return
        setQuality({source:src,levels:hls.levels.map((level,index) => ({value:String(index),label:qualityLabel(level.height)})),selected:'-1',native:false})
      })
      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) { failed(); hls?.destroy(); hls = undefined; hlsRef.current = null }
      })
      hls.loadSource(src)
      hls.attachMedia(video)
    }).catch(() => { if (!disposed) void nativePlayback() })
    return () => {
      flush()
      disposed = true
      controller.abort()
      hlsRef.current = null
      nativeResume.current = null
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
  const qualityMenu = (
    <div className="rp-player-quality">
            <label className="rp-sr" htmlFor={qualityId}>คุณภาพวิดีโอ</label>
            <select aria-describedby={qualityHintId} id={qualityId} disabled={quality.source !== src || !!error} value={quality.source === src ? quality.selected : '-1'} onChange={event => {
              const selected = event.target.value
              setQuality(previous => ({...previous,selected}))
              if (hlsRef.current) hlsRef.current.nextLevel = Number(selected)
              else if (quality.native && videoRef.current) {
                const video = videoRef.current
                nativeResume.current = { position: video.currentTime, playing: !video.paused }
                video.src = selected === '-1' ? src : new URL(selected, new URL(src, window.location.href)).href
                video.load()
              }
            }}>
              <option value="-1">อัตโนมัติ</option>
              {quality.source === src && quality.levels.map(level => <option key={level.value} value={level.value}>{level.label}</option>)}
            </select>
            <span className="rp-sr" id={qualityHintId}>{quality.source === src && quality.levels.length === 0 ? 'เบราว์เซอร์จัดการคุณภาพอัตโนมัติ' : 'แสดงเฉพาะความละเอียดที่ไฟล์นี้มี'}</span>
          </div>
  )
  return (
    <div ref={playerRef} className="rp-hls-player">
      <div className="rp-hls-stage">
        <video
          ref={videoRef}
          tabIndex={0}
          onKeyDown={event => {
            const video = event.currentTarget
            if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); if (video.paused) void video.play().catch(() => {}); else video.pause() }
            else if (['ArrowLeft', 'ArrowRight'].includes(event.key) && Number.isFinite(video.duration)) { event.preventDefault(); video.currentTime = Math.max(0, Math.min(video.duration, video.currentTime + (event.key === 'ArrowRight' ? 5 : -5))) }
          }}
          playsInline
          preload="metadata"
          aria-label={title}
          style={{ width: '100%', aspectRatio: '16 / 9', display: 'block', background: '#000' }}
        />
        <PlaybackControls videoRef={videoRef} playerRef={playerRef} quality={qualityMenu} />
      </div>
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
