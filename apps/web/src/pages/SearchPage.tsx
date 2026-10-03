import { Icon, VideoGrid } from '../components/UI'
import type { Video } from '../types'

function normalize(value: string) {
  return value.normalize('NFKC').toLocaleLowerCase('th').trim().replace(/\s+/g, ' ')
}

export function SearchPage({ query, videos, loading }: { query: string; videos: Video[]; loading: boolean }) {
  const terms = normalize(query).split(' ').filter(Boolean)
  const results = terms.length ? videos.filter(video => {
    const text = normalize(`${video.title} ${video.category}`)
    return terms.every(term => text.includes(term))
  }) : []

  return (
    <main className="rp-page search-page">
      <div className="rp-page-heading">
        <div className="rp-eyebrow">Discover your next story</div>
        <h1>ผลการค้นหา</h1>
        <p className="search-result-label" role="status">
          {terms.length ? loading ? 'กำลังค้นหาวิดีโอ…' : <>พบ {results.length} รายการสำหรับ <strong>“{query.trim()}”</strong></> : 'ค้นหาเรื่องที่อยากดูจากช่องค้นหาด้านบน'}
        </p>
      </div>

      {terms.length && loading ? (
        <VideoGrid videos={videos.slice(0, 4)} loading />
      ) : results.length ? (
        <VideoGrid videos={results} />
      ) : (
        <section className="search-empty" aria-labelledby="search-empty-heading">
          <span className="search-empty-icon"><Icon name="search" /></span>
          <h2 id="search-empty-heading">{terms.length ? 'ยังไม่พบวิดีโอที่ตรงกัน' : 'เรื่องต่อไปที่อยากดูคืออะไร?'}</h2>
          <p>{terms.length ? 'ลองใช้คำสั้นลง ตรวจตัวสะกด หรือค้นหาด้วยหมวดหมู่' : 'พิมพ์ชื่อวิดีโอหรือหมวดหมู่ เช่น ความรู้ แล้วกด Enter หรือปุ่มค้นหา'}</p>
          <a className="rp-softbutton rp-linkbutton" href="#/videos">ดูวิดีโอทั้งหมด</a>
        </section>
      )}
    </main>
  )
}
