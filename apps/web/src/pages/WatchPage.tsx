import { useEffect, useRef, useState } from 'react'
import { Icon, SaveButton, Skeleton, VideoSection } from '../components/UI'
import type { Video, Viewer } from '../types'
import { HlsPlayer } from '../components/HlsPlayer'
import { durationLabel } from '../lib/video-time'

function time(seconds: number) {
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}

export function WatchPage({
  video,
  videos,
  viewer,
  loading,
  saved,
  onToggle,
  initialPosition,
  onProgress,
}: {
  video: Video
  videos: Video[]
  viewer: Viewer
  loading: boolean
  saved: boolean
  onToggle: () => void
  initialPosition: number
  onProgress: (videoId: number, position: number, flush?: boolean) => Promise<void>
}) {
  const progress = useRef(onProgress)
  useEffect(() => {
    progress.current = onProgress
  }, [onProgress])
  const [playing, setPlaying] = useState(false)
  const [muted, setMuted] = useState(false)
  const [position, setPosition] = useState(() =>
    initialPosition >= video.durationSeconds ? 0 : initialPosition,
  )
  const [started, setStarted] = useState(false)
  const [quality, setQuality] = useState('auto')
  const locked = viewer === 'guest' || viewer === 'user'
  const atEnd = position >= video.durationSeconds
  useEffect(() => {
    if (video.playbackUrl || !playing || locked || loading || atEnd) return
    const timer = window.setInterval(
      () => setPosition((current) => Math.min(video.durationSeconds, current + 1)),
      1000,
    )
    return () => window.clearInterval(timer)
  }, [playing, locked, loading, atEnd, video.durationSeconds, video.playbackUrl])
  useEffect(() => {
    if (started && !locked && !loading) void progress.current(video.id, position).catch(() => {})
  }, [started, locked, loading, video.id, position])
  function togglePlayback() {
    setStarted(true)
    if (atEnd) {
      setPosition(0)
      setPlaying(true)
    } else setPlaying(!playing)
  }
  return (
    <main className="rp-watch">
      <div className="rp-watch-top">
        <a href={`#/videos/${video.id}`}>
          <Icon name="back" />
          กลับหน้ารายละเอียด
        </a>
        <span className="rp-counter">
          {video.processingStatus ? 'รับชมวิดีโอ' : 'ตัวอย่างเครื่องเล่น'}
        </span>
      </div>
      <section className="rp-player" aria-label="เครื่องเล่นวิดีโอ" aria-busy={loading}>
        {loading ? (
          <Skeleton shape="player" />
        ) : locked ? (
          <div className="rp-player-center">
            <strong>วิดีโอนี้สำหรับสมาชิก</strong>
            <p>
              {viewer === 'guest'
                ? 'เข้าสู่ระบบด้วยบัญชีสมาชิกเพื่อรับชม'
                : 'บัญชีทั่วไปยังไม่มีแพ็กเกจสมาชิกสำหรับรับชม'}
            </p>
            <a
              href={viewer === 'guest' ? '#/login' : '#/profile'}
              className="rp-mainbutton rp-linkbutton"
            >
              {viewer === 'guest' ? 'เข้าสู่ระบบ' : 'ดูข้อมูลสมาชิก'}
            </a>
            {viewer === 'guest' && (
              <a className="auth-inline-link" href="#/register">
                ยังไม่มีบัญชี? สร้างบัญชี
              </a>
            )}
          </div>
        ) : video.playbackUrl ? (
          <HlsPlayer
            src={video.playbackUrl}
            title={video.title}
            initialPosition={initialPosition}
            onProgress={(seconds, flush) => onProgress(video.id, seconds, flush)}
          />
        ) : video.processingStatus ? (
          <div className="rp-player-center">
            <strong>วิดีโอยังไม่พร้อมรับชม</strong>
            <p>
              {video.processingStatus === 'failed' ? 'แปลงไฟล์ไม่สำเร็จ' : 'กำลังเตรียมไฟล์วิดีโอ'}
            </p>
          </div>
        ) : (
          <>
            <div className="rp-player-center">
              <button
                type="button"
                onClick={togglePlayback}
                aria-label={
                  playing && !atEnd ? 'หยุดตัวอย่างเครื่องเล่น' : 'เล่นตัวอย่างเครื่องเล่น'
                }
                aria-pressed={playing && !atEnd}
              >
                <Icon name={playing && !atEnd ? 'pause' : 'play'} />
              </button>
              <strong>พื้นที่เล่นวิดีโอ</strong>
              <p role="status">
                {atEnd
                  ? 'ดูตัวอย่างจบแล้ว · เล่นอีกครั้งได้'
                  : playing
                    ? 'กำลังจำลองเวลาเล่น · บันทึกประวัติในเดโม่'
                    : 'ยังไม่มีไฟล์วิดีโอ · ตัวอย่างเครื่องเล่น'}
              </p>
            </div>
            <div className="rp-player-controls">
              <input
                type="range"
                min="0"
                max={video.durationSeconds}
                value={position}
                onChange={(event) => {
                  setStarted(true)
                  setPosition(Number(event.target.value))
                }}
                aria-label="ตำแหน่งวิดีโอตัวอย่าง"
                aria-valuetext={`${Math.floor(position / 60)} นาที ${position % 60} วินาที จาก ${Math.floor(video.durationSeconds / 60)} นาที ${video.durationSeconds % 60} วินาที`}
              />
              <div className="rp-control-row">
                <button
                  type="button"
                  onClick={togglePlayback}
                  aria-label={playing && !atEnd ? 'หยุดตัวอย่าง' : 'เล่นตัวอย่าง'}
                >
                  <Icon name={playing && !atEnd ? 'pause' : 'play'} />
                </button>
                <button
                  type="button"
                  onClick={() => setMuted(!muted)}
                  aria-label={muted ? 'เปิดเสียงตัวอย่าง' : 'ปิดเสียงตัวอย่าง'}
                  aria-pressed={muted}
                >
                  <Icon name={muted ? 'muted' : 'volume'} />
                </button>
                <span className="rp-time">
                  {time(position)} / {time(video.durationSeconds)}
                </span>
                <select
                  className="rp-quality"
                  aria-label="คุณภาพวิดีโอตัวอย่าง"
                  value={quality}
                  onChange={(event) => setQuality(event.target.value)}
                >
                  <option value="auto">อัตโนมัติ</option>
                  <option value="1080">1080p</option>
                  <option value="720">720p</option>
                  <option value="480">480p</option>
                </select>
              </div>
            </div>
          </>
        )}
      </section>
      <section className="rp-video-about">
        <div>
          {loading ? (
            <>
              <Skeleton shape="title" />
              <Skeleton shape="meta" />
              <Skeleton shape="line" />
            </>
          ) : (
            <>
              <h1>{video.title}</h1>
              <div className="rp-meta">
                <span>{video.category}</span>
                <span>·</span>
                <span>
                  {durationLabel(video.durationSeconds)}
                  {!video.processingStatus && ' (ตัวอย่าง)'}
                </span>
                <span className="rp-tag">HD</span>
              </div>
              <p className="rp-description">{video.description}</p>
            </>
          )}
        </div>
        {!loading && (
          <div className="rp-actions">
            <SaveButton saved={saved} onToggle={onToggle} />
          </div>
        )}
      </section>
      <VideoSection
        title="รับชมต่อเรื่องไหนดี"
        videos={videos.filter((item) => item.id !== video.id).slice(0, 4)}
        loading={loading}
      />
    </main>
  )
}
