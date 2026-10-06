import { useEffect, useMemo, useState } from 'react'
import { AppRoutes } from './AppRoutes'
import { SearchBar } from './components/SearchBar'
import { apiAuthEnabled } from './lib/auth'
import { useSession } from './hooks/useSession'
import { useVideoCatalog } from './hooks/useVideoCatalog'
import { useAccountLibrary } from './hooks/useAccountLibrary'
import { membershipPlans } from './data/plans'
import type { PreviewAccount } from './pages/AuthPage'
import type { Profile, Viewer, WatchHistoryEntry } from './types'
import './App.css'

function readPath() {
  return window.location.hash.slice(1) || '/'
}
function usePath() {
  const [path, setPath] = useState(readPath)
  useEffect(() => {
    const change = () => {
      setPath(readPath())
      window.scrollTo(0, 0)
    }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])
  return path
}
const emptyProfile: Profile = {
  username: 'demo_member',
  name: 'ผู้ใช้ตัวอย่าง',
  email: 'member@example.com',
  emailPending: false,
}
export default function App() {
  const path = usePath()
  const session = useSession()
  const [demoViewer, setDemoViewer] = useState<Viewer>('guest')
  const viewer: Viewer = apiAuthEnabled
    ? session.user?.role === 'admin'
      ? 'admin'
      : session.user?.member
        ? 'member'
        : session.user
          ? 'user'
          : 'guest'
    : demoViewer
  const catalog = useVideoCatalog(viewer === 'admin', path, session.epoch)
  const library = useAccountLibrary(session.user, session.epoch, session.epochRef, path)
  const routeVideos = useMemo(() => {
    const merged = new Map(catalog.videos.map((video) => [video.id, video]))
    for (const video of library.videos) if (!merged.has(video.id)) merged.set(video.id, video)
    return [...merged.values()]
  }, [catalog.videos, library.videos])
  const [demoProfile, setDemoProfile] = useState(emptyProfile)
  const [accounts, setAccounts] = useState<PreviewAccount[]>([])
  const [activePlan, setActivePlan] = useState('free')
  const [demoSaved, setDemoSaved] = useState(new Set<number>())
  const [demoHistory, setDemoHistory] = useState<WatchHistoryEntry[]>([])
  const [forceLoading, setForceLoading] = useState(false)
  const [monitoringError, setMonitoringError] = useState(false)
  const profile: Profile = session.user
    ? {
        username: session.user.username,
        name: session.user.name,
        email: session.user.email,
        emailPending: false,
      }
    : demoProfile
  const accountKey = apiAuthEnabled
    ? `user-${session.user?.id ?? 'guest'}-${session.epoch}`
    : demoProfile.email
  const saved = apiAuthEnabled ? library.saved : demoSaved
  const history = apiAuthEnabled ? library.history : demoHistory
  useEffect(() => {
    const region = document.getElementById('content')
    const heading = region?.querySelector('h1')
    if (heading) heading.tabIndex = -1
    ;(heading ?? region)?.focus({ preventScroll: true })
  }, [path])
  const toggle = (id: number) => {
    if (apiAuthEnabled) {
      void library.toggle(id)
      return
    }
    setDemoSaved((items) => {
      const next = new Set(items)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  const progress = async (id: number, position: number, flush = false) => {
    if (apiAuthEnabled) {
      await library.progress(id, position, flush)
      return
    }
    if (viewer === 'member' || viewer === 'admin')
      setDemoHistory((items) => [
        { videoId: id, position, watchedAt: Date.now() },
        ...items.filter((e) => e.videoId !== id),
      ])
  }
  const loginPreview = (email: string) => {
    const a = accounts.find((item) => item.email === email)
    setDemoProfile({ ...emptyProfile, ...a, email })
    setDemoViewer('user')
    setActivePlan('free')
    setDemoSaved(new Set())
    setDemoHistory([])
    window.location.hash = '/profile'
  }
  const logout = () => {
    if (apiAuthEnabled) {
      void session.logout()
      return
    }
    setDemoViewer('guest')
    setActivePlan('free')
    setDemoSaved(new Set())
    setDemoHistory([])
    setDemoProfile(emptyProfile)
    window.location.hash = '/login'
  }
  const activatePlan = (id: string) => {
    if (apiAuthEnabled) {
      session.setNotice('แจ้งแอดมินเพื่อเปิดแพ็กเกจสมาชิก ไม่มีการเรียกเก็บเงินผ่านเว็บไซต์')
      return
    }
    if (viewer === 'guest' || !membershipPlans.some((p) => p.id === id && !p.free)) return
    setActivePlan(id)
    setDemoViewer(viewer === 'admin' ? 'admin' : 'member')
    if (!/^\/watch\/\d+$/.test(path)) window.location.hash = '/profile'
  }
  const adminPage = path.startsWith('/admin/')
  return (
    <div className="app-shell">
      <a
        className="skip-link"
        href="#content"
        onClick={(e) => {
          e.preventDefault()
          document.getElementById('content')?.focus()
        }}
      >
        ข้ามไปเนื้อหา
      </a>
      <header className="rp-nav">
        <a className="rp-brand" href="#/" aria-label="Rachata Plus หน้าแรก">
          RACHATA<span>+</span>
        </a>
        <nav className="rp-navlinks" aria-label="เมนูหลัก">
          <a href="#/" aria-current={path === '/' ? 'page' : undefined}>
            {adminPage ? 'กลับเว็บไซต์' : 'หน้าแรก'}
          </a>
          <a href="#/videos" aria-current={path === '/videos' ? 'page' : undefined}>
            วิดีโอทั้งหมด
          </a>
          {viewer === 'admin' && (
            <>
              <a
                href="#/admin/videos"
                aria-current={
                  path === '/admin/videos' || path === '/admin/upload' ? 'page' : undefined
                }
              >
                จัดการวิดีโอ
              </a>
            </>
          )}
        </nav>
        <SearchBar key={path} path={path} />
        <div className="rp-navend">
          {adminPage && viewer === 'admin' && <span className="rp-adminbadge">ADMIN</span>}
          {viewer === 'guest' ? (
            <>
              <a
                className="rp-signin rp-linkbutton"
                href="#/login"
                aria-current={path === '/login' ? 'page' : undefined}
              >
                เข้าสู่ระบบ
              </a>
              <a
                className="rp-mainbutton rp-linkbutton"
                href="#/register"
                aria-current={path === '/register' ? 'page' : undefined}
              >
                สร้างบัญชี
              </a>
            </>
          ) : (
            <>
              <a
                className="rp-accountbutton rp-linkbutton"
                href="#/profile"
                aria-label="โปรไฟล์"
                aria-current={path === '/profile' ? 'page' : undefined}
              >
                <span className="rp-accountdot">RP</span>โปรไฟล์
              </a>
              <a
                className="rp-history-link"
                href="#/history"
                aria-current={path === '/history' ? 'page' : undefined}
              >
                ประวัติ
              </a>
              <button
                type="button"
                className="rp-signout"
                onClick={logout}
                disabled={session.signingOut}
              >
                {session.signingOut ? 'กำลังออกจากระบบ…' : 'ออกจากระบบ'}
              </button>
            </>
          )}
        </div>
      </header>
      <div id="content" tabIndex={-1}>
        <p className={session.notice ? 'rp-demo-note' : 'rp-sr'} role="status">
          {session.notice}
        </p>
        {catalog.error && (
          <div role="alert">
            <p>{catalog.error}</p>
            <button className="rp-softbutton" onClick={catalog.retry}>
              ลองโหลดวิดีโอใหม่
            </button>
          </div>
        )}
        {apiAuthEnabled && library.error && (
          <div role="alert">
            <p>{library.error}</p>
            <button className="rp-softbutton" onClick={library.retry}>
              ลองโหลดหรือบันทึกใหม่
            </button>
            <button className="rp-softbutton" onClick={() => void session.revalidate()}>
              ตรวจสถานะบัญชี
            </button>
          </div>
        )}
        <AppRoutes
          key={path + accountKey}
          path={path}
          forceLoading={forceLoading || catalog.loading || (apiAuthEnabled && library.loading)}
          viewer={viewer}
          account={{
            profile,
            key: accountKey,
            activePlan: apiAuthEnabled ? session.user?.plan || "free" : activePlan,
            expiresAt: session.user?.expiresAt ?? 0,
            userId: session.user?.id ?? 0,
            user: session.user,
            refresh: session.revalidate,
            saveProfile: apiAuthEnabled
              ? session.saveProfile
              : async (next) => {
                  setDemoProfile(next)
                },
            savePassword: session.savePassword,
            authenticated: session.authenticated,
            activatePlan,
          }}
          catalog={{ ...catalog, videos: routeVideos }}
          library={{ saved, history, toggle, progress }}
          demo={{
            preview: accounts.at(-1) ?? null,
            accounts,
            register: (username, name, email) =>
              setAccounts((items) => [...items, { username, name, email }]),
            login: loginPreview,
            monitoringError,
            retryMonitoring: () => setMonitoringError(false),
          }}
        />
        {apiAuthEnabled && path === '/profile' && library.cursors.savedNext && (
          <button
            className="rp-softbutton"
            disabled={library.paging}
            onClick={() => void library.loadMore('saved')}
          >
            โหลดดูภายหลังเพิ่มเติม
          </button>
        )}
        {apiAuthEnabled && path === '/history' && library.cursors.historyNext && (
          <button
            className="rp-softbutton"
            disabled={library.paging}
            onClick={() => void library.loadMore('history')}
          >
            โหลดประวัติเพิ่มเติม
          </button>
        )}
      </div>
      <footer className="rp-bottom">
        <span>RACHATA+ / เรื่องราวดี ๆ อยู่ที่นี่</span>
        <span>{apiAuthEnabled ? 'Rachata Plus' : 'แบบร่าง · ข้อมูลตัวอย่าง'}</span>
      </footer>
      {(import.meta.env.DEV || !apiAuthEnabled) && (
        <details className="demo-tools">
          <summary>โหมดทดลอง UI</summary>
          <p>
            {apiAuthEnabled
              ? 'บัญชี วิดีโอและสมาชิกเชื่อม API แล้ว · Dashboard ยังเป็นเดโม่'
              : 'ข้อมูลตัวอย่างจะหายเมื่อรีเฟรช'}
          </p>
          <div className="demo-controls">
            {!apiAuthEnabled && (
              <label>
                สิทธิ์ตัวอย่าง
                <select value={viewer} onChange={(e) => setDemoViewer(e.target.value as Viewer)}>
                  <option value="guest">ผู้เยี่ยมชม</option>
                  <option value="user">บัญชีทั่วไป</option>
                  <option value="member">สมาชิก</option>
                  <option value="admin">แอดมิน</option>
                </select>
              </label>
            )}
            <label>
              <input
                type="checkbox"
                checked={forceLoading}
                onChange={(e) => setForceLoading(e.target.checked)}
              />
              แสดง Skeleton ค้างไว้
            </label>
            <label>
              <input
                type="checkbox"
                checked={monitoringError}
                onChange={(e) => setMonitoringError(e.target.checked)}
              />
              จำลอง Cloud Monitoring ขัดข้อง
            </label>
          </div>
        </details>
      )}
    </div>
  )
}
