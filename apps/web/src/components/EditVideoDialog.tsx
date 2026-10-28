import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import type { Video } from '../types'
import { editVideo } from '../lib/videos'

export function EditVideoDialog({ video, actorId, onClose, onSaved }: {
  video: Video; actorId: number; onClose: () => void; onSaved: (video: Video) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const firstInput = useRef<HTMLInputElement>(null)
  const controller = useRef<AbortController | null>(null)
  const saving = useRef(false)
  const [title, setTitle] = useState(video.title)
  const [description, setDescription] = useState(video.description)
  const [category, setCategory] = useState(video.category)
  const [cover, setCover] = useState<File | null>(null)
  const [preview, setPreview] = useState(video.coverUrl || '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    const element = dialog.current
    element?.showModal(); firstInput.current?.focus()
    return () => { controller.current?.abort(); element?.close() }
  }, [])
  useEffect(() => {
    if (!cover) { setPreview(video.coverUrl || ''); return }
    const url = URL.createObjectURL(cover); setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [cover, video.coverUrl])
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (saving.current) return
    saving.current = true; setBusy(true); setError('')
    const request = new AbortController(); controller.current = request
    try {
      const next = await editVideo(video, actorId, title.trim(), description.trim(), category, cover, request.signal)
      if (!request.signal.aborted) { onSaved(next); onClose() }
    } catch (failure) {
      if (!request.signal.aborted) setError(failure instanceof Error ? failure.message : 'บันทึกไม่สำเร็จ กรุณาลองใหม่')
    } finally { saving.current = false; if (!request.signal.aborted) setBusy(false) }
  }
  return <dialog ref={dialog} className="admin-delete-dialog admin-edit-dialog" aria-labelledby="edit-video-heading" onCancel={event => { event.preventDefault(); if (!saving.current) onClose() }}>
    <h2 id="edit-video-heading">แก้ไขข้อมูลวิดีโอ</h2>
    <p>แก้ข้อมูลและภาพปกได้โดยไม่ต้องอัปโหลดหรือแปลงวิดีโอใหม่</p>
    <form onSubmit={event => void save(event)} aria-busy={busy}>
      <label className="rp-field">ชื่อวิดีโอ<input ref={firstInput} required maxLength={120} value={title} disabled={busy} onChange={event => setTitle(event.target.value)} /></label>
      <label className="rp-field">คำอธิบาย<textarea maxLength={2000} value={description} disabled={busy} onChange={event => setDescription(event.target.value)} /></label>
      <label className="rp-field">หมวดหมู่<select value={category} disabled={busy} onChange={event => setCategory(event.target.value)}>{['บันเทิง', 'ความรู้', 'ไลฟ์สไตล์', 'อื่น ๆ'].map(value => <option key={value}>{value}</option>)}</select></label>
      <label className="rp-field">เปลี่ยนภาพปก (ไม่เลือกจะใช้ภาพเดิม)<input type="file" accept="image/jpeg,image/png" disabled={busy} onChange={event => {
        const file = event.target.files?.[0] ?? null
        if (file && (!['image/jpeg', 'image/png'].includes(file.type) || !file.size)) {
          setError('เลือก JPEG หรือ PNG ที่ไม่ว่าง'); event.target.value = ''; setCover(null); return
        }
        setError(''); setCover(file)
      }} /><small>JPEG หรือ PNG · แนะนำภาพแนวนอน</small></label>
      {preview && <img className="admin-edit-cover" src={preview} alt="ตัวอย่างภาพปก" />}
      <p role="alert">{error}</p>
      <div className="rp-actions"><button type="button" className="rp-softbutton" disabled={busy} onClick={onClose}>ยกเลิก</button><button className="rp-mainbutton" disabled={busy}>{busy ? 'กำลังบันทึก…' : 'บันทึกการแก้ไข'}</button></div>
    </form>
  </dialog>
}
