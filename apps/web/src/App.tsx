import { useCallback, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { MessagePage } from './components/UI'
import { sampleVideos } from './data/videos'
import { CatalogPage, DetailPage, HomePage } from './pages/Browse'
import { ProfilePage } from './pages/ProfilePage'
import { UploadPage } from './pages/UploadPage'
import { WatchPage } from './pages/WatchPage'
import { AuthPage } from './pages/AuthPage'
import type { PreviewAccount } from './pages/AuthPage'
import { PlansPage } from './pages/PlansPage'
import { membershipPlans } from './data/plans'
import { SearchBar, getSearchQuery } from './components/SearchBar'
import { SearchPage } from './pages/SearchPage'
import type { Profile, Video, Viewer } from './types'
import './App.css'

function readPath() { return window.location.hash.slice(1) || '/' }

// A tiny hash router keeps links, refresh and browser Back working without a server.
function usePath() {
  const [path, setPath] = useState(readPath)
  useEffect(() => {
    const change = () => { setPath(readPath()); window.scrollTo(0, 0) }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])
  return path
}

function PageContent({ path, forceLoading, viewer, videos, profile, onSaveProfile, onCreate, saved, onToggle, previewAccount, onRegisterPreview, onLoginPreview, activePlan, onActivatePlan }: {
  path: string; forceLoading: boolean; viewer: Viewer; videos: Video[]; profile: Profile;
  onSaveProfile: (profile: Profile) => void; onCreate: (video: Omit<Video, 'id'>) => number;
  saved: Set<number>; onToggle: (id: number) => void;
  previewAccount: PreviewAccount | null;
  onRegisterPreview: (name: string, email: string) => void;
  onLoginPreview: (email: string) => void;
  activePlan: string;
  onActivatePlan: (planId: string) => void;
}) {
  const [pending, setPending] = useState(true)
  useEffect(() => { const timer = window.setTimeout(() => setPending(false), 650); return () => window.clearTimeout(timer) }, [])
  const loading = pending || forceLoading
  const match = path.match(/^\/(videos|watch)\/(\d+)$/)
  let content: ReactNode
  if (path === '/') content = <HomePage videos={videos} loading={loading} />
  else if (path === '/videos') content = <CatalogPage videos={videos} loading={loading} />
  else if (path.split('?')[0] === '/search') content = <SearchPage query={getSearchQuery(path)} videos={videos} loading={loading} />
  else if (match) {
    const video = videos.find(item => item.id === Number(match[2]))
    content = !video ? <MessagePage title="ไม่พบวิดีโอ" message="วิดีโอที่เพิ่มในโหมดทดลองจะหายเมื่อรีเฟรช ลองเลือกวิดีโอจากหน้าแรกอีกครั้ง" /> : match[1] === 'watch'
      ? viewer === 'user'
        ? <PlansPage key={viewer} viewer={viewer} activePlan={activePlan} loading={loading} video={video} onActivate={onActivatePlan} />
        : <WatchPage video={video} videos={videos} viewer={viewer} loading={loading} saved={saved.has(video.id)} onToggle={() => onToggle(video.id)} />
      : <DetailPage video={video} videos={videos} loading={loading} saved={saved.has(video.id)} onToggle={() => onToggle(video.id)} />
  } else if (path === '/admin/upload') content = viewer === 'admin' ? <UploadPage onCreate={onCreate} /> : <MessagePage title="สำหรับผู้ดูแลระบบ" message="หน้านี้ใช้สำหรับแอดมิน ในโหมดทดลองสามารถเปลี่ยนสิทธิ์ตัวอย่างได้ที่ด้านล่าง" />
  else if (path === '/plans') content = <PlansPage key={`${viewer}-${activePlan}`} viewer={viewer} activePlan={activePlan} loading={loading} onActivate={onActivatePlan} />
  else if (path === '/profile' && viewer !== 'guest') content = <ProfilePage profile={profile} onSave={onSaveProfile} loading={loading} subscribed={viewer === 'member' || viewer === 'admin'} planName={membershipPlans.find(item => item.id === activePlan)?.name ?? 'Free'} />
  else if (['/login', '/register', '/profile'].includes(path)) content = <AuthPage mode={path === '/register' ? 'register' : 'login'} initialEmail={previewAccount?.email ?? ''} onRegisterPreview={onRegisterPreview} onLoginPreview={onLoginPreview} />
  else content = <MessagePage title="ไม่พบหน้านี้" message="กลับหน้าแรกเพื่อเลือกวิดีโอหรือเปิดโปรไฟล์" />
  const showLoading = loading && !['/admin/upload', '/login', '/register'].includes(path) && !(path === '/profile' && viewer === 'guest')
  return <div aria-busy={showLoading}>{content}<span className="rp-sr" role="status">{showLoading ? 'กำลังโหลดข้อมูลตัวอย่าง' : 'พร้อมใช้งาน'}</span></div>
}

export default function App() {
  const path = usePath()
  const [viewer, setViewer] = useState<Viewer>('guest')
  const [activePlan, setActivePlan] = useState('free')
  const [previewAccount, setPreviewAccount] = useState<PreviewAccount | null>(null)
  const [forceLoading, setForceLoading] = useState(false)
  const [videos, setVideos] = useState(sampleVideos)
  const [profile, setProfile] = useState<Profile>({ name: 'ผู้ใช้ตัวอย่าง', email: 'member@example.com', emailPending: false })
  const [saved, setSaved] = useState<Set<number>>(() => new Set())
  const createVideo = useCallback((video: Omit<Video, 'id'>) => {
    const id = Date.now()
    setVideos(current => [...current, { ...video, id }])
    return id
  }, [])
  function toggleSaved(id: number) {
    setSaved(current => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next })
  }
  function registerPreview(name: string, email: string) {
    setPreviewAccount({ name, email })
  }
  function loginPreview(email: string) {
    setProfile({ name: previewAccount?.email === email ? previewAccount.name : 'ผู้ใช้ตัวอย่าง', email, emailPending: false })
    setViewer('user')
    setActivePlan('free')
    window.location.hash = '/profile'
  }
  function logoutPreview() {
    setViewer('guest')
    setActivePlan('free')
    setProfile({ name: 'ผู้ใช้ตัวอย่าง', email: 'member@example.com', emailPending: false })
    setSaved(new Set())
    window.location.hash = '/login'
  }
  function activatePlan(planId: string) {
    if (viewer === 'guest' || !membershipPlans.some(plan => plan.id === planId && !plan.free)) return
    setActivePlan(planId)
    setViewer(viewer === 'admin' ? 'admin' : 'member')
    // A gated video keeps its URL, so activating the demo resumes that same video.
    if (!/^\/watch\/\d+$/.test(path)) window.location.hash = '/profile'
  }
  function changePreviewViewer(value: Viewer) {
    setViewer(value)
    setActivePlan(value === 'member' || value === 'admin' ? 'standard' : 'free')
  }
  const adminPage = path === '/admin/upload'
  return <div className="app-shell">
    <a className="skip-link" href="#content" onClick={event => { event.preventDefault(); document.getElementById('content')?.focus() }}>ข้ามไปเนื้อหา</a>
    <header className="rp-nav"><a className="rp-brand" href="#/" aria-label="Rachata Plus หน้าแรก">RACHATA<span>+</span></a>
      <nav className="rp-navlinks" aria-label="เมนูหลัก"><a href="#/" aria-current={path === '/' ? 'page' : undefined}>{adminPage ? 'กลับเว็บไซต์' : 'หน้าแรก'}</a><a href="#/videos" aria-current={path === '/videos' ? 'page' : undefined}>วิดีโอทั้งหมด</a><a href="#/plans" aria-current={path === '/plans' ? 'page' : undefined}>แพ็กเกจ</a>{viewer === 'admin' && <a href="#/admin/upload" aria-current={adminPage ? 'page' : undefined}>{adminPage ? 'อัปโหลดวิดีโอ' : 'จัดการวิดีโอ'}</a>}</nav>
      <SearchBar key={path} path={path} />
      <div className="rp-navend">
        {adminPage && viewer === 'admin' && <span className="rp-adminbadge">ADMIN</span>}
        {viewer === 'guest' ? <>
          <a className="rp-signin rp-linkbutton" href="#/login" aria-current={path === '/login' ? 'page' : undefined}>เข้าสู่ระบบ</a>
          <a className="rp-mainbutton rp-linkbutton" href="#/register" aria-current={path === '/register' ? 'page' : undefined}>สร้างบัญชี</a>
        </> : <>
          <a className="rp-accountbutton rp-linkbutton" href="#/profile" aria-label="โปรไฟล์" aria-current={path === '/profile' ? 'page' : undefined}><span className="rp-accountdot">RP</span>โปรไฟล์</a>
          <button type="button" className="rp-signout" onClick={logoutPreview}>ออกจากระบบ</button>
        </>}
      </div>
    </header>
    <div id="content" tabIndex={-1}><PageContent key={path} path={path} forceLoading={forceLoading} viewer={viewer} videos={videos} profile={profile} onSaveProfile={setProfile} onCreate={createVideo} saved={saved} onToggle={toggleSaved} previewAccount={previewAccount} onRegisterPreview={registerPreview} onLoginPreview={loginPreview} activePlan={activePlan} onActivatePlan={activatePlan} /></div>
    <footer className="rp-bottom"><span>RACHATA+ / เรื่องราวดี ๆ อยู่ที่นี่</span><span>แบบร่าง · ข้อมูลตัวอย่าง</span></footer>
    <details className="demo-tools"><summary>โหมดทดลอง UI</summary><p>ข้อมูลอยู่ในหน้านี้เท่านั้น รีเฟรชแล้วเริ่มใหม่ · ยังไม่เชื่อม API หรือ AWS</p><div className="demo-controls"><label>สิทธิ์ตัวอย่าง<select value={viewer} onChange={event => changePreviewViewer(event.target.value as Viewer)}><option value="guest">ผู้เยี่ยมชม</option><option value="user">บัญชีทั่วไป</option><option value="member">สมาชิกแบบชำระเงิน</option><option value="admin">แอดมิน</option></select></label><label><input type="checkbox" checked={forceLoading} onChange={event => setForceLoading(event.target.checked)} />แสดง Skeleton ค้างไว้</label></div></details>
  </div>
}
