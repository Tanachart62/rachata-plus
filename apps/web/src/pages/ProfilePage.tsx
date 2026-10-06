import { moveTab } from '../lib/tabs'
import { ProfileSessions } from '../components/ProfileSessions'
import type { ApiUser } from '../lib/auth'
import { ChangePasswordForm } from '../components/ChangePasswordForm'
import { Icon, Skeleton, VideoSection } from '../components/UI'
import type { Profile, Video } from '../types'
import { apiAuthEnabled } from '../lib/auth'
import { useProfileForm } from '../hooks/useProfileForm'
import { PasswordField } from '../components/PasswordField'

export function ProfilePage({
  profile,
  onSave,
  onPassword,
  loading,
  subscribed,
  planName,
  expiresAt,
  savedVideos,
  sessionUser,
  refreshSession,
}: {
  profile: Profile
  onSave: (profile: Profile, password: string, signal?: AbortSignal) => Promise<void>
  onPassword: (password: string, next: string, signal?: AbortSignal) => Promise<void>
  loading: boolean
  subscribed: boolean
  planName: string
  expiresAt: number
  savedVideos: Video[]
  sessionUser: ApiUser | null
  refreshSession: () => Promise<void>
}) {
  const {
    editing,
    setEditing,
    draft,
    setDraft,
    tab,
    setTab,
    message,
    setMessage,
    failure,
    setFailure,
    pending,
    currentPassword,
    setCurrentPassword,
    errors,
    editButton,
    closeEditor,
    submit,
  } = useProfileForm(profile, onSave)
  return (
    <main className="rp-page">
      <div className="rp-page-heading">
        <div className="rp-eyebrow" lang="en">
          My account
        </div>
        <h1>โปรไฟล์ของฉัน</h1>
        <p>ดูแลข้อมูลส่วนตัวและสิทธิ์สมาชิกของคุณ</p>
      </div>
      <div className="profile-history-link">
        <a className="rp-softbutton rp-linkbutton" href="#/history">
          ประวัติการรับชมและดูต่อ →
        </a>
      </div>
      <div className="rp-profile-layout">
        <aside className="rp-profile-sidebar">
          {loading ? <Skeleton shape="avatar" /> : <div className="rp-avatar">รูปโปรไฟล์</div>}
          <div>
            {loading ? (
              <>
                <Skeleton shape="cardtitle" />
                <Skeleton shape="cardsub" />
              </>
            ) : (
              <>
                <div className="rp-profile-name">{profile.name}</div>
                <div className="rp-profile-email">{profile.email}</div>
                <span className="rp-pill">
                  {subscribed ? (apiAuthEnabled ? 'สมาชิก' : 'สมาชิกตัวอย่าง') : 'บัญชีทั่วไป'}
                </span>
              </>
            )}
          </div>
          <nav className="rp-profile-menu" aria-label="บัญชีของฉัน" role="tablist">
            {(['account', 'membership'] as const).map((value) => (
              <button
                key={value}
                id={`profile-${value}`}
                type="button"
                role="tab"
                tabIndex={tab === value ? 0 : -1}
                aria-selected={tab === value}
                aria-controls="profile-panel"
                onClick={() => setTab(value)}
                onKeyDown={(event) =>
                  moveTab(event, ['account', 'membership'] as const, tab, setTab, 'profile-')
                }
              >
                <Icon name={value === 'account' ? 'user' : 'check'} />
                {value === 'account' ? 'ข้อมูลส่วนตัว' : 'สมาชิกของฉัน'}
              </button>
            ))}
          </nav>
        </aside>
        <div
          className="rp-profile-main"
          id="profile-panel"
          role="tabpanel"
          tabIndex={0}
          aria-labelledby={`profile-${tab}`}
        >
          {loading ? (
            <section className="rp-surface">
              <Skeleton shape="meta" />
              <Skeleton shape="field" />
              <Skeleton shape="field" />
              <Skeleton shape="line2" />
            </section>
          ) : (
            <>
              <div className={message ? 'rp-success' : 'rp-sr'} role="status">
                {message}
              </div>
              <p className={failure ? 'auth-error' : 'rp-sr'} role="alert">
                {failure}
              </p>
              {tab === 'account' &&
                (editing ? (
                  <form className="rp-surface" noValidate aria-busy={pending} onSubmit={submit}>
                    <div className="rp-profile-head">
                      <h2>แก้ไขข้อมูลส่วนตัว</h2>
                    </div>
                    <p className="rp-demo-note">ช่องที่มี * จำเป็นต้องกรอก</p>
                    <div className="rp-profile-fields">
                      <label className="rp-field">
                        ชื่อ–นามสกุล *
                        <input
                          id="profile-name"
                          autoComplete="name"
                          aria-invalid={!!errors.name}
                          aria-describedby={errors.name ? 'profile-name-error' : undefined}
                          name="name"
                          required
                          maxLength={100}
                          value={draft.name}
                          onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                        />
                        {errors.name && (
                          <span id="profile-name-error" className="auth-error">
                            {errors.name}
                          </span>
                        )}
                      </label>
                      <label className="rp-field">
                        อีเมล *
                        <input
                          id="profile-email"
                          autoComplete="email"
                          aria-invalid={!!errors.email}
                          aria-describedby={errors.email ? 'profile-email-error' : undefined}
                          name="email"
                          type="email"
                          required
                          maxLength={254}
                          value={draft.email}
                          onChange={(event) => setDraft({ ...draft, email: event.target.value })}
                        />
                        {errors.email && (
                          <span id="profile-email-error" className="auth-error">
                            {errors.email}
                          </span>
                        )}
                      </label>
                    </div>
                    <p className="rp-sr" role="alert">
                      {Object.keys(errors).length
                        ? `กรุณาตรวจสอบ ${Object.keys(errors).length} ช่องที่มีข้อผิดพลาด`
                        : ''}
                    </p>
                    {apiAuthEnabled && draft.email.trim().toLowerCase() !== profile.email && (
                      <PasswordField
                        id="profile-password"
                        label="รหัสผ่านปัจจุบันเพื่อยืนยันการเปลี่ยนอีเมล"
                        value={currentPassword}
                        onChange={setCurrentPassword}
                        error={errors.password}
                        autoComplete="current-password"
                        disabled={pending}
                      />
                    )}
                    <p className="rp-demo-note">
                      {apiAuthEnabled
                        ? 'เมื่อเปลี่ยนอีเมล ให้เข้าสู่ระบบครั้งถัดไปด้วยอีเมลใหม่ อุปกรณ์อื่นจะออกจากระบบ'
                        : 'ใช้ข้อมูลสมมุติสำหรับทดลอง'}
                    </p>
                    <div className="rp-formfooter">
                      <button
                        type="button"
                        className="rp-softbutton"
                        disabled={pending}
                        onClick={() => {
                          closeEditor()
                          setDraft(profile)
                          setMessage('')
                          setFailure('')
                        }}
                      >
                        ยกเลิก
                      </button>
                      <button className="rp-mainbutton" type="submit" disabled={pending}>
                        <Icon name="check" />
                        {pending ? 'กำลังบันทึก…' : 'บันทึกการเปลี่ยนแปลง'}
                      </button>
                    </div>
                  </form>
                ) : (
                  <section className="rp-surface">
                    <div className="rp-profile-head">
                      <h2>ข้อมูลส่วนตัว</h2>
                      <button
                        type="button"
                        className="rp-edit-button"
                        ref={editButton}
                        onClick={() => {
                          setDraft(profile)
                          setEditing(true)
                          setMessage('')
                          setFailure('')
                        }}
                      >
                        แก้ไขโปรไฟล์
                      </button>
                    </div>
                    <dl className="rp-info-grid">
                      <div>
                        <dt>ชื่อผู้ใช้</dt>
                        <dd>{profile.username}</dd>
                      </div>
                      <div>
                        <dt>ชื่อ–นามสกุล</dt>
                        <dd>{profile.name}</dd>
                      </div>
                      <div>
                        <dt>อีเมล</dt>
                        <dd>
                          {profile.email}
                          {profile.emailPending && (
                            <>
                              <br />
                              <span className="rp-pill">รอยืนยันอีเมล · ตัวอย่าง</span>
                            </>
                          )}
                        </dd>
                      </div>
                    </dl>
                  </section>
                ))}
              {tab === 'account' && <ChangePasswordForm onSave={onPassword} />}
              {apiAuthEnabled && sessionUser && <ProfileSessions user={sessionUser} refresh={refreshSession} />}
              <section className="rp-surface rp-membership">
                <div className="rp-eyebrow" lang="en">
                  Rachata Plus membership
                </div>
                <div className="rp-profile-head">
                  <h2>{subscribed ? 'แพ็กเกจ ' + planName : 'บัญชีทั่วไป · Free'}</h2>
                  <span className={`rp-pill ${subscribed ? 'rp-positive' : ''}`}>
                    {subscribed
                      ? apiAuthEnabled
                        ? 'มีสิทธิ์สมาชิก'
                        : 'เปิดสิทธิ์เดโม่แล้ว'
                      : 'แพ็กเกจฟรี'}
                  </span>
                </div>
                <p>
                  {subscribed
                    ? 'รับชมวิดีโอสำหรับสมาชิกได้ทุกเรื่อง'
                    : 'สร้างบัญชีแล้ว ยังต้องมีแพ็กเกจสมาชิกเพื่อรับชมวิดีโอเต็ม'}
                </p>
                <dl className="rp-info-grid">
                  <div>
                    <dt>วันหมดอายุ</dt>
                    <dd>{expiresAt ? new Date(expiresAt).toLocaleString("th-TH") : "—"}</dd>
                  </div>
                  <div>
                    <dt>สถานะการชำระเงิน</dt>
                    <dd>{subscribed ? 'ไม่มีการเรียกเก็บเงินผ่านเว็บไซต์' : 'ไม่มีค่าใช้จ่าย'}</dd>
                  </div>
                </dl>
                <div className="plans-membership-action">
                  <a className="rp-mainbutton rp-linkbutton" href="#/plans">
                    {subscribed ? 'ดูแพ็กเกจอื่น' : 'อัปเกรดแพ็กเกจ'}
                  </a>
                </div>
              </section>
              <VideoSection title="ดูภายหลัง" videos={savedVideos} loading={loading} />
              {!savedVideos.length && (
                <p>กดดูภายหลังจากหน้ารายละเอียดวิดีโอเพื่อเก็บรายการไว้ที่นี่</p>
              )}
            </>
          )}
        </div>
      </div>
    </main>
  )
}
