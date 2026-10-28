import type { Video } from '../types'

export function Icon({ name }: { name: 'play' | 'pause' | 'plus' | 'check' | 'back' | 'upload' | 'user' | 'close' | 'volume' | 'muted' | 'search' | 'eye' | 'eye-off' }) {
  const shapes = {
    eye: <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></>,
    'eye-off': <><path d="m3 3 18 18M10.6 5.1 12 5c6.5 0 10 7 10 7a20 20 0 0 1-3 3.9M6.2 6.2A23 23 0 0 0 2 12s3.5 7 10 7a13 13 0 0 0 5.8-1.4M9.9 9.9a3 3 0 0 0 4.2 4.2" /></>,
    search: <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 5 5" /></>,
    play: <path d="m8 5 11 7-11 7Z" />,
    pause: <><path d="M8 5v14M16 5v14" /></>,
    plus: <path d="M12 5v14M5 12h14" />,
    check: <path d="m5 12 4 4L19 6" />,
    back: <path d="m10 5-7 7 7 7M3 12h18" />,
    upload: <path d="M12 16V3m-5 5 5-5 5 5M4 15v6h16v-6" />,
    user: <><circle cx="12" cy="8" r="4" /><path d="M4 21v-2a8 8 0 0 1 16 0v2" /></>,
    close: <path d="m6 6 12 12M6 18 18 6" />,
    volume: <><path d="m11 4-6 5H2v6h3l6 5ZM16 8a6 6 0 0 1 0 8m3-11a10 10 0 0 1 0 14" /></>,
    muted: <><path d="m11 4-6 5H2v6h3l6 5Zm5 5 6 6m-6 0 6-6" /></>,
  }
  return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{shapes[name]}</svg>
}

export function Skeleton({ shape }: { shape: string }) {
  return <div className={`rp-skeleton rp-sk-${shape}`} aria-hidden="true" />
}

export function VideoGrid({ videos, loading = false }: { videos: Video[]; loading?: boolean }) {
  return <div className="rp-grid">{videos.map(video => loading
    ? <div className="rp-sk-card" key={video.id}><Skeleton shape="cover" /><Skeleton shape="cardtitle" /><Skeleton shape="cardsub" /></div>
    : <a className="rp-video" key={video.id} href={`#/videos/${video.id}`} aria-label={`ดูรายละเอียด ${video.title}`}>
        <span className="rp-cover" aria-hidden="true">{video.coverUrl ? <img src={video.coverUrl} alt="" loading="lazy" /> : "พื้นที่ภาพปก"}<span className="rp-duration">—:—</span></span>
        <span className="rp-cardtitle">{video.title}</span><span className="rp-cardsub">{video.category} · สำหรับสมาชิก</span>
      </a>)}</div>
}

export function VideoSection({ title, videos, loading = false }: { title: string; videos: Video[]; loading?: boolean }) {
  return <section className="rp-section" aria-label={title}><div className="rp-sectionhead"><h2>{title}</h2><span className="rp-counter">{String(videos.length).padStart(2, '0')} รายการ</span></div><VideoGrid videos={videos} loading={loading} /></section>
}

export function SaveButton({ saved, onToggle }: { saved: boolean; onToggle: () => void }) {
  return <button type="button" className="rp-softbutton" onClick={onToggle} aria-pressed={saved}><Icon name={saved ? 'check' : 'plus'} />{saved ? 'บันทึกแล้ว' : 'ดูภายหลัง'}</button>
}

export function MessagePage({ title, message, actionHref = '#/', actionLabel = 'กลับหน้าแรก' }: { title: string; message: string; actionHref?: string; actionLabel?: string }) {
  return <main className="rp-login"><h1>{title}</h1><p>{message}</p><a className="rp-mainbutton rp-linkbutton" href={actionHref}>{actionLabel}</a></main>
}
