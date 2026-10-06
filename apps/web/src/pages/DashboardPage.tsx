import { useEffect, useRef, useState } from 'react'
import { Skeleton } from '../components/UI'
import type { Video } from '../types'

export function DashboardPage({ videos, loading, monitoringError, onRetry }: { videos: Video[]; loading: boolean; monitoringError: boolean; onRetry: () => void }) {
  const [days, setDays] = useState(7)
  const [retrying, setRetrying] = useState(false)
  const resourcesHeading = useRef<HTMLHeadingElement>(null)
  const [notice, setNotice] = useState('')
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  const daily = Array.from({ length: days }, (_, index) => ({ day: index + 1, views: 120 + (index * 73 + 41) % 340, minutes: 800 + (index * 137 + 59) % 1800 }))
  const views = daily.reduce((sum, day) => sum + day.views, 0)
  const minutes = daily.reduce((sum, day) => sum + day.minutes, 0)
  const maxViews = Math.max(...daily.map(day => day.views))
  const format = (value: number) => value.toLocaleString('th-TH')
  const topVideos = videos.filter(video => video.publicationStatus === 'published').slice(0, 5)
  function retry() {
    if (retrying) return
    setRetrying(true)
    timer.current = window.setTimeout(() => { onRetry(); setRetrying(false); setNotice('โหลดข้อมูลทรัพยากรตัวอย่างแล้ว'); if (document.activeElement?.id === 'monitoring-retry' || document.activeElement === document.body) requestAnimationFrame(() => resourcesHeading.current?.focus()) }, 700)
  }
  return <main className="rp-page dashboard-page">
    <div className="admin-heading rp-page-heading"><div><div className="rp-eyebrow" lang="en">Studio overview</div><h1>Dashboard แอดมิน</h1><p>ภาพรวมการรับชมและทรัพยากรของระบบ</p></div><label className="rp-field dashboard-range">ช่วงข้อมูลตัวอย่าง<select value={days} onChange={event => setDays(Number(event.target.value))}><option value={7}>7 วัน</option><option value={30}>30 วัน</option></select></label></div>
    <div className="dashboard-demo-banner"><span className="rp-pill">ข้อมูลจำลอง</span><p>ตัวเลขและกราฟใช้สำหรับออกแบบหน้าจอ ยังไม่ได้ดึงข้อมูลจากผู้ชมจริงหรือ AWS CloudWatch</p></div>
    <p className="rp-sr" role="status">{loading ? "" : `ข้อมูล ${days} วัน ยอดรับชม ${format(views)} ครั้ง`}</p><p className="rp-sr" role="status">{retrying ? "กำลังลองโหลดข้อมูลใหม่" : notice}</p>
    {loading ? <div className="dashboard-loading" aria-label="กำลังโหลด Dashboard"><div className="admin-stat-grid">{[0, 1, 2, 3].map(id => <Skeleton key={id} shape="metric" />)}</div><Skeleton shape="chart" /><Skeleton shape="chart" /></div> : <>
      <div className="admin-stat-grid dashboard-metrics" aria-label="สถิติการรับชมตัวอย่าง">{[
        ['ยอดรับชม', format(views), `รวม ${days} วัน · จำลอง`], ['เวลารับชม', format(Math.round(minutes / 60)), 'ชั่วโมง · จำลอง'], ['เวลาเฉลี่ยต่อการชม', (minutes / views).toFixed(1), 'นาที · จำลอง'], ['วิดีโอเผยแพร่', String(videos.filter(video => video.publicationStatus === 'published').length), 'จากคลังเดโม่ปัจจุบัน'],
      ].map(([label, value, note]) => <div className="admin-stat" key={label}><span>{label}</span><strong>{value}</strong><small>{note}</small></div>)}</div>
      <section className="rp-surface dashboard-chart"><div className="dashboard-section-heading"><div><h2>แนวโน้มการรับชม</h2><p>ยอดรับชมรายวัน · ข้อมูลตัวอย่าง {days} วัน</p></div><span className="chart-legend">จำนวนครั้ง</span></div>
        <div className="dashboard-bars" role="img" aria-label={`กราฟยอดรับชมตัวอย่าง ${days} วัน รวม ${views} ครั้ง รายละเอียดอยู่ในตารางด้านล่าง`}>
          {daily.map(day => <div className="dashboard-bar-column" key={day.day}><div className="dashboard-bar" style={{ height: `${day.views / maxViews * 100}%` }} title={`วันที่ ${day.day}: ${day.views} ครั้ง`} /></div>)}
        </div><div className="chart-axis"><span>วันที่ 1</span><span>วันที่ {days}</span></div>
        <details className="dashboard-data"><summary>ดูตัวเลขรายวัน</summary><div className="dashboard-table-wrap" role="region" aria-label="ตารางยอดรับชมรายวัน" tabIndex={0}><table><caption>ข้อมูลจำลองสำหรับกราฟ</caption><thead><tr><th scope="col">วัน</th><th scope="col">ยอดรับชม</th><th scope="col">เวลารับชม (นาที)</th></tr></thead><tbody>{daily.map(day => <tr key={day.day}><th scope="row">{day.day}</th><td>{format(day.views)}</td><td>{format(day.minutes)}</td></tr>)}</tbody></table></div></details>
      </section>
      <section className="rp-surface dashboard-resources"><div className="dashboard-section-heading"><div><h2 ref={resourcesHeading} tabIndex={-1}>ทรัพยากรระบบ</h2><p>ตัวอย่างค่าล่าสุดจาก Cloud Monitoring</p></div><span className="rp-pill">จำลอง</span></div>
        {monitoringError ? <div className="dashboard-error" role="status"><strong>ไม่สามารถดึงข้อมูลทรัพยากรคลาวด์ได้ในขณะนี้</strong><p>สถิติการรับชมด้านบนยังแสดงได้ ลองเชื่อมต่ออีกครั้ง</p><button id="monitoring-retry" type="button" className="rp-softbutton" onClick={retry} disabled={retrying}>{retrying ? 'กำลังลองใหม่…' : 'ลองใหม่'}</button></div> : <div className="resource-grid"><div><span>CPU</span><strong>32<small>%</small></strong><meter min={0} max={100} value={32} aria-label="CPU จำลอง 32 เปอร์เซ็นต์" /><p>การใช้งานเฉลี่ย · ตัวอย่าง</p></div><div><span>Bandwidth</span><strong>{days === 7 ? '86.4' : '370.2'}<small>GB</small></strong><p>ข้อมูลที่ส่งออกใน {days} วัน · ตัวอย่าง</p></div><div><span>Storage</span><strong>214<small>GB</small></strong><p>ไฟล์ต้นฉบับและ HLS · ตัวอย่าง</p></div></div>}
      </section>
      <section className="rp-surface dashboard-popular"><div className="dashboard-section-heading"><div><h2>วิดีโอยอดนิยม</h2><p>ลำดับและยอดชมสมมุติเพื่อแสดงรูปแบบ</p></div><a href="#/admin/videos">จัดการวิดีโอ →</a></div>{topVideos.length ? <ol>{topVideos.map((video, index) => <li key={video.id}><span className="dashboard-rank">{String(index + 1).padStart(2, '0')}</span><div><a href={`#/videos/${video.id}`}>{video.title}</a><small>{video.category}</small></div><strong>{format(Math.floor(views * [0.24, 0.19, 0.15, 0.11, 0.08][index]))}<small>ครั้ง · จำลอง</small></strong></li>)}</ol> : <p>ยังไม่มีวิดีโอที่เผยแพร่ในคลัง</p>}</section>
    </>}
  </main>
}
