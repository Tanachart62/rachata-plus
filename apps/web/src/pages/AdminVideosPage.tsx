import { useEffect, useRef, useState } from 'react'
import { Icon, Skeleton } from '../components/UI'
import type { PublicationStatus, Video } from '../types'
import { apiVideosEnabled } from '../lib/videos'

const labels: Record<PublicationStatus, string> = { draft: 'ฉบับร่าง', published: 'เผยแพร่แล้ว', hidden: 'ซ่อน' }

function DeleteVideoDialog({ video, onCancel, onConfirm }: { video: Video; onCancel: () => void; onConfirm: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancel = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    const element = dialog.current
    element?.showModal()
    cancel.current?.focus()
    return () => element?.close()
  }, [])
  return <dialog ref={dialog} className="admin-delete-dialog" aria-labelledby="delete-title" aria-describedby="delete-description" onCancel={event => { event.preventDefault(); onCancel() }} onKeyDown={event => {
    if (event.key !== 'Tab') return
    const buttons = dialog.current?.querySelectorAll<HTMLButtonElement>('button')
    if (!buttons?.length) return
    const first = buttons[0], last = buttons[buttons.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
  }}>
    <div className="rp-eyebrow" lang="en">Delete video</div>
    <h2 id="delete-title">ลบวิดีโอนี้?</h2>
    <p className="delete-video-name">{video.title}</p>
    <p id="delete-description">วิดีโอจะถูกนำออกจากรายการและดูภายหลัง{apiVideosEnabled ? ' และลบไฟล์ในเครื่อง การลบนี้ไม่สามารถย้อนกลับได้' : ' การลบในรอบเดโม่นี้ไม่สามารถย้อนกลับได้'}</p>
    <div className="rp-actions"><button ref={cancel} type="button" className="rp-softbutton" onClick={onCancel}>ยกเลิก</button><button type="button" className="rp-mainbutton" onClick={onConfirm}>ยืนยันลบวิดีโอ</button></div>
  </dialog>
}

export function AdminVideosPage({ videos, loading, onStatusChange, onDelete, onRetry }: {
  videos: Video[]; loading: boolean;
  onStatusChange: (id: number, status: PublicationStatus) => void | Promise<void>;
  onDelete: (id: number) => void | Promise<void>;
  onRetry: (id: number) => Promise<void>;
}) {
  const [filter, setFilter] = useState<PublicationStatus | 'all'>('all')
  const [query, setQuery] = useState('')
  const [deleting, setDeleting] = useState<Video | null>(null)
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const deleteTrigger = useRef<HTMLButtonElement | null>(null)
  const term = query.normalize('NFKC').trim().toLocaleLowerCase('th')
  const results = videos.filter(video => (filter === 'all' || video.publicationStatus === filter) && `${video.title} ${video.category}`.normalize('NFKC').toLocaleLowerCase('th').includes(term))
  async function changeStatus(video: Video, status: PublicationStatus) {
    if (busy) return
    setBusy(true)
    try {
      await onStatusChange(video.id, status)
      setNotice(`${video.title} · ${labels[status]}`)
      if (filter !== 'all' && filter !== status) requestAnimationFrame(() => heading.current?.focus())
    } catch (error) { setNotice(error instanceof Error ? error.message : 'เปลี่ยนสถานะไม่สำเร็จ') }
    finally { setBusy(false) }
  }
  return <main className="rp-page admin-videos">
    <div className="admin-heading rp-page-heading"><div><div className="rp-eyebrow" lang="en">Content studio</div><h1 ref={heading} tabIndex={-1}>จัดการวิดีโอ</h1><p>ดูแลรายการและเลือกวิดีโอที่จะแสดงให้สมาชิกเห็น</p></div><a href="#/admin/upload" className="rp-mainbutton rp-linkbutton"><Icon name="upload" />อัปโหลดวิดีโอ</a></div>
    <div className="admin-stat-grid" aria-label="สรุปวิดีโอ">
      {(['all', 'published', 'hidden', 'draft'] as const).map(value => <div className="admin-stat" key={value}><span>{value === 'all' ? 'วิดีโอทั้งหมด' : labels[value]}</span><strong>{loading ? '—' : value === 'all' ? videos.length : videos.filter(video => video.publicationStatus === value).length}</strong></div>)}
    </div>
    <section className="rp-surface admin-library" aria-label="คลังวิดีโอ">
      <div className="admin-toolbar"><label className="rp-field">ค้นหาในคลัง<input id="admin-search" type="search" placeholder="ชื่อวิดีโอ หรือหมวดหมู่" value={query} maxLength={120} onChange={event => setQuery(event.target.value)} /></label><label className="rp-field">สถานะ<select value={filter} onChange={event => setFilter(event.target.value as typeof filter)}><option value="all">ทุกสถานะ</option><option value="published">เผยแพร่แล้ว</option><option value="hidden">ซ่อน</option><option value="draft">ฉบับร่าง</option></select></label></div>
      <p className="admin-notice" role="status">{notice}</p><p className="rp-sr" role="status">{loading ? "" : `พบ ${results.length} รายการ`}</p>
      {loading ? <div aria-label="กำลังโหลดคลังวิดีโอ">{[0, 1, 2].map(id => <div className="admin-row" key={id}><Skeleton shape="admin-row" /></div>)}</div> : results.length ? <>
        <p className="admin-result-count">{results.length} รายการ</p>
        <ul className="admin-video-list">{results.map(video => <li className="admin-row" key={video.id}>
          <div className="admin-cover" aria-hidden="true"><Icon name="play" /></div>
          <div className="admin-video-info"><h2>{video.title}</h2><span>{video.category}</span><span className={`admin-status status-${video.publicationStatus}`}>{labels[video.publicationStatus]}</span>{video.processingStatus && <span>{({ pending: 'รอแปลงไฟล์', processing: 'กำลังแปลง HLS', ready: 'พร้อมรับชม', failed: 'แปลงไม่สำเร็จ' })[video.processingStatus]}</span>}{video.processingError && <span className="auth-error">{video.processingError}</span>}</div>
          <div className="admin-row-actions"><a href={`#/videos/${video.id}`} className="rp-softbutton rp-linkbutton" aria-label={`ดูรายละเอียด ${video.title}`}>ดูรายละเอียด</a>{video.processingStatus === 'failed' && <button type="button" className="rp-softbutton" disabled={busy} onClick={async () => { setBusy(true); try { await onRetry(video.id); setNotice('ส่งวิดีโอเข้าคิวใหม่แล้ว') } catch { setNotice('ส่งเข้าคิวไม่สำเร็จ กรุณาลองใหม่') } finally { setBusy(false) } }}>ลองแปลงใหม่</button>}{video.publicationStatus === 'published' ? <button type="button" className="rp-softbutton" disabled={busy} aria-label={`ซ่อน ${video.title}`} onClick={() => void changeStatus(video, 'hidden')}>ซ่อน</button> : <button type="button" className="rp-softbutton" disabled={busy || (apiVideosEnabled && video.processingStatus !== 'ready')} aria-label={`เผยแพร่ ${video.title}`} onClick={() => void changeStatus(video, 'published')}>เผยแพร่</button>}<button type="button" className="admin-delete-button" disabled={busy} aria-label={`ลบ ${video.title}`} onClick={event => { deleteTrigger.current = event.currentTarget; setDeleting(video) }}>ลบ</button></div>
        </li>)}</ul>
      </> : <div className="admin-empty"><h2>{videos.length ? 'ไม่พบวิดีโอที่ตรงกัน' : 'ยังไม่มีวิดีโอในคลัง'}</h2><p>{videos.length ? 'ลองเปลี่ยนคำค้นหรือเลือกทุกสถานะ' : 'เริ่มจากอัปโหลดวิดีโอ แล้วเผยแพร่เมื่อพร้อม'}</p>{videos.length ? <button className="rp-softbutton" type="button" onClick={() => { setQuery(''); setFilter('all'); requestAnimationFrame(() => document.getElementById('admin-search')?.focus()) }}>ล้างตัวกรอง</button> : <a className="rp-softbutton rp-linkbutton" href="#/admin/upload">อัปโหลดวิดีโอแรก</a>}</div>}
    </section>
    <p className="rp-demo-note">{apiVideosEnabled ? 'บันทึกข้อมูลในฐานข้อมูลบนเครื่อง เผยแพร่ได้เมื่อแปลงเสร็จแล้ว' : 'การเปลี่ยนสถานะและลบเป็นการจำลอง ข้อมูลจะกลับค่าเริ่มต้นเมื่อรีเฟรชหน้า'}</p>
    {deleting && <DeleteVideoDialog video={deleting} onCancel={() => { if (busy) return; setDeleting(null); requestAnimationFrame(() => deleteTrigger.current?.focus()) }} onConfirm={async () => {
      if (busy) return
      setBusy(true)
      try { await onDelete(deleting.id); setNotice(`ลบ ${deleting.title} แล้ว`); setDeleting(null); requestAnimationFrame(() => heading.current?.focus()) }
      catch (error) { setNotice(error instanceof Error ? error.message : 'ลบไม่สำเร็จ'); setDeleting(null); requestAnimationFrame(() => deleteTrigger.current?.focus()) }
      finally { setBusy(false) }
    }} />}
  </main>
}
