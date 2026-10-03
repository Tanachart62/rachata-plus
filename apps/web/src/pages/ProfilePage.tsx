import { useState } from 'react'
import { Icon, Skeleton } from '../components/UI'
import type { Profile } from '../types'

export function ProfilePage({ profile, onSave, loading, subscribed, planName }: { profile: Profile; onSave: (profile: Profile) => void; loading: boolean; subscribed: boolean; planName: string }) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(profile)
  const [tab, setTab] = useState<'account' | 'membership'>('account')
  const [message, setMessage] = useState('')
  return <main className="rp-page"><div className="rp-page-heading"><div className="rp-eyebrow">My account</div><h1>โปรไฟล์ของฉัน</h1><p>ดูแลข้อมูลส่วนตัวและสิทธิ์สมาชิกของคุณ</p></div>
    <div className="rp-profile-layout"><aside className="rp-profile-sidebar">{loading ? <Skeleton shape="avatar" /> : <div className="rp-avatar">รูปโปรไฟล์</div>}<div>{loading ? <><Skeleton shape="cardtitle" /><Skeleton shape="cardsub" /></> : <><div className="rp-profile-name">{profile.name}</div><div className="rp-profile-email">{profile.email}</div><span className="rp-pill">{subscribed ? 'สมาชิกตัวอย่าง' : 'บัญชีทั่วไป'}</span></>}</div>
      <nav className="rp-profile-menu" aria-label="บัญชีของฉัน" role="tablist">{(['account', 'membership'] as const).map(value => <button key={value} id={`profile-${value}`} type="button" role="tab" aria-selected={tab === value} aria-controls="profile-panel" onClick={() => setTab(value)} onKeyDown={event => { if (['ArrowLeft', 'ArrowRight'].includes(event.key)) { event.preventDefault(); const next = tab === 'account' ? 'membership' : 'account'; setTab(next); document.getElementById(`profile-${next}`)?.focus() } }}><Icon name={value === 'account' ? 'user' : 'check'} />{value === 'account' ? 'ข้อมูลส่วนตัว' : 'สมาชิกของฉัน'}</button>)}</nav>
    </aside><div className="rp-profile-main" id="profile-panel" role="tabpanel" aria-labelledby={`profile-${tab}`}>
      {loading ? <section className="rp-surface"><Skeleton shape="meta" /><Skeleton shape="field" /><Skeleton shape="field" /><Skeleton shape="line2" /></section> : <>
        {message && <div className="rp-success" role="status">{message}</div>}
        {tab === 'account' && (editing ? <form className="rp-surface" onSubmit={event => {
          event.preventDefault()
          const next = { name: draft.name.trim(), email: draft.email.trim().toLowerCase(), emailPending: profile.emailPending || draft.email.trim().toLowerCase() !== profile.email }
          if (!next.name) { setMessage('กรุณาใส่ชื่อที่แสดง'); return }
          onSave(next); setEditing(false); setMessage('บันทึกในตัวอย่างแล้ว ข้อมูลจะกลับค่าเริ่มต้นเมื่อรีเฟรช')
        }}><div className="rp-profile-head"><h2>แก้ไขข้อมูลส่วนตัว</h2></div><div className="rp-profile-fields">
          <label className="rp-field">ชื่อที่แสดง *<input name="name" required maxLength={100} value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} /></label>
          <label className="rp-field">อีเมล *<input name="email" type="email" required maxLength={254} value={draft.email} onChange={event => setDraft({ ...draft, email: event.target.value })} /></label>
        </div><p className="rp-demo-note">ใช้ข้อมูลสมมุติสำหรับทดลอง · ระบบจริงจะต้องยืนยันอีเมลใหม่</p><div className="rp-formfooter"><button type="button" className="rp-softbutton" onClick={() => { setEditing(false); setDraft(profile); setMessage('') }}>ยกเลิก</button><button className="rp-mainbutton" type="submit"><Icon name="check" />บันทึกการเปลี่ยนแปลง</button></div></form> : <section className="rp-surface"><div className="rp-profile-head"><h2>ข้อมูลส่วนตัว</h2><button type="button" className="rp-edit-button" onClick={() => { setDraft(profile); setEditing(true); setMessage('') }}>แก้ไขโปรไฟล์</button></div><dl className="rp-info-grid"><div><dt>ชื่อที่แสดง</dt><dd>{profile.name}</dd></div><div><dt>อีเมล</dt><dd>{profile.email}{profile.emailPending && <><br /><span className="rp-pill">รอยืนยันอีเมล · ตัวอย่าง</span></>}</dd></div></dl></section>)}
        <section className="rp-surface rp-membership"><div className="rp-eyebrow">Rachata Plus membership</div><div className="rp-profile-head"><h2>{subscribed ? "แพ็กเกจ " + planName : 'บัญชีทั่วไป · Free'}</h2><span className={`rp-pill ${subscribed ? 'rp-positive' : ''}`}>{subscribed ? 'เปิดสิทธิ์เดโม่แล้ว' : 'แพ็กเกจฟรี'}</span></div><p>{subscribed ? 'รับชมวิดีโอสำหรับสมาชิกได้ทุกเรื่อง' : 'สร้างบัญชีแล้ว ยังต้องมีแพ็กเกจสมาชิกเพื่อรับชมวิดีโอเต็ม'}</p><dl className="rp-info-grid"><div><dt>วันหมดอายุ</dt><dd>—</dd></div><div><dt>สถานะการชำระเงิน</dt><dd>{subscribed ? 'เดโม่ · ไม่เรียกเก็บเงินจริง' : 'ไม่มีค่าใช้จ่าย'}</dd></div></dl><div className="plans-membership-action"><a className="rp-mainbutton rp-linkbutton" href="#/plans">{subscribed ? 'ดูแพ็กเกจอื่น' : 'อัปเกรดแพ็กเกจ'}</a></div></section>
      </>}
    </div></div>
  </main>
}


