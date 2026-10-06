import { useEffect, useRef, useState } from 'react'
import { PlanCard, PlanSkeleton } from '../components/PlanCard'
import { Icon } from '../components/UI'
import { membershipPlans } from '../data/plans'
import type { Video, Viewer } from '../types'
import { apiAuthEnabled } from '../lib/auth'

const selectablePlans = membershipPlans.filter((plan) => !plan.free)

interface PlansPageProps {
  viewer: Viewer
  activePlan: string
  loading: boolean
  video?: Video
  onActivate: (planId: string) => void
  onRefresh: () => Promise<void>
}

export function PlansPage({ viewer, activePlan, loading, video, onActivate, onRefresh }: PlansPageProps) {
  const [selected, setSelected] = useState(activePlan === 'free' ? 'standard' : activePlan)
  const [review, setReview] = useState(false)
  const [activating, setActivating] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  const submitting = useRef(false)
  const reviewHeading = useRef<HTMLHeadingElement>(null)
  const plan = selectablePlans.find((item) => item.id === selected) ?? selectablePlans[0]
  const backHref = video ? `#/videos/${video.id}` : viewer === 'guest' ? '#/' : '#/profile'
  const current = membershipPlans.find((item) => item.id === activePlan) ?? membershipPlans[0]

  useEffect(() => () => window.clearTimeout(timer.current), [])
  useEffect(() => {
    if (review) reviewHeading.current?.focus()
  }, [review])

  function activate() {
    if (submitting.current || viewer === 'guest' || plan.free) return
    if (apiAuthEnabled) {
      setActivating(true)
      void onRefresh().finally(() => setActivating(false))
      return
    }
    submitting.current = true
    setActivating(true)
    timer.current = window.setTimeout(() => onActivate(plan.id), 650)
  }

  return (
    <main className="plans-page">
      <a className="plans-back" href={backHref}>
        <Icon name="back" />
        {video ? 'กลับหน้ารายละเอียด' : 'ย้อนกลับ'}
      </a>
      <div className="plans-heading">
        <div className="rp-eyebrow" lang="en">
          Rachata Plus · Membership
        </div>
        <h1>เลือกแพ็กเกจที่ใช่สำหรับคุณ</h1>
        <p>
          {video
            ? `อยากดู “${video.title}” แบบเต็มเรื่อง? เลือกแพ็กเกจเพื่อรับชมต่อ`
            : 'บัญชีฟรีดูรายละเอียดและตัวอย่างได้ อัปเกรดเพื่อปลดล็อกวิดีโอเต็มทุกเรื่อง'}
        </p>
        <span className="plans-status">
          {apiAuthEnabled && (viewer === 'member' || viewer === 'admin')
            ? viewer === 'admin' && activePlan === 'free' ? 'สิทธิ์ปัจจุบัน: ผู้ดูแลระบบ' : `แพ็กเกจปัจจุบัน: ${current.name}`
            : viewer === 'guest' || current.free
              ? 'สิทธิ์ปัจจุบัน: ใช้งานฟรี'
              : `แพ็กเกจปัจจุบัน: ${current.name} · เดโม่`}
        </span>
      </div>

      {loading ? (
        <div className="plans-grid" aria-label="กำลังโหลดแพ็กเกจ">
          {selectablePlans.map((item) => (
            <PlanSkeleton key={item.id} />
          ))}
        </div>
      ) : review ? (
        <section className="plans-review">
          <div className="rp-eyebrow">{apiAuthEnabled ? "การเปิดสิทธิ์โดยแอดมิน" : "ยืนยันการเลือก · Demo only"}</div>
          <h2 ref={reviewHeading} tabIndex={-1}>
            แพ็กเกจ {plan.name}
          </h2>
          <p>{apiAuthEnabled ? "แจ้งอีเมลบัญชีและแพ็กเกจที่เลือกให้แอดมินเปิดสิทธิ์ จากนั้นตรวจสถานะสมาชิกหรือกลับไปดูโปรไฟล์ ไม่มีการเรียกเก็บเงินผ่านเว็บไซต์" : "ทดลองปลดล็อกหน้ารับชมวิดีโอเต็ม ไม่มีการเรียกเก็บเงินและไม่ต้องกรอกข้อมูลบัตร"}</p>
          <dl className="rp-info-grid">
            <div>
              <dt>ค่าใช้จ่ายในเดโม่นี้</dt>
              <dd>0 THB</dd>
            </div>
            <div>
              <dt>ราคาแพ็กเกจ</dt>
              <dd>{plan.price}</dd>
            </div>
          </dl>
          <p className="plans-note">
            {apiAuthEnabled ? "สิทธิ์ที่แอดมินเปิดจะบันทึกในฐานข้อมูลและหมดอายุตามวันที่กำหนด วิดีโอปัจจุบันสูงสุด 720p ทุกแพ็กเกจ คุณภาพและจำนวนอุปกรณ์บนการ์ดยังเป็นรายละเอียดตัวอย่าง" : "สิทธิ์เดโม่อยู่เฉพาะในหน้าเว็บ เมื่อรีเฟรชจะกลับเป็นสถานะเริ่มต้น คุณภาพและจำนวนอุปกรณ์เป็นรายละเอียดตัวอย่าง"}
          </p>
          <div className="rp-actions">
            <button
              type="button"
              className="rp-softbutton"
              disabled={activating}
              onClick={() => {
                setReview(false)
                requestAnimationFrame(() =>
                  document
                    .querySelector<HTMLInputElement>('input[name="membership-plan"]:checked')
                    ?.focus(),
                )
              }}
            >
              กลับไปเลือก
            </button>
            {viewer === 'guest' ? (
              <a className="rp-mainbutton rp-linkbutton" href="#/login">
                {apiAuthEnabled ? "เข้าสู่ระบบ" : "เข้าสู่ระบบเพื่อทดลอง"}
              </a>
            ) : (
              <button
                type="button"
                className="rp-mainbutton"
                disabled={activating}
                onClick={activate}
              >
                {apiAuthEnabled ? activating ? 'กำลังตรวจสถานะ…' : 'ตรวจสถานะสมาชิก' : activating ? 'กำลังเปิดสิทธิ์เดโม่…' : 'ยืนยันแพ็กเกจเดโม่'}
              </button>
            )}
          </div>
          <span className="rp-sr" role="status">
            {activating ? apiAuthEnabled ? 'กำลังตรวจสถานะสมาชิก' : 'กำลังเปิดสิทธิ์เดโม่ ไม่มีการเรียกเก็บเงิน' : ''}
          </span>
        </section>
      ) : (
        <>
          <fieldset className="plans-fieldset">
            <legend className="rp-sr">เลือกแพ็กเกจสมาชิกหนึ่งรายการ</legend>
            <div className="plans-grid">
              {selectablePlans.map((item) => (
                <PlanCard
                  key={item.id}
                  plan={item}
                  selected={selected === item.id}
                  onSelect={setSelected}
                />
              ))}
            </div>
          </fieldset>
          <div className="plans-summary">
            <div aria-live="polite">
              <span className="plan-label">แพ็กเกจที่เลือก</span>
              <h2>{plan.name}</h2>
              <p>{apiAuthEnabled ? "เปิดสิทธิ์ผ่านแอดมิน · ไม่มีการชำระเงินผ่านเว็บไซต์" : "ปลดล็อกวิดีโอเต็ม · ทดลองได้โดยไม่ต้องชำระเงิน"}</p>
            </div>
            <button
              type="button"
              className="rp-mainbutton rp-linkbutton"
              onClick={() => setReview(true)}
            >
              ดำเนินการต่อ
            </button>
          </div>
        </>
      )}
    </main>
  )
}
