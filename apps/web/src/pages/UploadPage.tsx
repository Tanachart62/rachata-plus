import { useEffect, useState } from 'react'
import { Icon } from '../components/UI'
import type { Video } from '../types'

type Phase = 'idle' | 'uploading' | 'processing' | 'ready'
const status: Record<Phase, string> = { idle: 'ยังไม่เผยแพร่', uploading: 'กำลังอัปโหลด', processing: 'กำลังเตรียมวิดีโอ', ready: 'พร้อมรับชม' }

export function UploadPage({ onCreate }: { onCreate: (video: Omit<Video, 'id'>) => number }) {
  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('')
  const [phase, setPhase] = useState<Phase>('idle')
  const [percent, setPercent] = useState(0)
  const [error, setError] = useState('')
  const [dragging, setDragging] = useState(false)
  const [createdId, setCreatedId] = useState<number | null>(null)
  const busy = phase === 'uploading' || phase === 'processing'

  // The timers simulate progress only. No file is sent to a server.
  useEffect(() => {
    if (phase !== 'uploading') return
    let progress = 0
    const timer = window.setInterval(() => {
      progress += 10
      setPercent(progress)
      if (progress >= 100) { window.clearInterval(timer); setPhase('processing') }
    }, 180)
    return () => window.clearInterval(timer)
  }, [phase])

  useEffect(() => {
    if (phase !== 'processing') return
    const timer = window.setTimeout(() => {
      setCreatedId(onCreate({ title: title.trim(), description: description.trim(), category, durationSeconds: 750 }))
      setPhase('ready')
    }, 900)
    return () => window.clearTimeout(timer)
  }, [phase, title, description, category, onCreate])

  function chooseFile(next: File | undefined) {
    if (!next || busy) return
    setError(''); setPhase('idle'); setPercent(0); setCreatedId(null)
    if (!/\.(mp4|webm|mov)$/i.test(next.name) || !next.size) {
      setFile(null); setError('เลือกไฟล์ MP4, WebM หรือ MOV ที่ไม่ใช่ไฟล์ว่าง'); return
    }
    setFile(next)
  }

  return <main className="rp-page"><div className="rp-crumb">สำหรับผู้ดูแลระบบ <span>/</span> วิดีโอ <span>/</span> อัปโหลด</div><div className="rp-page-heading"><div className="rp-eyebrow">Content studio</div><h1>อัปโหลดวิดีโอ</h1><p>เพิ่มเรื่องราวใหม่ให้สมาชิกของคุณ</p></div>
    <div className="rp-upload-layout"><form className="rp-surface" onSubmit={event => { event.preventDefault(); if (busy) return; if (!file) { setError('กรุณาเลือกไฟล์วิดีโอก่อน'); return } if (!title.trim()) { setError('กรุณาใส่ชื่อวิดีโอ'); return } setError(''); setPercent(0); setCreatedId(null); setPhase('uploading') }}>
      <fieldset disabled={busy || phase === 'ready'}><h2><span className="rp-section-number">01</span>ไฟล์วิดีโอ</h2>
        <label className="rp-drop" data-drag={dragging} onDragOver={event => { event.preventDefault(); if (!busy && phase !== 'ready') setDragging(true) }} onDragLeave={() => setDragging(false)} onDrop={event => { event.preventDefault(); setDragging(false); if (phase !== 'ready') chooseFile(event.dataTransfer.files[0]) }}><Icon name="upload" /><strong>{file ? 'เลือกไฟล์ใหม่' : 'ลากไฟล์วิดีโอมาวางที่นี่'}</strong><small>หรือคลิกเพื่อเลือกไฟล์ · MP4, WebM, MOV</small><input type="file" accept=".mp4,.webm,.mov" aria-label="เลือกไฟล์วิดีโอ" onChange={event => { chooseFile(event.target.files?.[0]); event.target.value = '' }} /></label>
        {file && <div className="rp-file-row"><div><strong>{file.name}</strong><small>{(file.size / 1024 / 1024).toFixed(1)} MB · พร้อมสำหรับตัวอย่าง</small></div><button type="button" aria-label="นำไฟล์ออก" onClick={() => { setFile(null); setError('') }}><Icon name="close" /></button></div>}
        <div className="rp-formsection"><h2><span className="rp-section-number">02</span>รายละเอียดวิดีโอ</h2>
          <label className="rp-field">ชื่อวิดีโอ *<input required maxLength={120} value={title} placeholder="ใส่ชื่อวิดีโอ" onChange={event => setTitle(event.target.value)} /></label>
          <label className="rp-field">คำอธิบาย<textarea maxLength={2000} value={description} placeholder="วิดีโอนี้เกี่ยวกับอะไร" onChange={event => setDescription(event.target.value)} /></label>
          <label className="rp-field">หมวดหมู่ *<select required value={category} onChange={event => setCategory(event.target.value)}><option value="">เลือกหมวดหมู่</option><option>บันเทิง</option><option>ความรู้</option><option>ไลฟ์สไตล์</option><option>อื่น ๆ</option></select></label>
        </div>
      </fieldset>
      {error && <p className="rp-error" role="alert">{error}</p>}
      <div className="rp-formfooter"><p>วิดีโอจะพร้อมรับชมหลังเตรียมไฟล์เสร็จ</p>{phase === 'ready' ? <button type="button" className="rp-softbutton" onClick={() => { setFile(null); setTitle(''); setDescription(''); setCategory(''); setPhase('idle'); setCreatedId(null); setPercent(0) }}>เพิ่มวิดีโออีกเรื่อง</button> : <button type="submit" className="rp-mainbutton" disabled={busy}><Icon name="upload" />จำลองการอัปโหลด</button>}</div><p className="rp-demo-note">แบบร่างนี้ไม่ได้ส่งไฟล์ออกจากเครื่อง · เปลี่ยนหน้าระหว่างอัปโหลดจะยกเลิกการจำลอง</p>
    </form><aside><section className="rp-surface" aria-label="ตัวอย่างการ์ดวิดีโอ"><h2>ตัวอย่างการแสดงผล</h2><div className="rp-cover">พื้นที่ภาพปก</div><div className="rp-preview-title">{title.trim() || 'ชื่อวิดีโอ'}</div><div className="rp-preview-meta">{category || 'สำหรับสมาชิก'} · ภาพปกยังว่าง</div><div className="rp-preview-status"><span>สถานะตัวอย่าง</span><span className={`rp-pill ${phase === 'ready' ? 'rp-positive' : ''}`} role="status">{status[phase]}</span></div></section>
      {phase !== 'idle' && <section className="rp-upload-status" aria-label="ความคืบหน้าการอัปโหลดตัวอย่าง"><div className="rp-progress-label"><strong>{status[phase]}</strong><span>{percent}%</span></div><div className="rp-progress" role="progressbar" aria-label="ความคืบหน้าตัวอย่าง" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><div className="rp-progress-fill" style={{ width: `${percent}%` }} /></div><p>{phase === 'ready' ? 'เพิ่มการ์ดตัวอย่างแล้ว ยังไม่มีวิดีโอถูกเผยแพร่จริง' : phase === 'processing' ? 'ตัวอย่าง: เตรียมคุณภาพวิดีโอสำหรับรับชม' : 'กำลังจำลองการอัปโหลดไฟล์'}</p>{phase === 'ready' ? <a className="rp-softbutton rp-linkbutton" href={`#/watch/${createdId}`}>ดูหน้ารับชม</a> : <button type="button" className="rp-softbutton" onClick={() => { setPhase('idle'); setPercent(0) }}>ยกเลิก</button>}</section>}
    </aside></div>
  </main>
}
