import { useEffect, useRef, useState } from 'react'
import { PlanCard, PlanSkeleton } from '../components/PlanCard'
import { Icon } from '../components/UI'
import { membershipPlans } from '../data/plans'
import type { Video, Viewer } from '../types'

interface PlansPageProps {
  viewer: Viewer
  activePlan: string
  loading: boolean
  video?: Video
  onActivate: (planId: string) => void
}

export function PlansPage({ viewer, activePlan, loading, video, onActivate }: PlansPageProps) {
  const [selected, setSelected] = useState(activePlan === 'free' ? 'standard' : activePlan)
  const [review, setReview] = useState(false)
  const [activating, setActivating] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  const submitting = useRef(false)
  const reviewHeading = useRef<HTMLHeadingElement>(null)
  const plan = membershipPlans.find(item => item.id === selected) ?? membershipPlans[0]
  const backHref = video ? `#/videos/${video.id}` : viewer === 'guest' ? '#/' : '#/profile'
  const current = membershipPlans.find(item => item.id === activePlan) ?? membershipPlans[0]

  useEffect(() => () => window.clearTimeout(timer.current), [])
  useEffect(() => { if (review) reviewHeading.current?.focus() }, [review])

  function activate() {
    if (submitting.current || viewer === 'guest' || plan.free) return
    submitting.current = true
    setActivating(true)
    timer.current = window.setTimeout(() => onActivate(plan.id), 650)
  }

  return (
    <main className="plans-page">
      <a className="plans-back" href={backHref}><Icon name="back" />{video ? 'กลับหน้ารายละเอียด' : 'ย้อนกลับ'}</a>
      <div className="plans-heading">
        <div className="rp-eyebrow">Rachata Plus · Membership</div>
        <h1>เลือกแพ็กเกจที่ใช่สำหรับคุณ</h1>
        <p>{video ? `อยากดู “${video.title}” แบบเต็มเรื่อง? เลือกแพ็กเกจเพื่อรับชมต่อ` : 'บัญชีฟรีดูรายละเอียดและตัวอย่างได้ อัปเกรดเพื่อปลดล็อกวิดีโอเต็มทุกเรื่อง'}</p>
        <span className="plans-status">{viewer === 'guest' ? 'เริ่มต้นด้วยบัญชีฟรี' : `แพ็กเกจปัจจุบัน: ${current.name}${current.free ? '' : ' · เดโม่'}`}</span>
      </div>

      {loading ? <div className="plans-grid" aria-label="กำลังโหลดแพ็กเกจ">{membershipPlans.map(item => <PlanSkeleton key={item.id} />)}</div> : review ? (
        <section className="plans-review" aria-busy={activating}>
          <div className="rp-eyebrow">ยืนยันการเลือก · Demo only</div>
          <h2 ref={reviewHeading} tabIndex={-1}>แพ็กเกจ {plan.name}</h2>
          <p>ทดลองปลดล็อกหน้ารับชมวิดีโอเต็ม ไม่มีการเรียกเก็บเงินและไม่ต้องกรอกข้อมูลบัตร</p>
          <dl className="rp-info-grid">
            <div><dt>ค่าใช้จ่ายในเดโม่นี้</dt><dd>0 บาท</dd></div>
            <div><dt>ราคาบริการจริง</dt><dd>ยังไม่กำหนด</dd></div>
          </dl>
          <p className="plans-note">สิทธิ์นี้อยู่เฉพาะในหน้าเว็บ เมื่อรีเฟรชจะกลับเป็นสถานะเริ่มต้น คุณภาพและจำนวนอุปกรณ์เป็นรายละเอียดตัวอย่าง</p>
          <div className="rp-actions">
            <button type="button" className="rp-softbutton" disabled={activating} onClick={() => setReview(false)}>กลับไปเลือก</button>
            {viewer === 'guest' ? <a className="rp-mainbutton rp-linkbutton" href="#/login">เข้าสู่ระบบเพื่อทดลอง</a> : <button type="button" className="rp-mainbutton" disabled={activating} onClick={activate}>{activating ? 'กำลังเปิดสิทธิ์เดโม่…' : 'ยืนยันแพ็กเกจเดโม่'}</button>}
          </div>
          <span className="rp-sr" role="status">{activating ? 'กำลังเปิดสิทธิ์เดโม่ ไม่มีการเรียกเก็บเงิน' : ''}</span>
        </section>
      ) : (
        <>
          <fieldset className="plans-fieldset">
            <legend className="rp-sr">เลือกแพ็กเกจสมาชิกหนึ่งรายการ</legend>
            <div className="plans-grid">{membershipPlans.map(item => <PlanCard key={item.id} plan={item} selected={selected === item.id} onSelect={setSelected} />)}</div>
          </fieldset>
          <p className="plans-note">รายละเอียดแพ็กเกจเป็นตัวอย่างสำหรับออกแบบ ยังไม่กำหนดราคาจริง การเลือกแพ็กเกจในเดโม่นี้ไม่มีการเรียกเก็บเงิน</p>
          <div className="plans-summary">
            <div aria-live="polite"><span className="plan-label">แพ็กเกจที่เลือก</span><h2>{plan.name}</h2><p>{plan.free ? 'ดูรายละเอียดและตัวอย่างได้ วิดีโอเต็มต้องมีแพ็กเกจ' : 'ปลดล็อกวิดีโอเต็ม · ทดลองได้โดยไม่ต้องชำระเงิน'}</p></div>
            {plan.free ? <a className="rp-mainbutton rp-linkbutton" href={viewer === 'guest' ? '#/register' : backHref}>{viewer === 'guest' ? 'สร้างบัญชีฟรี' : activePlan === 'free' ? 'ใช้บัญชีฟรีต่อ' : 'กลับโดยไม่เปลี่ยนแพ็กเกจ'}</a> : <button type="button" className="rp-mainbutton rp-linkbutton" onClick={() => setReview(true)}>ดำเนินการต่อ</button>}
          </div>
        </>
      )}
    </main>
  )
}
