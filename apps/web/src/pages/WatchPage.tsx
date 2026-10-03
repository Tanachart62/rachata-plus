import { useState } from 'react'
import { Icon, SaveButton, Skeleton, VideoSection } from '../components/UI'
import type { Video, Viewer } from '../types'

function time(seconds: number) {
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}

export function WatchPage({ video, videos, viewer, loading, saved, onToggle }: { video: Video; videos: Video[]; viewer: Viewer; loading: boolean; saved: boolean; onToggle: () => void }) {
  const [playing, setPlaying] = useState(false)
  const [muted, setMuted] = useState(false)
  const [position, setPosition] = useState(0)
  const [quality, setQuality] = useState('auto')
  const locked = viewer === 'guest' || viewer === 'user'
  return <main className="rp-watch"><div className="rp-watch-top"><a href={`#/videos/${video.id}`}><Icon name="back" />กลับหน้ารายละเอียด</a><span className="rp-counter">ตัวอย่างเครื่องเล่น</span></div>
    <section className="rp-player" aria-label="เครื่องเล่นวิดีโอตัวอย่าง" aria-busy={loading}>{loading ? <Skeleton shape="player" /> : locked ? <div className="rp-player-center"><strong>วิดีโอนี้สำหรับสมาชิก</strong><p>{viewer === 'guest' ? 'เข้าสู่ระบบด้วยบัญชีสมาชิกเพื่อรับชม' : 'บัญชีทั่วไปยังไม่มีแพ็กเกจสมาชิกสำหรับรับชม'}</p><a href={viewer === 'guest' ? '#/login' : '#/profile'} className="rp-mainbutton rp-linkbutton">{viewer === 'guest' ? 'เข้าสู่ระบบ' : 'ดูข้อมูลสมาชิก'}</a>{viewer === 'guest' && <a className="auth-inline-link" href="#/register">ยังไม่มีบัญชี? สร้างบัญชี</a>}</div> : <>
      <div className="rp-player-center"><button type="button" onClick={() => setPlaying(!playing)} aria-label={playing ? 'หยุดตัวอย่างเครื่องเล่น' : 'เล่นตัวอย่างเครื่องเล่น'} aria-pressed={playing}><Icon name={playing ? 'pause' : 'play'} /></button><strong>พื้นที่เล่นวิดีโอ</strong><p role="status">{playing ? 'ตัวอย่างสถานะ: กำลังเล่น' : 'ยังไม่มีไฟล์วิดีโอ · ตัวอย่างเครื่องเล่น'}</p></div>
      <div className="rp-player-controls"><input type="range" min="0" max={video.durationSeconds} value={position} onChange={event => setPosition(Number(event.target.value))} aria-label="ตำแหน่งวิดีโอตัวอย่าง" aria-valuetext={time(position)} /><div className="rp-control-row">
        <button type="button" onClick={() => setPlaying(!playing)} aria-label={playing ? 'หยุดตัวอย่าง' : 'เล่นตัวอย่าง'}><Icon name={playing ? 'pause' : 'play'} /></button>
        <button type="button" onClick={() => setMuted(!muted)} aria-label={muted ? 'เปิดเสียงตัวอย่าง' : 'ปิดเสียงตัวอย่าง'} aria-pressed={muted}><Icon name={muted ? 'muted' : 'volume'} /></button>
        <span className="rp-time">{time(position)} / {time(video.durationSeconds)}</span>
        <select className="rp-quality" aria-label="คุณภาพวิดีโอตัวอย่าง" value={quality} onChange={event => setQuality(event.target.value)}><option value="auto">อัตโนมัติ</option><option value="1080">1080p</option><option value="720">720p</option><option value="480">480p</option></select>
      </div></div>
    </>}</section>
    <section className="rp-video-about"><div>{loading ? <><Skeleton shape="title" /><Skeleton shape="meta" /><Skeleton shape="line" /></> : <><h1>{video.title}</h1><div className="rp-meta"><span>{video.category}</span><span>·</span><span>12 นาที 30 วินาที (ตัวอย่าง)</span><span className="rp-tag">HD</span></div><p className="rp-description">{video.description}</p></>}</div>{!loading && <div className="rp-actions"><SaveButton saved={saved} onToggle={onToggle} /></div>}</section>
    <VideoSection title="รับชมต่อเรื่องไหนดี" videos={videos.filter(item => item.id !== video.id).slice(0, 4)} loading={loading} />
  </main>
}
