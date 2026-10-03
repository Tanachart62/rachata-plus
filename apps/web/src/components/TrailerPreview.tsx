import { useState } from 'react'
import { Icon } from './UI'
import type { Video } from '../types'

export function TrailerPreview({ video }: { video: Video }) {
  const [open, setOpen] = useState(false)
  const [playing, setPlaying] = useState(false)
  const id = `trailer-${video.id}`

  return (
    <section className="trailer-preview" aria-label="ตัวอย่างวิดีโอที่ดูฟรี">
      <div className="trailer-heading">
        <div><h2>ลองดูก่อนเริ่มเรื่อง</h2><p>ตัวอย่างวิดีโอดูได้ฟรี ไม่ต้องเลือกแพ็กเกจ</p></div>
        <button type="button" className="rp-softbutton rp-linkbutton" aria-expanded={open} aria-controls={id} onClick={() => { setOpen(!open); setPlaying(false) }}>
          <Icon name={open ? 'close' : 'play'} />{open ? 'ปิดตัวอย่าง' : 'ดูตัวอย่างฟรี'}
        </button>
      </div>
      {open && (
        <div className="rp-player trailer-player" id={id}>
          <div className="rp-player-center">
            <button type="button" aria-label={playing ? 'หยุดตัวอย่างฟรี' : 'เล่นตัวอย่างฟรี'} aria-pressed={playing} onClick={() => setPlaying(!playing)}><Icon name={playing ? 'pause' : 'play'} /></button>
            <strong>ตัวอย่าง · {video.title}</strong>
            <p role="status">{playing ? 'จำลองการเล่นตัวอย่างฟรี' : 'ยังไม่มีไฟล์ตัวอย่าง · เครื่องเล่นจำลอง'}</p>
          </div>
        </div>
      )}
    </section>
  )
}
