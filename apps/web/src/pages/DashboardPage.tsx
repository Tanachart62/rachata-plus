import { useEffect, useState } from 'react'
import { Skeleton } from '../components/UI'
import { isObject, jsonRequest } from '../lib/http'

type Counts = Record<string, number>
interface DashboardData {
  asOf: number
  accounts: Counts
  videos: Counts
  queue: Counts
  jobs: { id: number; title: string; status: string; updatedAt: number }[]
  activities: { kind: string; label: string; detail: string; at: number }[]
}
const labels: Record<string, string> = {
  total: 'ทั้งหมด', members: 'สมาชิกที่ใช้งานได้', suspended: 'บัญชีถูกระงับ',
  pending: 'รอแปลง', processing: 'กำลังแปลง', ready: 'พร้อมรับชม', failed: 'แปลงไม่สำเร็จ',
  draft: 'ฉบับร่าง', published: 'เผยแพร่แล้ว', hidden: 'ซ่อน', deleted: 'ลบแล้ว',
  grant: 'ให้สิทธิ์สมาชิก', revoke: 'ยกเลิกสมาชิก', suspend: 'ระงับบัญชี', reactivate: 'เปิดคืนบัญชี',
  logout_all: 'ออกจากระบบทุกอุปกรณ์', logout_session: 'ยกเลิกเซสชัน', set_role: 'เปลี่ยนสิทธิ์บัญชี',
  reset_password: 'รีเซ็ตรหัสผ่าน', delete_account: 'ลบบัญชี',
}
const date = (value: number) => new Date(value).toLocaleString('th-TH')
const format = (value: number) => value.toLocaleString('th-TH')
const count = (value: unknown) => Number.isSafeInteger(value) && (value as number) >= 0
function readDashboard(value: unknown, actorId: number): DashboardData {
  if (!isObject(value) || value.actorId !== actorId || !count(value.asOf)) throw new Error('invalid_dashboard')
  for (const [group, keys] of [
    ['accounts', ['total', 'members', 'suspended']], ['videos', ['total', 'published', 'draft', 'hidden']],
    ['queue', ['pending', 'processing', 'ready', 'failed']],
  ] as const) {
    const values = value[group]
    if (!isObject(values) || keys.some(key => !count(values[key]))) throw new Error('invalid_dashboard')
  }
  if (!Array.isArray(value.jobs) || value.jobs.length > 10 || value.jobs.some(job => !isObject(job) ||
    !count(job.id) || typeof job.title !== 'string' || !['pending', 'processing', 'failed'].includes(job.status as string) || !count(job.updatedAt)) ||
    !Array.isArray(value.activities) || value.activities.length > 20 || value.activities.some(event => !isObject(event) ||
      !['account', 'video'].includes(event.kind as string) || typeof event.label !== 'string' || typeof event.detail !== 'string' || !count(event.at))) throw new Error('invalid_dashboard')
  return value as unknown as DashboardData
}
export function DashboardPage({ actorId }: { actorId: number }) {
  const [data, setData] = useState<DashboardData | null>(null)
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(false)
    jsonRequest('/auth/admin/dashboard', { signal: controller.signal }).then(value => {
      const result = readDashboard(value, actorId)
      if (!controller.signal.aborted) setData(result)
    }).catch(() => {
      if (!controller.signal.aborted) { setError(true); setData(null) }
    }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [actorId, revision])
  useEffect(() => {
    const timer = window.setInterval(() => { if (!document.hidden) setRevision(value => value + 1) }, 30000)
    return () => window.clearInterval(timer)
  }, [])
  function counters(values: Counts) {
    return <dl className="rp-info-grid">{Object.entries(values).map(([key, value]) => <div key={key}><dt>{labels[key]}</dt><dd>{format(value)}</dd></div>)}</dl>
  }
  return <main className="rp-page dashboard-page">
    <div className="admin-heading rp-page-heading"><div><h1>Dashboard แอดมิน</h1><p>ภาพรวมสมาชิก วิดีโอ และคิวแปลงจากฐานข้อมูล</p></div>
      <button type="button" className="rp-softbutton" disabled={loading} onClick={() => setRevision(value => value + 1)}>{loading ? 'กำลังโหลด…' : 'โหลดข้อมูลใหม่'}</button>
    </div>
    <p className="rp-sr" role="status">{loading ? 'กำลังโหลด Dashboard' : error ? 'โหลดข้อมูลไม่สำเร็จ' : 'อัปเดต Dashboard แล้ว'}</p>
    {error ? <div className="dashboard-error" role="alert"><strong>โหลดข้อมูล Dashboard ไม่สำเร็จ</strong><p>ตรวจการเชื่อมต่อและสิทธิ์แอดมิน แล้วกดโหลดข้อมูลใหม่</p></div> : !data ?
      <div className="admin-stat-grid" aria-label="กำลังโหลดข้อมูล">{[0, 1, 2, 3].map(id => <Skeleton key={id} shape="metric" />)}</div> : <>
      <p className="rp-demo-note">ข้อมูล ณ {date(data.asOf)} · อัปเดตทุก 30 วินาทีขณะเปิดหน้านี้</p>
      <div className="admin-stat-grid dashboard-metrics" aria-label="ภาพรวมระบบ">{[
        ['บัญชีทั้งหมด', data.accounts.total, 'ไม่รวมบัญชีที่ลบแล้ว'],
        ['สมาชิกที่ใช้งานได้', data.accounts.members, 'บัญชีปกติที่มีแพ็กเกจยังไม่หมดอายุ'],
        ['วิดีโอทั้งหมด', data.videos.total, 'ไม่รวมวิดีโอที่ลบแล้ว'],
        ['งานรอและกำลังแปลง', data.queue.pending + data.queue.processing, 'สถานะล่าสุดจากคิวจริง'],
      ].map(([label, value, note]) => <div className="admin-stat" key={label}><span>{label}</span><strong>{format(value as number)}</strong><small>{note}</small></div>)}</div>
      <section className="rp-surface dashboard-resources"><div className="dashboard-section-heading"><h2>สมาชิก</h2><a href="#/admin/members">จัดการสมาชิก →</a></div>{counters(data.accounts)}</section>
      <section className="rp-surface dashboard-resources"><div className="dashboard-section-heading"><h2>สถานะวิดีโอ</h2><a href="#/admin/videos">จัดการวิดีโอ →</a></div>{counters(data.videos)}</section>
      <section className="rp-surface dashboard-resources"><div className="dashboard-section-heading"><h2>คิวแปลงวิดีโอ</h2><a href="#/admin/videos">จัดการและสั่งแปลงใหม่ →</a></div>{counters(data.queue)}
        {data.jobs.length ? <div className="dashboard-table-wrap" tabIndex={0} role="region" aria-label="รายการงานแปลง"><table className="dashboard-live-table"><caption>แสดงสูงสุด 10 รายการ: กำลังแปลง ตามด้วยรอคิวและงานล้มเหลว</caption><thead><tr><th scope="col">วิดีโอ</th><th scope="col">สถานะ</th><th scope="col">อัปเดตล่าสุด</th></tr></thead><tbody>{data.jobs.map(job => <tr key={job.id}><td>{job.title}</td><td>{labels[job.status]}</td><td>{date(job.updatedAt)}</td></tr>)}</tbody></table></div> : <p>ไม่มีงานรอแปลง กำลังแปลง หรือแปลงล้มเหลว</p>}
      </section>
      <section className="rp-surface dashboard-resources"><div className="dashboard-section-heading"><div><h2>กิจกรรมล่าสุด</h2><p>คำสั่งบัญชีจากประวัติระบบ และการอัปเดตล่าสุดของวิดีโอแต่ละรายการ สูงสุด 20 รายการ</p></div></div>
        {data.activities.length ? <ul className="dashboard-activity-list">{data.activities.map((event, index) => <li key={`${event.kind}-${event.at}-${index}`}><div><strong>{event.kind === 'account' ? labels[event.label] ?? event.label : `อัปเดตวิดีโอ: ${event.label}`}</strong><p>{event.kind === 'account' ? event.detail : event.detail.split(':').map(value => labels[value] ?? value).join(' · ')}</p></div><time dateTime={new Date(event.at).toISOString()}>{date(event.at)}</time></li>)}</ul> : <p>ยังไม่มีกิจกรรมที่บันทึกไว้</p>}
      </section>
    </>}
  </main>
}
