import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { MessagePage, Skeleton } from './components/UI'
import { CatalogPage, DetailPage, HomePage } from './pages/Browse'
import { ProfilePage } from './pages/ProfilePage'
import { UploadPage } from './pages/UploadPage'
import { AdminVideosPage } from './pages/AdminVideosPage'
import { HistoryPage } from './pages/HistoryPage'
import { DashboardPage } from './pages/DashboardPage'
import { AdminNav } from './components/AdminNav'
import { AdminMembersPage } from './pages/AdminMembersPage'
import { WatchPage } from './pages/WatchPage'
import { AuthPage } from './pages/AuthPage'
import type { PreviewAccount } from './pages/AuthPage'
import { PlansPage } from './pages/PlansPage'
import { membershipPlans } from './data/plans'
import { getSearchQuery } from './lib/search'
import { SearchPage } from './pages/SearchPage'
import type { Profile, PublicationStatus, Video, Viewer, WatchHistoryEntry } from './types'
import { apiAuthEnabled } from './lib/auth'
import type { ApiUser } from './lib/auth'


interface RouteProps {
  path: string
  forceLoading: boolean
  viewer: Viewer
  account: {
    profile: Profile
    key: string
    activePlan: string
    expiresAt: number
    userId: number
    user: ApiUser | null
    refresh: () => Promise<void>
    saveProfile: (profile: Profile, password: string, signal?: AbortSignal) => Promise<void>
    savePassword: (password: string, next: string, signal?: AbortSignal) => Promise<void>
    authenticated: (user: ApiUser) => void
    activatePlan: (id: string) => void
  }
  catalog: {
    videos: Video[]
    create: (video: Omit<Video, 'id'>) => number
    uploaded: (video: Video) => void
    publish: (id: number, status: PublicationStatus) => Promise<void>
    remove: (id: number) => Promise<void>
    retryProcessing: (id: number) => Promise<void>
  }
  library: {
    saved: Set<number>
    history: WatchHistoryEntry[]
    toggle: (id: number) => void
    progress: (id: number, position: number, flush?: boolean) => Promise<void>
  }
  demo: {
    preview: PreviewAccount | null
    accounts: PreviewAccount[]
    register: (username: string, name: string, email: string) => void
    login: (email: string) => void
    monitoringError: boolean
    retryMonitoring: () => void
  }
}
export function AppRoutes({
  path,
  forceLoading,
  viewer,
  account,
  catalog,
  library,
  demo,
}: RouteProps) {
  const {
    profile,
    key: accountKey,
    activePlan,
    saveProfile: onSaveProfile,
    savePassword: onPassword,
    authenticated: onAuthenticated,
    activatePlan: onActivatePlan,
  } = account
  const {
    videos: allVideos,
    create: onCreate,
    uploaded: onUploaded,
    publish: onStatusChange,
    remove: onDelete,
    retryProcessing: onRetryVideo,
  } = catalog
  const { saved, history, toggle: onToggle, progress: onWatchProgress } = library
  const {
    preview: previewAccount,
    accounts: existingAccounts,
    register: onRegisterPreview,
    login: onLoginPreview,
    monitoringError,
    retryMonitoring: onRetryMonitoring,
  } = demo
  const [pending, setPending] = useState(true)
  useEffect(() => {
    const timer = window.setTimeout(() => setPending(false), 650)
    return () => window.clearTimeout(timer)
  }, [])
  useEffect(() => {
    const region = document.getElementById('content')
    const heading = region?.querySelector('h1')
    const titles: Record<string, string> = {
      '/': 'หน้าแรก',
      '/videos': 'วิดีโอทั้งหมด',
      '/search': 'ค้นหาวิดีโอ',
      '/profile': 'โปรไฟล์ของฉัน',
      '/history': 'ประวัติการรับชม',
      '/plans': 'เลือกแพ็กเกจ',
      '/login': 'เข้าสู่ระบบ',
      '/register': 'สร้างบัญชี',
      '/admin/upload': 'อัปโหลดวิดีโอ',
      '/admin/videos': 'จัดการวิดีโอ',
      '/admin/dashboard': 'Dashboard แอดมิน',
      '/admin/members': 'จัดการสมาชิก',
    }
    const video = allVideos.find(
      (item) => path === `/videos/${item.id}` || path === `/watch/${item.id}`,
    )
    const title =
      path === '/'
        ? 'หน้าแรก'
        : heading?.textContent || video?.title || titles[path.split('?')[0]] || 'ไม่พบหน้านี้'
    const prefix = /^\/watch\/\d+$/.test(path)
      ? 'รับชม · '
      : /^\/videos\/\d+$/.test(path)
        ? 'รายละเอียด · '
        : ''
    document.title = `${prefix}${title} | Rachata Plus`
    // Do not steal focus if someone has already moved to a field during loading.
    if (
      heading &&
      (document.activeElement === region || document.activeElement === document.body)
    ) {
      heading.tabIndex = -1
      heading.focus({ preventScroll: true })
    }
  }, [path, pending, forceLoading, viewer, allVideos])
  const loading = pending || forceLoading
  const videos = allVideos.filter((video) => video.publicationStatus === 'published')
  const match = path.match(/^\/(videos|watch)\/(\d+)$/)
  let content: ReactNode
  if (loading && !allVideos.length && (path === '/' || path === '/videos' || !!match))
    content = (
      <main className="rp-page">
        <h1>
          {path === '/' ? 'หน้าแรก' : path === '/videos' ? 'วิดีโอทั้งหมด' : 'กำลังโหลดวิดีโอ'}
        </h1>
        <Skeleton shape="player" />
      </main>
    )
  else if (path === '/') content = <HomePage videos={videos} loading={loading} />
  else if (path === '/videos') content = <CatalogPage videos={videos} loading={loading} />
  else if (path.split('?')[0] === '/search')
    content = <SearchPage query={getSearchQuery(path)} videos={videos} loading={loading} />
  else if (match) {
    const video = (viewer === 'admin' ? allVideos : videos).find(
      (item) => item.id === Number(match[2]),
    )
    content = !video ? (
      <MessagePage
        title="ไม่พบวิดีโอ"
        message="วิดีโอนี้ยังไม่เผยแพร่หรือถูกนำออกจากรายการแล้ว ลองเลือกวิดีโอจากหน้าแรก"
      />
    ) : match[1] === 'watch' ? (
      viewer === 'user' ? (
        <PlansPage
          key={viewer}
          viewer={viewer}
          activePlan={activePlan}
          loading={loading}
          video={video}
          onActivate={onActivatePlan}
          onRefresh={account.refresh}
        />
      ) : (
        <WatchPage
          key={`${video.id}-${accountKey}`}
          initialPosition={history.find((entry) => entry.videoId === video.id)?.position ?? 0}
          onProgress={onWatchProgress}
          video={video}
          videos={videos}
          viewer={viewer}
          loading={loading}
          saved={saved.has(video.id)}
          onToggle={() => onToggle(video.id)}
        />
      )
    ) : (
      <DetailPage
        video={video}
        videos={videos}
        loading={loading}
        saved={saved.has(video.id)}
        onToggle={() => onToggle(video.id)}
      />
    )
  } else if (path === '/admin/upload')
    content =
      viewer === 'admin' ? (
        <UploadPage onCreate={onCreate} onUploaded={onUploaded} />
      ) : (
        <MessagePage
          title="สำหรับผู้ดูแลระบบ"
          message="ต้องเข้าสู่ระบบด้วยบัญชีแอดมินเพื่ออัปโหลดไฟล์จริง"
        />
      )
  else if (path === '/admin/members')
    content = viewer === 'admin' && apiAuthEnabled
      ? <AdminMembersPage actorId={account.userId} onChanged={account.refresh} />
      : <MessagePage title="สำหรับผู้ดูแลระบบ" message="เข้าสู่ระบบด้วยบัญชีแอดมินเพื่อจัดการสมาชิกจริง" />
  else if (path === '/admin/videos')
    content =
      viewer === 'admin' ? (
        <AdminVideosPage
          videos={allVideos}
          loading={loading}
          onStatusChange={onStatusChange}
          onDelete={onDelete}
          onRetry={onRetryVideo}
        />
      ) : (
        <MessagePage
          title="สำหรับผู้ดูแลระบบ"
          message="ต้องเข้าสู่ระบบด้วยบัญชีแอดมินเพื่อจัดการไฟล์จริง"
        />
      )
  else if (path === '/history')
    content =
      viewer !== 'guest' ? (
        <HistoryPage history={history} videos={videos} loading={loading} />
      ) : (
        <MessagePage
          title="เข้าสู่ระบบเพื่อดูประวัติ"
          message="ประวัติการรับชมจะแสดงเฉพาะบัญชีของคุณ"
          actionHref="#/login"
          actionLabel="เข้าสู่ระบบ"
        />
      )
  else if (path === '/admin/dashboard')
    content =
      viewer === 'admin' ? (
        <DashboardPage
          videos={allVideos}
          loading={loading}
          monitoringError={monitoringError}
          onRetry={onRetryMonitoring}
        />
      ) : (
        <MessagePage title="สำหรับผู้ดูแลระบบ" message="Dashboard นี้ใช้สำหรับแอดมิน" />
      )
  else if (path === '/plans')
    content = (
      <PlansPage
        key={`${viewer}-${activePlan}`}
        viewer={viewer}
        activePlan={activePlan}
        loading={loading}
        onActivate={onActivatePlan}
        onRefresh={account.refresh}
      />
    )
  else if (path === '/profile' && viewer !== 'guest')
    content = (
      <ProfilePage
        key={accountKey}
        savedVideos={videos.filter((video) => saved.has(video.id))}
        profile={profile}
        onSave={onSaveProfile}
        onPassword={onPassword}
        loading={loading}
        subscribed={viewer === 'member' || viewer === 'admin'}
        expiresAt={account.expiresAt}
        sessionUser={viewer === "admin" ? null : account.user}
        refreshSession={account.refresh}
        planName={
          viewer === 'admin' && activePlan === 'free' ? 'สิทธิ์ผู้ดูแลระบบ'
            : (membershipPlans.find((item) => item.id === activePlan)?.name ?? 'Free')
        }
      />
    )
  else if (['/login', '/register', '/profile'].includes(path))
    content = (
      <AuthPage
        existingAccounts={existingAccounts}
        mode={path === '/register' ? 'register' : 'login'}
        initialEmail={previewAccount?.email ?? ''}
        onRegisterPreview={onRegisterPreview}
        onLoginPreview={onLoginPreview}
        onAuthenticated={onAuthenticated}
      />
    )
  else
    content = (
      <MessagePage title="ไม่พบหน้านี้" message="กลับหน้าแรกเพื่อเลือกวิดีโอหรือเปิดโปรไฟล์" />
    )
  const hasLoadingState =
    ['/', '/videos', '/plans'].includes(path) ||
    path.split('?')[0] === '/search' ||
    !!(
      match &&
      (viewer === 'admin' ? allVideos : videos).some((video) => video.id === Number(match[2]))
    ) ||
    (['/profile', '/history'].includes(path) && viewer !== 'guest') ||
    (['/admin/videos', '/admin/dashboard'].includes(path) && viewer === 'admin')
  const showLoading = loading && hasLoadingState
  return (
    <>
      <div aria-busy={showLoading}>
        {viewer === 'admin' && path.startsWith('/admin/') && <AdminNav path={path} />}
        {content}
      </div>
      <span className="rp-sr" role="status">
        {showLoading
          ? apiAuthEnabled
            ? 'กำลังโหลดข้อมูล'
            : 'กำลังโหลดข้อมูลตัวอย่าง'
          : 'พร้อมใช้งาน'}
      </span>
    </>
  )
}
