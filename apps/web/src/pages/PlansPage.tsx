import { useEffect, useRef, useState } from 'react'
import { PlanCard, PlanSkeleton } from '../components/PlanCard'
import { Icon } from '../components/UI'
import { membershipPlans } from '../data/plans'
import type { Video, Viewer } from '../types'
import { accountError, apiAuthEnabled, checkoutDemoPlan } from '../lib/auth'
import type { ApiUser } from '../lib/auth'

const rank = (id: string) => membershipPlans.findIndex(plan => plan.id === id)

const selectablePlans = membershipPlans.filter((plan) => !plan.free)

interface PlansPageProps {
  viewer: Viewer
  activePlan: string
  loading: boolean
  video?: Video
  onActivate: (planId: string) => void
  user: ApiUser | null
  onConfirmed: (user: ApiUser) => void
}

export function PlansPage({ viewer, activePlan, loading, video, onActivate, user, onConfirmed }: PlansPageProps) {
  const [selected, setSelected] = useState(selectablePlans.find(plan => rank(plan.id) > rank(activePlan))?.id ?? activePlan)
  const [review, setReview] = useState(false)
  const [activating, setActivating] = useState(false)
  const [error, setError] = useState('')
  const controller = useRef<AbortController | null>(null)
  const submitting = useRef(false)
  const reviewHeading = useRef<HTMLHeadingElement>(null)
  const plan = selectablePlans.find((item) => item.id === selected) ?? selectablePlans[0]
  const backHref = video ? `#/videos/${video.id}` : viewer === 'guest' ? '#/' : '#/profile'
  const current = membershipPlans.find((item) => item.id === activePlan) ?? membershipPlans[0]

  const eligible = viewer !== 'admin' && rank(plan.id) > rank(activePlan)
  useEffect(() => () => controller.current?.abort(), [])
  useEffect(() => {
    if (review) reviewHeading.current?.focus()
  }, [review])

  async function activate() {
    if (submitting.current || viewer === 'guest' || !eligible) return
    submitting.current = true
    setActivating(true)
    setError('')
    const request = new AbortController()
    controller.current = request
    try {
      if (apiAuthEnabled) {
        if (!user) return
        onConfirmed(await checkoutDemoPlan(user, plan.id, request.signal))
      } else onActivate(plan.id)
    } catch (cause) {
      if (!request.signal.aborted) setError(accountError(cause))
    } finally {
      submitting.current = false
      if (!request.signal.aborted) setActivating(false)
    }
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

      <p className="plans-note">{apiAuthEnabled ? 'Basic สูงสุด 1080p / 2 จอ, Standard สูงสุด 1440p (2K) / 2 จอ, Premium สูงสุด 2160p (4K) / 4 จอ ตามความละเอียดที่ต้นฉบับมี จอที่เปิดเครื่องเล่นจะใช้สิทธิ์หนึ่งจอ ปิดหน้ารับชมเพื่อคืนสิทธิ์ หากขาดการเชื่อมต่อจะคืนภายใน 90 วินาที ไม่มีการเรียกเก็บเงินจริง' : 'โหมดทดลอง UI แสดงแพ็กเกจตัวอย่าง ไม่ได้บังคับความละเอียดหรือจำนวนจอและไม่มีการเรียกเก็บเงินจริง'}</p>

      {loading ? (
        <div className="plans-grid" aria-label="กำลังโหลดแพ็กเกจ">
          {selectablePlans.map((item) => (
            <PlanSkeleton key={item.id} />
          ))}
        </div>
      ) : review ? (
        <section className="plans-review">
          <div className="rp-eyebrow">ยืนยันชำระเงิน</div>
          <h2 ref={reviewHeading} tabIndex={-1}>
            แพ็กเกจ {plan.name}
          </h2>
          <p>ตรวจสอบแพ็กเกจและราคา ก่อนกดยืนยันชำระเงิน ไม่มีการเรียกเก็บเงินจริงและไม่ต้องกรอกข้อมูลบัตร</p>
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
            {apiAuthEnabled ? "ยืนยันแล้วสิทธิ์จะบันทึกในฐานข้อมูล บัญชีฟรีเริ่มใช้งาน 30 วัน ส่วนการอัปเกรดคงวันหมดอายุเดิมไว้ รับชมได้ตามคุณภาพและจำนวนจอของแพ็กเกจ ความละเอียดขึ้นอยู่กับไฟล์ต้นฉบับ" : "สิทธิ์เดโม่อยู่เฉพาะในหน้าเว็บ เมื่อรีเฟรชจะกลับเป็นสถานะเริ่มต้น การเลือกแพ็กเกจในโหมดนี้ไม่ได้เปลี่ยนคุณภาพวิดีโอหรือจำกัดการรับชมพร้อมกัน"}
          </p>
          <p className={error ? 'auth-error' : 'rp-sr'} role="alert">{error}</p>
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
                disabled={activating || !eligible}
                onClick={() => void activate()}
              >
                {activating ? 'กำลังยืนยัน…' : 'ยืนยันชำระเงิน'}
              </button>
            )}
          </div>
          <span className="rp-sr" role="status">
            {activating ? 'กำลังเปิดสิทธิ์ ไม่มีการเรียกเก็บเงินจริง' : ''}
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
                  disabled={viewer === 'admin' || rank(item.id) <= rank(activePlan)}
                  unavailableLabel={viewer === 'admin' ? 'ผู้ดูแลระบบมีสิทธิ์รับชมอยู่แล้ว' : item.id === activePlan ? 'แพ็กเกจปัจจุบัน' : 'ไม่สามารถลดแพ็กเกจได้'}
                />
              ))}
            </div>
          </fieldset>
          <div className="plans-summary">
            <div aria-live="polite">
              <span className="plan-label">แพ็กเกจที่เลือก</span>
              <h2>{plan.name}</h2>
              <p>{eligible ? 'อัปเกรดได้เท่านั้น · ยืนยันชำระเงิน ไม่มีการหักเงินจริง' : viewer === 'admin' ? 'ผู้ดูแลระบบมีสิทธิ์รับชมอยู่แล้ว' : 'คุณใช้แพ็กเกจสูงสุดแล้ว'}</p>
            </div>
            <button
              type="button"
              className="rp-mainbutton rp-linkbutton"
              disabled={!eligible}
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
