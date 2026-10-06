import { readUser } from './auth'
import type { ApiUser } from './auth'
import { ApiError, isObject, jsonRequest } from './http'

export type MembershipAction = 'grant' | 'revoke' | 'suspend' | 'reactivate' | 'logout_all' | 'logout_session'
export interface DeviceSession {
  id: string
  device: string
  createdAt: number
  expiresAt: number
  current: boolean
}
export async function listDeviceSessions(actorId: number, userId: number, cursor = '', signal?: AbortSignal, own = false) {
  const path = own ? '/auth/sessions?' : `/auth/admin/memberships/${userId}/sessions?`
  const data = await jsonRequest(path + new URLSearchParams({ cursor }), { signal })
  if (!isObject(data) || data.actorId !== actorId || data.userId !== userId) throw new ApiError('account_changed')
  if (!Array.isArray(data.sessions) || data.sessions.length > 50 || typeof data.nextCursor !== 'string') throw new ApiError('service_unavailable')
  const sessions = data.sessions.map((value): DeviceSession => {
    if (!isObject(value) || typeof value.id !== 'string' || !/^[1-9]\d*$/.test(value.id) ||
      typeof value.device !== 'string' || !Number.isSafeInteger(value.createdAt) || !Number.isSafeInteger(value.expiresAt) || typeof value.current !== 'boolean')
      throw new ApiError('service_unavailable')
    return value as unknown as DeviceSession
  })
  return { sessions, nextCursor: data.nextCursor }
}
export interface MembershipEvent {
  action: MembershipAction
  plan: string
  expiresAt: number
  createdAt: number
  actor: string
  sessionId: string
}
export async function listMembershipAccounts(actorId: number, query: string, cursor = '', signal?: AbortSignal) {
  const data = await jsonRequest('/auth/admin/memberships/accounts?' + new URLSearchParams({ q: query, cursor }), { signal })
  if (!isObject(data) || data.actorId !== actorId) throw new ApiError('account_changed')
  if (!Array.isArray(data.users) || data.users.length > 50 || typeof data.nextCursor !== 'string')
    throw new ApiError('service_unavailable')
  return { users: data.users.map(user => readUser({ user })), nextCursor: data.nextCursor }
}
export async function findMembership(actorId: number, email: string, signal?: AbortSignal) {
  const data = await jsonRequest('/auth/admin/memberships?' + new URLSearchParams({ email }), { signal })
  if (!isObject(data) || data.actorId !== actorId) throw new ApiError('account_changed')
  if (!Array.isArray(data.events) || data.events.length > 20) throw new ApiError('service_unavailable')
  const events = data.events.map((event): MembershipEvent => {
    if (!isObject(event) || !['grant', 'revoke', 'suspend', 'reactivate', 'logout_all', 'logout_session'].includes(event.action as string) ||
      typeof event.plan !== 'string' || !Number.isSafeInteger(event.expiresAt) ||
      !Number.isSafeInteger(event.createdAt) || typeof event.actor !== 'string' || typeof event.sessionId !== 'string')
      throw new ApiError('service_unavailable')
    return event as unknown as MembershipEvent
  })
  return { user: readUser(data), events }
}
export async function manageMembership(actorId: number, user: ApiUser, action: MembershipAction, plan: string, days: number, signal?: AbortSignal, sessionId?: string) {
  const data = await jsonRequest(`/auth/admin/memberships/${user.id}`, {
    body: { expectedUserId: actorId, expectedRevision: user.membershipRevision, action, plan, days, sessionId }, signal,
  })
  if (!isObject(data) || data.actorId !== actorId) throw new ApiError('account_changed')
  const next = readUser(data)
  if (next.id !== user.id) throw new ApiError('account_changed')
  return next
}

export async function revokeOwnDeviceSession(userId: number, sessionId: string, signal?: AbortSignal) {
  const data = await jsonRequest(`/auth/sessions/${sessionId}/revoke`, { body: { expectedUserId: userId }, signal })
  if (!isObject(data) || data.actorId !== userId) throw new ApiError('account_changed')
  const user = readUser(data)
  if (user.id !== userId) throw new ApiError('account_changed')
  return user
}
