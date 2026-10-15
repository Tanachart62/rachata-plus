import { useEffect, useState } from 'react'
import type { ReactNode, RefObject } from 'react'
import { Icon } from './UI'

function playbackTime(value: number) {
  const seconds = Math.max(0, Math.floor(Number.isFinite(value) ? value : 0))
  const hours = Math.floor(seconds / 3600)
  return `${hours ? hours + ':' : ''}${String(Math.floor(seconds / 60) % 60).padStart(hours ? 2 : 1, '0')}:${String(seconds % 60).padStart(2, '0')}`
}

export function PlaybackControls({ videoRef, playerRef, quality }: {
  videoRef: RefObject<HTMLVideoElement | null>
  playerRef: RefObject<HTMLDivElement | null>
  quality: ReactNode
}) {
  const [media, setMedia] = useState({ position: 0, duration: 0, paused: true, muted: false, volume: 1 })
  const [fullscreen, setFullscreen] = useState(false)
  const [notice, setNotice] = useState('')
  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    const update = () => setMedia({ position: Number.isFinite(video.currentTime) ? video.currentTime : 0, duration: Number.isFinite(video.duration) ? video.duration : 0, paused: video.paused, muted: video.muted, volume: video.volume })
    const events = ['loadedmetadata', 'durationchange', 'timeupdate', 'play', 'pause', 'ended', 'volumechange', 'emptied']
    events.forEach(event => video.addEventListener(event, update))
    const changed = () => setFullscreen(document.fullscreenElement === playerRef.current)
    document.addEventListener('fullscreenchange', changed)
    return () => {
      events.forEach(event => video.removeEventListener(event, update))
      document.removeEventListener('fullscreenchange', changed)
    }
  }, [videoRef, playerRef])
  async function togglePlayback() {
    const video = videoRef.current
    if (!video) return
    setNotice('')
    if (video.paused) {
      try { await video.play() } catch { setNotice('ยังเล่นไม่ได้ กรุณารอไฟล์โหลดแล้วลองอีกครั้ง') }
    } else video.pause()
  }
  async function toggleFullscreen() {
    setNotice('')
    try {
      if (document.fullscreenElement === playerRef.current) await document.exitFullscreen()
      else await playerRef.current?.requestFullscreen()
    } catch { setNotice('เปิดเต็มจอไม่ได้ กรุณาลองอีกครั้ง') }
  }
  return <div className="rp-hls-controls" role="group" aria-label="ควบคุมวิดีโอ">
    <input className="rp-hls-seek" type="range" aria-label="ตำแหน่งการเล่น" aria-valuetext={`${playbackTime(media.position)} จาก ${playbackTime(media.duration)}`} min={0} max={media.duration || 0} step={1} value={Math.min(media.position, media.duration)} disabled={!media.duration} onChange={event => {
      const position = Number(event.target.value)
      if (videoRef.current) videoRef.current.currentTime = position
      setMedia(previous => ({ ...previous, position }))
    }} />
    <div className="rp-hls-buttons">
      <button type="button" aria-label={media.paused ? 'เล่นวิดีโอ' : 'หยุดวิดีโอชั่วคราว'} onClick={() => void togglePlayback()}><Icon name={media.paused ? 'play' : 'pause'} /></button>
      <span className="rp-hls-time">{playbackTime(media.position)} / {playbackTime(media.duration)}</span>
      <button type="button" aria-label={media.muted || !media.volume ? 'เปิดเสียง' : 'ปิดเสียง'} onClick={() => {
        const video = videoRef.current
        if (!video) return
        if (!video.volume) { video.volume = 1; video.muted = false } else video.muted = !video.muted
      }}><Icon name={media.muted || !media.volume ? 'muted' : 'volume'} /></button>
      <input className="rp-hls-volume" type="range" aria-label="ระดับเสียง" min={0} max={1} step={0.05} value={media.muted ? 0 : media.volume} onChange={event => {
        const volume = Number(event.target.value)
        if (videoRef.current) { videoRef.current.volume = volume; videoRef.current.muted = false }
        setMedia(previous => ({ ...previous, volume, muted: false }))
      }} />
      {quality}
      <button type="button" aria-label={fullscreen ? 'ออกจากเต็มจอ' : 'เต็มจอ'} onClick={() => void toggleFullscreen()}><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d={fullscreen ? 'M9 3v6H3m12-6v6h6M3 15h6v6m6 0v-6h6' : 'M9 3H3v6m12-6h6v6M3 15v6h6m6 0h6v-6'} /></svg></button>
    </div>
    <p className={notice ? 'rp-hls-notice' : 'rp-sr'} role="status">{notice}</p>
  </div>
}
