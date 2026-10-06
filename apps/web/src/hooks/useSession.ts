import { useCallback, useEffect, useRef, useState } from 'react'
import type { Profile } from '../types'
import {
  accountError,
  apiAuthEnabled,
  AuthError,
  changeAccountPassword,
  currentAccount,
  logoutAccount,
  updateAccount,
} from '../lib/auth'
import type { ApiUser } from '../lib/auth'

export function useSession() {
  const [user, setUser] = useState<ApiUser | null>(null)
  const [epoch, setEpoch] = useState(0)
  const epochRef = useRef(0)
  const userRef = useRef<ApiUser | null>(null)
  const [notice, setNotice] = useState(apiAuthEnabled ? 'กำลังตรวจสถานะบัญชี…' : '')
  const [signingOut, setSigningOut] = useState(false)
  const channel = useRef<BroadcastChannel | null>(null)
  const restore = useRef<AbortController | null>(null)
  const apply = useCallback((next: ApiUser | null, invalidate = false) => {
    if (invalidate || userRef.current?.id !== next?.id) {
      epochRef.current++
      setEpoch(epochRef.current)
    }
    userRef.current = next
    setUser(next)
    setNotice('')
  }, [])
  const revalidate = useCallback(async () => {
    if (!apiAuthEnabled) return
    restore.current?.abort()
    const controller = new AbortController()
    restore.current = controller
    const version = epochRef.current
    try {
      const next = await currentAccount(controller.signal)
      if (!controller.signal.aborted && version === epochRef.current) apply(next)
    } catch (error) {
      if (!controller.signal.aborted && version === epochRef.current) setNotice(accountError(error))
    }
  }, [apply])
  useEffect(() => {
    if (!apiAuthEnabled) return
    if (typeof BroadcastChannel !== 'undefined') {
      const bus = new BroadcastChannel('rachata-account')
      channel.current = bus
      bus.onmessage = () => {
        void revalidate()
      }
    }
    const visible = () => {
      if (document.visibilityState === 'visible') void revalidate()
    }
    window.addEventListener('focus', visible)
    document.addEventListener('visibilitychange', visible)
    void revalidate()
    const timer = window.setInterval(visible, 60000)
    return () => {
      window.clearInterval(timer)
      restore.current?.abort()
      channel.current?.close()
      channel.current = null
      window.removeEventListener('focus', visible)
      document.removeEventListener('visibilitychange', visible)
    }
  }, [revalidate])
  useEffect(() => {
    if (!apiAuthEnabled || !user?.member || !user.expiresAt) return
    const remaining = user.expiresAt - Date.now()
    if (remaining <= 0) {
      if (userRef.current?.id === user.id) apply({ ...userRef.current, member: false, plan: '', expiresAt: 0 })
      return
    }
    const timer = window.setTimeout(() => {
      if (Date.now() >= user.expiresAt && userRef.current?.id === user.id)
        apply({ ...userRef.current, member: false, plan: '', expiresAt: 0 })
      void revalidate()
    }, Math.min(remaining + 50, 2147483647))
    return () => window.clearTimeout(timer)
  }, [user, revalidate, apply])
  const broadcast = useCallback(() => channel.current?.postMessage('changed'), [])
  const authenticated = (next: ApiUser) => {
    restore.current?.abort()
    apply(next, true)
    broadcast()
    window.location.hash = '/profile'
  }
  const logout = async () => {
    if (signingOut || !user) return
    setSigningOut(true)
    const version = epochRef.current
    try {
      await logoutAccount(user.id)
      if (version !== epochRef.current) return
      apply(null, true)
      broadcast()
      window.location.hash = '/login'
    } catch (error) {
      setNotice(accountError(error))
      if (error instanceof AuthError && error.code === 'account_changed') void revalidate()
    } finally {
      setSigningOut(false)
    }
  }
  const owner = user?.id
  const ownerEpoch = epoch
  const current = () =>
    owner !== undefined && userRef.current?.id === owner && epochRef.current === ownerEpoch
  const saveProfile = async (next: Profile, password: string, signal?: AbortSignal) => {
    if (!current()) throw new AuthError('account_changed')
    const result = await updateAccount(owner!, next.name, next.email, password, signal)
    if (!current()) throw new AuthError('account_changed')
    apply(result)
    broadcast()
  }
  const savePassword = async (password: string, next: string, signal?: AbortSignal) => {
    if (!current()) throw new AuthError('account_changed')
    await changeAccountPassword(owner!, password, next, signal)
    if (!current()) throw new AuthError('account_changed')
    broadcast()
  }
  return {
    user,
    epoch,
    epochRef,
    notice,
    setNotice,
    signingOut,
    authenticated,
    logout,
    revalidate,
    saveProfile,
    savePassword,
  }
}
