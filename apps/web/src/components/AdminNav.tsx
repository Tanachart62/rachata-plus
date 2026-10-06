export function AdminNav({ path }: { path: string }) {
  return <nav className="admin-subnav" aria-label="เมนูผู้ดูแลระบบ">{[
    ['/admin/dashboard', 'Dashboard'], ['/admin/members', 'สมาชิก'], ['/admin/videos', 'คลังวิดีโอ'], ['/admin/upload', 'อัปโหลด'],
  ].map(([href, label]) => <a key={href} href={`#${href}`} aria-current={path === href ? 'page' : undefined}>{label}</a>)}</nav>
}
