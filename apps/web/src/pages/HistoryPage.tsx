import { useMemo, useState } from 'react'
import { Icon, Skeleton, VideoGrid } from '../components/UI'
import type { Video, WatchHistoryEntry } from '../types'
import { apiAuthEnabled } from '../lib/auth'

export function HistoryPage({
  history,
  videos,
  loading,
  saved,
}: {
  history: WatchHistoryEntry[]
  videos: Video[]
  loading: boolean
  saved: Set<number>
}) {
  const [filter, setFilter] = useState('all')
  const savedVideos = videos.filter(video => saved.has(video.id))
  const available = useMemo(() => {
    const byId = new Map(videos.map((video) => [video.id, video]))
    return history
      .flatMap((entry) => {
        const video = byId.get(entry.videoId)
        return video ? [{ ...entry, video }] : []
      })
      .sort((a, b) => b.watchedAt - a.watchedAt)
  }, [history, videos])
  const results = available.filter(
    (entry) =>
      filter === 'all' ||
      (filter === 'complete'
        ? entry.video.durationSeconds > 0 && entry.position >= entry.video.durationSeconds
        : entry.position < entry.video.durationSeconds),
  )
  return (
    <main className="rp-page history-page">
      <div className="rp-page-heading">
        <div className="rp-eyebrow" lang="en">
          Pick up where you left off
        </div>
        <h1>ประวัติและดูภายหลัง</h1>
        <p>กลับไปดูเรื่องราวที่ค้างไว้ หรือเลือกวิดีโอที่บันทึกไว้ดูภายหลัง</p>
      </div>
      <div className="history-filters" role="group" aria-label="เลือกประวัติหรือดูภายหลัง">
        {[
          ['all', 'ทั้งหมด'],
          ['continue', 'ยังดูไม่จบ'],
          ['complete', 'ดูจบแล้ว'],
          ['saved', 'ดูภายหลัง'],
        ].map(([value, label]) => (
          <button
            type="button"
            key={value}
            aria-pressed={filter === value}
            onClick={() => setFilter(value)}
          >
            {label}
          </button>
        ))}
      </div>
      <p className="rp-sr" role="status">
        {loading ? '' : filter === 'saved' ? `ดูภายหลัง ${savedVideos.length} รายการ` : `พบประวัติ ${results.length} รายการ`}
      </p>
      {loading ? (
        <div className="history-grid" aria-label="กำลังโหลดประวัติ">
          {[0, 1, 2].map((id) => (
            <div key={id}>
              <Skeleton shape="cover" />
              <Skeleton shape="cardtitle" />
              <Skeleton shape="cardsub" />
            </div>
          ))}
        </div>
      ) : filter === 'saved' ? (
        savedVideos.length ? <VideoGrid videos={savedVideos} /> : <section className="history-empty">
          <h2>ยังไม่มีรายการดูภายหลัง</h2>
          <p>กดปุ่มดูภายหลังในหน้ารายละเอียดหรือหน้ารับชม เพื่อบันทึกวิดีโอไว้ที่นี่</p>
          <a href="#/videos" className="rp-softbutton rp-linkbutton">สำรวจวิดีโอ</a>
        </section>
      ) : results.length ? (
        <div className="history-grid">
          {results.map((entry) => {
            const complete =
              entry.video.durationSeconds > 0 && entry.position >= entry.video.durationSeconds
            const percent =
              entry.video.durationSeconds > 0
                ? Math.min(100, Math.round((entry.position / entry.video.durationSeconds) * 100))
                : 0
            return (
              <article className="history-card" key={entry.videoId}>
                <a
                  className="rp-cover"
                  href={`#/videos/${entry.videoId}`}
                  aria-label={`ดูรายละเอียด ${entry.video.title}`}
                >
                  {entry.video.coverUrl ? <img src={entry.video.coverUrl} alt="" loading="lazy" /> : <span aria-hidden="true">พื้นที่ภาพปก</span>}
                </a>
                <div className="history-card-body">
                  <div className="history-card-meta">
                    <span>{entry.video.category}</span>
                    <span>{complete ? 'ดูจบแล้ว' : 'กำลังรับชม'}</span>
                  </div>
                  <h2>{entry.video.title}</h2>
                  <div
                    className="rp-progress"
                    role="progressbar"
                    aria-label={`ความคืบหน้า ${entry.video.title}`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={percent}
                  >
                    <div className="rp-progress-fill" style={{ width: `${percent}%` }} />
                  </div>
                  <p className="history-position">
                    {Math.floor(entry.position / 60)}:{String(entry.position % 60).padStart(2, '0')}{' '}
                    / {Math.floor(entry.video.durationSeconds / 60)}:
                    {String(entry.video.durationSeconds % 60).padStart(2, '0')} · {percent}%
                  </p>
                  <p className="history-date">
                    ดูล่าสุด{' '}
                    {new Intl.DateTimeFormat('th-TH', {
                      dateStyle: 'medium',
                      timeStyle: 'short',
                    }).format(entry.watchedAt)}
                  </p>
                  <a
                    className="rp-mainbutton rp-linkbutton"
                    href={`#/watch/${entry.videoId}`}
                    aria-label={`${complete ? 'ดูอีกครั้ง' : 'ดูต่อ'} ${entry.video.title}`}
                  >
                    <Icon name="play" />
                    {complete ? 'ดูอีกครั้ง' : 'ดูต่อ'}
                  </a>
                </div>
              </article>
            )
          })}
        </div>
      ) : (
        <section className="history-empty">
          <span className="history-empty-icon">
            <Icon name="play" />
          </span>
          <h2>
            {available.length ? 'ไม่มีรายการในหมวดนี้' : 'คุณยังไม่มีประวัติการรับชมภาพยนตร์'}
          </h2>
          <p>
            {available.length
              ? 'ลองเลือกตัวกรองอื่นเพื่อดูรายการของคุณ'
              : 'เมื่อเริ่มเล่นวิดีโอ ประวัติและตำแหน่งที่ดูจะปรากฏที่นี่'}
          </p>
          <a href="#/videos" className="rp-softbutton rp-linkbutton">
            สำรวจวิดีโอ
          </a>
        </section>
      )}
      <p className="rp-demo-note">
        {apiAuthEnabled
          ? 'ประวัติและดูภายหลังบันทึกแยกตามบัญชี ใช้ได้หลังรีเฟรชหรือเข้าสู่ระบบจากอุปกรณ์อื่น'
          : 'ประวัติและดูภายหลังในโหมดจำลองจะหายเมื่อรีเฟรช'}{' '}
        · แสดงเฉพาะวิดีโอที่ยังเผยแพร่
      </p>
    </main>
  )
}
