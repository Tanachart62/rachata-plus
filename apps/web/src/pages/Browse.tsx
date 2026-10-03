import { useState } from 'react'
import { Icon, SaveButton, Skeleton, VideoGrid, VideoSection } from '../components/UI'
import type { Video } from '../types'
import { TrailerPreview } from '../components/TrailerPreview'

function Hero({ video, loading, detail = false, saved = false, onToggle = () => {} }: { video: Video; loading: boolean; detail?: boolean; saved?: boolean; onToggle?: () => void }) {
  return <section className="rp-hero" aria-label={detail ? 'รายละเอียดวิดีโอ' : 'วิดีโอแนะนำ'}>
    <span className="rp-art-label">พื้นที่ภาพแบนเนอร์</span>
    <div className="rp-hero-copy">{loading ? <><Skeleton shape="short" /><Skeleton shape="title" /><Skeleton shape="meta" /><Skeleton shape="line" /><Skeleton shape="line2" /><div className="rp-actions"><Skeleton shape="button" /><Skeleton shape="button2" /></div></> : <>
      <div className="rp-eyebrow">{detail ? 'Rachata Plus · Video' : 'Featured on Rachata+'}</div>
      <h1>{detail ? video.title : 'ชื่อวิดีโอแนะนำ'}</h1>
      <div className="rp-meta"><span>{video.category}</span><span>·</span><span>12 นาที 30 วินาที (ตัวอย่าง)</span><span className="rp-tag">HD</span></div>
      <p className="rp-description">{video.description}</p>
      <div className="rp-actions"><a className="rp-mainbutton rp-linkbutton" href={`#/${detail ? 'watch' : 'videos'}/${video.id}`}><Icon name="play" />{detail ? 'รับชมวิดีโอ' : 'ดูรายละเอียด'}</a>{detail && <SaveButton saved={saved} onToggle={onToggle} />}</div>
      {detail && <p className="rp-access">รับชมวิดีโอเต็มสำหรับสมาชิก</p>}
    </>}</div>
    {!detail && <div className="rp-hero-index"><span>01</span> / 04</div>}
  </section>
}

export function HomePage({ videos, loading }: { videos: Video[]; loading: boolean }) {
  return <><Hero video={videos[0]} loading={loading} /><main className="rp-catalog"><VideoSection title="แนะนำสำหรับคุณ" videos={videos.slice(0, 4)} loading={loading} /><VideoSection title="เพิ่มเข้ามาใหม่" videos={videos.slice(-4).reverse()} loading={loading} /></main></>
}

export function CatalogPage({ videos, loading }: { videos: Video[]; loading: boolean }) {
  return <main className="rp-page"><div className="rp-page-heading"><div className="rp-eyebrow">Explore</div><h1>วิดีโอทั้งหมด</h1></div><VideoGrid videos={videos} loading={loading} /></main>
}

export function DetailPage({ video, videos, loading, saved, onToggle }: { video: Video; videos: Video[]; loading: boolean; saved: boolean; onToggle: () => void }) {
  const [tab, setTab] = useState<'suggested' | 'details'>('suggested')
  return <><a className="rp-back" href="#/"><Icon name="back" />กลับหน้าแรก</a><Hero video={video} loading={loading} detail saved={saved} onToggle={onToggle} /><main className="rp-catalog">
    {!loading && <TrailerPreview video={video} />}
    <div className="rp-tabs" role="tablist" aria-label="ข้อมูลวิดีโอ">{(['suggested', 'details'] as const).map(value => <button key={value} type="button" role="tab" id={`tab-${value}`} aria-selected={tab === value} aria-controls="detail-panel" onClick={() => setTab(value)} onKeyDown={event => { if (['ArrowLeft', 'ArrowRight'].includes(event.key)) { event.preventDefault(); const next = tab === 'suggested' ? 'details' : 'suggested'; setTab(next); document.getElementById(`tab-${next}`)?.focus() } }}>{value === 'suggested' ? 'วิดีโอที่คล้ายกัน' : 'รายละเอียด'}</button>)}</div>
    <section role="tabpanel" id="detail-panel" aria-labelledby={`tab-${tab}`}>{tab === 'suggested' ? <VideoGrid videos={videos.filter(item => item.id !== video.id).slice(0, 4)} loading={loading} /> : <div className="rp-panel"><h2>เกี่ยวกับวิดีโอนี้</h2>{loading ? <><Skeleton shape="line" /><Skeleton shape="line2" /></> : <><p>{video.description}</p><dl><dt>ชื่อเรื่อง</dt><dd>{video.title}</dd><dt>หมวดหมู่</dt><dd>{video.category}</dd><dt>ระยะเวลา</dt><dd>12 นาที 30 วินาที (ตัวอย่าง)</dd></dl></>}</div>}</section>
  </main></>
}
