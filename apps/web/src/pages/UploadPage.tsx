import { useEffect, useRef, useState } from 'react'
import { Icon } from '../components/UI'
import type { Video } from '../types'
import { apiVideosEnabled, getVideo, uploadVideo } from '../lib/videos'

type Phase = 'idle' | 'uploading' | 'processing' | 'ready' | 'failed'
const status: Record<Phase, string> = { idle: 'ยังไม่เผยแพร่', uploading: 'กำลังอัปโหลด', processing: 'กำลังแปลงเป็น HLS', ready: 'ฉบับร่าง', failed: 'ไม่สำเร็จ' }

export function UploadPage({ onCreate, onUploaded }: { onCreate: (video: Omit<Video, 'id'>) => number; onUploaded: (video: Video) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('')
  const [phase, setPhase] = useState<Phase>('idle')
  const [percent, setPercent] = useState(0)
  const [errors, setErrors] = useState<{ file?: string; title?: string; category?: string }>({})
  function focusField(field: string) { requestAnimationFrame(() => document.getElementById(`upload-${field}`)?.focus()) }
  const [dragging, setDragging] = useState(false)
  const [createdId, setCreatedId] = useState<number | null>(null)
  const [serverError, setServerError] = useState('')
  const uploadController = useRef<AbortController | null>(null)
  const busy = phase === 'uploading' || phase === 'processing'

  // Demo mode keeps the original simulation; API mode sends the actual file.
  useEffect(() => {
    if (phase !== 'uploading') return
    if (apiVideosEnabled) {
      if (!file) return
      const controller = new AbortController()
      uploadController.current = controller
      uploadVideo(file, title.trim(), description.trim(), category, setPercent, controller.signal).then(video => {
        if (controller.signal.aborted) return
        onUploaded(video); setCreatedId(video.id); setPercent(100); setPhase('processing')
      }).catch(error => {
        if (!controller.signal.aborted) { setServerError(error instanceof Error ? error.message : 'อัปโหลดไม่สำเร็จ'); setPhase('failed') }
      })
      return () => controller.abort()
    }
    let progress = 0
    const timer = window.setInterval(() => {
      progress += 10
      setPercent(progress)
      if (progress >= 100) { window.clearInterval(timer); setPhase('processing') }
    }, 180)
    return () => window.clearInterval(timer)
  }, [phase, file, title, description, category, onUploaded])

  useEffect(() => {
    if (phase !== 'processing') return
    if (apiVideosEnabled) {
      if (createdId === null) return
      const controller = new AbortController()
      let timer: number | undefined
      const poll = async () => {
        try {
          const video = await getVideo(createdId, controller.signal)
          if (controller.signal.aborted) return
          onUploaded(video)
          if (video.processingStatus === 'ready') {
            setPhase('ready'); requestAnimationFrame(() => document.getElementById('upload-done')?.focus()); return
          }
          if (video.processingStatus === 'failed') { setServerError(video.processingError || 'แปลงไฟล์ไม่สำเร็จ'); setPhase('failed'); return }
          timer = window.setTimeout(() => void poll(), 1500)
        } catch {
          if (!controller.signal.aborted) { setServerError('ตรวจสถานะไม่สำเร็จ เปิดคลังวิดีโอเพื่อดูความคืบหน้า'); setPhase('failed') }
        }
      }
      void poll()
      return () => { controller.abort(); window.clearTimeout(timer) }
    }
    const timer = window.setTimeout(() => {
      setCreatedId(onCreate({ title: title.trim(), description: description.trim(), category, durationSeconds: 750, publicationStatus: 'draft' }))
      setPhase('ready')
      if (document.activeElement?.id === 'upload-cancel') requestAnimationFrame(() => document.getElementById('upload-done')?.focus())
    }, 900)
    return () => window.clearTimeout(timer)
  }, [phase, createdId, title, description, category, onCreate, onUploaded])

  function chooseFile(next: File | undefined) {
    if (!next || busy) return
    setErrors({}); setPhase('idle'); setPercent(0); setCreatedId(null)
    if (!/\.(mp4|webm|mov)$/i.test(next.name) || !next.size || (apiVideosEnabled && next.size > 100 * 1024 * 1024)) {
      setFile(null); setErrors({ file: 'เลือกไฟล์ MP4, WebM หรือ MOV ที่ไม่ว่าง และขนาดไม่เกิน 100 MB' }); focusField('file'); return
    }
    setFile(next)
  }

  return <main className="rp-page"><div className="rp-crumb">สำหรับผู้ดูแลระบบ <span>/</span> <a href="#/admin/videos">จัดการวิดีโอ</a> <span>/</span> อัปโหลด</div><div className="rp-page-heading"><div className="rp-eyebrow" lang="en">Content studio</div><h1>อัปโหลดวิดีโอ</h1><p>บันทึกเป็นฉบับร่าง แล้วเผยแพร่เมื่อพร้อมในหน้าจัดการวิดีโอ</p></div>
    <div className="rp-upload-layout"><form className="rp-surface" noValidate onSubmit={event => {
      event.preventDefault()
      if (busy || phase === 'ready') return
      const issues: typeof errors = {}
      if (!file) issues.file = 'กรุณาเลือกไฟล์วิดีโอ MP4, WebM หรือ MOV'
      if (!title.trim()) issues.title = 'กรุณาใส่ชื่อวิดีโอ'
      if (!category) issues.category = 'กรุณาเลือกหมวดหมู่'
      setErrors(issues)
      const first = Object.keys(issues)[0]
      if (first) { focusField(first); return }
      setPercent(0); setCreatedId(null); setPhase('uploading')
      setServerError('')
      requestAnimationFrame(() => document.getElementById('upload-cancel')?.focus())
    }}>
      <p className="rp-demo-note">ช่องที่มี * จำเป็นต้องกรอก · เลือกไฟล์ได้ด้วยคีย์บอร์ดหรือการลากวาง</p>
      <fieldset disabled={busy || phase === 'ready'}><legend className="rp-sr">ข้อมูลวิดีโอใหม่</legend><h2><span className="rp-section-number">01</span>ไฟล์วิดีโอ</h2>
        <label className="rp-drop" data-drag={dragging} onDragOver={event => { event.preventDefault(); if (!busy && phase !== 'ready') setDragging(true) }} onDragLeave={() => setDragging(false)} onDrop={event => { event.preventDefault(); setDragging(false); if (phase !== 'ready') chooseFile(event.dataTransfer.files[0]) }}><Icon name="upload" /><strong id="upload-file-label">{file ? 'เลือกไฟล์วิดีโอใหม่ *' : 'เลือกไฟล์วิดีโอ *'}</strong><small id="upload-file-hint">คลิกหรือลากไฟล์มาวาง · MP4, WebM, MOV</small><input id="upload-file" type="file" accept=".mp4,.webm,.mov" aria-required="true" aria-labelledby="upload-file-label" aria-invalid={!!errors.file} aria-describedby={`upload-file-hint${errors.file ? " upload-file-error" : ""}`} onChange={event => { chooseFile(event.target.files?.[0]); event.target.value = '' }} /></label>
        {errors.file && <p id="upload-file-error" className="auth-error">{errors.file}</p>}
        {file && <div className="rp-file-row"><div><strong>{file.name}</strong><small>{(file.size / 1024 / 1024).toFixed(1)} MB · {apiVideosEnabled ? 'พร้อมอัปโหลด' : 'พร้อมสำหรับตัวอย่าง'}</small></div><button type="button" aria-label="นำไฟล์ออก" onClick={() => { setFile(null); setErrors({}); focusField('file') }}><Icon name="close" /></button></div>}
        <div className="rp-formsection"><h2><span className="rp-section-number">02</span>รายละเอียดวิดีโอ</h2>
          <label className="rp-field">ชื่อวิดีโอ *<input id="upload-title" required aria-invalid={!!errors.title} aria-describedby={errors.title ? "upload-title-error" : undefined} maxLength={120} value={title} placeholder="ใส่ชื่อวิดีโอ" onChange={event => setTitle(event.target.value)} />{errors.title && <span id="upload-title-error" className="auth-error">{errors.title}</span>}</label>
          <label className="rp-field">คำอธิบาย<textarea maxLength={2000} value={description} placeholder="วิดีโอนี้เกี่ยวกับอะไร" onChange={event => setDescription(event.target.value)} /></label>
          <label className="rp-field">หมวดหมู่ *<select id="upload-category" required aria-invalid={!!errors.category} aria-describedby={errors.category ? "upload-category-error" : undefined} value={category} onChange={event => setCategory(event.target.value)}><option value="">เลือกหมวดหมู่</option><option>บันเทิง</option><option>ความรู้</option><option>ไลฟ์สไตล์</option><option>อื่น ๆ</option></select>{errors.category && <span id="upload-category-error" className="auth-error">{errors.category}</span>}</label>
        </div>
      </fieldset>
      <p className="rp-sr" role="alert">{Object.keys(errors).length ? `กรุณาตรวจสอบ ${Object.keys(errors).length} ช่องที่มีข้อผิดพลาด` : ''}</p>
      <div className="rp-formfooter"><p>เมื่อเตรียมไฟล์เสร็จจะบันทึกเป็นฉบับร่าง</p>{phase === 'ready' ? <button type="button" className="rp-softbutton" onClick={() => { setFile(null); setTitle(''); setDescription(''); setCategory(''); setPhase('idle'); setCreatedId(null); setPercent(0); setErrors({}); setServerError(''); focusField('file') }}>เพิ่มวิดีโออีกเรื่อง</button> : <button type="submit" className="rp-mainbutton" disabled={busy || createdId !== null}><Icon name="upload" />{apiVideosEnabled ? 'อัปโหลดวิดีโอ' : 'จำลองการอัปโหลด'}</button>}</div><p className="rp-demo-note">{apiVideosEnabled ? 'ไฟล์ไม่เกิน 100 MB · เก็บและแปลงในเครื่อง หลังส่งสำเร็จจะทำงานต่อแม้เปลี่ยนหน้า ตรวจสถานะได้ในคลังวิดีโอ' : 'แบบร่างนี้ไม่ได้ส่งไฟล์ออกจากเครื่อง · เปลี่ยนหน้าระหว่างอัปโหลดจะยกเลิกการจำลอง'}</p><p className={serverError ? 'auth-error' : 'rp-sr'} role="alert">{serverError}</p>{phase === 'failed' && createdId !== null && <a className="rp-softbutton rp-linkbutton" href="#/admin/videos">เปิดคลังวิดีโอเพื่อตรวจสถานะหรือลองแปลงใหม่</a>}
    </form><aside><section className="rp-surface" aria-label="ตัวอย่างการ์ดวิดีโอ"><h2>ตัวอย่างการแสดงผล</h2><div className="rp-cover">พื้นที่ภาพปก</div><div className="rp-preview-title">{title.trim() || 'ชื่อวิดีโอ'}</div><div className="rp-preview-meta">{category || 'สำหรับสมาชิก'} · ภาพปกยังว่าง</div><div className="rp-preview-status"><span>{apiVideosEnabled ? 'สถานะวิดีโอ' : 'สถานะตัวอย่าง'}</span><span className={`rp-pill ${phase === 'ready' ? 'rp-positive' : ''}`} role="status">{phase === "ready" ? "บันทึกฉบับร่างแล้ว" : status[phase]}</span></div></section>
      {phase !== 'idle' && <section className="rp-upload-status" aria-label="ความคืบหน้าการอัปโหลด"><div className="rp-progress-label"><strong role="status">{status[phase]}</strong><span>{percent}%</span></div><div className="rp-progress" role="progressbar" aria-label="ความคืบหน้าการส่งไฟล์" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><div className="rp-progress-fill" style={{ width: `${percent}%` }} /></div><p>{phase === 'ready' ? 'บันทึกฉบับร่างแล้ว ไปที่จัดการวิดีโอเพื่อเผยแพร่' : phase === 'processing' ? 'กำลังเตรียม HLS สำหรับรับชม เปอร์เซ็นต์ด้านบนเป็นความคืบหน้าการส่งไฟล์' : phase === 'failed' ? 'ตรวจข้อความข้อผิดพลาดแล้วลองใหม่' : apiVideosEnabled ? 'กำลังส่งไฟล์ไปยัง API ในเครื่อง' : 'กำลังจำลองการอัปโหลดไฟล์'}</p>{phase === 'ready' ? <div className="rp-actions"><a id="upload-done" className="rp-mainbutton rp-linkbutton" href="#/admin/videos">จัดการวิดีโอ</a><a className="rp-softbutton rp-linkbutton" href={`#/videos/${createdId}`}>ดูตัวอย่างฉบับร่าง</a></div> : <button id="upload-cancel" type="button" className="rp-softbutton" onClick={() => { if (apiVideosEnabled && createdId !== null) { window.location.hash = '/admin/videos'; return } uploadController.current?.abort(); setPhase('idle'); setPercent(0); focusField('file') }}>{apiVideosEnabled && createdId !== null ? 'กลับไปคลังวิดีโอ' : 'ยกเลิก'}</button>}</section>}
    </aside></div>
  </main>
}
