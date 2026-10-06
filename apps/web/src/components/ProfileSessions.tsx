import { useState } from 'react'
import type { ApiUser } from '../lib/auth'
import { AccountSessions } from './AccountSessions'

export function ProfileSessions({ user, refresh }: { user: ApiUser; refresh: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  return <section className="rp-surface membership-admin-form">
    <AccountSessions own actorId={user.id} user={user} disabled={busy} onBusy={setBusy}
      onRevoked={async () => { setNotice('ออกจากระบบในอุปกรณ์ที่เลือกแล้ว'); await refresh() }} />
    <p role="status">{notice}</p>
  </section>
}
